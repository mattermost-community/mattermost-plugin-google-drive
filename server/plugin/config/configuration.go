package config

import (
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"strings"

	"github.com/pkg/errors"
)

// configuration captures the plugin's external configuration as exposed in the Mattermost server
// configuration, as well as values computed from the configuration. Any public fields will be
// deserialized from the Mattermost server configuration in OnConfigurationChange.
//
// As plugins are inherently concurrent (hooks being called asynchronously), and the plugin
// configuration can change at any time, access to the configuration must be synchronized. The
// strategy used in this plugin is to guard a pointer to the configuration, and clone the entire
// struct whenever it changes. You may replace this with whatever strategy you choose.
//
// If you add non-reference types to your configuration struct, be sure to rewrite Clone as a deep
// copy appropriate for your types.
type Configuration struct {
	GoogleOAuthClientID     string `json:"googleoauthclientid"`
	GoogleOAuthClientSecret string `json:"googleoauthclientsecret"`
	EncryptionKey           string `json:"encryptionkey"`
	QueriesPerMinute        int    `json:"queriesperminute"`
	BurstSize               int    `json:"burstsize"`
}

func (c *Configuration) ToMap() (map[string]interface{}, error) {
	var out map[string]interface{}
	data, err := json.Marshal(c)
	if err != nil {
		return nil, err
	}
	err = json.Unmarshal(data, &out)
	if err != nil {
		return nil, err
	}

	return out, nil
}

func (c *Configuration) SetDefaults() (bool, error) {
	changed := false

	if c.EncryptionKey == "" {
		secret, err := generateSecret()
		if err != nil {
			return false, err
		}

		c.EncryptionKey = secret
		changed = true
	}

	return changed, nil
}

func (c *Configuration) Sanitize() {
	c.GoogleOAuthClientID = strings.TrimSpace(c.GoogleOAuthClientID)
	c.GoogleOAuthClientSecret = strings.TrimSpace(c.GoogleOAuthClientSecret)
}

func (c *Configuration) IsOAuthConfigured() bool {
	return c.GoogleOAuthClientID != "" && c.GoogleOAuthClientSecret != ""
}

func (c *Configuration) ClientConfiguration() map[string]interface{} {
	return map[string]interface{}{}
}

// Clone shallow copies the configuration. Your implementation may require a deep copy if
// your configuration has reference types.
func (c *Configuration) Clone() *Configuration {
	var clone = *c
	return &clone
}

// IsValid checks if all needed fields are set.
func (c *Configuration) IsValid() error {
	if c.GoogleOAuthClientID == "" {
		return errors.New("must have a Google OAuth client id")
	}
	if c.GoogleOAuthClientSecret == "" {
		return errors.New("must have a Google OAuth client secret")
	}

	if c.EncryptionKey == "" {
		return errors.New("must have an encryption key")
	}

	if c.QueriesPerMinute <= 0 {
		return errors.New("queries per minute must be greater than 0")
	}

	if c.BurstSize <= 0 {
		return errors.New("burst size must be greater than 0")
	}

	return nil
}

func generateSecret() (string, error) {
	b := make([]byte, 256)
	_, err := rand.Read(b)
	if err != nil {
		return "", err
	}
	s := base64.RawStdEncoding.EncodeToString(b)

	s = s[:32]

	return s, nil
}
