package plugin

import (
	"encoding/json"

	"golang.org/x/oauth2"
)

func (p *Plugin) storeGoogleUserToken(userID string, token *oauth2.Token) error {
	config := p.getConfiguration()

	jsonToken, err := json.Marshal(token)
	if err != nil {
		return err
	}

	encryptedToken, err := encrypt([]byte(config.EncryptionKey), string(jsonToken))
	if err != nil {
		return err
	}

	if _, err := p.client.KV.Set(getUserTokenKey(userID), []byte(encryptedToken)); err != nil {
		return err
	}

	return nil
}
