package google

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"

	"github.com/mattermost/mattermost/server/public/plugin"

	"github.com/mattermost-community/mattermost-plugin-google-drive/server/plugin/kvstore"
	"github.com/mattermost-community/mattermost-plugin-google-drive/server/plugin/model"

	"golang.org/x/time/rate"
	"google.golang.org/api/drive/v3"
	"google.golang.org/api/googleapi"
)

type DriveService struct {
	service *drive.Service
	limiter *rate.Limiter
	papi    plugin.API
	userID  string
	kvstore kvstore.KVStore
}

func (ds DriveService) parseGoogleErrors(err error) {
	if googleErr, ok := err.(*googleapi.Error); ok {
		reason := ""
		if len(googleErr.Errors) > 0 {
			for _, error := range googleErr.Errors {
				if error.Reason != "" {
					reason = error.Reason
					break
				}
			}
		}
		if reason == "userRateLimitExceeded" {
			err = ds.kvstore.StoreUserRateLimitExceeded(ds.userID)
			if err != nil {
				ds.papi.LogError("Failed to store user rate limit exceeded", "userID", ds.userID, "err", err)
				return
			}
		}
		if reason == "rateLimitExceeded" && len(googleErr.Details) > 0 {
			for _, detail := range googleErr.Details {
				byteData, _ := json.Marshal(detail)
				var errDetail *model.ErrorDetail
				jsonErr := json.Unmarshal(byteData, &errDetail)
				if jsonErr != nil {
					ds.papi.LogError("Failed to parse error details", "err", jsonErr)
					continue
				}

				if errDetail != nil {
					// Even if the original "reason" is rateLimitExceeded, we need to check the QuotaLimit field in the metadata because it might only apply to this specific user.
					if errDetail.Reason == "RATE_LIMIT_EXCEEDED" && errDetail.Metadata.QuotaLimit == "defaultPerMinutePerUser" {
						err = ds.kvstore.StoreUserRateLimitExceeded(ds.userID)
						if err != nil {
							ds.papi.LogError("Failed to store user rate limit exceeded", "userID", ds.userID, "err", err)
							return
						}
					} else if errDetail.Reason == "RATE_LIMIT_EXCEEDED" && errDetail.Metadata.QuotaLimit == "defaultPerMinutePerProject" {
						err = ds.kvstore.StoreProjectRateLimitExceeded()
						if err != nil {
							ds.papi.LogError("Failed to store rate limit exceeded", "err", err)
							return
						}
					}
				}
			}
		}
	}
}

func (ds DriveService) checkRateLimits(ctx context.Context) error {
	userIsRateLimited, err := ds.kvstore.GetUserRateLimitExceeded(ds.userID)
	if err != nil {
		return err
	}
	if userIsRateLimited {
		return errors.New("user rate limit exceeded")
	}

	projectIsRateLimited, err := ds.kvstore.GetProjectRateLimitExceeded()
	if err != nil {
		return err
	}
	if projectIsRateLimited {
		return errors.New("project rate limit exceeded")
	}

	err = ds.limiter.WaitN(ctx, 1)
	if err != nil {
		return err
	}

	return nil
}

func (ds DriveService) About(ctx context.Context, fields googleapi.Field) (*drive.About, error) {
	err := ds.checkRateLimits(ctx)
	if err != nil {
		return nil, err
	}

	da, err := ds.service.About.Get().Fields(fields).Do()
	if err != nil {
		ds.parseGoogleErrors(err)
		return nil, err
	}
	return da, nil
}

func (ds DriveService) WatchChannel(ctx context.Context, startPageToken *drive.StartPageToken, requestChannel *drive.Channel) (*drive.Channel, error) {
	err := ds.checkRateLimits(ctx)
	if err != nil {
		return nil, err
	}

	da, err := ds.service.Changes.Watch(startPageToken.StartPageToken, requestChannel).Do()
	if err != nil {
		ds.parseGoogleErrors(err)
		return nil, err
	}
	return da, nil
}

func (ds DriveService) StopChannel(ctx context.Context, channel *drive.Channel) error {
	err := ds.checkRateLimits(ctx)
	if err != nil {
		return err
	}
	err = ds.service.Channels.Stop(channel).Do()
	if err != nil {
		ds.parseGoogleErrors(err)
		return err
	}
	return nil
}

func (ds DriveService) ChangesList(ctx context.Context, pageToken string) (*drive.ChangeList, error) {
	err := ds.checkRateLimits(ctx)
	if err != nil {
		return nil, err
	}
	changes, err := ds.service.Changes.List(pageToken).Fields("*").Do()
	if err != nil {
		ds.parseGoogleErrors(err)
		return nil, err
	}
	return changes, nil
}

func (ds DriveService) GetStartPageToken(ctx context.Context) (*drive.StartPageToken, error) {
	err := ds.checkRateLimits(ctx)
	if err != nil {
		return nil, err
	}
	tokenResponse, err := ds.service.Changes.GetStartPageToken().Do()
	if err != nil {
		ds.parseGoogleErrors(err)
		return nil, err
	}
	return tokenResponse, nil
}

func (ds DriveService) GetComments(ctx context.Context, fileID string, commentID string) (*drive.Comment, error) {
	err := ds.checkRateLimits(ctx)
	if err != nil {
		return nil, err
	}
	comment, err := ds.service.Comments.Get(fileID, commentID).Fields("*").IncludeDeleted(true).Do()
	if err != nil {
		ds.parseGoogleErrors(err)
		return nil, err
	}
	return comment, nil
}

func (ds DriveService) CreateReply(ctx context.Context, fileID string, commentID string, reply *drive.Reply) (*drive.Reply, error) {
	err := ds.checkRateLimits(ctx)
	if err != nil {
		return nil, err
	}
	googleReply, err := ds.service.Replies.Create(fileID, commentID, reply).Fields("*").Do()
	if err != nil {
		ds.parseGoogleErrors(err)
		return nil, err
	}
	return googleReply, nil
}

func (ds DriveService) CreateFile(ctx context.Context, file *drive.File, fileReader []byte) (*drive.File, error) {
	err := ds.checkRateLimits(ctx)
	if err != nil {
		return nil, err
	}
	googleFile, err := ds.service.Files.Create(file).Media(bytes.NewReader(fileReader)).Do()
	if err != nil {
		ds.parseGoogleErrors(err)
		return nil, err
	}
	return googleFile, nil
}

func (ds DriveService) GetFile(ctx context.Context, fileID string) (*drive.File, error) {
	err := ds.checkRateLimits(ctx)
	if err != nil {
		return nil, err
	}
	file, err := ds.service.Files.Get(fileID).Fields("*").Do()
	if err != nil {
		ds.parseGoogleErrors(err)
		return nil, err
	}
	return file, nil
}

func (ds DriveService) CreatePermission(ctx context.Context, fileID string, permission *drive.Permission) (*drive.Permission, error) {
	err := ds.checkRateLimits(ctx)
	if err != nil {
		return nil, err
	}
	googlePermission, err := ds.service.Permissions.Create(fileID, permission).Do()
	if err != nil {
		ds.parseGoogleErrors(err)
		return nil, err
	}
	return googlePermission, nil
}
