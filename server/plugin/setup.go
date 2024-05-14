package plugin

import (
	"strings"

	"github.com/mattermost/mattermost/server/public/model"
	"github.com/mattermost/mattermost/server/public/plugin"
)

func (p *Plugin) isAuthorizedSysAdmin(userID string) (bool, error) {
	user, err := p.client.User.Get(userID)
	if err != nil {
		return false, err
	}
	if !strings.Contains(user.Roles, "system_admin") {
		return false, nil
	}
	return true, nil
}

func (p *Plugin) handleSetup(c *plugin.Context, args *model.CommandArgs, parameters []string) string {
	userID := args.UserId
	isSysAdmin, err := p.isAuthorizedSysAdmin(userID)
	if err != nil {
		p.client.Log.Warn("Failed to check if user is System Admin", "error", err.Error())
		return "Error checking user's permissions"
	}

	if !isSysAdmin {
		return "Only System Admins are allowed to set up the plugin."
	}

	err = p.flowManager.StartSetupWizard(userID)

	if err != nil {
		return err.Error()
	}

	return ""
}
