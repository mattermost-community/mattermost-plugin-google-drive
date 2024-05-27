package plugin

import (
	"strings"

	"github.com/mattermost/mattermost/server/public/model"
	"github.com/mattermost/mattermost/server/public/plugin"
)

const commandHelp = `* |/drive connect| - Connect to your Google account
* |/drive disconnect| - Disconnect your Google account
* |/drive create [doc/slide/sheet]| - Create and share Google documents, spreadsheets and presentations right from Mattermost.
* |/drive notifications start| - Enable notification for Google files sharing and comments on files.
* |/drive notifications stop| - Disable notification for Google files sharing and comments on files.
* |/drive help| - Get help for available slash commands.
* |/drive about| - Display build information about the plugin.`

func (p *Plugin) handleHelp(_ *plugin.Context, _ *model.CommandArgs, _ []string) string {
	return "###### Mattermost Google Drive Plugin - Slash Command Help\n" + strings.ReplaceAll(commandHelp, "|", "`")
}
