package plugin

import (
	"testing"

	"github.com/golang/mock/gomock"
	mock_pluginapi "github.com/mattermost-community/mattermost-plugin-google-drive/server/plugin/pluginapi/mocks"
	"github.com/mattermost/mattermost/server/public/plugin/plugintest"
	"github.com/mattermost/mattermost/server/public/pluginapi"

	mock_google "github.com/mattermost-community/mattermost-plugin-google-drive/server/plugin/google/mocks"

	mock_store "github.com/mattermost-community/mattermost-plugin-google-drive/server/plugin/kvstore/mocks"
)

type TestEnvironment struct {
	plugin  *Plugin
	mockAPI *plugintest.API
}

func SetupTestEnvironment(t *testing.T) *TestEnvironment {
	p := Plugin{}

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
	e.plugin.Client = pluginapi.NewClient(e.plugin.API, e.plugin.Driver)
}

// revive:disable-next-line:unexported-return
func GetMockSetup(t *testing.T) (*mock_store.MockKVStore, *mock_google.MockClientInterface, *mock_google.MockDriveInterface, *mock_google.MockDriveActivityInterface, *mock_pluginapi.MockClusterMutex, *mock_pluginapi.MockCluster) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockKvStore := mock_store.NewMockKVStore(ctrl)
	mockGoogleClient := mock_google.NewMockClientInterface(ctrl)
	mockGoogleDrive := mock_google.NewMockDriveInterface(ctrl)
	mockDriveActivity := mock_google.NewMockDriveActivityInterface(ctrl)
	mockClusterMutex := mock_pluginapi.NewMockClusterMutex(ctrl)
	mockCluster := mock_pluginapi.NewMockCluster(ctrl)

	return mockKvStore, mockGoogleClient, mockGoogleDrive, mockDriveActivity, mockClusterMutex, mockCluster
}
