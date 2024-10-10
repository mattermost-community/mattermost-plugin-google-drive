package plugin

import (
	"github.com/mattermost/mattermost/server/public/model"
	"github.com/mattermost/mattermost/server/public/plugin"
)

func (p *Plugin) handleDisconnect(c *plugin.Context, args *model.CommandArgs, _ []string) string {
	encryptedToken, err := p.KVStore.GetGoogleUserToken(args.UserId)
	if err != nil {
		p.Client.Log.Error("Failed to disconnect google account", "error", err)
		return "Encountered an error disconnecting Google account."
	}

	if len(encryptedToken) == 0 {
		return "There is no google account connected to your mattermost account."
	}

	err = p.KVStore.DeleteGoogleUserToken(args.UserId)
	if err != nil {
		p.Client.Log.Error("Failed to disconnect google account", "error", err)
		return "Encountered an error disconnecting Google account."
	}
	return "Disconnected your Google account."
}
