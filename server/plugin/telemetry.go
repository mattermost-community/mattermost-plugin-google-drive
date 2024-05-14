package plugin

import (
	"github.com/mattermost/mattermost/server/public/pluginapi/experimental/bot/logger"
	"github.com/mattermost/mattermost/server/public/pluginapi/experimental/telemetry"
)

const (
	keysPerPage = 1000
)

func (p *Plugin) TrackEvent(event string, properties map[string]interface{}) {
	err := p.tracker.TrackEvent(event, properties)
	if err != nil {
		p.client.Log.Debug("Error sending telemetry event", "event", event, "error", err.Error())
	}
}

func (p *Plugin) TrackUserEvent(event, userID string, properties map[string]interface{}) {
	err := p.tracker.TrackUserEvent(event, userID, properties)
	if err != nil {
		p.client.Log.Debug("Error sending user telemetry event", "event", event, "error", err.Error())
	}
}

func (p *Plugin) initializeTelemetry() {
	var err error

	// Telemetry client
	p.telemetryClient, err = telemetry.NewRudderClient()
	if err != nil {
		p.client.Log.Debug("Telemetry client not started", "error", err.Error())
		return
	}

	// Get config values
	p.tracker = telemetry.NewTracker(
		p.telemetryClient,
		p.client.System.GetDiagnosticID(),
		p.client.System.GetServerVersion(),
		manifest.Id,
		manifest.Version,
		"drive",
		telemetry.NewTrackerConfig(p.client.Configuration.GetConfig()),
		logger.New(p.API),
	)
}
