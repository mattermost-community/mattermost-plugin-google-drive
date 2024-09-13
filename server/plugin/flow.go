package plugin

import (
	"fmt"
	"strings"

	"github.com/gorilla/mux"
	"github.com/pkg/errors"

	"github.com/mattermost/mattermost/server/public/model"
	"github.com/mattermost/mattermost/server/public/pluginapi"
	"github.com/mattermost/mattermost/server/public/pluginapi/experimental/flow"
)

type Tracker interface {
	TrackEvent(event string, properties map[string]interface{})
	TrackUserEvent(event, userID string, properties map[string]interface{})
}

type FlowManager struct {
	client           *pluginapi.Client
	botUserID        string
	router           *mux.Router
	getConfiguration func() *Configuration

	tracker Tracker

	setupFlow        *flow.Flow
	oauthFlow        *flow.Flow
	announcementFlow *flow.Flow
}

func (p *Plugin) NewFlowManager() *FlowManager {
	fm := &FlowManager{
		client:           p.client,
		botUserID:        p.BotUserID,
		router:           p.router,
		getConfiguration: p.getConfiguration,

		tracker: p,
	}

	fm.setupFlow = fm.newFlow("setup").WithSteps(
		fm.stepWelcome(),

		fm.stepOAuthInfo(),
		fm.stepOAuthInput(),
		fm.stepOAuthConnect(),

		fm.stepAnnouncementQuestion(),
		fm.stepAnnouncementConfirmation(),

		fm.doneStep(),

		fm.stepCancel("setup"),
	)

	return fm
}

func (fm *FlowManager) doneStep() flow.Step {
	return flow.NewStep(stepDone).
		WithText(":tada: You've successfully setup Google Drive Plugin.").
		OnRender(fm.onDone).Terminal()
}

func (fm *FlowManager) onDone(f *flow.Flow) {
	fm.trackCompleteSetupWizard(f.UserID)
}

func (fm *FlowManager) newFlow(name flow.Name) *flow.Flow {
	flow, _ := flow.NewFlow(
		name,
		fm.client,
		manifest.Id,
		fm.botUserID,
	)

	flow.InitHTTP(fm.router)
	return flow
}

const (
	stepOAuthInfo    flow.Name = "oauth-info"
	stepOAuthInput   flow.Name = "oauth-input"
	stepOAuthConnect flow.Name = "oauth-connect"

	stepWelcome flow.Name = "welcome"
	stepDone    flow.Name = "done"
	stepCancel  flow.Name = "cancel"

	stepAnnouncementQuestion     flow.Name = "announcement-question"
	stepAnnouncementConfirmation flow.Name = "announcement-confirmation"

	keyIsOAuthConfigured = "IsOAuthConfigured"
)

func cancelButton() flow.Button {
	return flow.Button{
		Name:    "Cancel setup",
		Color:   flow.ColorDanger,
		OnClick: flow.Goto(stepCancel),
	}
}

func (fm *FlowManager) stepCancel(command string) flow.Step {
	return flow.NewStep(stepCancel).
		Terminal().
		WithText(fmt.Sprintf("Google Drive integration setup has stopped. Restart setup later by running `/google-drive %s`. Learn more about the plugin [here](%s).", command, manifest.HomepageURL)).
		WithColor(flow.ColorDanger)
}

func continueButtonF(f func(f *flow.Flow) (flow.Name, flow.State, error)) flow.Button {
	return flow.Button{
		Name:    "Continue",
		Color:   flow.ColorPrimary,
		OnClick: f,
	}
}

func continueButton(next flow.Name) flow.Button {
	return continueButtonF(flow.Goto(next))
}

func (fm *FlowManager) getBaseState() flow.State {
	config := fm.getConfiguration()
	return flow.State{
		keyIsOAuthConfigured: config.IsOAuthConfigured(),
	}
}

func (fm *FlowManager) StartSetupWizard(userID string) error {
	state := fm.getBaseState()

	err := fm.setupFlow.ForUser(userID).Start(state)
	if err != nil {
		return err
	}

	fm.client.Log.Debug("Started setup wizard", "userID", userID)

	fm.trackStartSetupWizard(userID, false)

	return nil
}

func (fm *FlowManager) trackStartSetupWizard(userID string, fromInvite bool) {
	fm.tracker.TrackUserEvent("setup_wizard_start", userID, map[string]interface{}{
		"from_invite": fromInvite,
		"time":        model.GetMillis(),
	})
}

func (fm *FlowManager) trackCompleteSetupWizard(userID string) {
	fm.tracker.TrackUserEvent("setup_wizard_complete", userID, map[string]interface{}{
		"time": model.GetMillis(),
	})
}

func (fm *FlowManager) StartOauthWizard(userID string) error {
	state := fm.getBaseState()

	err := fm.oauthFlow.ForUser(userID).Start(state)
	if err != nil {
		return err
	}

	fm.trackStartOauthWizard(userID)

	return nil
}

func (fm *FlowManager) trackStartOauthWizard(userID string) {
	fm.tracker.TrackUserEvent("oauth_wizard_start", userID, map[string]interface{}{
		"time": model.GetMillis(),
	})
}

func (fm *FlowManager) trackCompleteOauthWizard(userID string) {
	fm.tracker.TrackUserEvent("oauth_wizard_complete", userID, map[string]interface{}{
		"time": model.GetMillis(),
	})
}

func (fm *FlowManager) stepWelcome() flow.Step {
	welcomePretext := ":wave: Welcome to your Google Drive integration! [Learn more](https://github.com/mattermost/mattermost-plugin-google-drive#readme)"

	welcomeText := `
Just a few configuration steps to go!
- **Step 1:** Register an OAuth application in Google Cloud Console and enter OAuth values.
- **Step 2:** Connect your Google account
`

	return flow.NewStep(stepWelcome).
		WithText(welcomeText).
		WithPretext(welcomePretext).
		WithButton(continueButton(""))
}

func (fm *FlowManager) stepOAuthInfo() flow.Step {
	oauthPretext := `
##### :white_check_mark: Step 1: Register an OAuth Application in Google Cloud Console
You must first register the Mattermost Google Drive Plugin as an authorized OAuth app.`
	oauthMessage := fmt.Sprintf(
		"1. Create a new Project. You would need to redirect to [Google Cloud Console](https://console.cloud.google.com/home/dashboard) and select the option to **New project**. Then, select the name and the organization (optional).\n"+
			"2. Select APIs. After creating a project, on the left side menu on **APIs & Services**, then, select the first option **Enabled APIs & Services** and wait, the page will redirect.\n"+
			"3. Click on **Enable APIs and Services** option. Once you are in the API Library, search and enable following APIs:\n"+
			"	- Google Drive API\n"+
			"	- Google Docs API\n"+
			"	- Google Slides API\n"+
			"	- Google Sheets API\n"+
			"	- Google Drive Activity API\n"+
			"4. Go back to **APIs & Services** menu.\n"+
			"5. Create a new OAuth consent screen. Select the option **OAuth consent screen**  on the menu bar. If you would like to limit your application to organization-only users, select **Internal**, otherwise, select **External** option, then, fill the form with the data you would use for your project.\n"+
			"6. Go back to **APIs & Services** menu.\n"+
			"7. Create a new Client. Select the option **Credentials**, and on the menu bar, select **Create credentials**, a dropdown menu will be displayed, then, select **OAuth Client ID** option.\n"+
			"8. Then, a select input will ask the type of Application type that will be used, select **Web application**, then, fill the form, and on **Authorized redirect URIs** add the following URI.\n"+
			"	- Redirect URI: `%s/plugins/%s/oauth/complete`\n"+
			"9. After the Client has been configured, on the main page of **Credentials**, on the submenu **OAuth 2.0 Client IDs** will be displayed the new Client and the info can be accessible whenever you need it.",
		*fm.client.Configuration.GetConfig().ServiceSettings.SiteURL, manifest.Id,
	)

	return flow.NewStep(stepOAuthInfo).
		WithPretext(oauthPretext).
		WithText(oauthMessage).
		WithButton(continueButton(stepOAuthInput)).
		WithButton(cancelButton())
}

func (fm *FlowManager) stepOAuthInput() flow.Step {
	return flow.NewStep(stepOAuthInput).
		WithText("Click the Continue button below to open a dialog to enter the **Google OAuth Client ID** and **Google OAuth Client Secret**.").
		WithButton(flow.Button{
			Name:  "Continue",
			Color: flow.ColorPrimary,
			Dialog: &model.Dialog{
				Title:            "Google OAuth values",
				IntroductionText: "Please enter the **Google OAuth Client ID** and **Google OAuth Client Secret** you copied in a previous step.{{ if .IsOAuthConfigured }}\n\n**Any existing OAuth configuration will be overwritten.**{{end}}",
				SubmitLabel:      "Save & continue",
				Elements: []model.DialogElement{
					{
						DisplayName: "Google Client ID",
						Name:        "client_id",
						Type:        "text",
						SubType:     "text",
						Placeholder: "Enter Google OAuth Client ID",
					},
					{
						DisplayName: "Google Client Secret",
						Name:        "client_secret",
						Type:        "text",
						SubType:     "text",
						Placeholder: "Enter Google OAuth Client Secret",
					},
				},
			},
			OnDialogSubmit: fm.submitOAuthConfig,
		}).
		WithButton(cancelButton())
}

func (fm *FlowManager) submitOAuthConfig(f *flow.Flow, submitted map[string]interface{}) (flow.Name, flow.State, map[string]string, error) {
	errorList := map[string]string{}

	clientIDRaw, ok := submitted["client_id"]
	if !ok {
		return "", nil, nil, errors.New("client_id missing")
	}
	clientID, ok := clientIDRaw.(string)
	if !ok {
		return "", nil, nil, errors.New("client_id is not a string")
	}

	clientID = strings.TrimSpace(clientID)
	clientSecretRaw, ok := submitted["client_secret"]
	if !ok {
		return "", nil, nil, errors.New("client_secret missing")
	}
	clientSecret, ok := clientSecretRaw.(string)
	if !ok {
		return "", nil, nil, errors.New("client_secret is not a string")
	}

	clientSecret = strings.TrimSpace(clientSecret)

	if len(errorList) != 0 {
		return "", nil, errorList, nil
	}

	config := fm.getConfiguration()
	config.GoogleOAuthClientID = clientID
	config.GoogleOAuthClientSecret = clientSecret

	configMap, err := config.ToMap()
	if err != nil {
		return "", nil, nil, err
	}

	err = fm.client.Configuration.SavePluginConfig(configMap)
	if err != nil {
		return "", nil, nil, errors.Wrap(err, "failed to save plugin config")
	}

	return "", nil, nil, nil
}

func (fm *FlowManager) stepOAuthConnect() flow.Step {
	connectPretext := "##### :white_check_mark: Connect your Google account"
	connectURL := fmt.Sprintf("%s/plugins/%s/oauth/connect", *fm.client.Configuration.GetConfig().ServiceSettings.SiteURL, manifest.Id)
	connectText := fmt.Sprintf("Go [here](%s) to connect your account.", connectURL)
	return flow.NewStep(stepOAuthConnect).
		WithText(connectText).
		WithPretext(connectPretext).
		OnRender(func(f *flow.Flow) { fm.trackCompleteOauthWizard(f.UserID) }).
		Next("")
}

func (fm *FlowManager) StartAnnouncementWizard(userID string) error {
	state := fm.getBaseState()

	err := fm.announcementFlow.ForUser(userID).Start(state)
	if err != nil {
		return err
	}

	fm.trackStartAnnouncementWizard(userID)

	return nil
}

func (fm *FlowManager) trackStartAnnouncementWizard(userID string) {
	fm.tracker.TrackUserEvent("announcement_wizard_start", userID, map[string]interface{}{
		"time": model.GetMillis(),
	})
}

func (fm *FlowManager) stepAnnouncementQuestion() flow.Step {
	defaultMessage := "Hi team,\n" +
		"\n" +
		"We've set up the Mattermost Google Drive plugin to enable document creation, file uploads and file activity notifications in Mattermost. To get started, run the `/google-drive connect` slash command from any channel within Mattermost to connect your Google account. See the [documentation](https://github.com/mattermost/mattermost-plugin-google-drive/) for details on using the Google Drive plugin."

	return flow.NewStep(stepAnnouncementQuestion).
		WithText("Want to let your team know?").
		WithButton(flow.Button{
			Name:  "Send Message",
			Color: flow.ColorPrimary,
			Dialog: &model.Dialog{
				Title:       "Notify your team",
				SubmitLabel: "Send message",
				Elements: []model.DialogElement{
					{
						DisplayName: "To",
						Name:        "channel_id",
						Type:        "select",
						Placeholder: "Select channel",
						DataSource:  "channels",
					},
					{
						DisplayName: "Message",
						Name:        "message",
						Type:        "textarea",
						Default:     defaultMessage,
						HelpText:    "You can edit this message before sending it.",
					},
				},
			},
			OnDialogSubmit: fm.submitChannelAnnouncement,
		}).
		WithButton(flow.Button{
			Name:    "Not now",
			Color:   flow.ColorDefault,
			OnClick: flow.Goto(stepDone),
		})
}

func (fm *FlowManager) stepAnnouncementConfirmation() flow.Step {
	return flow.NewStep(stepAnnouncementConfirmation).
		WithText("Message to ~{{ .ChannelName }} was sent.").
		Next("").
		OnRender(func(f *flow.Flow) { fm.trackCompleteAnnouncementWizard(f.UserID) })
}

func (fm *FlowManager) submitChannelAnnouncement(f *flow.Flow, submitted map[string]interface{}) (flow.Name, flow.State, map[string]string, error) {
	channelIDRaw, ok := submitted["channel_id"]
	if !ok {
		return "", nil, nil, errors.New("channel_id missing")
	}
	channelID, ok := channelIDRaw.(string)
	if !ok {
		return "", nil, nil, errors.New("channel_id is not a string")
	}

	channel, err := fm.client.Channel.Get(channelID)
	if err != nil {
		return "", nil, nil, errors.Wrap(err, "failed to get channel")
	}

	messageRaw, ok := submitted["message"]
	if !ok {
		return "", nil, nil, errors.New("message is not a string")
	}
	message, ok := messageRaw.(string)
	if !ok {
		return "", nil, nil, errors.New("message is not a string")
	}

	post := &model.Post{
		UserId:    f.UserID,
		ChannelId: channel.Id,
		Message:   message,
	}
	err = fm.client.Post.CreatePost(post)
	if err != nil {
		return "", nil, nil, errors.Wrap(err, "failed to create announcement post")
	}

	return stepAnnouncementConfirmation, flow.State{
		"ChannelName": channel.Name,
	}, nil, nil
}

func (fm *FlowManager) trackCompleteAnnouncementWizard(userID string) {
	fm.tracker.TrackUserEvent("announcement_wizard_complete", userID, map[string]interface{}{
		"time": model.GetMillis(),
	})
}
