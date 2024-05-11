package plugin

import (
	"strings"

	"github.com/mattermost/mattermost/server/public/model"
	"github.com/mattermost/mattermost/server/public/plugin"
)

const commandHelp = `* |/gdrive connect| - Connect to your Google account
* |/gdrive disconnect| - Disconnect your Google account
* |/gdrive create [doc/slide/sheet]| - Create and share Google documents, spreadsheets and presentations right from Mattermost.
* |/gdrive notifications start| - Enable notification for Google files sharing and comments on files.
* |/gdrive notifications stop| - Disable notification for Google files sharing and comments on files.
* |/gdrive help| - Get help for available slash commands.
* |/gdrive about| - Display build information about the plugin.`

func (p *Plugin) handleHelp(_ *plugin.Context, _ *model.CommandArgs, _ []string) string {
	return "###### Mattermost Google Drive Plugin - Slash Command Help\n" + strings.ReplaceAll(commandHelp, "|", "`")
}
