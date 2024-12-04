package utils

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/base64"
	"fmt"
	"io"

	"github.com/pkg/errors"
)

func GetInlineImage(text, imageURL string) string {
	return fmt.Sprintf("![%s](%s)", text, imageURL)
}

func GetHyperlink(text, url string) string {
	return fmt.Sprintf("[%s](%s)", text, url)
}

func encode(encrypted []byte) []byte {
	encoded := make([]byte, base64.URLEncoding.EncodedLen(len(encrypted)))
	base64.URLEncoding.Encode(encoded, encrypted)
	return encoded
}

func decode(encoded []byte) ([]byte, error) {
	decoded := make([]byte, base64.URLEncoding.DecodedLen(len(encoded)))
	n, err := base64.URLEncoding.Decode(decoded, encoded)
	if err != nil {
		return nil, err
	}
	return decoded[:n], nil
}

func Encrypt(key []byte, data []byte) ([]byte, error) {
	block, err := aes.NewCipher(key)
	if err != nil {
		return []byte(""), errors.Wrap(err, "could not create a cipher block, check key")
	}

	aesgcm, err := cipher.NewGCM(block)
	if err != nil {
		return []byte(""), err
	}

	nonce := make([]byte, aesgcm.NonceSize())
	if _, err = io.ReadFull(rand.Reader, nonce); err != nil {
		return []byte(""), err
	}

	sealed := aesgcm.Seal(nil, nonce, data, nil)
	return encode(append(nonce, sealed...)), nil
}

func Decrypt(key []byte, data []byte) ([]byte, error) {
	block, err := aes.NewCipher(key)
	if err != nil {
		return []byte(""), errors.Wrap(err, "could not create a cipher block, check key")
	}

	aesgcm, err := cipher.NewGCM(block)
	if err != nil {
		return []byte(""), err
	}

	decoded, err := decode(data)
	if err != nil {
		return []byte(""), err
	}

	nonceSize := aesgcm.NonceSize()
	if len(decoded) < nonceSize {
		return []byte(""), errors.New("token too short")
	}

	nonce, encrypted := decoded[:nonceSize], decoded[nonceSize:]
	plain, err := aesgcm.Open(nil, nonce, encrypted, nil)
	if err != nil {
		return []byte(""), err
	}

	return plain, nil
}

// LastN returns the last n characters of a string, with the rest replaced by *.
// At most 3 characters are replaced. The rest is cut off.
func LastN(s string, n int) string {
	if n < 0 {
		return ""
	}

	out := []byte(s)
	if len(out) > n+3 {
		out = out[len(out)-n-3:]
	}
	for i := range out {
		if i < len(out)-n {
			out[i] = '*'
		}
	}

	return string(out)
}
