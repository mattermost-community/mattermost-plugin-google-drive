package model

type WatchChannelData struct {
	ChannelID        string            `json:"channel_id"`
	ResourceID       string            `json:"resource_id"`
	MMUserID         string            `json:"mm_user_id"`
	Expiration       int64             `json:"expiration"`
	Token            string            `json:"token"`
	PageToken        string            `json:"page_token"`
	FileLastActivity map[string]string `json:"file_last_activity"`
}
