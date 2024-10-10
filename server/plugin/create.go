package plugin

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"time"

	"github.com/mattermost/mattermost/server/public/model"
	"github.com/mattermost/mattermost/server/public/plugin"
	"golang.org/x/oauth2"
	"golang.org/x/text/cases"
	"golang.org/x/text/language"
	"google.golang.org/api/drive/v3"
	"google.golang.org/api/option"
)

func (p *Plugin) sendFileCreatedMessage(channelID, fileID, userID, message string, shareInChannel bool, authToken *oauth2.Token) error {
	ctx := context.Background()
	conf := p.getOAuthConfig()
	srv, err := drive.NewService(ctx, option.WithTokenSource(conf.TokenSource(ctx, authToken)))
	if err != nil {
		p.API.LogError("Failed to create drive client", "err", err)
		return err
	}
	file, err := srv.Files.Get(fileID).Fields("webViewLink", "id", "owners", "permissions", "name", "iconLink", "thumbnailLink", "createdTime").Do()
	if err != nil {
		p.API.LogError("Failed to fetch  file", "err", err, "fileID", fileID)
		return err
	}

	createdTime, _ := time.Parse(time.RFC3339, file.CreatedTime)
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

func (p *Plugin) handleFilePermissions(userID string, fileID string, fileAccess string, channelID string) error {
	permissions := make([]*drive.Permission, 0)
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
				}
			}
		}
	}

	ctx := context.Background()
	conf := p.getOAuthConfig()

	authToken, _ := p.getGoogleUserToken(userID)
	srv, err := drive.NewService(ctx, option.WithTokenSource(conf.TokenSource(ctx, authToken)))
	if err != nil {
		p.API.LogError("Failed to create drive client", "err", err)
		return err
	}

	for _, permission := range permissions {
		_, err := srv.Permissions.Create(fileID, permission).Do()
		if err != nil {
			p.API.LogError("Something went wrong while updating permissions for file", "err", err, "fileID", fileID)
			return err
		}
	}
	return nil
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

	dialog.Dialog.Elements = append(dialog.Dialog.Elements, model.DialogElement{
		DisplayName: "File Access",
		Name:        "file_access",
		Type:        "select",
		Options: []*model.PostActionOptions{
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
		},
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
