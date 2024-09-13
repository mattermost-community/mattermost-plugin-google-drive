package main

import (
	"github.com/mattermost/mattermost-plugin-google-drive/server/plugin"

	mmplugin "github.com/mattermost/mattermost/server/public/plugin"
)

func main() {
	mmplugin.ClientMain(plugin.NewPlugin())
}
