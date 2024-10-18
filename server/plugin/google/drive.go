package google

import (
	"bytes"
	"context"

	driveV2 "google.golang.org/api/drive/v2"
	"google.golang.org/api/drive/v3"

	"google.golang.org/api/googleapi"
)

type DriveService struct {
	service *drive.Service
	GoogleServiceBase
}

type DriveServiceV2 struct {
	serviceV2 *driveV2.Service
	GoogleServiceBase
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

func (ds DriveServiceV2) About(ctx context.Context, fields googleapi.Field) (*driveV2.About, error) {
	err := ds.checkRateLimits(ctx)
	if err != nil {
		return nil, err
	}

	da, err := ds.serviceV2.About.Get().Fields(fields).Do()
	if err != nil {
		ds.parseGoogleErrors(err)
		return nil, err
	}
	return da, nil
}
