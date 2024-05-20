package plugin

import (
	"github.com/mattermost/mattermost/server/public/model"
	"github.com/mattermost/mattermost/server/public/plugin"
)

func (p *Plugin) handleDisconnect(c *plugin.Context, args *model.CommandArgs, _ []string) string {
	err := p.client.KV.Delete(args.UserId + "_token")
	if err != nil {
		p.client.Log.Error("Failed to disconnect google account", "error", err)
		return "Encountered an error disconnecting Google account."
	}
	return "Disconnected your Google account."
}
