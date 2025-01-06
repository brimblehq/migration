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

type SingleServerTempKeyError struct {
	Server types.Server
}

type ValidationError struct {
	Server string
	Reason string
}

const (
	AuthMethodKeyPath = "key_path"
	AuthMethodTemp    = "temp_key"
)

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

func ValidateServerAuth(servers []types.Server, useTemp bool) error {
	seen := make(map[string]bool)
	seenPublicIPs := make(map[string]string)
	seenPrivateIPs := make(map[string]string)
	var errors []ValidationError

	if len(servers) == 1 && servers[0].AuthMethod == AuthMethodTemp {
		return &SingleServerTempKeyError{
			Server: servers[0],
		}
	}

	for _, server := range servers {
		identifier := fmt.Sprintf("%s:%s:%s", server.Host, server.PublicIP, server.PrivateIP)

		if seen[identifier] {
			errors = append(errors, ValidationError{
				Server: server.Host,
				Reason: fmt.Sprintf("duplicate server configuration (public IP: %s, private IP: %s)",
					server.PublicIP, server.PrivateIP),
			})
		}
		seen[identifier] = true

		if existingHost, exists := seenPublicIPs[server.PublicIP]; exists {
			errors = append(errors, ValidationError{
				Server: server.Host,
				Reason: fmt.Sprintf("public IP %s already used by server %s",
					server.PublicIP, existingHost),
			})
		}
		seenPublicIPs[server.PublicIP] = server.Host

		if existingHost, exists := seenPrivateIPs[server.PrivateIP]; exists {
			errors = append(errors, ValidationError{
				Server: server.Host,
				Reason: fmt.Sprintf("private IP %s already used by server %s",
					server.PrivateIP, existingHost),
			})
		}
		seenPrivateIPs[server.PrivateIP] = server.Host

		if useTemp && server.AuthMethod == AuthMethodKeyPath {
			errors = append(errors, ValidationError{
				Server: server.Host,
				Reason: "configured to use key_path but --temp-ssh flag is provided",
			})
		}
		if !useTemp && server.AuthMethod == AuthMethodTemp {
			errors = append(errors, ValidationError{
				Server: server.Host,
				Reason: "configured to use temp_key but --temp-ssh flag is not provided",
			})
		}
	}

	if len(errors) > 0 {
		var errorMsg strings.Builder
		errorMsg.WriteString("Configuration validation failed:\n")
		for _, err := range errors {
			errorMsg.WriteString(fmt.Sprintf("- Server '%s': %s\n", err.Server, err.Reason))
		}
		return fmt.Errorf(errorMsg.String())
	}

	return nil
}

func (e *SingleServerTempKeyError) Error() string {
	return "single server with temp_key configuration detected"
}

func HandleServerAuth(server types.Server, tempManager *TempSSHManager, useTemp bool) (*SSHClient, error) {

	if useTemp {
		if tempManager == nil {
			return nil, fmt.Errorf("temp SSH requested but manager not initialized for server %s", server.Host)
		}
		sshConfig, err := tempManager.GetSSHConfig(server.Host)
		if err != nil {
			return nil, fmt.Errorf("failed to get temp SSH config for %s: %w", server.Host, err)
		}
		return NewSSHClient(server, sshConfig)
	}

	switch server.AuthMethod {
	case AuthMethodTemp:
		return nil, fmt.Errorf("machine %s configured to use temp_key but --temp-ssh flag not provided", server.Host)
	case AuthMethodKeyPath, "":
		return NewSSHClient(server, nil)
	default:
		return nil, fmt.Errorf("unsupported auth_method '%s' for server %s", server.AuthMethod, server.Host)
	}
}

func (s *SSHClient) Close() error {
	s.output.Clear()
	return s.Client.Close()
}
