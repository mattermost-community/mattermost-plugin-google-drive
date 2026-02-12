package plugin

import (
	"context"
	"fmt"
	"testing"

	"github.com/golang/mock/gomock"

	mock_google "github.com/mattermost-community/mattermost-plugin-google-drive/server/plugin/google/mocks"
	mock_store "github.com/mattermost-community/mattermost-plugin-google-drive/server/plugin/kvstore/mocks"
	"github.com/mattermost-community/mattermost-plugin-google-drive/server/plugin/utils"

	mattermostModel "github.com/mattermost/mattermost/server/public/model"

	"google.golang.org/api/drive/v3"
)

func TestNewComment(t *testing.T) {
	te := SetupTestEnvironment(t)
	defer te.Cleanup(t)

	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockKVStore := mock_store.NewMockKVStore(ctrl)
	mockGoogleClient := mock_google.NewMockClientInterface(ctrl)
	mockGoogleDrive := mock_google.NewMockDriveInterface(ctrl)

	te.plugin.KVStore = mockKVStore
	te.plugin.GoogleClient = mockGoogleClient
	te.plugin.initializeAPI()

	fileID := "fileID1"
	activity := GetSampleDriveactivityCommentResponse()
	file := GetSampleFile(fileID)

	commentID := activity.Activities[0].Targets[0].FileComment.LegacyCommentId
	comment := GetSampleComment(commentID)

	t.Run("handle new comment", func(t *testing.T) {
		mockGoogleDrive.EXPECT().GetComments(context.Background(), file.Id, commentID).Return(comment, nil)
		siteURL := "http://localhost"
		te.mockAPI.On("GetConfig").Return(&mattermostModel.Config{ServiceSettings: mattermostModel.ServiceSettings{SiteURL: &siteURL}})
		te.mockAPI.On("GetDirectChannel", "userID1", te.plugin.BotUserID).Return(&mattermostModel.Channel{Id: "channelId1"}, nil).Times(1)
		post := GetDMPost(te.plugin.BotUserID, comment, file, siteURL)
		te.mockAPI.On("CreatePost", post).Return(nil, nil).Times(1)
		te.plugin.handleAddedComment(context.Background(), mockGoogleDrive, fileID, "userID1", activity.Activities[0], file)
	})

	t.Run("handle new comment with markdown escaped", func(t *testing.T) {
		comment.Content = "> This is a quote"
		mockGoogleDrive.EXPECT().GetComments(context.Background(), file.Id, commentID).Return(comment, nil)
		siteURL := "http://localhost"
		te.mockAPI.On("GetConfig").Return(&mattermostModel.Config{ServiceSettings: mattermostModel.ServiceSettings{SiteURL: &siteURL}})
		te.mockAPI.On("GetDirectChannel", "userID1", te.plugin.BotUserID).Return(&mattermostModel.Channel{Id: "channelId1"}, nil).Times(1)
		post := GetDMPost(te.plugin.BotUserID, comment, file, siteURL)
		te.mockAPI.On("CreatePost", post).Return(nil, nil).Times(1)
		te.plugin.handleAddedComment(context.Background(), mockGoogleDrive, fileID, "userID1", activity.Activities[0], file)
	})
}

func TestNewCommentReply(t *testing.T) {
	te := SetupTestEnvironment(t)
	defer te.Cleanup(t)

	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockKVStore := mock_store.NewMockKVStore(ctrl)
	mockGoogleClient := mock_google.NewMockClientInterface(ctrl)
	mockGoogleDrive := mock_google.NewMockDriveInterface(ctrl)

	te.plugin.KVStore = mockKVStore
	te.plugin.GoogleClient = mockGoogleClient
	te.plugin.initializeAPI()

	fileID := "fileID1"
	activity := GetSampleDriveactivityCommentResponse()
	file := GetSampleFile(fileID)

	commentID := activity.Activities[0].Targets[0].FileComment.LegacyCommentId
	comment := GetSampleComment(commentID)
	comment.Replies = []*drive.Reply{
		{
			Content: "Reply to comment",
			Author: &drive.User{
				DisplayName: "User 1",
			},
		},
	}

	t.Run("handle new comment", func(t *testing.T) {
		mockGoogleDrive.EXPECT().GetComments(context.Background(), file.Id, commentID).Return(comment, nil)
		siteURL := "http://localhost"
		te.mockAPI.On("GetConfig").Return(&mattermostModel.Config{ServiceSettings: mattermostModel.ServiceSettings{SiteURL: &siteURL}})
		te.mockAPI.On("GetDirectChannel", "userID1", te.plugin.BotUserID).Return(&mattermostModel.Channel{Id: "channelId1"}, nil).Times(1)
		post := GetDMPost(te.plugin.BotUserID, comment, file, siteURL)
		post.Props["attachments"].([]any)[0].(map[string]any)["pretext"] = fmt.Sprintf("User 1 replied on %s %s", utils.GetInlineImage("File icon:", file.IconLink), utils.GetHyperlink(file.Name, ""))
		post.Props["attachments"].([]any)[0].(map[string]any)["text"] = fmt.Sprintf("Previous reply:\n%s\n> %s", comment.Content, "Reply to comment")
		te.mockAPI.On("CreatePost", post).Return(nil, nil).Times(1)
		te.plugin.handleReplyAdded(context.Background(), mockGoogleDrive, fileID, "userID1", activity.Activities[0], file)
	})

	t.Run("handle new comment with markdown escaped", func(t *testing.T) {
		comment.Replies[0].Content = "> Reply to comment"
		mockGoogleDrive.EXPECT().GetComments(context.Background(), file.Id, commentID).Return(comment, nil)
		siteURL := "http://localhost"
		te.mockAPI.On("GetConfig").Return(&mattermostModel.Config{ServiceSettings: mattermostModel.ServiceSettings{SiteURL: &siteURL}})
		te.mockAPI.On("GetDirectChannel", "userID1", te.plugin.BotUserID).Return(&mattermostModel.Channel{Id: "channelId1"}, nil).Times(1)
		post := GetDMPost(te.plugin.BotUserID, comment, file, siteURL)
		post.Props["attachments"].([]any)[0].(map[string]any)["pretext"] = fmt.Sprintf("User 1 replied on %s %s", utils.GetInlineImage("File icon:", file.IconLink), utils.GetHyperlink(file.Name, ""))
		post.Props["attachments"].([]any)[0].(map[string]any)["text"] = fmt.Sprintf("Previous reply:\n%s\n> %s", comment.Content, utils.MarkdownToHTMLEntities("> Reply to comment"))
		te.mockAPI.On("CreatePost", post).Return(nil, nil).Times(1)
		te.plugin.handleReplyAdded(context.Background(), mockGoogleDrive, fileID, "userID1", activity.Activities[0], file)
	})
}
