package ssh

import (
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/brimblehq/migration/internal/types"
	"golang.org/x/crypto/ssh"
)

type SSHClient struct {
	Client *ssh.Client
	config *ssh.ClientConfig
}

func NewSSHClient(server types.Server) (*SSHClient, error) {
	keyPath := server.KeyPath

	if strings.HasPrefix(keyPath, "~") {
		homeDir, err := os.UserHomeDir()
		if err != nil {
			return nil, fmt.Errorf("failed to resolve home directory: %v", err)
		}
		keyPath = filepath.Join(homeDir, keyPath[1:])
	}

	key, err := ioutil.ReadFile(keyPath)
	if err != nil {
		return nil, fmt.Errorf("%v: unable to read private key: %v", server, err)
	}

	passphrase := []byte("password")
	signer, err := ssh.ParsePrivateKeyWithPassphrase(key, passphrase)
	if err != nil {
		return nil, fmt.Errorf("unable to parse private key: %v", err)
	}

	config := &ssh.ClientConfig{
		User: server.Username,
		Auth: []ssh.AuthMethod{
			ssh.PublicKeys(signer),
		},
		HostKeyCallback: ssh.InsecureIgnoreHostKey(),
		Timeout:         10 * time.Second,
	}

	client, err := ssh.Dial("tcp", fmt.Sprintf("%s:22", server.Host), config)
	if err != nil {
		return nil, fmt.Errorf("failed to connect to %s: %v", server.Host, err)
	}

	return &SSHClient{
		Client: client,
		config: config,
	}, nil
}

func (s *SSHClient) ExecuteCommand(command string) error {
	session, err := s.Client.NewSession()
	if err != nil {
		return fmt.Errorf("failed to create session: %v", err)
	}
	defer session.Close()

	session.Stdout = os.Stdout
	session.Stderr = os.Stderr

	return session.Run(command)
}

func (s *SSHClient) ExecuteCommandWithOutput(command string) (string, error) {
	session, err := s.Client.NewSession()
	if err != nil {
		return "", fmt.Errorf("failed to create session: %v", err)
	}
	defer session.Close()

	output, err := session.Output(command)
	if err != nil {
		return "", fmt.Errorf("failed to execute command: %v", err)
	}

	return string(output), nil
}

func (s *SSHClient) Close() error {
	return s.Client.Close()
}
