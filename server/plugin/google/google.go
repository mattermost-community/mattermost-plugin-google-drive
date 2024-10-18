package google

import (
	"context"
	"encoding/json"
	"time"

	"github.com/mattermost/mattermost/server/public/plugin"
	"golang.org/x/oauth2"
	"golang.org/x/time/rate"
	"google.golang.org/api/drive/v3"
	"google.golang.org/api/option"

	"github.com/mattermost-community/mattermost-plugin-google-drive/server/plugin/config"
	"github.com/mattermost-community/mattermost-plugin-google-drive/server/plugin/kvstore"
	"github.com/mattermost-community/mattermost-plugin-google-drive/server/plugin/utils"
)

type Client struct {
	oauthConfig *oauth2.Config
	config      *config.Configuration
	kvstore     kvstore.KVStore
	papi        plugin.API
	limiter     *rate.Limiter
}

func NewGoogleClient(oauthConfig *oauth2.Config, config *config.Configuration, kvstore kvstore.KVStore, papi plugin.API) *Client {
	return &Client{
		oauthConfig: oauthConfig,
		config:      config,
		kvstore:     kvstore,
		papi:        papi,
		limiter:     rate.NewLimiter(rate.Every(time.Second), 10),
	}
}

func (g *Client) NewDriveService(ctx context.Context, userID string) (*DriveService, error) {
	authToken, err := g.getGoogleUserToken(userID)
	if err != nil {
		return nil, err
	}
	if !g.limiter.Allow() {
		err = g.limiter.WaitN(ctx, 1)
		if err != nil {
			return nil, err
		}
	}

	srv, err := drive.NewService(ctx, option.WithTokenSource(g.oauthConfig.TokenSource(ctx, authToken)))
	if err != nil {
		return nil, err
	}

	return &DriveService{
		service: srv,
		papi:    g.papi,
		limiter: g.limiter,
		userID:  userID,
		kvstore: g.kvstore,
	}, nil
}

func (g *Client) getGoogleUserToken(userID string) (*oauth2.Token, error) {
	encryptedToken, err := g.kvstore.GetGoogleUserToken(userID)
	if err != nil {
		return nil, err
	}

	if len(encryptedToken) == 0 {
		return nil, nil
	}

	decryptedToken, err := utils.Decrypt([]byte(g.config.EncryptionKey), string(encryptedToken))
	if err != nil {
		return nil, err
	}

	var oauthToken oauth2.Token
	err = json.Unmarshal([]byte(decryptedToken), &oauthToken)

	return &oauthToken, err
}
