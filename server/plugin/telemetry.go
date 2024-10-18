package plugin

import (
	"strings"

	"github.com/mattermost/mattermost/server/public/pluginapi"
	"github.com/mattermost/mattermost/server/public/pluginapi/experimental/bot/logger"
	"github.com/mattermost/mattermost/server/public/pluginapi/experimental/telemetry"
	"github.com/pkg/errors"
)

const (
	keysPerPage = 1000
)

func (p *Plugin) TrackEvent(event string, properties map[string]interface{}) {
	err := p.tracker.TrackEvent(event, properties)
	if err != nil {
		p.Client.Log.Debug("Error sending telemetry event", "event", event, "error", err.Error())
	}
}

func (p *Plugin) TrackUserEvent(event, userID string, properties map[string]interface{}) {
	err := p.tracker.TrackUserEvent(event, userID, properties)
	if err != nil {
		p.Client.Log.Debug("Error sending user telemetry event", "event", event, "error", err.Error())
	}
}

func (p *Plugin) initializeTelemetry() {
	var err error

	// Telemetry client
	p.telemetryClient, err = telemetry.NewRudderClient()
	if err != nil {
		p.Client.Log.Debug("Telemetry client not started", "error", err.Error())
		return
	}

	// Get config values
	p.tracker = telemetry.NewTracker(
		p.telemetryClient,
		p.Client.System.GetDiagnosticID(),
		p.Client.System.GetServerVersion(),
		Manifest.Id,
		Manifest.Version,
		"drive",
		telemetry.NewTrackerConfig(p.Client.Configuration.GetConfig()),
		logger.New(p.API),
	)
}

func (p *Plugin) getConnectedUserCount() (int64, error) {
	checker := func(key string) (keep bool, err error) {
		return strings.HasSuffix(key, "token"), nil
	}

	var count int64

	for i := 0; ; i++ {
		keys, err := p.Client.KV.ListKeys(i, keysPerPage, pluginapi.WithChecker(checker))
		if err != nil {
			return 0, errors.Wrapf(err, "failed to list keys - page, %d", i)
		}

		count += int64(len(keys))

		if len(keys) < keysPerPage {
			break
		}
	}

	return count, nil
}

func (p *Plugin) SendDailyTelemetry() {
	config := p.getConfiguration()

	connectedUserCount, err := p.getConnectedUserCount()
	if err != nil {
		p.Client.Log.Warn("Failed to get the number of connected users for telemetry", "error", err)
	}

	p.TrackEvent("stats", map[string]interface{}{
		"connected_user_count": connectedUserCount,
		"is_oauth_configured":  config.IsOAuthConfigured(),
	})
}
