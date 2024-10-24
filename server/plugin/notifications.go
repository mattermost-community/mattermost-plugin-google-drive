package plugin

import (
	"context"
	"fmt"
	"net/url"
	"slices"
	"time"

	"github.com/google/uuid"
	mattermostModel "github.com/mattermost/mattermost/server/public/model"
	"github.com/mattermost/mattermost/server/public/plugin"
	"github.com/pkg/errors"
	"google.golang.org/api/drive/v3"
	"google.golang.org/api/driveactivity/v2"

	"github.com/mattermost-community/mattermost-plugin-google-drive/server/plugin/google"
	"github.com/mattermost-community/mattermost-plugin-google-drive/server/plugin/model"
	"github.com/mattermost-community/mattermost-plugin-google-drive/server/plugin/utils"
)

func getCommentUsingDiscussionID(ctx context.Context, dSrv *google.DriveService, fileID string, activity *driveactivity.DriveActivity) (*drive.Comment, error) {
	if len(activity.Targets) == 0 ||
		activity.Targets[0].FileComment == nil ||
		activity.Targets[0].FileComment.LegacyDiscussionId == "" {
		return nil, errors.New("no legacyDiscussionId present in the activity")
	}
	commentID := activity.Targets[0].FileComment.LegacyDiscussionId
	comment, err := dSrv.GetComments(ctx, fileID, commentID)
	if err != nil {
		return nil, err
	}
	return comment, nil
}

func (p *Plugin) handleAddedComment(ctx context.Context, dSrv *google.DriveService, fileID, userID string, activity *driveactivity.DriveActivity, file *drive.File) {
	comment, err := getCommentUsingDiscussionID(ctx, dSrv, fileID, activity)
	if err != nil {
		p.API.LogError("Failed to get comment by legacyDiscussionId", "err", err, "userID", userID)
		return
	}
	quotedValue := ""
	if comment.QuotedFileContent != nil {
		quotedValue = comment.QuotedFileContent.Value
	}
	props := map[string]any{
		"attachments": []any{
			map[string]any{
				"pretext": fmt.Sprintf("%s commented on %s %s", comment.Author.DisplayName, utils.GetInlineImage("File icon:", file.IconLink), utils.GetHyperlink(file.Name, file.WebViewLink)),
				"text":    fmt.Sprintf("%s\n> %s", quotedValue, comment.Content),
				"actions": []any{
					map[string]any{
						"name": "Reply to comment",
						"integration": map[string]any{
							"url": fmt.Sprintf("%s/plugins/%s/api/v1/reply_dialog", *p.API.GetConfig().ServiceSettings.SiteURL, Manifest.Id),
							"context": map[string]any{
								"commentID": comment.Id,
								"fileID":    fileID,
							},
						},
					},
				},
			},
		},
	}
	p.createBotDMPost(userID, "", props)
}

func (p *Plugin) handleDeletedComment(userID string, activity *driveactivity.DriveActivity, file *drive.File) {
	urlToComment := activity.Targets[0].FileComment.LinkToDiscussion
	message := fmt.Sprintf("A comment was deleted in %s %s", utils.GetInlineImage("Google failed:", file.IconLink), utils.GetHyperlink(file.Name, urlToComment))
	p.createBotDMPost(userID, message, nil)
}

func (p *Plugin) handleReplyAdded(ctx context.Context, dSrv *google.DriveService, fileID, userID string, activity *driveactivity.DriveActivity, file *drive.File) {
	comment, err := getCommentUsingDiscussionID(ctx, dSrv, fileID, activity)
	if err != nil {
		p.API.LogError("Failed to get comment by legacyDiscussionId", "err", err, "userID", userID)
		return
	}
	urlToComment := activity.Targets[0].FileComment.LinkToDiscussion
	lastReply := ""
	lastReplyAuthor := ""
	onBeforeLast := ""
	if len(comment.Replies) > 0 {
		lastReply = comment.Replies[len(comment.Replies)-1].Content
		lastReplyAuthor = comment.Replies[len(comment.Replies)-1].Author.DisplayName
		if len(comment.Replies) > 1 {
			onBeforeLast = comment.Replies[len(comment.Replies)-2].Content
		} else {
			onBeforeLast = comment.Content
		}
	}
	props := map[string]any{
		"attachments": []any{
			map[string]any{
				"pretext": fmt.Sprintf("%s replied on %s %s", lastReplyAuthor, utils.GetInlineImage("File icon:", file.IconLink), utils.GetHyperlink(file.Name, urlToComment)),
				"text":    fmt.Sprintf("Previous reply:\n%s\n> %s", onBeforeLast, lastReply),
				"actions": []any{
					map[string]any{
						"name": "Reply to comment",
						"integration": map[string]any{
							"url": fmt.Sprintf("%s/plugins/%s/api/v1/reply_dialog", *p.API.GetConfig().ServiceSettings.SiteURL, Manifest.Id),
							"context": map[string]any{
								"commentID": comment.Id,
								"fileID":    fileID,
							},
						},
					},
				},
			},
		},
	}
	p.createBotDMPost(userID, "", props)
}

func (p *Plugin) handleReplyDeleted(userID string, activity *driveactivity.DriveActivity, file *drive.File) {
	urlToComment := activity.Targets[0].FileComment.LinkToDiscussion
	message := fmt.Sprintf("A comment reply was deleted in %s %s", utils.GetInlineImage("Google failed:", file.IconLink), utils.GetHyperlink(file.Name, urlToComment))
	p.createBotDMPost(userID, message, nil)
}

func (p *Plugin) handleResolvedComment(ctx context.Context, dSrv *google.DriveService, fileID, userID string, activity *driveactivity.DriveActivity, file *drive.File) {
	if len(activity.Targets) == 0 ||
		activity.Targets[0].FileComment == nil ||
		activity.Targets[0].FileComment.LegacyCommentId == "" {
		p.API.LogWarn("There is no legacyCommentId present in the activity", "userID", userID)
		return
	}
	commentID := activity.Targets[0].FileComment.LegacyCommentId
	comment, err := dSrv.GetComments(ctx, fileID, commentID)
	if err != nil {
		p.API.LogError("Failed to get comment by legacyCommentId", "err", err, "commentID", commentID, "userID", userID)
		return
	}
	urlToComment := activity.Targets[0].FileComment.LinkToDiscussion
	message := fmt.Sprintf("%s marked a thread as resolved in %s %s", comment.Author.DisplayName, utils.GetInlineImage("File icon:", file.IconLink), utils.GetHyperlink(file.Name, urlToComment))
	p.createBotDMPost(userID, message, nil)
}

func (p *Plugin) handleReopenedComment(ctx context.Context, dSrv *google.DriveService, fileID, userID string, activity *driveactivity.DriveActivity, file *drive.File) {
	comment, err := getCommentUsingDiscussionID(ctx, dSrv, fileID, activity)
	if err != nil {
		p.API.LogError("Failed to get comment by legacyDiscussionId", "err", err, "userID", userID)
		return
	}
	urlToComment := activity.Targets[0].FileComment.LinkToDiscussion
	message := fmt.Sprintf("%s reopened a thread in %s %s", comment.Author.DisplayName, utils.GetInlineImage("File icon:", file.IconLink), utils.GetHyperlink(file.Name, urlToComment))
	p.createBotDMPost(userID, message, nil)
}

func (p *Plugin) handleSuggestionReplyAdded(userID string, activity *driveactivity.DriveActivity, file *drive.File) {
	urlToComment := activity.Targets[0].FileComment.LinkToDiscussion
	message := fmt.Sprintf("%s added a new suggestion in %s %s", file.LastModifyingUser.DisplayName, utils.GetInlineImage("File icon:", file.IconLink), utils.GetHyperlink(file.Name, urlToComment))
	p.createBotDMPost(userID, message, nil)
}

func (p *Plugin) handleCommentNotifications(ctx context.Context, dSrv *google.DriveService, file *drive.File, userID string, activity *driveactivity.DriveActivity) {
	fileID := file.Id

	if ok := activity.PrimaryActionDetail.Comment.Post != nil; !ok {
		return
	}
	postSubType := activity.PrimaryActionDetail.Comment.Post.Subtype

	switch postSubType {
	case "ADDED":
		p.handleAddedComment(ctx, dSrv, fileID, userID, activity, file)
	case "DELETED":
		p.handleDeletedComment(userID, activity, file)
	case "REPLY_ADDED":
		p.handleReplyAdded(ctx, dSrv, fileID, userID, activity, file)
	case "REPLY_DELETED":
		p.handleReplyDeleted(userID, activity, file)
	case "RESOLVED":
		p.handleResolvedComment(ctx, dSrv, fileID, userID, activity, file)
	case "REOPENED":
		p.handleReopenedComment(ctx, dSrv, fileID, userID, activity, file)
	}

	suggestion := activity.PrimaryActionDetail.Comment.Suggestion
	if suggestion == nil {
		return
	}

	if suggestion.Subtype == "REPLY_ADDED" {
		p.handleSuggestionReplyAdded(userID, activity, file)
	}
}

func (p *Plugin) handleFileSharedNotification(file *drive.File, userID string) {
	config := p.API.GetConfig()
	userDisplay := p.getUserDisplayName(file.SharingUser, config)

	p.createBotDMPost(userID, userDisplay+" shared an item with you", map[string]any{
		"attachments": []any{map[string]any{
			"title":       file.Name,
			"title_link":  file.WebViewLink,
			"footer":      "Google Drive for Mattermost",
			"footer_icon": file.IconLink,
		}},
	})
}

func (p *Plugin) handleMultipleActivitiesNotification(file *drive.File, userID string) {
	p.createBotDMPost(userID, "There has been activity on this document", map[string]any{
		"attachments": []any{map[string]any{
			"title":       file.Name,
			"title_link":  file.WebViewLink,
			"footer":      "Google Drive for Mattermost",
			"footer_icon": file.IconLink,
		}},
	})
}

func (p *Plugin) startDriveWatchChannel(userID string) error {
	ctx := context.Background()
	driveService, err := p.GoogleClient.NewDriveService(ctx, userID)
	if err != nil {
		p.API.LogError("Failed to create Google Drive service", "err", err, "userID", userID)
		return err
	}

	startPageToken, err := driveService.GetStartPageToken(ctx)
	if err != nil {
		p.API.LogError("Failed to get start page token", "err", err)
		return err
	}

	url, err := url.Parse(fmt.Sprintf("%s/plugins/%s/api/v1/webhook", *p.Client.Configuration.GetConfig().ServiceSettings.SiteURL, Manifest.Id))
	if err != nil {
		p.API.LogError("Failed to parse webhook url", "err", err)
		return err
	}
	query := url.Query()
	query.Add("userID", userID)
	url.RawQuery = query.Encode()
	token := mattermostModel.NewRandomString(64)

	requestChannel := drive.Channel{
		Kind:       "api#channel",
		Address:    url.String(),
		Payload:    true,
		Id:         uuid.NewString(),
		Token:      token,
		Type:       "web_hook",
		Expiration: time.Now().Add(604800 * time.Second).UnixMilli(),
		Params: map[string]string{
			"userID": userID,
		},
	}

	channel, err := driveService.WatchChannel(ctx, startPageToken, &requestChannel)
	if err != nil {
		p.API.LogError("Failed to register watch on drive", "err", err, "requestChannel", requestChannel)
		return err
	}

	channelData := model.WatchChannelData{
		ChannelID:  channel.Id,
		ResourceID: channel.ResourceId,
		Expiration: channel.Expiration,
		Token:      channel.Token,
		MMUserID:   userID,
		PageToken:  startPageToken.StartPageToken,
	}
	err = p.KVStore.StoreWatchChannelData(userID, channelData)
	if err != nil {
		p.API.LogError("Failed to set Google Drive change channel data", "userID", userID, "channelData", channelData)
		return err
	}
	return nil
}

func isWatchChannelDataValid(watchChannelData *model.WatchChannelData) bool {
	return watchChannelData.ChannelID != "" && watchChannelData.Expiration != 0 && watchChannelData.MMUserID != "" && watchChannelData.ResourceID != ""
}

func (p *Plugin) startDriveActivityNotifications(userID string) string {
	watchChannelData, err := p.KVStore.GetWatchChannelData(userID)
	if err != nil {
		return "Something went wrong while starting Google Drive activity notifications. Please contact your organization admin for support."
	}

	if isWatchChannelDataValid(watchChannelData) {
		return "Google Drive activity notifications are already enabled for you."
	}

	err = p.startDriveWatchChannel(userID)
	if err != nil {
		return "Something went wrong while starting Google Drive activity notifications. Please contact your organization admin for support."
	}

	return "Successfully enabled Google Drive activity notifications."
}

func (p *Plugin) stopDriveActivityNotifications(userID string) string {
	ctx := context.Background()
	watchChannelData, err := p.KVStore.GetWatchChannelData(userID)
	if err != nil {
		p.API.LogError("Failed to get Google Drive change channel data", "userID", userID)
		return "Something went wrong while stopping Google Drive activity notifications. Please contact your organization admin for support."
	}

	if !isWatchChannelDataValid(watchChannelData) {
		return "Google Drive activity notifications are not enabled for you."
	}

	driveService, err := p.GoogleClient.NewDriveService(ctx, userID)
	if err != nil {
		p.API.LogError("Failed to create Google Drive service", "err", err, "userID", userID)
		return "Something went wrong while stopping Google Drive activity notifications. Please contact your organization admin for support."
	}

	err = p.KVStore.DeleteWatchChannelData(userID)
	if err != nil {
		p.API.LogError("Failed to delete Google Drive watch channel data", "err", err)
		return "Something went wrong while stopping Google Drive activity notifications. Please contact your organization admin for support."
	}

	err = driveService.StopChannel(ctx, &drive.Channel{
		Id:         watchChannelData.ChannelID,
		ResourceId: watchChannelData.ResourceID,
	})
	if err != nil {
		p.API.LogError("Failed to stop Google Drive change channel", "err", err)
		return "Something went wrong while stopping Google Drive activity notifications. Please contact your organization admin for support."
	}

	return "Successfully disabled Google Drive activity notifications."
}

func (p *Plugin) handleNotifications(c *plugin.Context, args *mattermostModel.CommandArgs, parameters []string) string {
	subcommand := parameters[0]

	allowedCommands := []string{"start", "stop"}
	if !slices.Contains(allowedCommands, subcommand) {
		return fmt.Sprintf("%s is not a valid notifications subcommand", subcommand)
	}

	switch subcommand {
	case "start":
		return p.startDriveActivityNotifications(args.UserId)
	case "stop":
		return p.stopDriveActivityNotifications(args.UserId)
	}
	return ""
}
