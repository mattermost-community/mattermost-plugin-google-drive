package plugin

import (
	"fmt"

	"github.com/mattermost/mattermost/server/public/model"
	"github.com/mattermost/mattermost/server/public/plugin"
)

func (p *Plugin) handleConnect(c *plugin.Context, args *model.CommandArgs, parameters []string) string {
	encryptedToken, err := p.KVStore.GetGoogleUserToken(args.UserId)
	if err != nil {
		return "Encountered an error connecting to Google Drive."
	}
	if len(encryptedToken) > 0 {
		return "You have already connected your Google account. If you want to reconnect then disconnect the account first using `/google-drive disconnect`."
	}
	siteURL := p.Client.Configuration.GetConfig().ServiceSettings.SiteURL
	if siteURL == nil {
		return "Encountered an error connecting to Google Drive."
	}

	return fmt.Sprintf("[Click here to link your Google account.](%s/plugins/%s/oauth/connect)", *siteURL, Manifest.Id)
}
