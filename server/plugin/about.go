package plugin

import (
	"github.com/mattermost/mattermost/server/public/model"
	"github.com/mattermost/mattermost/server/public/plugin"
	"github.com/mattermost/mattermost/server/public/pluginapi/experimental/command"
	"github.com/pkg/errors"
)

func (p *Plugin) handleAbout(c *plugin.Context, args *model.CommandArgs, parameters []string) string {
	text, err := command.BuildInfo(model.Manifest{
		Id:      Manifest.Id,
		Version: Manifest.Version,
		Name:    Manifest.Name,
	})
	if err != nil {
		text = errors.Wrap(err, "failed to get build info").Error()
	}
	return text
}
