package license

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/base64"
	"fmt"
)

func GenerateKey() (string, error) {
	key := make([]byte, 32)
	if _, err := rand.Read(key); err != nil {
		return "", fmt.Errorf("failed to generate key: %v", err)
	}

	return base64.StdEncoding.EncodeToString(key), nil
}

func Decrypt(encrypted string, key string) (string, error) {
	data, err := base64.StdEncoding.DecodeString(encrypted)
	if err != nil {
		return "", err
	}

	block, err := aes.NewCipher([]byte(key))
	if err != nil {
		return "", err
	}

	iv := make([]byte, 16)

	mode := cipher.NewCBCDecrypter(block, iv)

	decrypted := make([]byte, len(data))
	mode.CryptBlocks(decrypted, data)

	paddingLen := int(decrypted[len(decrypted)-1])
	return string(decrypted[:len(decrypted)-paddingLen]), nil
}
