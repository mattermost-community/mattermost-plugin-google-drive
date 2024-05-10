package main

import (
	"github.com/darkLord19/mattermost-plugin-google-drive/server/plugin"

	mmplugin "github.com/mattermost/mattermost/server/public/plugin"
)

func main() {
	mmplugin.ClientMain(plugin.NewPlugin())
}
