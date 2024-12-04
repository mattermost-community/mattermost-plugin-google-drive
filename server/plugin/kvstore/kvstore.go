package kvstore

import "github.com/mattermost-community/mattermost-plugin-google-drive/server/plugin/model"

type KVStore interface {
	StoreWatchChannelData(userID string, watchChannelData model.WatchChannelData) error
	GetWatchChannelData(userID string) (*model.WatchChannelData, error)
	GetWatchChannelDataUsingKey(key string) (*model.WatchChannelData, error)
	ListWatchChannelDataKeys(page, perPage int) ([]string, error)
	DeleteWatchChannelData(userID string) error

	StoreLastActivityForFile(userID, fileID, activityTime string) error
	GetLastActivityForFile(userID, fileID string) (string, error)

	StoreOAuthStateToken(key, value string) error
	GetOAuthStateToken(key string) ([]byte, error)
	DeleteOAuthStateToken(key string) error

	StoreGoogleUserToken(userID string, encryptedToken []byte) error
	GetGoogleUserToken(userID string) ([]byte, error)
	DeleteGoogleUserToken(userID string) error

	StoreUserRateLimitExceeded(serviceType string, userID string) error
	GetUserRateLimitExceeded(serviceType string, userID string) (bool, error)

	StoreProjectRateLimitExceeded(serviceType string) error
	GetProjectRateLimitExceeded(serviceType string) (bool, error)
}
