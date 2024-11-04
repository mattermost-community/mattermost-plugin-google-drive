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

	mattermostModel "github.com/mattermost/mattermost/server/public/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
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
	mockKvStore, mockGoogleClient, mockGoogleDrive, mockDriveActivity, _, _, _, _, mockCluster := GetMockSetup(t)

	for name, test := range map[string]struct {
		expectedStatusCode int
		envSetup           func(e *TestEnvironment)
		modifyRequest      func(*http.Request) *http.Request
	}{
		"No UserId provided": {
			expectedStatusCode: http.StatusBadRequest,
			envSetup: func(te *TestEnvironment) {
				watchChannelData := &model.WatchChannelData{
					ChannelID:  "",
					ResourceID: "",
					MMUserID:   "",
					Expiration: 0,
					Token:      "",
					PageToken:  "",
				}
				mockKvStore.EXPECT().GetWatchChannelData("").Return(watchChannelData, nil)
				te.mockAPI.On("LogError", mock.AnythingOfType("string"), mock.AnythingOfType("string"), mock.AnythingOfType("string")).Maybe()
			},
			modifyRequest: func(r *http.Request) *http.Request {
				r.URL.RawQuery = ""
				return r
			},
		},
		"Invalid Google token": {
			expectedStatusCode: http.StatusBadRequest,
			envSetup: func(te *TestEnvironment) {
				watchChannelData := GetSampleWatchChannelData()
				mockKvStore.EXPECT().GetWatchChannelData("userId1").Return(watchChannelData, nil)
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
				watchChannelData := &model.WatchChannelData{
					ChannelID:  "channelId1",
					ResourceID: "resourceId1",
					MMUserID:   "userId1",
					Expiration: 0,
					Token:      "token1",
					PageToken:  "",
				}
				mockKvStore.EXPECT().GetWatchChannelData("userId1").Return(watchChannelData, nil).MaxTimes(2)
				mockGoogleClient.EXPECT().NewDriveService(context.Background(), "userId1").Return(mockGoogleDrive, nil)
				mockCluster.EXPECT().NewMutex("drive_watch_notifications_userId1").Return(pluginapi.NewClusterMutexMock(), nil)
				te.mockAPI.On("KVSetWithOptions", "mutex_drive_watch_notifications_userId1", mock.Anything, mock.Anything).Return(true, nil)
				mockGoogleDrive.EXPECT().GetStartPageToken(context.Background()).Return(&drive.StartPageToken{
					StartPageToken: "newPageToken1",
				}, nil)
				mockGoogleDrive.EXPECT().ChangesList(context.Background(), "newPageToken1").Return(&drive.ChangeList{NewStartPageToken: "newPageToken2"}, nil)
				newWatchChannelData := &model.WatchChannelData{
					ChannelID:  "channelId1",
					ResourceID: "resourceId1",
					MMUserID:   "userId1",
					Expiration: 0,
					Token:      "token1",
					PageToken:  "newPageToken2",
				}
				mockKvStore.EXPECT().StoreWatchChannelData("userId1", *newWatchChannelData).Return(nil)
			},
		},
		"Ensure we only hit the changelist a maximum of 5 times": {
			expectedStatusCode: http.StatusOK,
			envSetup: func(te *TestEnvironment) {
				watchChannelData := GetSampleWatchChannelData()
				mockKvStore.EXPECT().GetWatchChannelData("userId1").Return(watchChannelData, nil).MaxTimes(2)
				mockGoogleClient.EXPECT().NewDriveService(context.Background(), "userId1").Return(mockGoogleDrive, nil)
				mockCluster.EXPECT().NewMutex("drive_watch_notifications_userId1").Return(pluginapi.NewClusterMutexMock(), nil)
				te.mockAPI.On("KVSetWithOptions", "mutex_drive_watch_notifications_userId1", mock.Anything, mock.Anything).Return(true, nil)
				mockGoogleDrive.EXPECT().ChangesList(context.Background(), "pageToken1").Return(&drive.ChangeList{NewStartPageToken: "", NextPageToken: "pageToken1", Changes: []*drive.Change{}}, nil).MaxTimes(5)
				mockKvStore.EXPECT().StoreWatchChannelData("userId1", *watchChannelData).Return(nil)
			},
		},
		"Ensure we don't send the user a notification if they have opened the file since the last change": {
			expectedStatusCode: http.StatusOK,
			envSetup: func(te *TestEnvironment) {
				watchChannelData := GetSampleWatchChannelData()
				mockKvStore.EXPECT().GetWatchChannelData("userId1").Return(watchChannelData, nil).MaxTimes(2)
				mockGoogleClient.EXPECT().NewDriveService(context.Background(), "userId1").Return(mockGoogleDrive, nil)
				mockCluster.EXPECT().NewMutex("drive_watch_notifications_userId1").Return(pluginapi.NewClusterMutexMock(), nil)
				te.mockAPI.On("KVSetWithOptions", "mutex_drive_watch_notifications_userId1", mock.Anything, mock.Anything).Return(true, nil)
				changeList := GetSampleChangeList()
				changeList.Changes[0].File.ViewedByMeTime = "2021-01-02T00:00:00.000Z"
				mockGoogleDrive.EXPECT().ChangesList(context.Background(), watchChannelData.PageToken).Return(changeList, nil).MaxTimes(1)
				watchChannelData = &model.WatchChannelData{
					ChannelID:  "channelId1",
					ResourceID: "resourceId1",
					MMUserID:   "userId1",
					Expiration: 0,
					Token:      "token1",
					PageToken:  changeList.NewStartPageToken,
				}
				mockKvStore.EXPECT().StoreWatchChannelData("userId1", *watchChannelData).Return(nil)
				mockGoogleClient.EXPECT().NewDriveActivityService(context.Background(), "userId1").Return(mockDriveActivity, nil)
				mockKvStore.EXPECT().StoreLastActivityForFile("userId1", "fileId1", "2021-01-02T00:00:00.000Z").Return(nil)
			},
		},
		"Ensure we only hit the drive activity api a maximum of 5 times": {
			expectedStatusCode: http.StatusOK,
			envSetup: func(te *TestEnvironment) {
				watchChannelData := GetSampleWatchChannelData()
				mockKvStore.EXPECT().GetWatchChannelData("userId1").Return(watchChannelData, nil).MaxTimes(2)
				mockGoogleClient.EXPECT().NewDriveService(context.Background(), "userId1").Return(mockGoogleDrive, nil)
				mockCluster.EXPECT().NewMutex("drive_watch_notifications_userId1").Return(pluginapi.NewClusterMutexMock(), nil)
				te.mockAPI.On("KVSetWithOptions", "mutex_drive_watch_notifications_userId1", mock.Anything, mock.Anything).Return(true, nil)
				changeList := GetSampleChangeList()
				mockGoogleDrive.EXPECT().ChangesList(context.Background(), watchChannelData.PageToken).Return(changeList, nil).MaxTimes(1)
				watchChannelData = &model.WatchChannelData{
					ChannelID:  "channelId1",
					ResourceID: "resourceId1",
					MMUserID:   "userId1",
					Expiration: 0,
					Token:      "token1",
					PageToken:  changeList.NewStartPageToken,
				}
				mockKvStore.EXPECT().StoreWatchChannelData("userId1", *watchChannelData).Return(nil)
				mockGoogleClient.EXPECT().NewDriveActivityService(context.Background(), "userId1").Return(mockDriveActivity, nil)
				mockKvStore.EXPECT().GetLastActivityForFile("userId1", changeList.Changes[0].File.Id).Return(changeList.Changes[0].File.ModifiedTime, nil)
				mockDriveActivity.EXPECT().Query(context.Background(), &driveactivity.QueryDriveActivityRequest{
					ItemName: fmt.Sprintf("items/%s", changeList.Changes[0].File.Id),
					Filter:   "time > \"" + changeList.Changes[0].File.ModifiedTime + "\"",
				}).Return(&driveactivity.QueryDriveActivityResponse{Activities: []*driveactivity.DriveActivity{}, NextPageToken: "newPage"}, nil).MaxTimes(1)
				mockDriveActivity.EXPECT().Query(context.Background(), &driveactivity.QueryDriveActivityRequest{
					ItemName:  fmt.Sprintf("items/%s", changeList.Changes[0].File.Id),
					Filter:    "time > \"" + changeList.Changes[0].File.ModifiedTime + "\"",
					PageToken: "newPage",
				}).Return(&driveactivity.QueryDriveActivityResponse{Activities: []*driveactivity.DriveActivity{}, NextPageToken: "newPage"}, nil).MaxTimes(4)
				mockKvStore.EXPECT().StoreLastActivityForFile("userId1", changeList.Changes[0].File.Id, changeList.Changes[0].File.ModifiedTime).Return(nil)
			},
		},
		"Send one bot DM if there are more than 6 activities in a file": {
			expectedStatusCode: http.StatusOK,
			envSetup: func(te *TestEnvironment) {
				watchChannelData := GetSampleWatchChannelData()
				mockKvStore.EXPECT().GetWatchChannelData("userId1").Return(watchChannelData, nil).MaxTimes(2)
				mockGoogleClient.EXPECT().NewDriveService(context.Background(), "userId1").Return(mockGoogleDrive, nil)
				mockCluster.EXPECT().NewMutex("drive_watch_notifications_userId1").Return(pluginapi.NewClusterMutexMock(), nil)
				te.mockAPI.On("KVSetWithOptions", "mutex_drive_watch_notifications_userId1", mock.Anything, mock.Anything).Return(true, nil)
				changeList := GetSampleChangeList()
				mockGoogleDrive.EXPECT().ChangesList(context.Background(), watchChannelData.PageToken).Return(changeList, nil).MaxTimes(1)
				watchChannelData = &model.WatchChannelData{
					ChannelID:  "channelId1",
					ResourceID: "resourceId1",
					MMUserID:   "userId1",
					Expiration: 0,
					Token:      "token1",
					PageToken:  changeList.NewStartPageToken,
				}
				mockKvStore.EXPECT().StoreWatchChannelData("userId1", *watchChannelData).Return(nil)
				mockGoogleClient.EXPECT().NewDriveActivityService(context.Background(), "userId1").Return(mockDriveActivity, nil)
				mockKvStore.EXPECT().GetLastActivityForFile("userId1", changeList.Changes[0].File.Id).Return(changeList.Changes[0].File.ModifiedTime, nil)
				mockDriveActivity.EXPECT().Query(context.Background(), &driveactivity.QueryDriveActivityRequest{
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
				mockKvStore.EXPECT().StoreLastActivityForFile("userId1", changeList.Changes[0].File.Id, changeList.Changes[0].File.ModifiedTime).Return(nil)
			},
		},
		"Send a notification for a permission change on a file": {
			expectedStatusCode: http.StatusOK,
			envSetup: func(te *TestEnvironment) {
				watchChannelData := GetSampleWatchChannelData()
				mockKvStore.EXPECT().GetWatchChannelData("userId1").Return(watchChannelData, nil).MaxTimes(2)
				mockGoogleClient.EXPECT().NewDriveService(context.Background(), "userId1").Return(mockGoogleDrive, nil)
				mockCluster.EXPECT().NewMutex("drive_watch_notifications_userId1").Return(pluginapi.NewClusterMutexMock(), nil)
				te.mockAPI.On("KVSetWithOptions", "mutex_drive_watch_notifications_userId1", mock.Anything, mock.Anything).Return(true, nil)
				changeList := GetSampleChangeList()
				mockGoogleDrive.EXPECT().ChangesList(context.Background(), watchChannelData.PageToken).Return(changeList, nil).MaxTimes(1)
				watchChannelData = &model.WatchChannelData{
					ChannelID:  "channelId1",
					ResourceID: "resourceId1",
					MMUserID:   "userId1",
					Expiration: 0,
					Token:      "token1",
					PageToken:  changeList.NewStartPageToken,
				}
				mockKvStore.EXPECT().StoreWatchChannelData("userId1", *watchChannelData).Return(nil)
				mockGoogleClient.EXPECT().NewDriveActivityService(context.Background(), "userId1").Return(mockDriveActivity, nil)
				mockKvStore.EXPECT().GetLastActivityForFile("userId1", changeList.Changes[0].File.Id).Return(changeList.Changes[0].File.ModifiedTime, nil)
				mockDriveActivity.EXPECT().Query(context.Background(), &driveactivity.QueryDriveActivityRequest{
					ItemName: fmt.Sprintf("items/%s", changeList.Changes[0].File.Id),
					Filter:   "time > \"" + changeList.Changes[0].File.ModifiedTime + "\"",
				}).Return(GetSampleDriveactivityPermissionResponse(), nil).MaxTimes(1)
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
				mockKvStore.EXPECT().StoreLastActivityForFile("userId1", changeList.Changes[0].File.Id, changeList.Changes[0].File.ModifiedTime).Return(nil)
			},
		},
		"Send a notification for a comment on a file": {
			expectedStatusCode: http.StatusOK,
			envSetup: func(te *TestEnvironment) {
				watchChannelData := GetSampleWatchChannelData()
				mockKvStore.EXPECT().GetWatchChannelData("userId1").Return(watchChannelData, nil).MaxTimes(2)
				mockGoogleClient.EXPECT().NewDriveService(context.Background(), "userId1").Return(mockGoogleDrive, nil)
				mockCluster.EXPECT().NewMutex("drive_watch_notifications_userId1").Return(pluginapi.NewClusterMutexMock(), nil)
				te.mockAPI.On("KVSetWithOptions", "mutex_drive_watch_notifications_userId1", mock.Anything, mock.Anything).Return(true, nil)
				changeList := GetSampleChangeList()
				mockGoogleDrive.EXPECT().ChangesList(context.Background(), watchChannelData.PageToken).Return(changeList, nil).MaxTimes(1)
				watchChannelData = &model.WatchChannelData{
					ChannelID:  "channelId1",
					ResourceID: "resourceId1",
					MMUserID:   "userId1",
					Expiration: 0,
					Token:      "token1",
					PageToken:  changeList.NewStartPageToken,
				}
				mockKvStore.EXPECT().StoreWatchChannelData("userId1", *watchChannelData).Return(nil)
				mockGoogleClient.EXPECT().NewDriveActivityService(context.Background(), "userId1").Return(mockDriveActivity, nil)
				file := changeList.Changes[0].File
				mockKvStore.EXPECT().GetLastActivityForFile("userId1", file.Id).Return(file.ModifiedTime, nil)
				activityResponse := GetSampleDriveactivityCommentResponse()
				mockDriveActivity.EXPECT().Query(context.Background(), &driveactivity.QueryDriveActivityRequest{
					ItemName: fmt.Sprintf("items/%s", file.Id),
					Filter:   "time > \"" + file.ModifiedTime + "\"",
				}).Return(activityResponse, nil).MaxTimes(1)
				commentID := activityResponse.Activities[0].Targets[0].FileComment.LegacyCommentId
				comment := GetSampleComment(commentID)
				mockGoogleDrive.EXPECT().GetComments(context.Background(), file.Id, commentID).Return(comment, nil)
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
				mockKvStore.EXPECT().StoreLastActivityForFile("userId1", changeList.Changes[0].File.Id, changeList.Changes[0].File.ModifiedTime).Return(nil)
			},
		},
	} {
		t.Run(name, func(t *testing.T) {
			assert := assert.New(t)
			te := SetupTestEnvironment(t)
			defer te.Cleanup(t)

			te.plugin.KVStore = mockKvStore
			te.plugin.GoogleClient = mockGoogleClient
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
	mockKvStore, mockGoogleClient, mockGoogleDrive, _, mockGoogleDocs, mockGoogleSheets, mockGoogleSlides, _, _ := GetMockSetup(t)

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
				mockGoogleClient.EXPECT().NewDocsService(ctx, "userId1").Return(mockGoogleDocs, nil)
				doc := GetSampleDoc()
				mockGoogleDocs.EXPECT().Create(ctx, &docs.Document{
					Title: "file name",
				}).Return(doc, nil)
				mockGoogleClient.EXPECT().NewDriveService(ctx, "userId1").Return(mockGoogleDrive, nil).Times(2)
				te.mockAPI.On("GetConfig").Return(nil)
				mockGoogleDrive.EXPECT().CreatePermission(ctx, doc.DocumentId, &drive.Permission{
					Role: "commenter",
					Type: "anyone",
				}).Return(&drive.Permission{}, nil).MaxTimes(1)
				file := GetSampleFile(doc.DocumentId)
				mockGoogleDrive.EXPECT().GetFile(ctx, doc.DocumentId).Return(file, nil)
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
				mockGoogleClient.EXPECT().NewSheetsService(ctx, "userId1").Return(mockGoogleSheets, nil)
				sheet := GetSampleSheet()
				mockGoogleSheets.EXPECT().Create(ctx, &sheets.Spreadsheet{
					Properties: &sheets.SpreadsheetProperties{
						Title: "file name",
					},
				}).Return(sheet, nil)
				mockGoogleClient.EXPECT().NewDriveService(ctx, "userId1").Return(mockGoogleDrive, nil).Times(2)
				te.mockAPI.On("GetConfig").Return(nil)
				mockGoogleDrive.EXPECT().CreatePermission(ctx, sheet.SpreadsheetId, &drive.Permission{
					Role: "writer",
					Type: "anyone",
				}).Return(&drive.Permission{}, nil).MaxTimes(1)
				file := GetSampleFile(sheet.SpreadsheetId)
				mockGoogleDrive.EXPECT().GetFile(ctx, sheet.SpreadsheetId).Return(file, nil)
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
				mockGoogleClient.EXPECT().NewSlidesService(ctx, "userId1").Return(mockGoogleSlides, nil)
				presentation := GetSamplePresentation()
				mockGoogleSlides.EXPECT().Create(ctx, &slides.Presentation{
					Title: "file name",
				}).Return(presentation, nil)
				mockGoogleClient.EXPECT().NewDriveService(ctx, "userId1").Return(mockGoogleDrive, nil).Times(2)
				te.mockAPI.On("GetConfig").Return(nil)
				mockGoogleDrive.EXPECT().CreatePermission(ctx, presentation.PresentationId, &drive.Permission{
					Role: "reader",
					Type: "anyone",
				}).Return(&drive.Permission{}, nil).MaxTimes(1)
				file := GetSampleFile(presentation.PresentationId)
				mockGoogleDrive.EXPECT().GetFile(ctx, presentation.PresentationId).Return(file, nil)
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
				mockGoogleClient.EXPECT().NewDocsService(ctx, "userId1").Return(mockGoogleDocs, nil)
				doc := GetSampleDoc()
				mockGoogleDocs.EXPECT().Create(ctx, &docs.Document{
					Title: "file name",
				}).Return(doc, nil)
				mockGoogleClient.EXPECT().NewDriveService(ctx, "userId1").Return(mockGoogleDrive, nil).Times(2)
				te.mockAPI.On("GetConfig").Return(nil)
				file := GetSampleFile(doc.DocumentId)
				mockGoogleDrive.EXPECT().GetFile(ctx, doc.DocumentId).Return(file, nil)
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
				mockGoogleClient.EXPECT().NewDocsService(ctx, "userId1").Return(mockGoogleDocs, nil)
				doc := GetSampleDoc()
				mockGoogleDocs.EXPECT().Create(ctx, &docs.Document{
					Title: "file name",
				}).Return(doc, nil)
				mockGoogleClient.EXPECT().NewDriveService(ctx, "userId1").Return(mockGoogleDrive, nil).Times(2)
				te.mockAPI.On("GetConfig").Return(nil)
				mockGoogleDrive.EXPECT().CreatePermission(ctx, doc.DocumentId, &drive.Permission{
					Role: "commenter",
					Type: "anyone",
				}).Return(&drive.Permission{}, nil).MaxTimes(1)
				file := GetSampleFile(doc.DocumentId)
				mockGoogleDrive.EXPECT().GetFile(ctx, doc.DocumentId).Return(file, nil)
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
				mockGoogleClient.EXPECT().NewDocsService(ctx, "userId1").Return(mockGoogleDocs, nil)
				doc := GetSampleDoc()
				mockGoogleDocs.EXPECT().Create(ctx, &docs.Document{
					Title: "file name",
				}).Return(doc, nil)
				mockGoogleClient.EXPECT().NewDriveService(ctx, "userId1").Return(mockGoogleDrive, nil).Times(2)
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

				mockGoogleDrive.EXPECT().CreatePermission(ctx, doc.DocumentId, &drive.Permission{
					Role:         "commenter",
					EmailAddress: users[0].Email,
					Type:         "user",
				}).Return(&drive.Permission{}, nil).MaxTimes(1)
				mockGoogleDrive.EXPECT().CreatePermission(ctx, doc.DocumentId, &drive.Permission{
					Role:         "commenter",
					EmailAddress: users[1].Email,
					Type:         "user",
				}).Return(&drive.Permission{}, nil).MaxTimes(1)
				file := GetSampleFile(doc.DocumentId)
				mockGoogleDrive.EXPECT().GetFile(ctx, doc.DocumentId).Return(file, nil)
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

			te.plugin.KVStore = mockKvStore
			te.plugin.GoogleClient = mockGoogleClient
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
	mockKvStore, mockGoogleClient, mockGoogleDrive, _, _, _, _, _, _ := GetMockSetup(t)

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
				mockGoogleClient.EXPECT().NewDriveService(ctx, "userId1").Return(mockGoogleDrive, nil)
				mockGoogleDrive.EXPECT().CreateFile(ctx, &drive.File{Name: "file name"}, []byte{}).Return(nil, nil)
				te.mockAPI.On("SendEphemeralPost", "userId1", mock.Anything).Return(nil)
			},
		},
	} {
		t.Run(name, func(t *testing.T) {
			assert := assert.New(t)
			te := SetupTestEnvironment(t)
			defer te.Cleanup(t)

			te.plugin.KVStore = mockKvStore
			te.plugin.GoogleClient = mockGoogleClient
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
	mockKvStore, mockGoogleClient, mockGoogleDrive, _, _, _, _, _, _ := GetMockSetup(t)

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
				mockGoogleClient.EXPECT().NewDriveService(ctx, "userId1").Return(mockGoogleDrive, nil)
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
				mockGoogleDrive.EXPECT().CreateFile(ctx, &drive.File{Name: "file name"}, []byte{}).Return(nil, nil).Times(2)
				te.mockAPI.On("SendEphemeralPost", "userId1", mock.Anything).Return(nil)
			},
		},
	} {
		t.Run(name, func(t *testing.T) {
			assert := assert.New(t)
			te := SetupTestEnvironment(t)
			defer te.Cleanup(t)

			te.plugin.KVStore = mockKvStore
			te.plugin.GoogleClient = mockGoogleClient
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
