package plugin

import "github.com/mattermost/mattermost/server/public/model"

// CreateBotDMPost posts a direct message using the bot account.
// Any error are not returned and instead logged.
func (p *Plugin) CreateBotDMPost(userID, message string, props map[string]any) {
	channel, err := p.client.Channel.GetDirect(userID, p.BotUserID)
	if err != nil {
		p.client.Log.Warn("Couldn't get bot's DM channel", "userID", userID, "error", err.Error())
		return
	}

	post := &model.Post{
		UserId:    p.BotUserID,
		ChannelId: channel.Id,
		Message:   message,
		Props:     props,
	}

	if err = p.client.Post.CreatePost(post); err != nil {
		p.client.Log.Warn("Failed to create DM post", "userID", userID, "post", post, "error", err.Error())
		return
	}
}
