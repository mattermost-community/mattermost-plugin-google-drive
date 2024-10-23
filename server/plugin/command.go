package plugin

import (
	"fmt"
	"strings"
	"unicode"

	"github.com/mattermost/mattermost/server/public/model"
	"github.com/mattermost/mattermost/server/public/plugin"
	"github.com/mattermost/mattermost/server/public/pluginapi/experimental/command"
	"github.com/pkg/errors"

	"github.com/mattermost-community/mattermost-plugin-google-drive/server/plugin/config"
)

type CommandHandleFunc func(c *plugin.Context, args *model.CommandArgs, parameters []string) string

const commandDescription = "Available commands: connect, disconnect, create, notifications, help, about"

func getAutocompleteData(config *config.Configuration) *model.AutocompleteData {
	if !config.IsOAuthConfigured() {
		drive := model.NewAutocompleteData("google-drive", "[command]", "Available commands: setup, about")

		setup := model.NewAutocompleteData("setup", "", "Set up the Google Drive plugin")
		setup.RoleID = model.SystemAdminRoleId
		drive.AddCommand(setup)

		about := command.BuildInfoAutocomplete("about")
		drive.AddCommand(about)

		return drive
	}

	drive := model.NewAutocompleteData("google-drive", "[command]", commandDescription)

	connect := model.NewAutocompleteData("connect", "", "Connect to your Google account")
	drive.AddCommand(connect)

	create := model.NewAutocompleteData("create", "[command]", "Create a new Google document, presentation or spreadsheet")
	drive.AddCommand(create)

	document := model.NewAutocompleteData("doc", "", "Create a new Google document")
	slide := model.NewAutocompleteData("slide", "", "Create a new Google presentation")
	sheet := model.NewAutocompleteData("sheet", "", "Create a new Google spreadsheet")
	create.AddCommand(document)
	create.AddCommand(slide)
	create.AddCommand(sheet)

	notifications := model.NewAutocompleteData("notifications", "[command]", "Configure Google Drive activity notifications")
	start := model.NewAutocompleteData("start", "", "Start Google Drive activity notifications")
	stop := model.NewAutocompleteData("stop", "", "Stop Google Drive activity notifications")
	notifications.AddCommand(start)
	notifications.AddCommand(stop)
	drive.AddCommand(notifications)

	help := model.NewAutocompleteData("help", "", "Display Slash Command help text")
	drive.AddCommand(help)

	about := command.BuildInfoAutocomplete("about")
	drive.AddCommand(about)

	disconnect := model.NewAutocompleteData("disconnect", "", "Disconnect your Google account")
	drive.AddCommand(disconnect)

	setup := model.NewAutocompleteData("setup", "", "Set up the Google Drive plugin")
	setup.RoleID = model.SystemAdminRoleId
	drive.AddCommand(setup)

	return drive
}

func (p *Plugin) getCommand(config *config.Configuration) (*model.Command, error) {
	iconData, err := command.GetIconData(&p.Client.System, "assets/icon-bg.svg")
	if err != nil {
		return nil, errors.Wrap(err, "failed to get icon data")
	}

	return &model.Command{
		Trigger:              "google-drive",
		AutoComplete:         true,
		AutoCompleteDesc:     commandDescription,
		AutoCompleteHint:     "[command]",
		AutocompleteData:     getAutocompleteData(config),
		AutocompleteIconData: iconData,
	}, nil
}

// parseCommand parses the entire command input string and retrieves the command, action and parameters
func parseCommand(input string) (command, action string, parameters []string) {
	split := make([]string, 0)
	current := ""
	inQuotes := false

	for _, char := range input {
		if unicode.IsSpace(char) {
			// keep whitespaces that are inside double qoutes
			if inQuotes {
				current += " "
				continue
			}

			// ignore successive whitespaces that are outside of double quotes
			if len(current) == 0 && !inQuotes {
				continue
			}

			// append the current word to the list & move on to the next word/expression
			split = append(split, current)
			current = ""
			continue
		}

		// append the current character to the current word
		current += string(char)

		if char == '"' {
			inQuotes = !inQuotes
		}
	}

	// append the last word/expression to the list
	if len(current) > 0 {
		split = append(split, current)
	}

	command = split[0]

	if len(split) > 1 {
		action = split[1]
	}

	if len(split) > 2 {
		parameters = split[2:]
	}

	return command, action, parameters
}

func (p *Plugin) ExecuteCommand(c *plugin.Context, args *model.CommandArgs) (*model.CommandResponse, *model.AppError) {
	cmd, action, parameters := parseCommand(args.Command)

	if cmd != "/google-drive" {
		return &model.CommandResponse{}, nil
	}

	if f, ok := p.CommandHandlers[action]; ok {
		message := f(c, args, parameters)
		if message != "" {
			post := &model.Post{
				UserId:    p.BotUserID,
				ChannelId: args.ChannelId,
				RootId:    args.RootId,
				Message:   message,
			}
			p.Client.Post.SendEphemeralPost(args.UserId, post)
		}
		return &model.CommandResponse{}, nil
	}

	return &model.CommandResponse{}, nil
}

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

func (p *Plugin) handleDisconnect(c *plugin.Context, args *model.CommandArgs, _ []string) string {
	encryptedToken, err := p.KVStore.GetGoogleUserToken(args.UserId)
	if err != nil {
		p.Client.Log.Error("Failed to disconnect google account", "error", err)
		return "Encountered an error disconnecting Google account."
	}

	if len(encryptedToken) == 0 {
		return "There is no Google account connected to your Mattermost account."
	}

	err = p.KVStore.DeleteGoogleUserToken(args.UserId)
	if err != nil {
		p.Client.Log.Error("Failed to disconnect Google account", "error", err)
		return "Encountered an error disconnecting Google account."
	}
	return "Disconnected your Google account."
}

func (p *Plugin) handleSetup(c *plugin.Context, args *model.CommandArgs, parameters []string) string {
	userID := args.UserId
	user, err := p.Client.User.Get(userID)
	if err != nil {
		p.Client.Log.Warn("Failed to check if user is System Admin", "error", err.Error())
		return "Error checking user's permissions"
	}
	if !strings.Contains(user.Roles, "system_admin") {
		return "Only System Admins are allowed to set up the plugin."
	}

	err = p.FlowManager.StartSetupWizard(userID)

	if err != nil {
		return err.Error()
	}

	return ""
}

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
