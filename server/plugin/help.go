package plugin

import (
	"strings"

	"github.com/mattermost/mattermost/server/public/model"
	"github.com/mattermost/mattermost/server/public/plugin"
)

const commandHelp = `* |/google-drive connect| - Connect to your Google account
* |/google-drive disconnect| - Disconnect your Google account
* |/google-drive create [doc/slide/sheet]| - Create and share Google documents, spreadsheets and presentations right from Mattermost.
* |/google-drive notifications start| - Enable notification for Google files sharing and comments on files.
* |/google-drive notifications stop| - Disable notification for Google files sharing and comments on files.
* |/google-drive help| - Get help for available slash commands.
* |/google-drive about| - Display build information about the plugin.`

func (p *Plugin) handleHelp(_ *plugin.Context, _ *model.CommandArgs, _ []string) string {
	return "###### Mattermost Google Drive Plugin - Slash Command Help\n" + strings.ReplaceAll(commandHelp, "|", "`")
}
