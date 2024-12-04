package oauth2

import (
	"context"

	oauth2package "golang.org/x/oauth2"

	"github.com/mattermost-community/mattermost-plugin-google-drive/server/plugin/config"
)

type Config interface {
	Exchange(ctx context.Context, code string) (*oauth2package.Token, error)
	AuthCodeURL(state string) string
	TokenSource(ctx context.Context, t *oauth2package.Token) oauth2package.TokenSource
	ReloadConfig(config *config.Configuration)
}
