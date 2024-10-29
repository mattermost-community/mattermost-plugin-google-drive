package plugin

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"google.golang.org/api/drive/v3"
	"google.golang.org/api/driveactivity/v2"

	"github.com/mattermost-community/mattermost-plugin-google-drive/server/plugin/model"
	"github.com/mattermost-community/mattermost-plugin-google-drive/server/plugin/pluginapi"
)

func TestServeHTTP(t *testing.T) {
	t.Run("No UserId provided", func(t *testing.T) {
		assert := assert.New(t)

		mockKvStore, _, _, _, _, _ := GetMockSetup(t)
		te := SetupTestEnvironment(t)
		defer te.Cleanup(t)

		te.plugin.KVStore = mockKvStore
		te.plugin.initializeAPI()

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

		w := httptest.NewRecorder()
		r := httptest.NewRequest(http.MethodPost, "/webhook", nil)
		r.Header.Set("X-Goog-Resource-State", "change")
		r.Header.Set("X-Goog-Channel-Token", "token")
		te.plugin.handleDriveWatchNotifications(nil, w, r)

		result := w.Result()
		require.NotNil(t, result)
		defer result.Body.Close()

		assert.Equal(http.StatusBadRequest, result.StatusCode)
	})
	t.Run("Invalid Google token", func(t *testing.T) {
		assert := assert.New(t)

		mockKvStore, mockGoogleClient, _, _, _, _ := GetMockSetup(t)
		te := SetupTestEnvironment(t)
		defer te.Cleanup(t)

		te.plugin.KVStore = mockKvStore
		te.plugin.GoogleClient = mockGoogleClient
		te.plugin.initializeAPI()

		watchChannelData := &model.WatchChannelData{
			ChannelID:  "channelId1",
			ResourceID: "resourceId1",
			MMUserID:   "userId1",
			Expiration: 0,
			Token:      "token1",
			PageToken:  "pageToken1",
		}
		mockKvStore.EXPECT().GetWatchChannelData("userId1").Return(watchChannelData, nil)
		te.mockAPI.On("LogError", mock.AnythingOfType("string"), mock.AnythingOfType("string"), mock.AnythingOfType("string")).Maybe()

		w := httptest.NewRecorder()
		r := httptest.NewRequest(http.MethodPost, "/webhook?userID=userId1", nil)
		r.Header.Set("X-Goog-Resource-State", "change")
		r.Header.Set("X-Goog-Channel-Token", "token")
		te.plugin.handleDriveWatchNotifications(nil, w, r)

		result := w.Result()
		require.NotNil(t, result)
		defer result.Body.Close()

		assert.Equal(http.StatusBadRequest, result.StatusCode)
	})

	t.Run("Happy path", func(t *testing.T) {
		assert := assert.New(t)

		mockKvStore, mockGoogleClient, mockGoogleDrive, mockDriveActivity, _, mockCluster := GetMockSetup(t)
		te := SetupTestEnvironment(t)
		defer te.Cleanup(t)

		te.plugin.KVStore = mockKvStore
		te.plugin.GoogleClient = mockGoogleClient
		te.plugin.initializeAPI()

		watchChannelData := &model.WatchChannelData{
			ChannelID:  "channelId1",
			ResourceID: "resourceId1",
			MMUserID:   "userId1",
			Expiration: 0,
			Token:      "token1",
			PageToken:  "pageToken1",
		}
		mockKvStore.EXPECT().GetWatchChannelData("userId1").Return(watchChannelData, nil).MaxTimes(2)
		mockGoogleClient.EXPECT().NewDriveService(context.Background(), "userId1").Return(mockGoogleDrive, nil)
		mockGoogleClient.EXPECT().NewDriveActivityService(context.Background(), "userId1").Return(mockDriveActivity, nil)
		te.mockAPI.On("LogError", mock.AnythingOfType("string"), mock.AnythingOfType("string"), mock.AnythingOfType("string")).Maybe()
		mockCluster.EXPECT().NewMutex("drive_watch_notifications_userId1").Return(pluginapi.NewClusterMutexMock(), nil)
		te.mockAPI.On("KVSetWithOptions", "mutex_drive_watch_notifications_userId1", mock.Anything, mock.Anything).Return(true, nil)
		mockGoogleDrive.EXPECT().ChangesList(context.Background(), "pageToken1").Return(&drive.ChangeList{
			Changes: []*drive.Change{
				{
					FileId: "fileId1",
					Kind:   "drive#change",
					File: &drive.File{
						Id:             "fileId1",
						ViewedByMeTime: "2020-01-01T00:00:00.000Z",
						ModifiedTime:   "2021-01-01T00:00:00.000Z",
					},
					DriveId: "driveId1",
					Removed: false,
					Time:    "2021-01-01T00:00:00.000Z",
				},
			},
			NewStartPageToken: "newPageToken2",
			NextPageToken:     "",
		}, nil)
		mockKvStore.EXPECT().GetLastActivityForFile("userId1", "fileId1").Return("2020-01-01T00:00:00.000Z", nil)
		watchChannelData = &model.WatchChannelData{
			ChannelID:  "channelId1",
			ResourceID: "resourceId1",
			MMUserID:   "userId1",
			Expiration: 0,
			Token:      "token1",
			PageToken:  "newPageToken2",
		}
		mockKvStore.EXPECT().StoreWatchChannelData("userId1", *watchChannelData).Return(nil)
		mockDriveActivity.EXPECT().Query(context.Background(), &driveactivity.QueryDriveActivityRequest{
			ItemName: fmt.Sprintf("items/%s", "fileId1"),
			Filter:   "time > \"2020-01-01T00:00:00.000Z\"",
		}).Return(&driveactivity.QueryDriveActivityResponse{}, nil)

		w := httptest.NewRecorder()
		r := httptest.NewRequest(http.MethodPost, "/webhook?userID=userId1", nil)
		r.Header.Set("X-Goog-Resource-State", "change")
		r.Header.Set("X-Goog-Channel-Token", "token1")
		ctx := &Context{
			Ctx:    context.Background(),
			UserID: "userId1",
			Log:    nil,
		}
		te.plugin.handleDriveWatchNotifications(ctx, w, r)

		result := w.Result()
		require.NotNil(t, result)
		defer result.Body.Close()

		assert.Equal(http.StatusOK, result.StatusCode)
	})
}
