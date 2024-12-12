package plugin

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/golang/mock/gomock"
	mattermostModel "github.com/mattermost/mattermost/server/public/model"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"golang.org/x/oauth2"
	"google.golang.org/api/docs/v1"
	"google.golang.org/api/drive/v3"
	"google.golang.org/api/driveactivity/v2"
	"google.golang.org/api/sheets/v4"
	"google.golang.org/api/slides/v1"

	"github.com/mattermost-community/mattermost-plugin-google-drive/server/plugin/model"
	"github.com/mattermost-community/mattermost-plugin-google-drive/server/plugin/pluginapi"
	"github.com/mattermost-community/mattermost-plugin-google-drive/server/plugin/utils"
)

func TestNotificationWebhook(t *testing.T) {
	mocks := GetMockSetup(t)

	for name, test := range map[string]struct {
		expectedStatusCode int
		envSetup           func(e *TestEnvironment)
		modifyRequest      func(*http.Request) *http.Request
	}{
		"No UserId provided": {
			expectedStatusCode: http.StatusInternalServerError,
			envSetup: func(te *TestEnvironment) {
				te.mockAPI.On("GetUser", "").Return(nil, &mattermostModel.AppError{})
				te.mockAPI.On("LogError", mock.AnythingOfType("string"), mock.AnythingOfType("string"), mock.Anything, mock.AnythingOfType("string"), mock.AnythingOfType("string"))
			},
			modifyRequest: func(r *http.Request) *http.Request {
				r.URL.RawQuery = ""
				return r
			},
		},
		"Invalid Google token": {
			expectedStatusCode: http.StatusBadRequest,
			envSetup: func(te *TestEnvironment) {
				te.mockAPI.On("GetUser", "userId1").Return(&mattermostModel.User{}, nil)
				watchChannelData := GetSampleWatchChannelData()
				mocks.MockKVStore.EXPECT().GetWatchChannelData("userId1").Return(watchChannelData, nil)
				te.mockAPI.On("LogError", mock.AnythingOfType("string"), mock.AnythingOfType("string"), mock.AnythingOfType("string")).Maybe()
			},
			modifyRequest: func(r *http.Request) *http.Request {
				r.Header.Set("X-Goog-Channel-Token", "token")
				return r
			},
		},
		"Page token missing from KVstore but retrieve from GetStartPageToken method": {
			expectedStatusCode: http.StatusOK,
			envSetup: func(te *TestEnvironment) {
				te.mockAPI.On("GetUser", "userId1").Return(&mattermostModel.User{}, nil)
				watchChannelData := &model.WatchChannelData{
					ChannelID:  "channelId1",
					ResourceID: "resourceId1",
					MMUserID:   "userId1",
					Expiration: 0,
					Token:      "token1",
					PageToken:  "",
				}
				mocks.MockKVStore.EXPECT().GetWatchChannelData("userId1").Return(watchChannelData, nil).MaxTimes(2)
				mocks.MockGoogleClient.EXPECT().NewDriveService(context.Background(), "userId1").Return(mocks.MockGoogleDrive, nil)
				mocks.MockCluster.EXPECT().NewMutex("drive_watch_notifications_userId1").Return(pluginapi.NewClusterMutexMock(), nil)
				te.mockAPI.On("KVSetWithOptions", "mutex_drive_watch_notifications_userId1", mock.Anything, mock.Anything).Return(true, nil)
				mocks.MockGoogleDrive.EXPECT().GetStartPageToken(context.Background()).Return(&drive.StartPageToken{
					StartPageToken: "newPageToken1",
				}, nil)
				mocks.MockGoogleDrive.EXPECT().ChangesList(context.Background(), "newPageToken1").Return(&drive.ChangeList{NewStartPageToken: "newPageToken2"}, nil)
				newWatchChannelData := &model.WatchChannelData{
					ChannelID:  "channelId1",
					ResourceID: "resourceId1",
					MMUserID:   "userId1",
					Expiration: 0,
					Token:      "token1",
					PageToken:  "newPageToken2",
				}
				mocks.MockKVStore.EXPECT().StoreWatchChannelData("userId1", *newWatchChannelData).Return(nil)
			},
		},
		"Ensure we only hit the changelist a maximum of 5 times": {
			expectedStatusCode: http.StatusOK,
			envSetup: func(te *TestEnvironment) {
				te.mockAPI.On("GetUser", "userId1").Return(&mattermostModel.User{}, nil)
				watchChannelData := GetSampleWatchChannelData()
				mocks.MockKVStore.EXPECT().GetWatchChannelData("userId1").Return(watchChannelData, nil).MaxTimes(2)
				mocks.MockGoogleClient.EXPECT().NewDriveService(context.Background(), "userId1").Return(mocks.MockGoogleDrive, nil)
				mocks.MockCluster.EXPECT().NewMutex("drive_watch_notifications_userId1").Return(pluginapi.NewClusterMutexMock(), nil)
				te.mockAPI.On("KVSetWithOptions", "mutex_drive_watch_notifications_userId1", mock.Anything, mock.Anything).Return(true, nil)
				mocks.MockGoogleDrive.EXPECT().ChangesList(context.Background(), "pageToken1").Return(&drive.ChangeList{NewStartPageToken: "", NextPageToken: "pageToken1", Changes: []*drive.Change{}}, nil).MaxTimes(5)
				mocks.MockKVStore.EXPECT().StoreWatchChannelData("userId1", *watchChannelData).Return(nil)
			},
		},
		"Ensure we don't send the user a notification if they have opened the file since the last change": {
			expectedStatusCode: http.StatusOK,
			envSetup: func(te *TestEnvironment) {
				te.mockAPI.On("GetUser", "userId1").Return(&mattermostModel.User{}, nil)
				watchChannelData := GetSampleWatchChannelData()
				mocks.MockKVStore.EXPECT().GetWatchChannelData("userId1").Return(watchChannelData, nil).MaxTimes(2)
				mocks.MockGoogleClient.EXPECT().NewDriveService(context.Background(), "userId1").Return(mocks.MockGoogleDrive, nil)
				mocks.MockCluster.EXPECT().NewMutex("drive_watch_notifications_userId1").Return(pluginapi.NewClusterMutexMock(), nil)
				te.mockAPI.On("KVSetWithOptions", "mutex_drive_watch_notifications_userId1", mock.Anything, mock.Anything).Return(true, nil)
				changeList := GetSampleChangeList()
				changeList.Changes[0].File.ViewedByMeTime = "2021-01-02T00:00:00.000Z"
				mocks.MockGoogleDrive.EXPECT().ChangesList(context.Background(), watchChannelData.PageToken).Return(changeList, nil).MaxTimes(1)
				watchChannelData = &model.WatchChannelData{
					ChannelID:  "channelId1",
					ResourceID: "resourceId1",
					MMUserID:   "userId1",
					Expiration: 0,
					Token:      "token1",
					PageToken:  changeList.NewStartPageToken,
				}
				mocks.MockKVStore.EXPECT().StoreWatchChannelData("userId1", *watchChannelData).Return(nil)
				mocks.MockGoogleClient.EXPECT().NewDriveActivityService(context.Background(), "userId1").Return(mocks.MockDriveActivity, nil)
				mocks.MockKVStore.EXPECT().StoreLastActivityForFile("userId1", "fileId1", "2021-01-02T00:00:00.000Z").Return(nil)
			},
		},
		"Ensure we only hit the drive activity api a maximum of 5 times": {
			expectedStatusCode: http.StatusOK,
			envSetup: func(te *TestEnvironment) {
				te.mockAPI.On("GetUser", "userId1").Return(&mattermostModel.User{}, nil)
				watchChannelData := GetSampleWatchChannelData()
				mocks.MockKVStore.EXPECT().GetWatchChannelData("userId1").Return(watchChannelData, nil).MaxTimes(2)
				mocks.MockGoogleClient.EXPECT().NewDriveService(context.Background(), "userId1").Return(mocks.MockGoogleDrive, nil)
				mocks.MockCluster.EXPECT().NewMutex("drive_watch_notifications_userId1").Return(pluginapi.NewClusterMutexMock(), nil)
				te.mockAPI.On("KVSetWithOptions", "mutex_drive_watch_notifications_userId1", mock.Anything, mock.Anything).Return(true, nil)
				changeList := GetSampleChangeList()
				mocks.MockGoogleDrive.EXPECT().ChangesList(context.Background(), watchChannelData.PageToken).Return(changeList, nil).MaxTimes(1)
				watchChannelData = &model.WatchChannelData{
					ChannelID:  "channelId1",
					ResourceID: "resourceId1",
					MMUserID:   "userId1",
					Expiration: 0,
					Token:      "token1",
					PageToken:  changeList.NewStartPageToken,
				}
				mocks.MockKVStore.EXPECT().StoreWatchChannelData("userId1", *watchChannelData).Return(nil)
				mocks.MockGoogleClient.EXPECT().NewDriveActivityService(context.Background(), "userId1").Return(mocks.MockDriveActivity, nil)
				mocks.MockKVStore.EXPECT().GetLastActivityForFile("userId1", changeList.Changes[0].File.Id).Return(changeList.Changes[0].File.ModifiedTime, nil)
				mocks.MockDriveActivity.EXPECT().Query(context.Background(), &driveactivity.QueryDriveActivityRequest{
					ItemName: fmt.Sprintf("items/%s", changeList.Changes[0].File.Id),
					Filter:   "time > \"" + changeList.Changes[0].File.ModifiedTime + "\"",
				}).Return(&driveactivity.QueryDriveActivityResponse{Activities: []*driveactivity.DriveActivity{}, NextPageToken: "newPage"}, nil).MaxTimes(1)
				mocks.MockDriveActivity.EXPECT().Query(context.Background(), &driveactivity.QueryDriveActivityRequest{
					ItemName:  fmt.Sprintf("items/%s", changeList.Changes[0].File.Id),
					Filter:    "time > \"" + changeList.Changes[0].File.ModifiedTime + "\"",
					PageToken: "newPage",
				}).Return(&driveactivity.QueryDriveActivityResponse{Activities: []*driveactivity.DriveActivity{}, NextPageToken: "newPage"}, nil).MaxTimes(4)
				mocks.MockKVStore.EXPECT().StoreLastActivityForFile("userId1", changeList.Changes[0].File.Id, changeList.Changes[0].File.ModifiedTime).Return(nil)
			},
		},
		"Send one bot DM if there are more than 6 activities in a file": {
			expectedStatusCode: http.StatusOK,
			envSetup: func(te *TestEnvironment) {
				te.mockAPI.On("GetUser", "userId1").Return(&mattermostModel.User{}, nil)
				watchChannelData := GetSampleWatchChannelData()
				mocks.MockKVStore.EXPECT().GetWatchChannelData("userId1").Return(watchChannelData, nil).MaxTimes(2)
				mocks.MockGoogleClient.EXPECT().NewDriveService(context.Background(), "userId1").Return(mocks.MockGoogleDrive, nil)
				mocks.MockCluster.EXPECT().NewMutex("drive_watch_notifications_userId1").Return(pluginapi.NewClusterMutexMock(), nil)
				te.mockAPI.On("KVSetWithOptions", "mutex_drive_watch_notifications_userId1", mock.Anything, mock.Anything).Return(true, nil)
				changeList := GetSampleChangeList()
				mocks.MockGoogleDrive.EXPECT().ChangesList(context.Background(), watchChannelData.PageToken).Return(changeList, nil).MaxTimes(1)
				watchChannelData = &model.WatchChannelData{
					ChannelID:  "channelId1",
					ResourceID: "resourceId1",
					MMUserID:   "userId1",
					Expiration: 0,
					Token:      "token1",
					PageToken:  changeList.NewStartPageToken,
				}
				mocks.MockKVStore.EXPECT().StoreWatchChannelData("userId1", *watchChannelData).Return(nil)
				mocks.MockGoogleClient.EXPECT().NewDriveActivityService(context.Background(), "userId1").Return(mocks.MockDriveActivity, nil)
				mocks.MockKVStore.EXPECT().GetLastActivityForFile("userId1", changeList.Changes[0].File.Id).Return(changeList.Changes[0].File.ModifiedTime, nil)
				mocks.MockDriveActivity.EXPECT().Query(context.Background(), &driveactivity.QueryDriveActivityRequest{
					ItemName: fmt.Sprintf("items/%s", changeList.Changes[0].File.Id),
					Filter:   "time > \"" + changeList.Changes[0].File.ModifiedTime + "\"",
				}).Return(&driveactivity.QueryDriveActivityResponse{Activities: []*driveactivity.DriveActivity{
					{
						PrimaryActionDetail: &driveactivity.ActionDetail{
							Comment: &driveactivity.Comment{},
						},
					},
					{
						PrimaryActionDetail: &driveactivity.ActionDetail{
							Comment: &driveactivity.Comment{},
						},
					},
					{
						PrimaryActionDetail: &driveactivity.ActionDetail{
							Comment: &driveactivity.Comment{},
						},
					},
					{
						PrimaryActionDetail: &driveactivity.ActionDetail{
							Comment: &driveactivity.Comment{},
						},
					},
					{
						PrimaryActionDetail: &driveactivity.ActionDetail{
							Comment: &driveactivity.Comment{},
						},
					},
					{
						PrimaryActionDetail: &driveactivity.ActionDetail{
							Comment: &driveactivity.Comment{},
						},
					},
				}, NextPageToken: ""}, nil).MaxTimes(1)
				te.mockAPI.On("GetDirectChannel", "userId1", te.plugin.BotUserID).Return(&mattermostModel.Channel{Id: "channelId1"}, nil).Times(1)
				te.mockAPI.On("CreatePost", mock.Anything).Return(nil, nil).Times(1)
				mocks.MockKVStore.EXPECT().StoreLastActivityForFile("userId1", changeList.Changes[0].File.Id, changeList.Changes[0].File.ModifiedTime).Return(nil)
			},
		},
		"Send a notification for a permission change on a file": {
			expectedStatusCode: http.StatusOK,
			envSetup: func(te *TestEnvironment) {
				te.mockAPI.On("GetUser", "userId1").Return(&mattermostModel.User{}, nil)
				watchChannelData := GetSampleWatchChannelData()
				mocks.MockKVStore.EXPECT().GetWatchChannelData("userId1").Return(watchChannelData, nil).MaxTimes(2)
				mocks.MockGoogleClient.EXPECT().NewDriveService(context.Background(), "userId1").Return(mocks.MockGoogleDrive, nil)
				mocks.MockCluster.EXPECT().NewMutex("drive_watch_notifications_userId1").Return(pluginapi.NewClusterMutexMock(), nil)
				te.mockAPI.On("KVSetWithOptions", "mutex_drive_watch_notifications_userId1", mock.Anything, mock.Anything).Return(true, nil)
				changeList := GetSampleChangeList()
				mocks.MockGoogleDrive.EXPECT().ChangesList(context.Background(), watchChannelData.PageToken).Return(changeList, nil).MaxTimes(1)
				watchChannelData = &model.WatchChannelData{
					ChannelID:  "channelId1",
					ResourceID: "resourceId1",
					MMUserID:   "userId1",
					Expiration: 0,
					Token:      "token1",
					PageToken:  changeList.NewStartPageToken,
				}
				mocks.MockKVStore.EXPECT().StoreWatchChannelData("userId1", *watchChannelData).Return(nil)
				mocks.MockGoogleClient.EXPECT().NewDriveActivityService(context.Background(), "userId1").Return(mocks.MockDriveActivity, nil)
				mocks.MockKVStore.EXPECT().GetLastActivityForFile("userId1", changeList.Changes[0].File.Id).Return(changeList.Changes[0].File.ModifiedTime, nil)
				activityResponse := GetSampleDriveactivityPermissionResponse()
				mocks.MockDriveActivity.EXPECT().Query(context.Background(), &driveactivity.QueryDriveActivityRequest{
					ItemName: fmt.Sprintf("items/%s", changeList.Changes[0].File.Id),
					Filter:   "time > \"" + changeList.Changes[0].File.ModifiedTime + "\"",
				}).Return(activityResponse, nil).MaxTimes(1)
				te.mockAPI.On("GetConfig").Return(nil)
				te.mockAPI.On("GetDirectChannel", "userId1", te.plugin.BotUserID).Return(&mattermostModel.Channel{Id: "channelId1"}, nil).Times(1)
				post := &mattermostModel.Post{
					UserId:    te.plugin.BotUserID,
					ChannelId: "channelId1",
					Message:   "Someone shared an item with you",
					Props: mattermostModel.StringInterface{
						"attachments": []any{map[string]any{
							"title":       changeList.Changes[0].File.Name,
							"title_link":  changeList.Changes[0].File.WebViewLink,
							"footer":      "Google Drive for Mattermost",
							"footer_icon": changeList.Changes[0].File.IconLink,
						}},
					},
				}
				te.mockAPI.On("CreatePost", post).Return(nil, nil).Times(1)
				mocks.MockKVStore.EXPECT().StoreLastActivityForFile("userId1", changeList.Changes[0].File.Id, activityResponse.Activities[0].Timestamp).Return(nil)
			},
		},
		"Send a notification for a comment on a file": {
			expectedStatusCode: http.StatusOK,
			envSetup: func(te *TestEnvironment) {
				te.mockAPI.On("GetUser", "userId1").Return(&mattermostModel.User{}, nil)
				watchChannelData := GetSampleWatchChannelData()
				mocks.MockKVStore.EXPECT().GetWatchChannelData("userId1").Return(watchChannelData, nil).MaxTimes(2)
				mocks.MockGoogleClient.EXPECT().NewDriveService(context.Background(), "userId1").Return(mocks.MockGoogleDrive, nil)
				mocks.MockCluster.EXPECT().NewMutex("drive_watch_notifications_userId1").Return(pluginapi.NewClusterMutexMock(), nil)
				te.mockAPI.On("KVSetWithOptions", "mutex_drive_watch_notifications_userId1", mock.Anything, mock.Anything).Return(true, nil)
				changeList := GetSampleChangeList()
				mocks.MockGoogleDrive.EXPECT().ChangesList(context.Background(), watchChannelData.PageToken).Return(changeList, nil).MaxTimes(1)
				watchChannelData = &model.WatchChannelData{
					ChannelID:  "channelId1",
					ResourceID: "resourceId1",
					MMUserID:   "userId1",
					Expiration: 0,
					Token:      "token1",
					PageToken:  changeList.NewStartPageToken,
				}
				mocks.MockKVStore.EXPECT().StoreWatchChannelData("userId1", *watchChannelData).Return(nil)
				mocks.MockGoogleClient.EXPECT().NewDriveActivityService(context.Background(), "userId1").Return(mocks.MockDriveActivity, nil)
				file := changeList.Changes[0].File
				mocks.MockKVStore.EXPECT().GetLastActivityForFile("userId1", file.Id).Return(file.ModifiedTime, nil)
				activityResponse := GetSampleDriveactivityCommentResponse()
				mocks.MockDriveActivity.EXPECT().Query(context.Background(), &driveactivity.QueryDriveActivityRequest{
					ItemName: fmt.Sprintf("items/%s", file.Id),
					Filter:   "time > \"" + file.ModifiedTime + "\"",
				}).Return(activityResponse, nil).MaxTimes(1)
				commentID := activityResponse.Activities[0].Targets[0].FileComment.LegacyCommentId
				comment := GetSampleComment(commentID)
				mocks.MockGoogleDrive.EXPECT().GetComments(context.Background(), file.Id, commentID).Return(comment, nil)
				siteURL := "http://localhost"
				te.mockAPI.On("GetConfig").Return(&mattermostModel.Config{ServiceSettings: mattermostModel.ServiceSettings{SiteURL: &siteURL}})
				te.mockAPI.On("GetDirectChannel", "userId1", te.plugin.BotUserID).Return(&mattermostModel.Channel{Id: "channelId1"}, nil).Times(1)
				post := &mattermostModel.Post{
					UserId:    te.plugin.BotUserID,
					ChannelId: "channelId1",
					Message:   "",
					Props: mattermostModel.StringInterface{
						"attachments": []any{
							map[string]any{
								"pretext": fmt.Sprintf("%s commented on %s %s", comment.Author.DisplayName, utils.GetInlineImage("File icon:", file.IconLink), utils.GetHyperlink(file.Name, file.WebViewLink)),
								"text":    fmt.Sprintf("%s\n> %s", "", comment.Content),
								"actions": []any{
									map[string]any{
										"name": "Reply to comment",
										"integration": map[string]any{
											"url": fmt.Sprintf("%s/plugins/%s/api/v1/reply_dialog", siteURL, Manifest.Id),
											"context": map[string]any{
												"commentID": comment.Id,
												"fileID":    file.Id,
											},
										},
									},
								},
							},
						},
					},
				}
				te.mockAPI.On("CreatePost", post).Return(nil, nil).Times(1)
				mocks.MockKVStore.EXPECT().StoreLastActivityForFile("userId1", changeList.Changes[0].File.Id, activityResponse.Activities[0].Timestamp).Return(nil)
			},
		},
	} {
		t.Run(name, func(t *testing.T) {
			assert := assert.New(t)
			te := SetupTestEnvironment(t)
			defer te.Cleanup(t)

			te.plugin.KVStore = mocks.MockKVStore
			te.plugin.GoogleClient = mocks.MockGoogleClient
			te.plugin.initializeAPI()

			test.envSetup(te)

			w := httptest.NewRecorder()
			r := httptest.NewRequest(http.MethodPost, "/webhook?userID=userId1", nil)
			r.Header.Set("X-Goog-Resource-State", "change")
			r.Header.Set("X-Goog-Channel-Token", "token1")
			ctx := &Context{
				Ctx:    context.Background(),
				UserID: "userId1",
				Log:    nil,
			}
			if test.modifyRequest != nil {
				r = test.modifyRequest(r)
			}
			te.plugin.handleDriveWatchNotifications(ctx, w, r)

			result := w.Result()
			require.NotNil(t, result)
			defer result.Body.Close()
			assert.Equal(test.expectedStatusCode, result.StatusCode)
		})
	}
}

func TestFileCreationEndpoint(t *testing.T) {
	mocks := GetMockSetup(t)

	for name, test := range map[string]struct {
		expectedStatusCode int
		submission         *mattermostModel.SubmitDialogRequest
		envSetup           func(ctx context.Context, te *TestEnvironment)
		fileType           string
	}{
		"No FileType parameter provided": {
			expectedStatusCode: http.StatusBadRequest,
			submission: &mattermostModel.SubmitDialogRequest{
				Submission: map[string]any{
					"name":             "file name",
					"file_access":      "all_comment",
					"message":          "file message",
					"share_in_channel": true,
				},
			},
			envSetup: func(ctx context.Context, te *TestEnvironment) {
				te.mockAPI.On("LogError", mock.AnythingOfType("string"), mock.AnythingOfType("string"), mock.AnythingOfType("string")).Maybe()
			},
			fileType: "",
		},
		"Create a doc with all_comment access": {
			fileType:           "doc",
			expectedStatusCode: http.StatusOK,
			submission: &mattermostModel.SubmitDialogRequest{
				Submission: map[string]any{
					"name":             "file name",
					"file_access":      "all_comment",
					"message":          "file message",
					"share_in_channel": false,
				},
			},
			envSetup: func(ctx context.Context, te *TestEnvironment) {
				mocks.MockGoogleClient.EXPECT().NewDocsService(ctx, "userId1").Return(mocks.MockGoogleDocs, nil)
				doc := GetSampleDoc()
				mocks.MockGoogleDocs.EXPECT().Create(ctx, &docs.Document{
					Title: "file name",
				}).Return(doc, nil)
				mocks.MockGoogleClient.EXPECT().NewDriveService(ctx, "userId1").Return(mocks.MockGoogleDrive, nil).Times(2)
				te.mockAPI.On("GetConfig").Return(nil)
				mocks.MockGoogleDrive.EXPECT().CreatePermission(ctx, doc.DocumentId, &drive.Permission{
					Role: "commenter",
					Type: "anyone",
				}).Return(&drive.Permission{}, nil).MaxTimes(1)
				file := GetSampleFile(doc.DocumentId)
				mocks.MockGoogleDrive.EXPECT().GetFile(ctx, doc.DocumentId).Return(file, nil)
				te.mockAPI.On("GetDirectChannel", "userId1", te.plugin.BotUserID).Return(&mattermostModel.Channel{Id: "channelId1"}, nil).Times(1)
				te.mockAPI.On("CreatePost", mock.Anything).Return(nil, nil).Times(1)
			},
		},
		"Create a sheet with all_edit access": {
			fileType:           "sheet",
			expectedStatusCode: http.StatusOK,
			submission: &mattermostModel.SubmitDialogRequest{
				Submission: map[string]any{
					"name":             "file name",
					"file_access":      "all_edit",
					"message":          "file message",
					"share_in_channel": false,
				},
			},
			envSetup: func(ctx context.Context, te *TestEnvironment) {
				mocks.MockGoogleClient.EXPECT().NewSheetsService(ctx, "userId1").Return(mocks.MockGoogleSheets, nil)
				sheet := GetSampleSheet()
				mocks.MockGoogleSheets.EXPECT().Create(ctx, &sheets.Spreadsheet{
					Properties: &sheets.SpreadsheetProperties{
						Title: "file name",
					},
				}).Return(sheet, nil)
				mocks.MockGoogleClient.EXPECT().NewDriveService(ctx, "userId1").Return(mocks.MockGoogleDrive, nil).Times(2)
				te.mockAPI.On("GetConfig").Return(nil)
				mocks.MockGoogleDrive.EXPECT().CreatePermission(ctx, sheet.SpreadsheetId, &drive.Permission{
					Role: "writer",
					Type: "anyone",
				}).Return(&drive.Permission{}, nil).MaxTimes(1)
				file := GetSampleFile(sheet.SpreadsheetId)
				mocks.MockGoogleDrive.EXPECT().GetFile(ctx, sheet.SpreadsheetId).Return(file, nil)
				te.mockAPI.On("GetDirectChannel", "userId1", te.plugin.BotUserID).Return(&mattermostModel.Channel{Id: "channelId1"}, nil).Times(1)
				te.mockAPI.On("CreatePost", mock.Anything).Return(nil, nil).Times(1)
			},
		},
		"Create a presentation with all_view access": {
			fileType:           "slide",
			expectedStatusCode: http.StatusOK,
			submission: &mattermostModel.SubmitDialogRequest{
				Submission: map[string]any{
					"name":             "file name",
					"file_access":      "all_view",
					"message":          "file message",
					"share_in_channel": false,
				},
			},
			envSetup: func(ctx context.Context, te *TestEnvironment) {
				mocks.MockGoogleClient.EXPECT().NewSlidesService(ctx, "userId1").Return(mocks.MockGoogleSlides, nil)
				presentation := GetSamplePresentation()
				mocks.MockGoogleSlides.EXPECT().Create(ctx, &slides.Presentation{
					Title: "file name",
				}).Return(presentation, nil)
				mocks.MockGoogleClient.EXPECT().NewDriveService(ctx, "userId1").Return(mocks.MockGoogleDrive, nil).Times(2)
				te.mockAPI.On("GetConfig").Return(nil)
				mocks.MockGoogleDrive.EXPECT().CreatePermission(ctx, presentation.PresentationId, &drive.Permission{
					Role: "reader",
					Type: "anyone",
				}).Return(&drive.Permission{}, nil).MaxTimes(1)
				file := GetSampleFile(presentation.PresentationId)
				mocks.MockGoogleDrive.EXPECT().GetFile(ctx, presentation.PresentationId).Return(file, nil)
				te.mockAPI.On("GetDirectChannel", "userId1", te.plugin.BotUserID).Return(&mattermostModel.Channel{Id: "channelId1"}, nil).Times(1)
				te.mockAPI.On("CreatePost", mock.Anything).Return(nil, nil).Times(1)
			},
		},
		"Create a private doc": {
			fileType:           "doc",
			expectedStatusCode: http.StatusOK,
			submission: &mattermostModel.SubmitDialogRequest{
				Submission: map[string]any{
					"name":             "file name",
					"file_access":      "private",
					"message":          "file message",
					"share_in_channel": false,
				},
			},
			envSetup: func(ctx context.Context, te *TestEnvironment) {
				mocks.MockGoogleClient.EXPECT().NewDocsService(ctx, "userId1").Return(mocks.MockGoogleDocs, nil)
				doc := GetSampleDoc()
				mocks.MockGoogleDocs.EXPECT().Create(ctx, &docs.Document{
					Title: "file name",
				}).Return(doc, nil)
				mocks.MockGoogleClient.EXPECT().NewDriveService(ctx, "userId1").Return(mocks.MockGoogleDrive, nil).Times(2)
				te.mockAPI.On("GetConfig").Return(nil)
				file := GetSampleFile(doc.DocumentId)
				mocks.MockGoogleDrive.EXPECT().GetFile(ctx, doc.DocumentId).Return(file, nil)
				te.mockAPI.On("GetDirectChannel", "userId1", te.plugin.BotUserID).Return(&mattermostModel.Channel{Id: "channelId1"}, nil).Times(1)
				te.mockAPI.On("CreatePost", mock.Anything).Return(nil, nil).Times(1)
			},
		},
		"Create a doc with all_comment access and share in channel": {
			fileType:           "doc",
			expectedStatusCode: http.StatusOK,
			submission: &mattermostModel.SubmitDialogRequest{
				ChannelId: "channelId1",
				Submission: map[string]any{
					"name":             "file name",
					"file_access":      "all_comment",
					"message":          "file message",
					"share_in_channel": true,
				},
			},
			envSetup: func(ctx context.Context, te *TestEnvironment) {
				mocks.MockGoogleClient.EXPECT().NewDocsService(ctx, "userId1").Return(mocks.MockGoogleDocs, nil)
				doc := GetSampleDoc()
				mocks.MockGoogleDocs.EXPECT().Create(ctx, &docs.Document{
					Title: "file name",
				}).Return(doc, nil)
				mocks.MockGoogleClient.EXPECT().NewDriveService(ctx, "userId1").Return(mocks.MockGoogleDrive, nil).Times(2)
				te.mockAPI.On("GetConfig").Return(nil)
				mocks.MockGoogleDrive.EXPECT().CreatePermission(ctx, doc.DocumentId, &drive.Permission{
					Role: "commenter",
					Type: "anyone",
				}).Return(&drive.Permission{}, nil).MaxTimes(1)
				file := GetSampleFile(doc.DocumentId)
				mocks.MockGoogleDrive.EXPECT().GetFile(ctx, doc.DocumentId).Return(file, nil)
				createdTime, err := time.Parse(time.RFC3339, file.CreatedTime)
				require.NoError(t, err)
				post := &mattermostModel.Post{
					UserId:    te.plugin.BotUserID,
					ChannelId: "channelId1",
					Message:   "file message",
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
				te.mockAPI.On("CreatePost", post).Return(nil, nil).Times(1)
			},
		},
		"Create a doc and allow channels members to comment": {
			fileType:           "doc",
			expectedStatusCode: http.StatusOK,
			submission: &mattermostModel.SubmitDialogRequest{
				ChannelId: "channelId1",
				Submission: map[string]any{
					"name":             "file name",
					"file_access":      "members_comment",
					"message":          "file message",
					"share_in_channel": true,
				},
			},
			envSetup: func(ctx context.Context, te *TestEnvironment) {
				mocks.MockGoogleClient.EXPECT().NewDocsService(ctx, "userId1").Return(mocks.MockGoogleDocs, nil)
				doc := GetSampleDoc()
				mocks.MockGoogleDocs.EXPECT().Create(ctx, &docs.Document{
					Title: "file name",
				}).Return(doc, nil)
				mocks.MockGoogleClient.EXPECT().NewDriveService(ctx, "userId1").Return(mocks.MockGoogleDrive, nil).Times(2)
				te.mockAPI.On("GetConfig").Return(nil)
				users := []*mattermostModel.User{
					{
						Email: "user1@mattermost.com",
						IsBot: false,
					},
					{
						Email: "user2@mattermost.com",
						IsBot: false,
					},
				}
				te.mockAPI.On("GetUsersInChannel", "channelId1", "username", 0, 100).Return(users, nil).Times(1)
				te.mockAPI.On("GetUsersInChannel", "channelId1", "username", 1, 100).Return([]*mattermostModel.User{}, nil).Times(1)

				mocks.MockGoogleDrive.EXPECT().CreatePermission(ctx, doc.DocumentId, &drive.Permission{
					Role:         "commenter",
					EmailAddress: users[0].Email,
					Type:         "user",
				}).Return(&drive.Permission{}, nil).MaxTimes(1)
				mocks.MockGoogleDrive.EXPECT().CreatePermission(ctx, doc.DocumentId, &drive.Permission{
					Role:         "commenter",
					EmailAddress: users[1].Email,
					Type:         "user",
				}).Return(&drive.Permission{}, nil).MaxTimes(1)
				file := GetSampleFile(doc.DocumentId)
				mocks.MockGoogleDrive.EXPECT().GetFile(ctx, doc.DocumentId).Return(file, nil)
				createdTime, err := time.Parse(time.RFC3339, file.CreatedTime)
				require.NoError(t, err)
				post := &mattermostModel.Post{
					UserId:    te.plugin.BotUserID,
					ChannelId: "channelId1",
					Message:   "file message",
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
				te.mockAPI.On("CreatePost", post).Return(nil, nil).Times(1)
			},
		},
	} {
		t.Run(name, func(t *testing.T) {
			assert := assert.New(t)
			te := SetupTestEnvironment(t)
			defer te.Cleanup(t)

			te.plugin.KVStore = mocks.MockKVStore
			te.plugin.GoogleClient = mocks.MockGoogleClient
			te.plugin.initializeAPI()

			w := httptest.NewRecorder()

			var body bytes.Buffer
			err := json.NewEncoder(&body).Encode(test.submission)
			if err != nil {
				require.NoError(t, err)
			}
			r := httptest.NewRequest(http.MethodPost, fmt.Sprintf("/create?type=%s", test.fileType), &body)
			r.Header.Set("Mattermost-User-ID", "userId1")
			ctx, _ := te.plugin.createContext(w, r)

			test.envSetup(ctx.Ctx, te)
			te.plugin.handleFileCreation(ctx, w, r)

			result := w.Result()
			require.NotNil(t, result)
			defer result.Body.Close()
			assert.Equal(test.expectedStatusCode, result.StatusCode)
		})
	}
}

func TestUploadFile(t *testing.T) {
	mocks := GetMockSetup(t)

	for name, test := range map[string]struct {
		expectedStatusCode int
		submission         *mattermostModel.SubmitDialogRequest
		envSetup           func(ctx context.Context, te *TestEnvironment)
	}{
		"No file provided": {
			expectedStatusCode: http.StatusBadRequest,
			submission: &mattermostModel.SubmitDialogRequest{
				Submission: map[string]any{},
			},
			envSetup: func(ctx context.Context, te *TestEnvironment) {
				te.mockAPI.On("LogError", mock.AnythingOfType("string"), mock.AnythingOfType("string"), mock.AnythingOfType("string"))
			},
		},
		"Empty fileID in submission": {
			expectedStatusCode: http.StatusBadRequest,
			submission: &mattermostModel.SubmitDialogRequest{
				Submission: map[string]any{
					"fileID": "",
				},
			},
			envSetup: func(ctx context.Context, te *TestEnvironment) {
				te.mockAPI.On("LogError", mock.AnythingOfType("string"), mock.AnythingOfType("string"), mock.AnythingOfType("string"))
			},
		},
		"Create File on Google Drive send ephemeral post": {
			expectedStatusCode: http.StatusOK,
			submission: &mattermostModel.SubmitDialogRequest{
				Submission: map[string]any{
					"fileID": "fileId1",
				},
			},
			envSetup: func(ctx context.Context, te *TestEnvironment) {
				te.mockAPI.On("GetFileInfo", "fileId1").Return(&mattermostModel.FileInfo{
					Id:     "fileId1",
					PostId: "postId1",
					Name:   "file name",
				}, nil)
				te.mockAPI.On("GetFile", "fileId1").Return([]byte{}, nil)
				mocks.MockGoogleClient.EXPECT().NewDriveService(ctx, "userId1").Return(mocks.MockGoogleDrive, nil)
				mocks.MockGoogleDrive.EXPECT().CreateFile(ctx, &drive.File{Name: "file name"}, []byte{}).Return(nil, nil)
				te.mockAPI.On("SendEphemeralPost", "userId1", mock.Anything).Return(nil)
			},
		},
	} {
		t.Run(name, func(t *testing.T) {
			assert := assert.New(t)
			te := SetupTestEnvironment(t)
			defer te.Cleanup(t)

			te.plugin.KVStore = mocks.MockKVStore
			te.plugin.GoogleClient = mocks.MockGoogleClient
			te.plugin.initializeAPI()

			w := httptest.NewRecorder()

			var body bytes.Buffer
			err := json.NewEncoder(&body).Encode(test.submission)
			if err != nil {
				require.NoError(t, err)
			}
			r := httptest.NewRequest(http.MethodPost, "/upload", &body)
			r.Header.Set("Mattermost-User-ID", "userId1")
			ctx, _ := te.plugin.createContext(w, r)

			test.envSetup(ctx.Ctx, te)
			te.plugin.handleFileUpload(ctx, w, r)

			result := w.Result()
			require.NotNil(t, result)
			defer result.Body.Close()
			assert.Equal(test.expectedStatusCode, result.StatusCode)
		})
	}
}

func TestUploadMultipleFiles(t *testing.T) {
	mocks := GetMockSetup(t)

	for name, test := range map[string]struct {
		expectedStatusCode int
		submission         *mattermostModel.SubmitDialogRequest
		envSetup           func(ctx context.Context, te *TestEnvironment)
	}{
		"No postId provided": {
			expectedStatusCode: http.StatusBadRequest,
			submission: &mattermostModel.SubmitDialogRequest{
				State: "",
			},
			envSetup: func(ctx context.Context, te *TestEnvironment) {
				te.mockAPI.On("GetPost", "").Return(nil, &mattermostModel.AppError{
					Message: "No post provided",
				})
				te.mockAPI.On("LogError", mock.AnythingOfType("string"), mock.AnythingOfType("string"), mock.AnythingOfType("string"), mock.Anything, mock.Anything)
			},
		},
		"PostId with multiple file attachments": {
			expectedStatusCode: http.StatusOK,
			submission: &mattermostModel.SubmitDialogRequest{
				State: "postId1",
			},
			envSetup: func(ctx context.Context, te *TestEnvironment) {
				te.mockAPI.On("GetPost", "postId1").Return(&mattermostModel.Post{
					Id:      "postId1",
					FileIds: []string{"fileId1", "fileId2"},
				}, nil)
				mocks.MockGoogleClient.EXPECT().NewDriveService(ctx, "userId1").Return(mocks.MockGoogleDrive, nil)
				te.mockAPI.On("GetFileInfo", "fileId1").Return(&mattermostModel.FileInfo{
					Id:     "fileId1",
					PostId: "postId1",
					Name:   "file name",
				}, nil)
				te.mockAPI.On("GetFile", "fileId1").Return([]byte{}, nil)
				te.mockAPI.On("GetFileInfo", "fileId2").Return(&mattermostModel.FileInfo{
					Id:     "fileId1",
					PostId: "postId1",
					Name:   "file name",
				}, nil)
				te.mockAPI.On("GetFile", "fileId2").Return([]byte{}, nil)
				mocks.MockGoogleDrive.EXPECT().CreateFile(ctx, &drive.File{Name: "file name"}, []byte{}).Return(nil, nil).Times(2)
				te.mockAPI.On("SendEphemeralPost", "userId1", mock.Anything).Return(nil)
			},
		},
	} {
		t.Run(name, func(t *testing.T) {
			assert := assert.New(t)
			te := SetupTestEnvironment(t)
			defer te.Cleanup(t)

			te.plugin.KVStore = mocks.MockKVStore
			te.plugin.GoogleClient = mocks.MockGoogleClient
			te.plugin.initializeAPI()

			w := httptest.NewRecorder()

			var body bytes.Buffer
			err := json.NewEncoder(&body).Encode(test.submission)
			if err != nil {
				require.NoError(t, err)
			}
			r := httptest.NewRequest(http.MethodPost, "/upload", &body)
			r.Header.Set("Mattermost-User-ID", "userId1")
			ctx, _ := te.plugin.createContext(w, r)

			test.envSetup(ctx.Ctx, te)
			te.plugin.handleAllFilesUpload(ctx, w, r)

			result := w.Result()
			require.NotNil(t, result)
			defer result.Body.Close()
			assert.Equal(test.expectedStatusCode, result.StatusCode)
		})
	}
}

func TestCompleteConnectUserToGoogle(t *testing.T) {
	mocks := GetMockSetup(t)
	userID := mattermostModel.NewRandomString(26)
	userID2 := mattermostModel.NewRandomString(26)

	for name, test := range map[string]struct {
		expectedStatusCode int
		envSetup           func(ctx context.Context, te *TestEnvironment)
		modifyRequest      func(*http.Request) *http.Request
	}{
		"No code in URL query": {
			expectedStatusCode: http.StatusBadRequest,
			envSetup: func(ctx context.Context, te *TestEnvironment) {
			},
			modifyRequest: func(r *http.Request) *http.Request {
				values := r.URL.Query()
				values.Del("code")
				r.URL.RawQuery = values.Encode()
				return r
			},
		},
		"No state in URL query": {
			expectedStatusCode: http.StatusBadRequest,
			envSetup: func(ctx context.Context, te *TestEnvironment) {
			},
			modifyRequest: func(r *http.Request) *http.Request {
				values := r.URL.Query()
				values.Del("state")
				r.URL.RawQuery = values.Encode()
				return r
			},
		},
		"State token does not match stored token": {
			expectedStatusCode: http.StatusBadRequest,
			envSetup: func(ctx context.Context, te *TestEnvironment) {
				mocks.MockKVStore.EXPECT().GetOAuthStateToken("oauthstate12345_"+userID).Return([]byte("randomState"), nil)
				mocks.MockKVStore.EXPECT().DeleteOAuthStateToken("oauthstate12345_" + userID).Return(nil)
			},
		},
		"State token does not contain the correct userID": {
			expectedStatusCode: http.StatusUnauthorized,
			envSetup: func(ctx context.Context, te *TestEnvironment) {
				mocks.MockKVStore.EXPECT().GetOAuthStateToken("oauthstate12345_"+userID2).Return([]byte("oauthstate12345_"+userID2), nil)
				mocks.MockKVStore.EXPECT().DeleteOAuthStateToken("oauthstate12345_" + userID2).Return(nil)
			},
			modifyRequest: func(r *http.Request) *http.Request {
				values := r.URL.Query()
				values.Set("state", "oauthstate12345_"+userID2)
				values.Set("code", "oauthcode")
				r.URL.RawQuery = values.Encode()
				return r
			},
		},
		"Success complete oauth setup": {
			expectedStatusCode: http.StatusOK,
			envSetup: func(ctx context.Context, te *TestEnvironment) {
				mocks.MockKVStore.EXPECT().GetOAuthStateToken("oauthstate12345_"+userID).Return([]byte("oauthstate12345_"+userID), nil)
				mocks.MockKVStore.EXPECT().DeleteOAuthStateToken("oauthstate12345_" + userID).Return(nil)
				mocks.MockOAuth2.EXPECT().Exchange(ctx, "oauthcode").Return(&oauth2.Token{
					AccessToken: "accessToken12345",
					TokenType:   "Bearer",
					Expiry:      time.Now().Add(time.Hour),
				}, nil)
				mocks.MockKVStore.EXPECT().StoreGoogleUserToken(userID, gomock.Any()).Return(nil)
				te.mockAPI.On("GetDirectChannel", userID, te.plugin.BotUserID).Return(&mattermostModel.Channel{Id: "channelId1"}, nil).Times(1)
				te.mockAPI.On("CreatePost", mock.Anything).Return(nil, nil).Times(1)
				mocks.MockTelemetry.EXPECT().TrackUserEvent("account_connected", userID, nil)
				te.mockAPI.On("PublishWebSocketEvent", "google_connect", map[string]interface{}{"connected": true, "google_client_id": "randomstring.apps.googleusercontent.com"}, &mattermostModel.WebsocketBroadcast{OmitUsers: map[string]bool(nil), UserId: userID, ChannelId: "", TeamId: "", ConnectionId: "", OmitConnectionId: "", ContainsSanitizedData: false, ContainsSensitiveData: false, ReliableClusterSend: false, BroadcastHooks: []string(nil), BroadcastHookArgs: []map[string]interface{}(nil)}).Times(1)
			},
		},
	} {
		t.Run(name, func(t *testing.T) {
			assert := assert.New(t)
			te := SetupTestEnvironment(t)
			defer te.Cleanup(t)

			te.plugin.KVStore = mocks.MockKVStore
			te.plugin.GoogleClient = mocks.MockGoogleClient
			te.plugin.oauthConfig = mocks.MockOAuth2
			te.plugin.tracker = mocks.MockTelemetry
			te.plugin.initializeAPI()

			w := httptest.NewRecorder()
			r := httptest.NewRequest(http.MethodGet, "/complete?code=oauthcode&state=oauthstate12345_"+userID, nil)
			r.Header.Set("Mattermost-User-ID", userID)
			ctx, _ := te.plugin.createContext(w, r)
			if test.modifyRequest != nil {
				r = test.modifyRequest(r)
			}

			test.envSetup(ctx.Ctx, te)
			te.plugin.completeConnectUserToGoogle(ctx, w, r)

			result := w.Result()
			require.NotNil(t, result)
			defer result.Body.Close()
			assert.Equal(test.expectedStatusCode, result.StatusCode)
		})
	}
}
