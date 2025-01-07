package core

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"strings"
	"time"

	"github.com/brimblehq/migration/internal/ssh"
	"github.com/google/uuid"
)

type ServerRegistration struct {
	Tag        string     `json:"tag"`
	Region     string     `json:"region"`
	URL        string     `json:"url"`
	ServerInfo ServerInfo `json:"serverInfo"`
}

type ServerInfo struct {
	ServerID         string     `json:"serverId"`
	Hostname         string     `json:"hostname"`
	IPAddress        string     `json:"ipAddress"`
	PrivateIPAddress string     `json:"privateIpAddress"`
	Specification    ServerSpec `json:"specification"`
}

type ServerSpec struct {
	CPU    string `json:"cpu"`
	Memory string `json:"memory"`
	Disk   string `json:"disk"`
	OS     string `json:"os"`
}

type RegistrationResponse struct {
	Message string `json:"message"`
	Data    struct {
		TunnelToken string `json:"tunnelToken"`
	} `json:"data"`
}

type ServerRegistrar struct {
	client         *http.Client
	sshClient      *ssh.SSHClient
	apiBaseURL     string
	licenseReponse *LicenseResponse
}

func NewServerRegistrar(sshClient *ssh.SSHClient, apiBaseURL string, licenseReponse *LicenseResponse) *ServerRegistrar {
	return &ServerRegistrar{
		client:         &http.Client{Timeout: 30 * time.Second},
		sshClient:      sshClient,
		apiBaseURL:     apiBaseURL,
		licenseReponse: licenseReponse,
	}
}

func (sr *ServerRegistrar) RegisterAndSetupTunnel(ctx context.Context, tag string) error {
	serverInfo, err := sr.gatherServerInfo(tag)

	if err != nil {
		return fmt.Errorf("failed to gather server info: %w", err)
	}

	resp, err := sr.registerServer(ctx, serverInfo)
	if err != nil {
		return fmt.Errorf("failed to register server: %w", err)
	}

	if err := sr.installCloudflared(); err != nil {
		return fmt.Errorf("failed to install cloudflared: %w", err)
	}

	if err := sr.setupTunnel(resp.Data.TunnelToken); err != nil {
		return fmt.Errorf("failed to setup tunnel: %w", err)
	}

	status, err := sr.getTunnelStatus()

	if err != nil {
		return fmt.Errorf("tunnel setup complete but status check failed: %w", err)
	}
	log.Printf("Tunnel Status: %s", status)

	return nil
}

func (sr *ServerRegistrar) gatherServerInfo(tag string) (*ServerRegistration, error) {
	var info ServerRegistration

	cpu, err := sr.sshClient.ExecuteCommandWithOutput("lscpu | grep 'Model name' | cut -d ':' -f 2 | xargs")

	if err != nil {
		return nil, err
	}

	mem, err := sr.sshClient.ExecuteCommandWithOutput("free -h | grep Mem: | awk '{print $2}'")

	if err != nil {
		return nil, err
	}

	disk, err := sr.sshClient.ExecuteCommandWithOutput("df -h / | tail -1 | awk '{print $2}'")

	if err != nil {
		return nil, err
	}

	os, err := sr.sshClient.ExecuteCommandWithOutput("lsb_release -d | cut -f 2")

	if err != nil {
		return nil, err
	}

	hostname, err := sr.sshClient.ExecuteCommandWithOutput("hostname")
	if err != nil {
		return nil, err
	}

	publicIP, err := sr.sshClient.ExecuteCommandWithOutput("curl -s ifconfig.me")

	if err != nil {
		return nil, err
	}

	privateIP, err := sr.sshClient.ExecuteCommandWithOutput("hostname -I | awk '{print $1}'")

	if err != nil {
		return nil, err
	}

	info = ServerRegistration{
		Tag:    tag,
		Region: "europe",
		URL:    fmt.Sprintf("http://%s:3000", strings.TrimSpace(privateIP)),
		ServerInfo: ServerInfo{
			ServerID:         fmt.Sprintf("srv-%s", uuid.New().String()),
			Hostname:         strings.TrimSpace(hostname),
			IPAddress:        strings.TrimSpace(publicIP),
			PrivateIPAddress: strings.TrimSpace(privateIP),
			Specification: ServerSpec{
				CPU:    strings.TrimSpace(cpu),
				Memory: strings.TrimSpace(mem),
				Disk:   strings.TrimSpace(disk),
				OS:     strings.TrimSpace(os),
			},
		},
	}

	return &info, nil
}

func (sr *ServerRegistrar) registerServer(ctx context.Context, info *ServerRegistration) (*RegistrationResponse, error) {
	jsonData, err := json.Marshal(info)
	if err != nil {
		return nil, err
	}

	req, err := http.NewRequestWithContext(ctx, "POST", sr.apiBaseURL+"/v1/license/server", bytes.NewBuffer(jsonData))
	if err != nil {
		return nil, err
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Brimble-Key", sr.licenseReponse.Key)

	resp, err := sr.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("server registration failed with status %d: %s", resp.StatusCode, string(body))
	}

	var regResp RegistrationResponse
	if err := json.NewDecoder(resp.Body).Decode(&regResp); err != nil {
		return nil, err
	}

	return &regResp, nil
}

func (sr *ServerRegistrar) installCloudflared() error {
	commands := []string{
		"sudo mkdir -p --mode=0755 /usr/share/keyrings",
		"curl -fsSL https://pkg.cloudflare.com/cloudflare-main.gpg | sudo tee /usr/share/keyrings/cloudflare-main.gpg >/dev/null",
		"echo 'deb [signed-by=/usr/share/keyrings/cloudflare-main.gpg] https://pkg.cloudflare.com/cloudflared focal main' | sudo tee /etc/apt/sources.list.d/cloudflared.list",
		"sudo apt-get update && sudo apt-get install cloudflared -y",
	}

	for _, cmd := range commands {
		if err := sr.sshClient.ExecuteCommand(cmd); err != nil {
			return fmt.Errorf("failed to execute command %q: %w", cmd, err)
		}
	}

	return nil
}

func (sr *ServerRegistrar) isCloudflaredRunning() (bool, error) {
	output, err := sr.sshClient.ExecuteCommandWithOutput("sudo systemctl is-active cloudflared")
	if err != nil {
		return false, nil
	}

	return strings.TrimSpace(output) == "active", nil
}

func (sr *ServerRegistrar) stopTunnel() error {
	commands := []string{
		"sudo cloudflared service uninstall",
		"sudo systemctl stop cloudflared",
		"sudo systemctl disable cloudflared",
	}

	for _, cmd := range commands {
		if err := sr.sshClient.ExecuteCommand(cmd); err != nil {
			log.Printf("Warning: command failed %q: %v", cmd, err)
		}
	}

	isRunning, err := sr.isCloudflaredRunning()
	if err != nil {
		return fmt.Errorf("failed to verify cloudflared status after stop: %w", err)
	}

	if isRunning {
		return fmt.Errorf("failed to stop cloudflared service")
	}

	return nil
}

func (sr *ServerRegistrar) getTunnelStatus() (string, error) {
	output, err := sr.sshClient.ExecuteCommandWithOutput("sudo cloudflared tunnel info")
	if err != nil {
		return "", fmt.Errorf("failed to get tunnel info: %w", err)
	}
	return output, nil
}

func (sr *ServerRegistrar) setupTunnel(tunnelToken string) error {
	isRunning, err := sr.isCloudflaredRunning()
	if err != nil {
		return fmt.Errorf("failed to check cloudflared status: %w", err)
	}

	if isRunning {
		if err := sr.stopTunnel(); err != nil {
			return fmt.Errorf("failed to stop existing tunnel: %w", err)
		}
	}

	cmd := fmt.Sprintf("sudo cloudflared service install %s", tunnelToken)
	if err := sr.sshClient.ExecuteCommand(cmd); err != nil {
		return fmt.Errorf("failed to install tunnel with new token: %w", err)
	}

	if err := sr.sshClient.ExecuteCommand("sudo systemctl start cloudflared"); err != nil {
		return fmt.Errorf("failed to start cloudflared service: %w", err)
	}

	isRunning, err = sr.isCloudflaredRunning()
	if err != nil {
		return fmt.Errorf("failed to verify cloudflared status: %w", err)
	}

	if !isRunning {
		return fmt.Errorf("cloudflared service failed to start")
	}

	return nil
}
