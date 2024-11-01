package plugin

import (
	"context"
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/pkg/errors"

	"github.com/mattermost/mattermost/server/public/model"
	"github.com/mattermost/mattermost/server/public/plugin"
	"golang.org/x/text/cases"
	"golang.org/x/text/language"
	"google.golang.org/api/drive/v3"
)

func (p *Plugin) sendFileCreatedMessage(ctx context.Context, channelID, fileID, userID, message string, shareInChannel bool) error {
	driveService, err := p.GoogleClient.NewDriveService(ctx, userID)
	if err != nil {
		return errors.Wrap(err, "failed to create Google Drive service")
	}
	file, err := driveService.GetFile(ctx, fileID)
	if err != nil {
		return errors.Wrap(err, "failed to fetch file")
	}

	createdTime, err := time.Parse(time.RFC3339, file.CreatedTime)
	if err != nil {
		return errors.Wrap(err, "failed to parse created time")
	}
	if shareInChannel {
		post := model.Post{
			UserId:    p.BotUserID,
			ChannelId: channelID,
			Message:   message,
			Props: map[string]any{
				"attachments": []any{map[string]any{
					"author_name": file.Owners[0].DisplayName,
					"author_icon": file.Owners[0].PhotoLink,
					"title":       file.Name,
					"title_link":  file.WebViewLink,
					"footer":      fmt.Sprintf("Google Drive for Mattermost | %s", createdTime),
					"footer_icon": file.IconLink,
				}},
			},
		}
		_, appErr := p.API.CreatePost(&post)
		if appErr != nil {
			p.API.LogWarn("Failed to create post", "err", appErr, "channelID", post.ChannelId, "rootId", post.RootId, "message", post.Message)
			return errors.New(appErr.DetailedError)
		}
	} else {
		p.createBotDMPost(userID, "", map[string]any{
			"attachments": []any{map[string]any{
				"pretext":     fmt.Sprintf("You created a new file with following message:\n > %s", message),
				"title":       file.Name,
				"title_link":  file.WebViewLink,
				"footer":      fmt.Sprintf("Google Drive for Mattermost | %s", createdTime),
				"footer_icon": file.IconLink,
			}},
		})
	}
	return nil
}

func (p *Plugin) handleFilePermissions(ctx context.Context, userID string, fileID string, fileAccess string, channelID string, fileName string) error {
	permissions := make([]*drive.Permission, 0)
	userMap := make(map[string]*model.User, 0)
	switch fileAccess {
	case "all_view":
		permissions = append(permissions, &drive.Permission{
			Role: "reader",
			Type: "anyone",
		})
	case "all_comment":
		permissions = append(permissions, &drive.Permission{
			Role: "commenter",
			Type: "anyone",
		})
	case "all_edit":
		permissions = append(permissions, &drive.Permission{
			Role: "writer",
			Type: "anyone",
		})
	case "members_view":
		{
			users := p.getAllChannelUsers(channelID)
			for _, user := range users {
				if !user.IsBot {
					permissions = append(permissions, &drive.Permission{
						Role:         "reader",
						EmailAddress: user.Email,
						Type:         "user",
					})
					userMap[user.Email] = user
				}
			}
		}
	case "members_comment":
		{
			users := p.getAllChannelUsers(channelID)
			for _, user := range users {
				if !user.IsBot {
					permissions = append(permissions, &drive.Permission{
						Role:         "commenter",
						EmailAddress: user.Email,
						Type:         "user",
					})
					userMap[user.Email] = user
				}
			}
		}
	case "members_edit":
		{
			users := p.getAllChannelUsers(channelID)
			for _, user := range users {
				if !user.IsBot {
					permissions = append(permissions, &drive.Permission{
						Role:         "writer",
						EmailAddress: user.Email,
						Type:         "user",
					})
					userMap[user.Email] = user
				}
			}
		}
	}

	driveService, err := p.GoogleClient.NewDriveService(ctx, userID)
	if err != nil {
		return errors.Wrap(err, "failed to create Google Drive service")
	}

	usersWithoutAccesss := []string{}
	config := p.API.GetConfig()
	var permissionError error

	for i, permission := range permissions {
		// Continue through the permissions loop when we encounter an error so we can inform the user who wasn't granted access.
		if permissionError != nil || i > 60 {
			usersWithoutAccesss = appendUsersWithoutAccessSlice(config, usersWithoutAccesss, userMap[permission.EmailAddress].Username, permission.EmailAddress)
			continue
		}
		_, err := driveService.CreatePermission(ctx, fileID, permission)
		if err != nil {
			usersWithoutAccesss = appendUsersWithoutAccessSlice(config, usersWithoutAccesss, userMap[permission.EmailAddress].Username, permission.EmailAddress)
			// This error will occur if the user is not allowed to share the file with someone outside of their domain.
			if strings.Contains(err.Error(), "shareOutNotPermitted") {
				continue
			}
			permissionError = err
		}
	}

	if len(usersWithoutAccesss) > 0 {
		p.createBotDMPost(userID, fmt.Sprintf("Failed to share file, \"%s\", with the following users: %s", fileName, strings.Join(usersWithoutAccesss, ", ")), nil)
	}

	return permissionError
}

func appendUsersWithoutAccessSlice(config *model.Config, usersWithoutAccesss []string, username string, email string) []string {
	if config.PrivacySettings.ShowEmailAddress == nil || !*config.PrivacySettings.ShowEmailAddress {
		usersWithoutAccesss = append(usersWithoutAccesss, "@"+username)
	} else {
		usersWithoutAccesss = append(usersWithoutAccesss, email)
	}

	return usersWithoutAccesss
}

func (p *Plugin) handleCreate(c *plugin.Context, args *model.CommandArgs, parameters []string) string {
	subcommand := parameters[0]

	allowedCommands := []string{"doc", "sheet", "slide"}
	if !slices.Contains(allowedCommands, subcommand) {
		return fmt.Sprintf("%s is not a valid create option", subcommand)
	}

	dialog := model.OpenDialogRequest{
		TriggerId: args.TriggerId,
		URL:       fmt.Sprintf("/plugins/%s/api/v1/create?type=%s", Manifest.Id, subcommand),
		Dialog: model.Dialog{
			CallbackId:     fmt.Sprintf("create_%s", subcommand),
			Title:          fmt.Sprintf("Create a Google %s", cases.Title(language.English, cases.NoLower).String(subcommand)),
			IconURL:        "http://www.mattermost.org/wp-content/uploads/2016/04/icon.png",
			Elements:       []model.DialogElement{},
			SubmitLabel:    "Create",
			NotifyOnCancel: false,
		},
	}

	dialog.Dialog.Elements = append(dialog.Dialog.Elements, model.DialogElement{
		DisplayName: "Name",
		Name:        "name",
		Type:        "text",
	})

	dialog.Dialog.Elements = append(dialog.Dialog.Elements, model.DialogElement{
		DisplayName: "Message",
		Name:        "message",
		Type:        "textarea",
		Optional:    true,
	})

	ctx := context.Background()
	serviceV2, err := p.GoogleClient.NewDriveV2Service(ctx, args.UserId)
	if err != nil {
		p.API.LogError("Failed to create drive client", "err", err)
		return "Failed to open file creation dialog. Please contact your system administrator."
	}

	about, err := serviceV2.About(ctx, "domainSharingPolicy")
	if err != nil {
		p.API.LogError("Failed to get user information", "err", err)
		return "Failed to open file creation dialog. Please contact your system administrator."
	}

	options := []*model.PostActionOptions{
		{
			Text:  "Keep file private",
			Value: "private",
		},
		{
			Text:  "Members of the channel can view",
			Value: "members_view",
		},
		{
			Text:  "Members of the channel can comment",
			Value: "members_comment",
		},
		{
			Text:  "Members of the channel can edit",
			Value: "members_edit",
		},
	}
	if strings.ToLower(about.DomainSharingPolicy) == "allowed" || strings.ToLower(about.DomainSharingPolicy) == "allowedwithwarning" {
		options = append(options, []*model.PostActionOptions{
			{
				Text:  "Anyone with the link can view",
				Value: "all_view",
			},
			{
				Text:  "Anyone with the link can comment",
				Value: "all_comment",
			},
			{
				Text:  "Anyone with the link can edit",
				Value: "all_edit",
			},
		}...)
	}

	dialog.Dialog.Elements = append(dialog.Dialog.Elements, model.DialogElement{
		DisplayName: "File Access",
		Name:        "file_access",
		Type:        "select",
		Options:     options,
	})

	dialog.Dialog.Elements = append(dialog.Dialog.Elements, model.DialogElement{
		DisplayName: "Share in this Channel",
		Name:        "share_in_channel",
		Type:        "bool",
		Placeholder: "Selecting this will share the file link as a message in the channel.",
		Optional:    true,
	})

	appErr := p.API.OpenInteractiveDialog(dialog)
	if appErr != nil {
		p.API.LogWarn("Failed to open interactive dialog", "err", appErr.DetailedError)
		return "Failed to open file creation dialog"
	}
	return ""
}
