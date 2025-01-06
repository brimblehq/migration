package ssh

import (
	"bufio"
	"bytes"
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/brimblehq/migration/internal/types"
	"github.com/brimblehq/migration/internal/ui"
	"golang.org/x/crypto/ssh"
)

type SSHClient struct {
	Client  *ssh.Client
	config  *ssh.ClientConfig
	output  *ui.TerminalOutput
	spinner *ui.StepSpinner
}

func NewSSHClient(server types.Server, config *ssh.ClientConfig) (*SSHClient, error) {
	var sshConfig *ssh.ClientConfig

	output := ui.NewTerminalOutput(server.Host)

	if config != nil {
		sshConfig = config
	} else {
		var err error
		sshConfig, err = createConfigFromKeyPath(server)
		if err != nil {
			return nil, err
		}
	}

	sshConfig.User = server.Username
	sshConfig.HostKeyCallback = ssh.InsecureIgnoreHostKey()
	sshConfig.Timeout = 10 * time.Second

	client, err := ssh.Dial("tcp", fmt.Sprintf("%s:22", server.Host), sshConfig)
	if err != nil {
		return nil, fmt.Errorf("failed to connect to %s: %v", server.Host, err)
	}

	return &SSHClient{
		Client: client,
		config: sshConfig,
		output: output,
	}, nil
}
func createConfigFromKeyPath(server types.Server) (*ssh.ClientConfig, error) {
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

	return &ssh.ClientConfig{
		Auth: []ssh.AuthMethod{
			ssh.PublicKeys(signer),
		},
	}, nil
}

func (s *SSHClient) ExecuteCommand(command string) error {
	session, err := s.Client.NewSession()
	if err != nil {
		return fmt.Errorf("failed to create session: %v", err)
	}
	defer session.Close()

	if s.spinner != nil {
		currentStep := strings.TrimPrefix(s.spinner.GetCurrentStep(), " ")
		s.spinner.Start(currentStep)
	}

	outPipe, err := session.StdoutPipe()
	if err != nil {
		return fmt.Errorf("failed to create stdout pipe: %v", err)
	}

	errPipe, err := session.StderrPipe()
	if err != nil {
		return fmt.Errorf("failed to create stderr pipe: %v", err)
	}

	if err := session.Start(command); err != nil {
		return fmt.Errorf("failed to start command: %v", err)
	}

	go func() {
		scanner := bufio.NewScanner(outPipe)
		for scanner.Scan() {
			if s.output != nil {
				s.output.WriteLine("%s", scanner.Text())
			}
		}
	}()

	go func() {
		scanner := bufio.NewScanner(errPipe)
		for scanner.Scan() {
			if s.output != nil {
				s.output.WriteLine("ERROR: %s", scanner.Text())
			}
		}
	}()

	return session.Wait()
}

func (s *SSHClient) ExecuteCommandWithOutput(command string) (string, error) {
	session, err := s.Client.NewSession()
	if err != nil {
		return "", fmt.Errorf("failed to create session: %v", err)
	}
	defer session.Close()

	if s.spinner != nil {
		currentStep := strings.TrimPrefix(s.spinner.GetCurrentStep(), " ")
		s.spinner.Start(currentStep)
	}

	outPipe, err := session.StdoutPipe()
	if err != nil {
		return "", fmt.Errorf("failed to create stdout pipe: %v", err)
	}

	var outputBuffer bytes.Buffer

	if err := session.Start(command); err != nil {
		return "", fmt.Errorf("failed to start command: %v", err)
	}

	scanner := bufio.NewScanner(outPipe)
	for scanner.Scan() {
		line := scanner.Text()
		outputBuffer.WriteString(line + "\n")
		if s.output != nil {
			s.output.WriteLine("%s", line)
		}
	}

	if err := session.Wait(); err != nil {
		return "", fmt.Errorf("command execution failed: %v", err)
	}

	return outputBuffer.String(), nil
}

func (s *SSHClient) Close() error {
	s.output.Clear()
	return s.Client.Close()
}
