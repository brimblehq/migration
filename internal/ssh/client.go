package ssh

import (
	"fmt"
	"io/ioutil"
	"os"
	"time"

	"github.com/brimblehq/migration/internal/types"
	"golang.org/x/crypto/ssh"
)

type SSHClient struct {
	client *ssh.Client
	config *ssh.ClientConfig
}

func NewSSHClient(server types.Server) (*SSHClient, error) {
	key, err := ioutil.ReadFile(os.ExpandEnv(server.KeyPath))

	if err != nil {
		return nil, fmt.Errorf("unable to read private key: %v", err)
	}

	signer, err := ssh.ParsePrivateKey(key)

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
		client: client,
		config: config,
	}, nil
}

func (s *SSHClient) ExecuteCommand(command string) error {
	session, err := s.client.NewSession()
	if err != nil {
		return fmt.Errorf("failed to create session: %v", err)
	}
	defer session.Close()

	session.Stdout = os.Stdout
	session.Stderr = os.Stderr

	return session.Run(command)
}

func (s *SSHClient) Close() error {
	return s.client.Close()
}
