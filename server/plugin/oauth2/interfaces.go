package oauth2

import (
	"context"

	oauth2package "golang.org/x/oauth2"
)

type Config interface {
	Exchange(ctx context.Context, code string) (*oauth2package.Token, error)
	AuthCodeURL(state string) string
	TokenSource(ctx context.Context, t *oauth2package.Token) oauth2package.TokenSource
}
