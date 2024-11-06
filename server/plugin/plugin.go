package plugin

import (
	"path/filepath"
	"reflect"
	"sync"
	"time"

	"github.com/gorilla/mux"
	mattermostModel "github.com/mattermost/mattermost/server/public/model"
	"github.com/mattermost/mattermost/server/public/plugin"
	"github.com/mattermost/mattermost/server/public/pluginapi"
	"github.com/mattermost/mattermost/server/public/pluginapi/cluster"
	"github.com/mattermost/mattermost/server/public/pluginapi/experimental/bot/poster"
	"github.com/mattermost/mattermost/server/public/pluginapi/experimental/telemetry"
	"github.com/pkg/errors"

	"github.com/mattermost-community/mattermost-plugin-google-drive/server/plugin/config"
	"github.com/mattermost-community/mattermost-plugin-google-drive/server/plugin/google"
	"github.com/mattermost-community/mattermost-plugin-google-drive/server/plugin/kvstore"
	"github.com/mattermost-community/mattermost-plugin-google-drive/server/plugin/model"
	"github.com/mattermost-community/mattermost-plugin-google-drive/server/plugin/oauth2"
	"github.com/mattermost-community/mattermost-plugin-google-drive/server/plugin/utils"
)

const (
	WSEventConfigUpdate = "config_update"
)

// Plugin implements the interface expected by the Mattermost server to communicate between the server and plugin processes.
type Plugin struct {
	plugin.MattermostPlugin

	Client  *pluginapi.Client
	KVStore kvstore.KVStore

	configurationLock sync.RWMutex
	configuration     *config.Configuration

	router *mux.Router

	telemetryClient telemetry.Client
	tracker         telemetry.Tracker

	BotUserID string
	poster    poster.Poster

	CommandHandlers map[string]CommandHandleFunc

	FlowManager *FlowManager

	oauthBroker *OAuthBroker
	oauthConfig oauth2.Config

	channelRefreshJob *cluster.Job

	GoogleClient google.ClientInterface
}

func (p *Plugin) ensurePluginAPIClient() {
	if p.Client == nil {
		p.Client = pluginapi.NewClient(p.API, p.Driver)
		p.KVStore = kvstore.NewKVStore(p.Client)
	}
}

func (p *Plugin) setDefaultConfiguration() error {
	config := p.getConfiguration()

	changed, err := config.SetDefaults()
	if err != nil {
		return err
	}

	if changed {
		configMap, err := config.ToMap()
		if err != nil {
			return err
		}

		err = p.Client.Configuration.SavePluginConfig(configMap)
		if err != nil {
			return err
		}
	}

	return nil
}

func (p *Plugin) refreshDriveWatchChannels() {
	page := 0
	perPage := 100

	worker := func(channels <-chan model.WatchChannelData, wg *sync.WaitGroup) {
		defer wg.Done()
		for channel := range channels {
			p.stopDriveActivityNotifications(channel.MMUserID)
			_ = p.startDriveWatchChannel(channel.MMUserID)
		}
	}

	var wg sync.WaitGroup
	channels := make(chan model.WatchChannelData)
	for i := 1; i <= 5; i++ {
		wg.Add(1)
		go worker(channels, &wg)
	}

	for {
		keys, err := p.KVStore.ListWatchChannelDataKeys(page, perPage)
		if err != nil {
			p.API.LogError("Failed to list keys", "err", err)
			break
		}

		if len(keys) == 0 {
			break
		}

		for _, key := range keys {
			var watchChannelData *model.WatchChannelData
			watchChannelData, err := p.KVStore.GetWatchChannelDataUsingKey(key)
			if err != nil {
				continue
			}
			if time.Until(time.Unix(watchChannelData.Expiration, 0)) < 24*time.Hour {
				channels <- *watchChannelData
			}
		}

		page++
	}
	close(channels)
	wg.Wait()
}

func (p *Plugin) OnActivate() error {
	p.ensurePluginAPIClient()

	siteURL := p.Client.Configuration.GetConfig().ServiceSettings.SiteURL
	if siteURL == nil || *siteURL == "" {
		return errors.New("siteURL is not set. Please set it and restart the plugin")
	}

	err := p.setDefaultConfiguration()
	if err != nil {
		return errors.Wrap(err, "failed to set default configuration")
	}

	p.initializeAPI()
	p.initializeTelemetry()

	p.oauthBroker = NewOAuthBroker(p.sendOAuthCompleteEvent)

	botID, err := p.Client.Bot.EnsureBot(&mattermostModel.Bot{
		OwnerId:     Manifest.Id,
		Username:    "google-drive",
		DisplayName: "Google Drive",
		Description: "Created by the Google Drive plugin.",
	}, pluginapi.ProfileImagePath(filepath.Join("assets", "profile.png")))
	if err != nil {
		return errors.Wrap(err, "failed to ensure Google Drive bot")
	}
	p.BotUserID = botID

	p.poster = poster.NewPoster(&p.Client.Post, p.BotUserID)

	p.FlowManager = p.NewFlowManager()

	// Google Drive watch api doesn't allow indefinite expiry of watch channels
	// so we need to refresh (close old channel and start new one) them before they get expired
	p.channelRefreshJob, err = cluster.Schedule(p.API, "refreshDriveWatchChannelsJob", cluster.MakeWaitForInterval(12*time.Hour), p.refreshDriveWatchChannels)
	if err != nil {
		return errors.Wrap(err, "failed to create a scheduled recurring job to refresh watch channels")
	}

	p.oauthConfig = oauth2.GetOAuthConfig(p.getConfiguration(), siteURL, Manifest.Id)

	p.GoogleClient = google.NewGoogleClient(p.oauthConfig, p.getConfiguration(), p.KVStore, p.API)
	return nil
}

func (p *Plugin) OnDeactivate() error {
	p.oauthBroker.Close()
	if err := p.telemetryClient.Close(); err != nil {
		p.Client.Log.Warn("Telemetry client failed to close", "error", err.Error())
	}
	if err := p.channelRefreshJob.Close(); err != nil {
		p.Client.Log.Warn("Channel refresh job failed to close", "error", err.Error())
	}
	return nil
}

func (p *Plugin) OnInstall(c *plugin.Context, event mattermostModel.OnInstallEvent) error {
	conf := p.getConfiguration()

	// Don't start wizard if OAuth is configured
	if conf.IsOAuthConfigured() {
		p.Client.Log.Debug("OAuth is already configured, skipping setup wizard",
			"GoogleOAuthClientID", utils.LastN(conf.GoogleOAuthClientID, 4),
			"GoogleOAuthClientSecret", utils.LastN(conf.GoogleOAuthClientSecret, 4),
		)
		return nil
	}

	return p.FlowManager.StartSetupWizard(event.UserId)
}

func (p *Plugin) OnSendDailyTelemetry() {
	p.SendDailyTelemetry()
}

func (p *Plugin) OnPluginClusterEvent(c *plugin.Context, ev mattermostModel.PluginClusterEvent) {
	p.HandleClusterEvent(ev)
}

// getConfiguration retrieves the active configuration under lock, making it safe to use
// concurrently. The active configuration may change underneath the client of this method, but
// the struct returned by this API call is considered immutable.
func (p *Plugin) getConfiguration() *config.Configuration {
	p.configurationLock.RLock()
	defer p.configurationLock.RUnlock()

	if p.configuration == nil {
		return &config.Configuration{}
	}

	return p.configuration
}

// setConfiguration replaces the active configuration under lock.
//
// Do not call setConfiguration while holding the configurationLock, as sync.Mutex is not
// reentrant. In particular, avoid using the plugin API entirely, as this may in turn trigger a
// hook back into the plugin. If that hook attempts to acquire this lock, a deadlock may occur.
//
// This method panics if setConfiguration is called with the existing configuration. This almost
// certainly means that the configuration was modified without being cloned and may result in
// an unsafe access.
func (p *Plugin) setConfiguration(configuration *config.Configuration) {
	p.configurationLock.Lock()
	defer p.configurationLock.Unlock()

	if configuration != nil && p.configuration == configuration {
		// Ignore assignment if the configuration struct is empty. Go will optimize the
		// allocation for same to point at the same memory address, breaking the check
		// above.
		if reflect.ValueOf(*configuration).NumField() == 0 {
			return
		}

		panic("setConfiguration called with the existing configuration")
	}

	p.configuration = configuration
}

// OnConfigurationChange is invoked when configuration changes may have been made.
func (p *Plugin) OnConfigurationChange() error {
	p.ensurePluginAPIClient()

	var configuration = new(config.Configuration)

	// Load the public configuration fields from the Mattermost server configuration.
	err := p.Client.Configuration.LoadPluginConfiguration(configuration)
	if err != nil {
		return errors.Wrap(err, "failed to load plugin configuration")
	}

	configuration.Sanitize()

	p.sendWebsocketEventIfNeeded(p.getConfiguration(), configuration)

	p.setConfiguration(configuration)

	command, err := p.getCommand(configuration)
	if err != nil {
		return errors.Wrap(err, "failed to get command")
	}

	err = p.Client.SlashCommand.Register(command)
	if err != nil {
		return errors.Wrap(err, "failed to register command")
	}
	// Some config changes require reloading tracking config
	if p.tracker != nil {
		p.tracker.ReloadConfig(telemetry.NewTrackerConfig(p.Client.Configuration.GetConfig()))
	}

	if p.GoogleClient != nil {
		p.GoogleClient.ReloadRateLimits(configuration.QueriesPerMinute, configuration.BurstSize)
	}
	return nil
}

func (p *Plugin) sendWebsocketEventIfNeeded(oldConfig, newConfig *config.Configuration) {
	// If the plugin just started, oldConfig is the zero value.
	// Hence, an unnecessary websocket event is sent.
	// Given that oldConfig is never nil, that case is hard to catch.
	if !reflect.DeepEqual(oldConfig.ClientConfiguration(), newConfig.ClientConfiguration()) {
		p.Client.Frontend.PublishWebSocketEvent(
			WSEventConfigUpdate,
			newConfig.ClientConfiguration(),
			&mattermostModel.WebsocketBroadcast{},
		)
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
