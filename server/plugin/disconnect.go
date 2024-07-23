package plugin

import (
	"github.com/mattermost/mattermost/server/public/model"
	"github.com/mattermost/mattermost/server/public/plugin"
)

func (p *Plugin) handleDisconnect(c *plugin.Context, args *model.CommandArgs, _ []string) string {
	var encryptedToken []byte
	_ = p.client.KV.Get(getUserTokenKey(args.UserId), &encryptedToken)
	if len(encryptedToken) == 0 {
		return "There is no google account connected to your mattermost account."
	}
	err := p.client.KV.Delete(getUserTokenKey(args.UserId))
	if err != nil {
		p.client.Log.Error("Failed to disconnect google account", "error", err)
		return "Encountered an error disconnecting Google account."
	}
	return "Disconnected your Google account."
}
