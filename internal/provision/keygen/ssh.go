package keygen

import (
	"crypto/rand"
	"crypto/rsa"
	"fmt"
	"strings"

	"golang.org/x/crypto/ssh"
)

func GenerateSSHKey() (string, error) {
	privateKey, err := rsa.GenerateKey(rand.Reader, 4096)

	if err != nil {
		return "", fmt.Errorf("failed to generate private key: %w", err)
	}

	publicKey, err := ssh.NewPublicKey(&privateKey.PublicKey)
	if err != nil {
		return "", fmt.Errorf("failed to generate public key: %w", err)
	}

	publicKeyStr := string(ssh.MarshalAuthorizedKey(publicKey))

	return strings.TrimSpace(publicKeyStr), nil
}
