package plugin

import (
	"fmt"
	"path/filepath"
	"sync"

	"github.com/gorilla/mux"
	"github.com/mattermost/mattermost/server/public/model"
	"github.com/mattermost/mattermost/server/public/plugin"
	"github.com/mattermost/mattermost/server/public/pluginapi"
	"github.com/mattermost/mattermost/server/public/pluginapi/experimental/bot/poster"
	"github.com/mattermost/mattermost/server/public/pluginapi/experimental/telemetry"
	"github.com/pkg/errors"
	"golang.org/x/oauth2"
	"golang.org/x/oauth2/google"
)

const (
	WSEventConfigUpdate = "config_update"
)

type kvStore interface {
	Set(key string, value any, options ...pluginapi.KVSetOption) (bool, error)
	ListKeys(page int, count int, options ...pluginapi.ListKeysOption) ([]string, error)
	Get(key string, o any) error
	Delete(key string) error
}

// Plugin implements the interface expected by the Mattermost server to communicate between the server and plugin processes.
type Plugin struct {
	plugin.MattermostPlugin

	client *pluginapi.Client
	store  kvStore

	configurationLock sync.RWMutex
	configuration     *Configuration

	router *mux.Router

	telemetryClient telemetry.Client
	tracker         telemetry.Tracker

	BotUserID string
	poster    poster.Poster

	CommandHandlers map[string]CommandHandleFunc

	flowManager *FlowManager

	oauthBroker *OAuthBroker
}

func (p *Plugin) ensurePluginAPIClient() {
	if p.client == nil {
		p.client = pluginapi.NewClient(p.API, p.Driver)
		p.store = &p.client.KV
	}
}

func NewPlugin() *Plugin {
	p := &Plugin{}
	p.CommandHandlers = map[string]CommandHandleFunc{
		"about":         p.handleAbout,
		"help":          p.handleHelp,
		"setup":         p.handleSetup,
		"connect":       p.handleConnect,
		"disconnect":    p.handleDisconnect,
		"create":        p.handleCreate,
		"notifications": p.handleNotifications,
	}
	return p
}

func (p *Plugin) OnActivate() error {
	p.ensurePluginAPIClient()

	siteURL := p.client.Configuration.GetConfig().ServiceSettings.SiteURL
	if siteURL == nil || *siteURL == "" {
		return errors.New("siteURL is not set. Please set it and restart the plugin")
	}

	p.initializeAPI()
	p.initializeTelemetry()

	p.oauthBroker = NewOAuthBroker(p.sendOAuthCompleteEvent)

	botID, err := p.client.Bot.EnsureBot(&model.Bot{
		OwnerId:     manifest.Id,
		Username:    "drive",
		DisplayName: "Google Drive",
		Description: "Created by the Google Drive plugin.",
	}, pluginapi.ProfileImagePath(filepath.Join("assets", "profile.png")))
	if err != nil {
		return errors.Wrap(err, "failed to ensure drive bot")
	}
	p.BotUserID = botID

	p.poster = poster.NewPoster(&p.client.Post, p.BotUserID)

	p.flowManager = p.NewFlowManager()

	return nil
}

func (p *Plugin) OnDeactivate() error {
	return nil
}

func (p *Plugin) OnInstall(c *plugin.Context, event model.OnInstallEvent) error {
	return nil
}

func (p *Plugin) getOAuthConfig() *oauth2.Config {
	config := p.getConfiguration()

	scopes := []string{
		"https://www.googleapis.com/auth/userinfo.profile",
		"https://www.googleapis.com/auth/userinfo.email",
		"https://www.googleapis.com/auth/drive.file",
		"https://www.googleapis.com/auth/drive.activity",
		"https://www.googleapis.com/auth/documents",
		"https://www.googleapis.com/auth/presentations",
		"https://www.googleapis.com/auth/spreadsheets",
	}

	return &oauth2.Config{
		ClientID:     config.GoogleOAuthClientID,
		ClientSecret: config.GoogleOAuthClientSecret,
		Scopes:       scopes,
		RedirectURL:  fmt.Sprintf("%s/plugins/%s/oauth/complete", *p.client.Configuration.GetConfig().ServiceSettings.SiteURL, manifest.Id),
		Endpoint:     google.Endpoint,
	}
}
