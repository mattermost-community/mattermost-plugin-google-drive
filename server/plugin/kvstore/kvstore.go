package kvstore

import "github.com/mattermost-community/mattermost-plugin-google-drive/server/plugin/model"

type KVStore interface {
	StoreWatchChannelData(userID string, watchChannelData model.WatchChannelData) error
	GetWatchChannelData(userID string) (*model.WatchChannelData, error)
	ListWatchChannelDataKeys(page, perPage int) ([]string, error)
	DeleteWatchChannelData(userID string) error

	StoreLastActivityForFile(userID, fileID, activityTime string) error
	GetLastActivityForFile(userID, fileID string) (string, error)

	StoreOAuthStateToken(key, value string) error
	GetOAuthStateToken(key string) ([]byte, error)
	DeleteOAuthStateToken(key string) error

	StoreGoogleUserToken(userID, encryptedToken string) error
	GetGoogleUserToken(userID string) ([]byte, error)
	DeleteGoogleUserToken(userID string) error
}
