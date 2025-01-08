package manager

import (
	"context"
	"embed"
	"fmt"
	"strings"

	"github.com/brimblehq/migration/assets"
	"github.com/brimblehq/migration/internal/core"
	"github.com/brimblehq/migration/internal/db"
	"github.com/brimblehq/migration/internal/ssh"
	"github.com/brimblehq/migration/internal/types"
)

type InstallationManager struct {
	sshClient       *ssh.SSHClient
	server          types.Server
	roles           []types.ClusterRole
	config          *types.Config
	files           embed.FS
	tailScaleToken  string
	DB              *db.PostgresDB
	LicenseResponse *types.LicenseResponse
}

func NewInstallationManager(client *ssh.SSHClient, server types.Server, roles []types.ClusterRole, config *types.Config, tailScaleToken string, db *db.PostgresDB, lisResp *types.LicenseResponse) *InstallationManager {
	return &InstallationManager{
		sshClient:       client,
		server:          server,
		roles:           roles,
		config:          config,
		files:           assets.MonitoringFiles,
		tailScaleToken:  tailScaleToken,
		DB:              db,
		LicenseResponse: lisResp,
	}
}

func (im *InstallationManager) InstallBasePackages() error {
	registrar := core.NewServerRegistrar(im.sshClient, "https://core.brimble.io", im.LicenseResponse)

	if err := registrar.RegisterAndSetupTunnel(context.Background(), im.LicenseResponse.Tag); err != nil {
		return fmt.Errorf("failed to register server and setup tunnel: %w", err)
	}

	commands := []string{
		"sudo apt-get update",
		"sudo apt-get upgrade -y",
		"sudo apt install -y curl unzip wget ufw coreutils gpg debian-keyring debian-archive-keyring apt-transport-https",
		"sudo apt update -y",

		fmt.Sprintf("sudo tailscale up --advertise-tags='tag:client-%s'", im.LicenseResponse.Tag),

		"curl -fsSL https://get.docker.com -o get-docker.sh",
		"sudo sh get-docker.sh",
		"sudo usermod -aG docker $USER",
		"sudo apt install -y docker-compose",

		fmt.Sprintf("curl -fsSL https://deb.nodesource.com/setup_%s | sudo -E bash -", im.config.ClusterConfig.Versions.NodeJS),
		"sudo apt-get install -y nodejs",

		"apt-get install -y redis-server",
		"systemctl enable redis-server",
		"systemctl start redis-server",

		"curl -o- https://raw.githubusercontent.com/nvm-sh/nvm/v0.40.1/install.sh | bash",
		"export NVM_DIR=\"$HOME/.nvm\" && [ -s \"$NVM_DIR/nvm.sh\" ] && . \"$NVM_DIR/nvm.sh\" && [ -s \"$NVM_DIR/bash_completion\" ] && . \"$NVM_DIR/bash_completion\" && nvm install 20 && nvm use 20",

		"/root/.nvm/versions/node/v20.18.1/bin/npm install --global yarn",
		"/root/.nvm/versions/node/v20.18.1/bin/npm install -g pm2",

		"curl -1sLf 'https://dl.cloudsmith.io/public/infisical/infisical-cli/setup.deb.sh' | sudo -E bash",
		"sudo apt-get update && sudo apt-get install -y infisical",

		"curl -sSL https://nixpacks.com/install.sh | bash",

		"curl -fsSL https://apt.releases.hashicorp.com/gpg | sudo tee /tmp/hashicorp.gpg > /dev/null",
		"sudo gpg --batch --yes --dearmor -o /usr/share/keyrings/hashicorp-archive-keyring.gpg /tmp/hashicorp.gpg",
		"sudo rm /tmp/hashicorp.gpg",
		"echo \"deb [arch=$(dpkg --print-architecture) signed-by=/usr/share/keyrings/hashicorp-archive-keyring.gpg] https://apt.releases.hashicorp.com $(lsb_release -cs) main\" | sudo tee /etc/apt/sources.list.d/hashicorp.list",
		"sudo apt update && sudo apt install -y nomad",
		"sudo apt-get install -y consul-cni",

		"curl -fsSL https://cdn.brimble.io/runner-linux -o runner.sh",
		"sudo chmod +x runner.sh",
		"sudo mv runner.sh /usr/local/bin/runner",

		"ARCH_CNI=$( [ $(uname -m) = aarch64 ] && echo arm64 || echo amd64) && CNI_PLUGIN_VERSION=v1.5.1 && curl -L -o cni-plugins.tgz \"https://github.com/containernetworking/plugins/releases/download/${CNI_PLUGIN_VERSION}/cni-plugins-linux-${ARCH_CNI}-${CNI_PLUGIN_VERSION}.tgz\" && sudo mkdir -p /opt/cni/bin && sudo tar -C /opt/cni/bin -xzf cni-plugins.tgz",
	}

	for _, cmd := range commands {
		if err := im.sshClient.ExecuteCommand(cmd); err != nil {
			return fmt.Errorf("failed to execute command %q: %v", cmd, err)
		}
	}
	return nil
}

func (im *InstallationManager) getMachineNodeName() (string, error) {
	machineID, err := im.sshClient.ExecuteCommandWithOutput("cat /etc/machine-id")

	if err != nil {
		return "", fmt.Errorf("failed to get machine-id: %v", err)
	}
	nodeName := fmt.Sprintf("nomad-client-%s", strings.TrimSpace(machineID[:10]))

	return nodeName, nil
}

func (im *InstallationManager) getServerCount() int {
	totalNodes := len(im.config.Servers)
	switch totalNodes {
	case 1, 2:
		return 1
	default:
		return totalNodes - 1
	}
}

func (im *InstallationManager) getNomadServerAddresses() []string {
	var servers []string
	numServers := im.getServerCount()
	for i := 0; i < numServers; i++ {
		//use tailscale private ip in production
		serverIP := im.config.Servers[i].PublicIP
		servers = append(servers, fmt.Sprintf("%s:4647", serverIP))
	}
	return servers
}

func quoteServerAddresses(addresses []string) []string {
	quoted := make([]string, len(addresses))
	for i, addr := range addresses {
		quoted[i] = fmt.Sprintf(`"%s"`, addr)
	}
	return quoted
}
