package oauth2

import (
	"context"
	"fmt"
	"net/url"

	oauth2package "golang.org/x/oauth2"
	"golang.org/x/oauth2/google"

	"github.com/mattermost-community/mattermost-plugin-google-drive/server/plugin/config"
)

type ConfigWrapper struct {
	oauth2Config *oauth2package.Config
}

func GetOAuthConfig(config *config.Configuration, siteURL *string, manifestID string) Config {
	scopes := []string{
		"https://www.googleapis.com/auth/userinfo.profile",
		"https://www.googleapis.com/auth/userinfo.email",
		"https://www.googleapis.com/auth/drive.file",
		"https://www.googleapis.com/auth/drive.activity",
		"https://www.googleapis.com/auth/documents",
		"https://www.googleapis.com/auth/presentations",
		"https://www.googleapis.com/auth/spreadsheets",
		"https://www.googleapis.com/auth/drive",
	}

	redirectURL := fmt.Sprintf("%s/plugins/%s/oauth/complete",
		*siteURL,
		url.PathEscape(manifestID))

	return &ConfigWrapper{
		oauth2Config: &oauth2package.Config{
			ClientID:     config.GoogleOAuthClientID,
			ClientSecret: config.GoogleOAuthClientSecret,
			Scopes:       scopes,
			RedirectURL:  redirectURL,
			Endpoint:     google.Endpoint,
		},
	}
}

func (oauth2 *ConfigWrapper) Exchange(ctx context.Context, code string) (*oauth2package.Token, error) {
	return oauth2.oauth2Config.Exchange(ctx, code)
}

func (oauth2 *ConfigWrapper) AuthCodeURL(state string) string {
	return oauth2.oauth2Config.AuthCodeURL(state, oauth2package.AccessTypeOffline, oauth2package.SetAuthURLParam("prompt", "consent"))
}

func (oauth2 *ConfigWrapper) TokenSource(ctx context.Context, t *oauth2package.Token) oauth2package.TokenSource {
	return oauth2.oauth2Config.TokenSource(ctx, t)
}

// create a function to reload the config
func (oauth2 *ConfigWrapper) ReloadConfig(config *config.Configuration) {
	oauth2.oauth2Config.ClientID = config.GoogleOAuthClientID
	oauth2.oauth2Config.ClientSecret = config.GoogleOAuthClientSecret
}
