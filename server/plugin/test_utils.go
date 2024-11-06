package plugin

import (
	"testing"

	"github.com/golang/mock/gomock"
	"github.com/mattermost/mattermost/server/public/plugin/plugintest"
	"github.com/mattermost/mattermost/server/public/pluginapi"
	"google.golang.org/api/docs/v1"
	"google.golang.org/api/drive/v3"
	"google.golang.org/api/driveactivity/v2"
	"google.golang.org/api/sheets/v4"
	"google.golang.org/api/slides/v1"

	"github.com/mattermost-community/mattermost-plugin-google-drive/server/plugin/config"
	"github.com/mattermost-community/mattermost-plugin-google-drive/server/plugin/model"

	mock_pluginapi "github.com/mattermost-community/mattermost-plugin-google-drive/server/plugin/pluginapi/mocks"

	mock_google "github.com/mattermost-community/mattermost-plugin-google-drive/server/plugin/google/mocks"

	mock_store "github.com/mattermost-community/mattermost-plugin-google-drive/server/plugin/kvstore/mocks"

	mock_oauth2 "github.com/mattermost-community/mattermost-plugin-google-drive/server/plugin/oauth2/mocks"
)

type TestEnvironment struct {
	plugin  *Plugin
	mockAPI *plugintest.API
}

func SetupTestEnvironment(t *testing.T) *TestEnvironment {

	p := Plugin{
		BotUserID:   "bot_user_id",
		oauthBroker: NewOAuthBroker(func(event OAuthCompleteEvent) {}),
	}

	e := &TestEnvironment{
		plugin: &p,
	}
	e.ResetMocks(t)

	return e
}

func (e *TestEnvironment) Cleanup(t *testing.T) {
	t.Helper()
	e.mockAPI.AssertExpectations(t)
}

func (e *TestEnvironment) ResetMocks(t *testing.T) {
	e.mockAPI = &plugintest.API{}
	e.plugin.SetAPI(e.mockAPI)
	e.plugin.Client = pluginapi.NewClient(e.mockAPI, e.plugin.Driver)
	e.plugin.configuration = &config.Configuration{
		QueriesPerMinute:        60,
		BurstSize:               10,
		GoogleOAuthClientID:     "randomstring.apps.googleusercontent.com",
		GoogleOAuthClientSecret: "googleoauthclientsecret",
		EncryptionKey:           "encryptionkey123",
	}
}

// revive:disable-next-line:unexported-return
func GetMockSetup(t *testing.T) (*mock_store.MockKVStore, *mock_google.MockClientInterface, *mock_google.MockDriveInterface, *mock_google.MockDriveActivityInterface, *mock_google.MockDocsInterface, *mock_google.MockSheetsInterface, *mock_google.MockSlidesInterface, *mock_pluginapi.MockClusterMutex, *mock_pluginapi.MockCluster, *mock_oauth2.MockConfigInterface, *mock_pluginapi.MockTracker) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockKvStore := mock_store.NewMockKVStore(ctrl)
	mockGoogleClient := mock_google.NewMockClientInterface(ctrl)
	mockGoogleDrive := mock_google.NewMockDriveInterface(ctrl)
	mockDriveActivity := mock_google.NewMockDriveActivityInterface(ctrl)
	mockGoogleDocs := mock_google.NewMockDocsInterface(ctrl)
	mockGoogleSheets := mock_google.NewMockSheetsInterface(ctrl)
	mockGoogleSlides := mock_google.NewMockSlidesInterface(ctrl)
	mockClusterMutex := mock_pluginapi.NewMockClusterMutex(ctrl)
	mockCluster := mock_pluginapi.NewMockCluster(ctrl)
	mockOAuth2 := mock_oauth2.NewMockConfigInterface(ctrl)
	mockTelemetry := mock_pluginapi.NewMockTracker(ctrl)

	return mockKvStore, mockGoogleClient, mockGoogleDrive, mockDriveActivity, mockGoogleDocs, mockGoogleSheets, mockGoogleSlides, mockClusterMutex, mockCluster, mockOAuth2, mockTelemetry
}

func GetSampleChangeList() *drive.ChangeList {
	return &drive.ChangeList{
		Changes: []*drive.Change{
			{
				FileId:  "fileId1",
				Kind:    "drive#change",
				File:    GetSampleFile("fileId1"),
				DriveId: "driveId1",
				Removed: false,
				Time:    "2021-01-01T00:00:00.000Z",
			},
		},
		NewStartPageToken: "newPageToken2",
		NextPageToken:     "",
	}
}

func GetSampleDriveactivityCommentResponse() *driveactivity.QueryDriveActivityResponse {
	return &driveactivity.QueryDriveActivityResponse{
		Activities: []*driveactivity.DriveActivity{
			{
				PrimaryActionDetail: &driveactivity.ActionDetail{
					Comment: &driveactivity.Comment{
						Post: &driveactivity.Post{
							Subtype: "ADDED",
						},
					},
				},
				Actors: []*driveactivity.Actor{
					{
						User: &driveactivity.User{
							KnownUser: &driveactivity.KnownUser{
								IsCurrentUser: false,
							},
						},
					},
				},
				Targets: []*driveactivity.Target{
					{
						FileComment: &driveactivity.FileComment{
							LegacyCommentId:    "commentId1",
							LegacyDiscussionId: "commentId1",
						},
					},
				},
			},
		},
		NextPageToken: "",
	}
}

func GetSampleDriveactivityPermissionResponse() *driveactivity.QueryDriveActivityResponse {
	return &driveactivity.QueryDriveActivityResponse{
		Activities: []*driveactivity.DriveActivity{
			{
				PrimaryActionDetail: &driveactivity.ActionDetail{
					PermissionChange: &driveactivity.PermissionChange{},
				},
				Actors: []*driveactivity.Actor{
					{
						User: &driveactivity.User{
							KnownUser: &driveactivity.KnownUser{
								IsCurrentUser: false,
							},
						},
					},
				},
				Targets: []*driveactivity.Target{},
			},
		},
		NextPageToken: "",
	}
}

func GetSampleComment(commentID string) *drive.Comment {
	return &drive.Comment{
		Content: "comment1",
		Id:      commentID,
		Author: &drive.User{
			DisplayName: "author1",
		},
	}
}

func GetSampleWatchChannelData() *model.WatchChannelData {
	return &model.WatchChannelData{
		ChannelID:  "channelId1",
		ResourceID: "resourceId1",
		MMUserID:   "userId1",
		Expiration: 0,
		Token:      "token1",
		PageToken:  "pageToken1",
	}
}

func GetSampleDoc() *docs.Document {
	return &docs.Document{
		Title:      "doc1",
		DocumentId: "docId1",
	}
}

func GetSampleSheet() *sheets.Spreadsheet {
	return &sheets.Spreadsheet{
		Properties: &sheets.SpreadsheetProperties{
			Title: "sheet1",
		},
		SpreadsheetId: "sheetId1",
	}
}

func GetSamplePresentation() *slides.Presentation {
	return &slides.Presentation{
		Title:          "presentation1",
		PresentationId: "presentationId1",
	}
}

func GetSampleFile(fileID string) *drive.File {
	return &drive.File{
		Id:             fileID,
		ViewedByMeTime: "2020-01-01T00:00:00.000Z",
		ModifiedTime:   "2021-01-01T00:00:00.000Z",
		CreatedTime:    "2021-01-01T00:00:00.000Z",
		SharingUser:    &drive.User{},
		WebViewLink:    "https://drive.google.com/file/d/fileId1/view",
		Name:           "file1",
		IconLink:       "https://drive.google.com/file/d/fileId1/view/icon",
		Owners: []*drive.User{
			{
				DisplayName: "owner1",
				PhotoLink:   "https://drive.google.com/file/d/fileId1/view/photo",
			},
		},
	}
}
