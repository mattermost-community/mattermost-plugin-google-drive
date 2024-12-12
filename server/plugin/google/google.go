package google

import (
	"context"
	"encoding/json"
	"errors"

	"github.com/mattermost/mattermost/server/public/plugin"
	oauth2package "golang.org/x/oauth2"
	"golang.org/x/time/rate"
	"google.golang.org/api/docs/v1"
	driveV2 "google.golang.org/api/drive/v2"
	"google.golang.org/api/drive/v3"
	"google.golang.org/api/driveactivity/v2"
	"google.golang.org/api/googleapi"
	"google.golang.org/api/option"
	"google.golang.org/api/sheets/v4"
	"google.golang.org/api/slides/v1"

	"github.com/mattermost-community/mattermost-plugin-google-drive/server/plugin/config"
	"github.com/mattermost-community/mattermost-plugin-google-drive/server/plugin/kvstore"
	"github.com/mattermost-community/mattermost-plugin-google-drive/server/plugin/model"
	"github.com/mattermost-community/mattermost-plugin-google-drive/server/plugin/oauth2"
	"github.com/mattermost-community/mattermost-plugin-google-drive/server/plugin/utils"
)

type Client struct {
	oauthConfig  oauth2.Config
	config       *config.Configuration
	kvstore      kvstore.KVStore
	papi         plugin.API
	driveLimiter *rate.Limiter
}

type googleServiceBase struct {
	serviceType string
	limiter     *rate.Limiter
	papi        plugin.API
	userID      string
	kvstore     kvstore.KVStore
}

const (
	driveServiceType         = "drive"
	docsServiceType          = "docs"
	slidesServiceType        = "slides"
	sheetsServiceType        = "sheets"
	driveActivityServiceType = "driveactivity"
)

func NewGoogleClient(oauthConfig oauth2.Config, config *config.Configuration, kvstore kvstore.KVStore, papi plugin.API) ClientInterface {
	maximumQueriesPerSecond := float64(config.QueriesPerMinute) / 60
	burstSize := config.BurstSize

	return &Client{
		oauthConfig:  oauthConfig,
		config:       config,
		kvstore:      kvstore,
		papi:         papi,
		driveLimiter: rate.NewLimiter(rate.Limit(maximumQueriesPerSecond), burstSize),
	}
}

func (g *Client) NewDriveService(ctx context.Context, userID string) (DriveInterface, error) {
	authToken, err := g.GetGoogleUserToken(userID)
	if err != nil {
		return nil, err
	}

	err = checkKVStoreLimitExceeded(g.kvstore, driveServiceType, userID)
	if err != nil {
		return nil, err
	}

	srv, err := drive.NewService(ctx, option.WithTokenSource(g.oauthConfig.TokenSource(ctx, authToken)))
	if err != nil {
		return nil, err
	}

	return &DriveService{
		service: srv,
		googleServiceBase: googleServiceBase{
			serviceType: driveServiceType,
			papi:        g.papi,
			limiter:     g.driveLimiter,
			userID:      userID,
			kvstore:     g.kvstore,
		},
	}, nil
}

func (g *Client) NewDriveV2Service(ctx context.Context, userID string) (DriveV2Interface, error) {
	authToken, err := g.GetGoogleUserToken(userID)
	if err != nil {
		return nil, err
	}

	err = checkKVStoreLimitExceeded(g.kvstore, driveServiceType, userID)
	if err != nil {
		return nil, err
	}

	srv, err := driveV2.NewService(ctx, option.WithTokenSource(g.oauthConfig.TokenSource(ctx, authToken)))
	if err != nil {
		return nil, err
	}

	return &DriveServiceV2{
		serviceV2: srv,
		googleServiceBase: googleServiceBase{
			serviceType: driveServiceType,
			papi:        g.papi,
			limiter:     g.driveLimiter,
			userID:      userID,
			kvstore:     g.kvstore,
		},
	}, nil
}

func (g *Client) NewDocsService(ctx context.Context, userID string) (DocsInterface, error) {
	authToken, err := g.GetGoogleUserToken(userID)
	if err != nil {
		return nil, err
	}

	err = checkKVStoreLimitExceeded(g.kvstore, docsServiceType, userID)
	if err != nil {
		return nil, err
	}

	srv, err := docs.NewService(ctx, option.WithTokenSource(g.oauthConfig.TokenSource(ctx, authToken)))
	if err != nil {
		return nil, err
	}

	return &DocsService{
		service: srv,
		googleServiceBase: googleServiceBase{
			serviceType: docsServiceType,
			papi:        g.papi,
			limiter:     nil,
			userID:      userID,
			kvstore:     g.kvstore,
		},
	}, nil
}

func (g *Client) NewSlidesService(ctx context.Context, userID string) (SlidesInterface, error) {
	authToken, err := g.GetGoogleUserToken(userID)
	if err != nil {
		return nil, err
	}

	err = checkKVStoreLimitExceeded(g.kvstore, slidesServiceType, userID)
	if err != nil {
		return nil, err
	}

	srv, err := slides.NewService(ctx, option.WithTokenSource(g.oauthConfig.TokenSource(ctx, authToken)))
	if err != nil {
		return nil, err
	}

	return &SlidesService{
		service: srv,
		googleServiceBase: googleServiceBase{
			serviceType: slidesServiceType,
			papi:        g.papi,
			limiter:     nil,
			userID:      userID,
			kvstore:     g.kvstore,
		},
	}, nil
}

func (g *Client) NewSheetsService(ctx context.Context, userID string) (SheetsInterface, error) {
	authToken, err := g.GetGoogleUserToken(userID)
	if err != nil {
		return nil, err
	}

	err = checkKVStoreLimitExceeded(g.kvstore, sheetsServiceType, userID)
	if err != nil {
		return nil, err
	}

	srv, err := sheets.NewService(ctx, option.WithTokenSource(g.oauthConfig.TokenSource(ctx, authToken)))
	if err != nil {
		return nil, err
	}

	return &SheetsService{
		service: srv,
		googleServiceBase: googleServiceBase{
			serviceType: sheetsServiceType,
			papi:        g.papi,
			limiter:     nil,
			userID:      userID,
			kvstore:     g.kvstore,
		},
	}, nil
}

func (g *Client) NewDriveActivityService(ctx context.Context, userID string) (DriveActivityInterface, error) {
	authToken, err := g.GetGoogleUserToken(userID)
	if err != nil {
		return nil, err
	}

	err = checkKVStoreLimitExceeded(g.kvstore, driveActivityServiceType, userID)
	if err != nil {
		return nil, err
	}

	srv, err := driveactivity.NewService(ctx, option.WithTokenSource(g.oauthConfig.TokenSource(ctx, authToken)))
	if err != nil {
		return nil, err
	}

	return &DriveActivityService{
		service: srv,
		googleServiceBase: googleServiceBase{
			serviceType: driveActivityServiceType,
			papi:        g.papi,
			limiter:     nil,
			userID:      userID,
			kvstore:     g.kvstore,
		},
	}, nil
}

func (g *Client) GetGoogleUserToken(userID string) (*oauth2package.Token, error) {
	encryptedToken, err := g.kvstore.GetGoogleUserToken(userID)
	if err != nil {
		return nil, err
	}

	if len(encryptedToken) == 0 {
		return nil, nil
	}

	decryptedToken, err := utils.Decrypt([]byte(g.config.EncryptionKey), encryptedToken)
	if err != nil {
		return nil, err
	}

	var oauthToken oauth2package.Token
	err = json.Unmarshal(decryptedToken, &oauthToken)

	return &oauthToken, err
}

func (ds googleServiceBase) checkForRateLimitErrors(apiErr error) error {
	if googleErr, ok := apiErr.(*googleapi.Error); ok {
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
			err := ds.kvstore.StoreUserRateLimitExceeded(ds.serviceType, ds.userID)
			if err != nil {
				return errors.Join(apiErr, err)
			}
		}
		if reason == "rateLimitExceeded" && len(googleErr.Details) > 0 {
			for _, detail := range googleErr.Details {
				byteData, _ := json.Marshal(detail)
				var errDetail *model.ErrorDetail
				jsonErr := json.Unmarshal(byteData, &errDetail)
				if jsonErr != nil {
					ds.papi.LogWarn("Failed to parse error details", "err", jsonErr)
					continue
				}

				if errDetail != nil {
					// Even if the original "reason" is rateLimitExceeded, we need to check the QuotaLimit field in the metadata because it might only apply to this specific user.
					if errDetail.Reason == "RATE_LIMIT_EXCEEDED" && errDetail.Metadata.QuotaLimit == "defaultPerMinutePerUser" {
						err := ds.kvstore.StoreUserRateLimitExceeded(ds.serviceType, ds.userID)
						if err != nil {
							return errors.Join(apiErr, err)
						}
					} else {
						err := ds.kvstore.StoreProjectRateLimitExceeded(ds.serviceType)
						if err != nil {
							return errors.Join(apiErr, err)
						}
					}
				}
			}
		}
	}

	return apiErr
}

func checkKVStoreLimitExceeded(kv kvstore.KVStore, serviceType string, userID string) error {
	userIsRateLimited, err := kv.GetUserRateLimitExceeded(serviceType, userID)
	if err != nil {
		return err
	}
	if userIsRateLimited {
		return errors.New("user rate limit exceeded for Google service: " + serviceType)
	}

	projectIsRateLimited, err := kv.GetProjectRateLimitExceeded(serviceType)
	if err != nil {
		return err
	}
	if projectIsRateLimited {
		return errors.New("project rate limit exceeded for Google service: " + serviceType)
	}

	return nil
}

func (ds googleServiceBase) checkRateLimits(ctx context.Context) error {
	err := checkKVStoreLimitExceeded(ds.kvstore, ds.serviceType, ds.userID)
	if err != nil {
		return err
	}
	if ds.limiter != nil {
		err = ds.limiter.WaitN(ctx, 1)
		if err != nil {
			return err
		}
	}

	return nil
}

func (g *Client) ReloadConfigs(newQueriesPerMinute int, newBurstSize int, oauthConfig oauth2.Config) {
	g.oauthConfig = oauthConfig
	g.driveLimiter.SetLimit(rate.Limit(float64(newQueriesPerMinute) / 60))
	g.driveLimiter.SetBurst(newBurstSize)
}
