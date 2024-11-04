package plugin

import (
	"context"
	"testing"
	"time"

	"github.com/golang/mock/gomock"
	mattermostModel "github.com/mattermost/mattermost/server/public/model"
	"google.golang.org/api/drive/v3"

	mock_google "github.com/mattermost-community/mattermost-plugin-google-drive/server/plugin/google/mocks"
	mock_store "github.com/mattermost-community/mattermost-plugin-google-drive/server/plugin/kvstore/mocks"
	"github.com/mattermost-community/mattermost-plugin-google-drive/server/plugin/model"
)

func TestRefreshDriveWatchChannels(t *testing.T) {
	te := SetupTestEnvironment(t)
	defer te.Cleanup(t)

	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockKVStore := mock_store.NewMockKVStore(ctrl)
	mockGoogleClient := mock_google.NewMockClientInterface(ctrl)
	mockGoogleDrive := mock_google.NewMockDriveInterface(ctrl)

	p := &Plugin{
		KVStore:      mockKVStore,
		Client:       te.plugin.Client,
		GoogleClient: mockGoogleClient,
	}

	t.Run("processes channels correctly", func(t *testing.T) {
		channel1 := &model.WatchChannelData{MMUserID: "userId1", Expiration: time.Now().Add(23 * time.Hour).Unix(), ChannelID: "channel1", ResourceID: "resource1"}
		channel2 := &model.WatchChannelData{MMUserID: "userId2", Expiration: time.Now().Add(23 * time.Hour).Unix(), ChannelID: "channel2", ResourceID: "resource2"}
		siteURL := "http://localhost"
		te.mockAPI.On("GetConfig").Return(&mattermostModel.Config{ServiceSettings: mattermostModel.ServiceSettings{SiteURL: &siteURL}})

		mockKVStore.EXPECT().ListWatchChannelDataKeys(gomock.Any(), gomock.Any()).Return([]string{"key1", "key2"}, nil).Times(1)
		mockKVStore.EXPECT().ListWatchChannelDataKeys(gomock.Any(), gomock.Any()).Return([]string{}, nil).Times(1)

		mockKVStore.EXPECT().GetWatchChannelDataUsingKey("key1").Return(channel1, nil).Times(1)
		mockKVStore.EXPECT().GetWatchChannelDataUsingKey("key2").Return(channel2, nil).Times(1)

		mockKVStore.EXPECT().GetWatchChannelData("userId1").Return(channel1, nil).Times(1)
		mockKVStore.EXPECT().GetWatchChannelData("userId2").Return(channel2, nil).Times(1)
		mockGoogleClient.EXPECT().NewDriveService(context.Background(), "userId1").Return(mockGoogleDrive, nil).Times(2)
		mockGoogleClient.EXPECT().NewDriveService(context.Background(), "userId2").Return(mockGoogleDrive, nil).Times(2)
		mockKVStore.EXPECT().DeleteWatchChannelData("userId1").Return(nil).Times(1)
		mockKVStore.EXPECT().DeleteWatchChannelData("userId2").Return(nil).Times(1)
		mockGoogleDrive.EXPECT().StopChannel(context.Background(), &drive.Channel{
			Id:         channel1.ChannelID,
			ResourceId: channel1.ResourceID,
		})
		mockGoogleDrive.EXPECT().StopChannel(context.Background(), &drive.Channel{
			Id:         channel2.ChannelID,
			ResourceId: channel2.ResourceID,
		})
		ctx := context.Background()
		startPageToken1 := &drive.StartPageToken{
			StartPageToken: "newPageToken1",
		}
		startPageToken2 := &drive.StartPageToken{
			StartPageToken: "newPageToken2",
		}
		mockGoogleDrive.EXPECT().GetStartPageToken(ctx).Return(startPageToken1, nil).Times(1)
		mockGoogleDrive.EXPECT().GetStartPageToken(ctx).Return(startPageToken2, nil).Times(1)

		channel1Data := model.WatchChannelData{
			ChannelID:  "channel1Id",
			ResourceID: channel1.ResourceID,
			Expiration: channel1.Expiration,
			Token:      channel1.Token,
			MMUserID:   "userId1",
			PageToken:  startPageToken1.StartPageToken,
		}
		channel2Data := model.WatchChannelData{
			ChannelID:  "channel2Id",
			ResourceID: channel2.ResourceID,
			Expiration: channel2.Expiration,
			Token:      channel2.Token,
			MMUserID:   "userId2",
			PageToken:  startPageToken2.StartPageToken,
		}

		mockGoogleDrive.EXPECT().WatchChannel(ctx, startPageToken1, gomock.Any()).Return(&drive.Channel{Id: "channel1Id", ResourceId: channel1.ResourceID, Expiration: channel1.Expiration, Token: channel1.Token}, nil).Times(1)
		mockGoogleDrive.EXPECT().WatchChannel(ctx, startPageToken2, gomock.Any()).Return(&drive.Channel{Id: "channel2Id", ResourceId: channel2.ResourceID, Expiration: channel2.Expiration, Token: channel2.Token}, nil).Times(1)
		mockKVStore.EXPECT().StoreWatchChannelData("userId1", channel1Data).Return(nil).Times(1)
		mockKVStore.EXPECT().StoreWatchChannelData("userId2", channel2Data).Return(nil).Times(1)

		p.refreshDriveWatchChannels()
	})
}
