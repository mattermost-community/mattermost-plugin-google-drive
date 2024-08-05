package plugin

import (
	"path/filepath"
	"sync"
	"time"

	"github.com/gorilla/mux"
	"github.com/mattermost/mattermost/server/public/model"
	"github.com/mattermost/mattermost/server/public/plugin"
	"github.com/mattermost/mattermost/server/public/pluginapi"
	"github.com/mattermost/mattermost/server/public/pluginapi/cluster"
	"github.com/mattermost/mattermost/server/public/pluginapi/experimental/bot/poster"
	"github.com/mattermost/mattermost/server/public/pluginapi/experimental/telemetry"
	"github.com/pkg/errors"
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

	channelRefreshJob *cluster.Job
}

func (p *Plugin) ensurePluginAPIClient() {
	if p.client == nil {
		p.client = pluginapi.NewClient(p.API, p.Driver)
		p.store = &p.client.KV
	}
}

func (p *Plugin) setDefaultConfiguration() error {
	config := p.getConfiguration()

	changed, err := config.setDefaults()
	if err != nil {
		return err
	}

	if changed {
		configMap, err := config.ToMap()
		if err != nil {
			return err
		}

		err = p.client.Configuration.SavePluginConfig(configMap)
		if err != nil {
			return err
		}
	}

	return nil
}

func (p *Plugin) refreshDriveWatchChannels() {
	page := 0
	perPage := 100

	worker := func(channels <-chan WatchChannelData, wg *sync.WaitGroup) {
		defer wg.Done()
		for channel := range channels {
			_ = p.startDriveWatchChannel(channel.MMUserID)
			p.stopDriveActivityNotifications(channel.MMUserID)
		}
	}

	var wg sync.WaitGroup
	channels := make(chan WatchChannelData)
	for i := 1; i <= 5; i++ {
		wg.Add(1)
		go worker(channels, &wg)
	}

	for {
		keys, err := p.client.KV.ListKeys(page, perPage, pluginapi.WithPrefix("drive_change_channels-"))
		if err != nil {
			p.API.LogError("Failed to list keys", "err", err)
			break
		}

		if len(keys) == 0 {
			break
		}

		for _, key := range keys {
			var watchChannelData WatchChannelData
			err = p.client.KV.Get(key, &watchChannelData)
			if err != nil {
				continue
			}
			if time.Until(time.Unix(watchChannelData.Expiration, 0)) < 24*time.Hour {
				channels <- watchChannelData
			}
		}

		page++
	}
	close(channels)
	wg.Wait()
}

func (p *Plugin) OnActivate() error {
	p.ensurePluginAPIClient()

	siteURL := p.client.Configuration.GetConfig().ServiceSettings.SiteURL
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

	botID, err := p.client.Bot.EnsureBot(&model.Bot{
		OwnerId:     manifest.Id,
		Username:    "google-drive",
		DisplayName: "Google Drive",
		Description: "Created by the Google Drive plugin.",
	}, pluginapi.ProfileImagePath(filepath.Join("assets", "profile.png")))
	if err != nil {
		return errors.Wrap(err, "failed to ensure drive bot")
	}
	p.BotUserID = botID

	p.poster = poster.NewPoster(&p.client.Post, p.BotUserID)

	p.flowManager = p.NewFlowManager()

	// google drive watch api doesn't allow indefinite expiry of watch channels
	// so we need to refresh(close old channel and start new one) them before they get expired
	p.channelRefreshJob, err = cluster.Schedule(p.API, "refreshDriveWatchChannelsJob", cluster.MakeWaitForInterval(12*time.Hour), p.refreshDriveWatchChannels)
	if err != nil {
		return errors.Wrap(err, "failed to create a scheduled recurring job to refresh watch channels")
	}
	return nil
}

func (p *Plugin) OnDeactivate() error {
	p.oauthBroker.Close()
	if err := p.telemetryClient.Close(); err != nil {
		p.client.Log.Warn("Telemetry client failed to close", "error", err.Error())
	}
	if err := p.channelRefreshJob.Close(); err != nil {
		p.client.Log.Warn("Channel refresh job failed to close", "error", err.Error())
	}
	return nil
}

func (p *Plugin) OnInstall(c *plugin.Context, event model.OnInstallEvent) error {
	conf := p.getConfiguration()

	// Don't start wizard if OAuth is configured
	if conf.IsOAuthConfigured() {
		p.client.Log.Debug("OAuth is already configured, skipping setup wizard",
			"GoogleOAuthClientID", lastN(conf.GoogleOAuthClientID, 4),
			"GoogleOAuthClientSecret", lastN(conf.GoogleOAuthClientSecret, 4),
		)
		return nil
	}

	return p.flowManager.StartSetupWizard(event.UserId)
}

func (p *Plugin) OnSendDailyTelemetry() {
	p.SendDailyTelemetry()
}

func (p *Plugin) OnPluginClusterEvent(c *plugin.Context, ev model.PluginClusterEvent) {
	p.HandleClusterEvent(ev)
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
