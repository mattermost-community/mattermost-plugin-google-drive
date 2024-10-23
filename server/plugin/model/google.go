package model

type ErrorMetadata struct {
	Consumer        string `json:"consumer"`
	QuotaLimit      string `json:"quota_limit"`
	QuotaLimitValue string `json:"quota_limit_value"`
	QuotaLocation   string `json:"quota_location"`
	QuotaMetric     string `json:"quota_metric"`
	Service         string `json:"service"`
}

type ErrorDetail struct {
	DetailType string        `json:"@type"`
	Domain     string        `json:"domain"`
	Metadata   ErrorMetadata `json:"metadata"`
	Reason     string        `json:"reason"`
}
