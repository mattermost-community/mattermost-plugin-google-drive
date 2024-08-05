package plugin

import (
	"encoding/json"
	"fmt"

	"github.com/mattermost/mattermost/server/public/model"
	"github.com/mattermost/mattermost/server/public/plugin"
	"golang.org/x/oauth2"
)

func (p *Plugin) getGoogleUserToken(userID string) (*oauth2.Token, error) {
	config := p.getConfiguration()

	var encryptedToken []byte
	err := p.client.KV.Get(getUserTokenKey(userID), &encryptedToken)
	if err != nil {
		return nil, err
	}

	if len(encryptedToken) == 0 {
		return nil, nil
	}

	decryptedToken, err := decrypt([]byte(config.EncryptionKey), string(encryptedToken))
	if err != nil {
		return nil, err
	}

	var oauthToken oauth2.Token
	err = json.Unmarshal([]byte(decryptedToken), &oauthToken)

	return &oauthToken, err
}

func (p *Plugin) isUserConnected(userID string) (bool, error) {
	var encryptedToken []byte
	err := p.client.KV.Get(getUserTokenKey(userID), &encryptedToken)
	if err != nil {
		return false, err
	}
	if len(encryptedToken) == 0 {
		return false, nil
	}
	return true, nil
}

func (p *Plugin) handleConnect(c *plugin.Context, args *model.CommandArgs, parameters []string) string {
	if connected, err := p.isUserConnected(args.UserId); connected && err == nil {
		return "You have already connected your Google account. If you want to reconnect then disconnect the account first using `/google-drive disconnect`."
	}
	siteURL := p.client.Configuration.GetConfig().ServiceSettings.SiteURL
	if siteURL == nil {
		return "Encountered an error connecting to Google Drive."
	}

	return fmt.Sprintf("[Click here to link your Google account.](%s/plugins/%s/oauth/connect)", *siteURL, manifest.Id)
}
