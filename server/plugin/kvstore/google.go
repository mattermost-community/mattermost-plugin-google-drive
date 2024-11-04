package kvstore

import (
	"fmt"
	"time"

	"github.com/pkg/errors"

	"github.com/mattermost-community/mattermost-plugin-google-drive/server/plugin/model"

	"github.com/mattermost/mattermost/server/public/pluginapi"
)

type Impl struct {
	client *pluginapi.Client
}

func NewKVStore(client *pluginapi.Client) KVStore {
	return Impl{
		client: client,
	}
}

func getWatchChannelDataKey(userID string) string {
	return fmt.Sprintf("drive_change_channels-%s", userID)
}

func getUserTokenKey(userID string) string {
	return fmt.Sprintf("%s_token", userID)
}

func getLastActivityKey(userID, fileID string) string {
	return fmt.Sprintf("last_activity-%s-%s", userID, fileID)
}

func getUserRateLimitKey(serviceType string, userID string) string {
	return fmt.Sprintf("user-rate_limited-%s-%s", serviceType, userID)
}

func getProjectRateLimitKey(serviceType string) string {
	return fmt.Sprintf("user-rate_limited-%s", serviceType)
}

func (kv Impl) GetWatchChannelData(userID string) (*model.WatchChannelData, error) {
	var watchChannelData model.WatchChannelData

	err := kv.client.KV.Get(getWatchChannelDataKey(userID), &watchChannelData)
	if err != nil {
		return nil, errors.Wrap(err, "failed to get watch channel data")
	}
	return &watchChannelData, nil
}

func (kv Impl) GetWatchChannelDataUsingKey(key string) (*model.WatchChannelData, error) {
	var watchChannelData model.WatchChannelData

	err := kv.client.KV.Get(key, &watchChannelData)
	if err != nil {
		return nil, errors.Wrap(err, "failed to get watch channel data")
	}
	return &watchChannelData, nil
}

func (kv Impl) ListWatchChannelDataKeys(page, perPage int) ([]string, error) {
	watchChannelKey := getWatchChannelDataKey("")
	keys, err := kv.client.KV.ListKeys(page, perPage, pluginapi.WithPrefix(watchChannelKey))
	if err != nil {
		return nil, errors.Wrap(err, "failed to list watch channel data keys")
	}
	return keys, nil
}

func (kv Impl) StoreWatchChannelData(userID string, watchChannelData model.WatchChannelData) error {
	saved, err := kv.client.KV.Set(getWatchChannelDataKey(userID), watchChannelData)
	if !saved && err != nil {
		return errors.Wrap(err, "database error occurred when trying to save watch channel data")
	} else if !saved && err == nil {
		return errors.New("Failed to set watch channel data")
	}
	return nil
}

func (kv Impl) DeleteWatchChannelData(userID string) error {
	err := kv.client.KV.Delete(getWatchChannelDataKey(userID))
	if err != nil {
		return errors.Wrap(err, "failed to delete watch channel data")
	}
	return nil
}

func (kv Impl) StoreLastActivityForFile(userID, fileID, activityTime string) error {
	key := getLastActivityKey(userID, fileID)
	saved, err := kv.client.KV.Set(key, activityTime)
	if !saved && err != nil {
		return errors.Wrap(err, "database error occurred when trying to save last activity for file")
	} else if !saved && err == nil {
		return errors.New("Failed to save last activity for file")
	}
	return nil
}

func (kv Impl) GetLastActivityForFile(userID, fileID string) (string, error) {
	var activityTime string

	err := kv.client.KV.Get(getLastActivityKey(userID, fileID), &activityTime)
	if err != nil {
		return "", errors.Wrap(err, "failed to get last activity for file")
	}
	return activityTime, nil
}

func (kv Impl) StoreOAuthStateToken(key, value string) error {
	saved, err := kv.client.KV.Set(key, []byte(value))
	if !saved && err != nil {
		return errors.Wrap(err, "database error occurred when trying to save OAuth state")
	} else if !saved && err == nil {
		return errors.New("Failed to save OAuth state")
	}
	return nil
}

func (kv Impl) GetOAuthStateToken(key string) ([]byte, error) {
	var state []byte

	err := kv.client.KV.Get(key, &state)
	if err != nil {
		return nil, errors.Wrap(err, "failed to get OAuth state")
	}
	return state, nil
}

func (kv Impl) DeleteOAuthStateToken(key string) error {
	err := kv.client.KV.Delete(key)
	if err != nil {
		return errors.Wrap(err, "failed to delete OAuth state")
	}
	return nil
}

func (kv Impl) StoreGoogleUserToken(userID string, encryptedToken string) error {
	saved, err := kv.client.KV.Set(getUserTokenKey(userID), []byte(encryptedToken))
	if !saved && err != nil {
		return errors.Wrap(err, "database error occurred when trying to save Google user token")
	} else if !saved && err == nil {
		return errors.New("Failed to save Google user token")
	}
	return nil
}

func (kv Impl) GetGoogleUserToken(userID string) ([]byte, error) {
	var token []byte

	err := kv.client.KV.Get(getUserTokenKey(userID), &token)
	if err != nil {
		return nil, errors.Wrap(err, "failed to get user token")
	}
	return token, nil
}

func (kv Impl) DeleteGoogleUserToken(userID string) error {
	err := kv.client.KV.Delete(getUserTokenKey(userID))
	if err != nil {
		return errors.Wrap(err, "failed to delete user token")
	}
	return nil
}

func (kv Impl) StoreUserRateLimitExceeded(serviceType string, userID string) error {
	saved, err := kv.client.KV.Set(getUserRateLimitKey(serviceType, userID), []byte("true"), pluginapi.SetExpiry(time.Second*10))
	if !saved && err != nil {
		return errors.Wrap(err, "database error occurred when trying to save user rate limit exceeded")
	} else if !saved && err == nil {
		return errors.New("Failed to save user rate limit exceeded")
	}
	return nil
}

func (kv Impl) GetUserRateLimitExceeded(serviceType string, userID string) (bool, error) {
	var rateLimitExceeded bool

	err := kv.client.KV.Get(getUserRateLimitKey(serviceType, userID), &rateLimitExceeded)
	if err != nil {
		return false, errors.Wrap(err, "failed to get user rate limit exceeded")
	}
	return rateLimitExceeded, nil
}

func (kv Impl) StoreProjectRateLimitExceeded(serviceType string) error {
	saved, err := kv.client.KV.Set(getProjectRateLimitKey(serviceType), []byte("true"), pluginapi.SetExpiry(time.Second*10))
	if !saved && err != nil {
		return errors.Wrap(err, "database error occurred when trying to save project rate limit exceeded")
	} else if !saved && err == nil {
		return errors.New("Failed to save project rate limit exceeded")
	}
	return nil
}

func (kv Impl) GetProjectRateLimitExceeded(serviceType string) (bool, error) {
	var rateLimitExceeded bool

	err := kv.client.KV.Get(getProjectRateLimitKey(serviceType), &rateLimitExceeded)
	if err != nil {
		return false, errors.Wrap(err, "failed to get project rate limit exceeded")
	}
	return rateLimitExceeded, nil
}
