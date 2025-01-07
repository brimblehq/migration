package manager

import (
	"context"
	"embed"
	"fmt"
	"log"
	"strings"
	"time"

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
	LicenseResponse *core.LicenseResponse
}

func NewInstallationManager(client *ssh.SSHClient, server types.Server, roles []types.ClusterRole, config *types.Config, tailScaleToken string, db *db.PostgresDB, lisResp *core.LicenseResponse) *InstallationManager {
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

func (im *InstallationManager) SetupConsulClient() error {
	serverHost := strings.Split(im.config.ClusterConfig.ConsulConfig.ServerAddress, ":")[0]

	checkCmd := "docker ps -a --format '{{.Names}}' | grep -w consul-client || true"

	output, err := im.sshClient.ExecuteCommandWithOutput(checkCmd)

	if err != nil {
		return fmt.Errorf("failed to check consul container: %v", err)
	}

	if strings.Contains(output, "consul-client") {
		stopCommands := []string{
			"docker stop consul-client",
			"docker rm consul-client",
		}

		for _, cmd := range stopCommands {
			if err := im.sshClient.ExecuteCommand(cmd); err != nil {
				return fmt.Errorf("failed to execute command %q: %v", cmd, err)
			}
		}
	}

	nodeName, err := im.getMachineNodeName()

	if err != nil {
		return fmt.Errorf("failed to setup consul container: %v", err)
	}

	runCmd := fmt.Sprintf(`docker run -d \
        --name consul-client \
        --network host \
        --restart unless-stopped \
        %s agent \
        -node=%s \
        -retry-join=%s:8301 \
        -client=0.0.0.0 \
        -bind=%s \
        -datacenter=%s`,
		im.config.ClusterConfig.ConsulConfig.ConsulImage,
		nodeName,
		serverHost,
		im.server.PublicIP,
		im.config.ClusterConfig.ConsulConfig.DataCenter,
	)

	if err := im.sshClient.ExecuteCommand(runCmd); err != nil {
		return fmt.Errorf("failed to start consul container: %v", err)
	}

	checkConsulCmd := "curl -s http://localhost:8500/v1/status/leader || true"
	for i := 0; i < 30; i++ {
		output, err := im.sshClient.ExecuteCommandWithOutput(checkConsulCmd)
		if err == nil && output != "" {
			return nil
		}
		time.Sleep(2 * time.Second)
	}

	return fmt.Errorf("consul client failed to become ready")
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

func (im *InstallationManager) SetupNomad() error {
	if err := im.cleanupNomadState(); err != nil {
		return fmt.Errorf("failed to cleanup nomad state: %v", err)
	}

	var serverBlock, clientBlock string

	nodeName, err := im.getMachineNodeName()

	if err != nil {
		return fmt.Errorf("failed to setup consul container: %v", err)
	}

	isServer := false
	isClient := false
	for _, role := range im.roles {
		if role == types.RoleServer {
			isServer = true
		}
		if role == types.RoleClient {
			isClient = true
		}
	}

	var nomadConfig string

	if len(im.config.Servers) == 1 {
		nomadConfig = im.getSingleNodeConfig(nodeName)
	} else {
		if isServer {
			serverBlock = fmt.Sprintf(`
	server {
		enabled = true
		bootstrap_expect = %d
	}`, im.getServerCount())
		}

		if isClient {
			clientBlock = fmt.Sprintf(`
	client {
		enabled = true
		servers = [%s]
	}`, strings.Join(quoteServerAddresses(im.getNomadServerAddresses()), ", "))
		}

		nomadConfig = fmt.Sprintf(`
	datacenter = "%s"
	data_dir = "/opt/nomad/data"
	bind_addr = "%s"
	
	advertise {
		http = "%s:4646"
		rpc = "%s:4647"
		serf = "%s:4648"
	}
	%s
	%s
	
	consul {
		address = "127.0.0.1:8500"
		token = "%s"
		client_service_name = "%s"
		auto_advertise = true
		server_auto_join = true
		client_auto_join = true
	}
	
	plugin "docker" {
		config {
			allow_privileged = true
			volumes {
				enabled = true
			}
		}
	}
	
	telemetry {
		collection_interval = "1s"
		disable_hostname = true
		prometheus_metrics = true
		publish_allocation_metrics = true
		publish_node_metrics = true
	}`,
			im.config.ClusterConfig.ConsulConfig.DataCenter,
			im.server.PublicIP,
			im.server.PublicIP,
			im.server.PublicIP,
			im.server.PublicIP,
			serverBlock,
			clientBlock,
			im.config.ClusterConfig.ConsulConfig.Token,
			nodeName,
		)
	}

	//use tailscale private ip in production

	checkServiceCmd := "systemctl is-enabled nomad || true"

	output, err := im.sshClient.ExecuteCommandWithOutput(checkServiceCmd)

	if err != nil {
		return fmt.Errorf("failed to check nomad service status: %v", err)
	}

	var serviceCommands []string
	if strings.TrimSpace(output) == "enabled" {
		serviceCommands = []string{
			"sudo systemctl daemon-reload",
			"sudo systemctl restart nomad",
		}
	} else {
		serviceCommands = []string{
			"sudo systemctl daemon-reload",
			"sudo systemctl enable nomad",
			"sudo systemctl start nomad",
		}
	}

	commands := []string{
		"sudo mkdir -p /etc/nomad.d",
		fmt.Sprintf(`echo '%s' | sudo tee /etc/nomad.d/nomad.hcl`, nomadConfig),
	}

	for _, cmd := range commands {
		if err := im.sshClient.ExecuteCommand(cmd); err != nil {
			return err
		}
	}

	for _, cmd := range serviceCommands {
		if err := im.sshClient.ExecuteCommand(cmd); err != nil {
			return err
		}
	}

	if err := im.checkNomadHealth(); err != nil {
		return fmt.Errorf("failed to verify nomad health: %v", err)
	}
	return nil
}

func (im *InstallationManager) cleanupNomadState() error {
	checkCmd := "systemctl is-active nomad || true"
	status, err := im.sshClient.ExecuteCommandWithOutput(checkCmd)
	if err != nil {
		return fmt.Errorf("failed to check nomad status: %v", err)
	}

	if strings.TrimSpace(status) == "active" {
		stopJobsCmd := "nomad job stop -purge -yes -detach '*'"
		if err := im.sshClient.ExecuteCommand(stopJobsCmd); err != nil {
			log.Printf("Note: Failed to stop nomad jobs: %v", err)
		}

		time.Sleep(10 * time.Second)

		stopCmd := "sudo systemctl stop nomad"
		if err := im.sshClient.ExecuteCommand(stopCmd); err != nil {
			return fmt.Errorf("failed to stop nomad: %v", err)
		}

		time.Sleep(5 * time.Second)

		killCmd := "sudo pkill -9 nomad || true"
		if err := im.sshClient.ExecuteCommand(killCmd); err != nil {
			log.Printf("Note: Failed to force kill nomad processes: %v", err)
		}

		time.Sleep(2 * time.Second)
	}

	commands := []string{
		"for m in $(mount | grep nomad | awk '{print $3}'); do sudo umount $m || true; done",
		"sudo rm -rf /opt/nomad/data/*",
		"sudo rm -rf /opt/nomad/data/server/raft/*",
		"sudo rm -f /etc/nomad.d/nomad.hcl",
		"sudo mkdir -p /opt/nomad/data/server",
		"sudo mkdir -p /opt/nomad/data/client",
		"sudo mkdir -p /opt/nomad/data/alloc",
		"sudo chmod -R 700 /opt/nomad/data",
	}

	for _, cmd := range commands {
		if err := im.sshClient.ExecuteCommand(cmd); err != nil {
			return fmt.Errorf("failed to execute cleanup command %q: %v", cmd, err)
		}
	}
	return nil
}

func (im *InstallationManager) checkNomadHealth() error {
	maxRetries := 20
	for i := 0; i < maxRetries; i++ {
		statusCmd := "systemctl is-active nomad"
		status, _ := im.sshClient.ExecuteCommandWithOutput(statusCmd)
		if strings.TrimSpace(status) != "active" {
			time.Sleep(2 * time.Second)
			continue
		}

		logsCmd := `journalctl -u nomad --since '30 seconds ago' | grep -i error | grep -v "client.host_stats" | grep -v "failed to find disk usage" || true`
		logs, _ := im.sshClient.ExecuteCommandWithOutput(logsCmd)

		if logs == "" {
			healthCmd := "curl -s http://127.0.0.1:4646/v1/agent/health"
			health, err := im.sshClient.ExecuteCommandWithOutput(healthCmd)
			if err == nil && strings.Contains(health, "ok") {
				isServer := false
				for _, role := range im.roles {
					if role == types.RoleServer {
						isServer = true
						break
					}
				}

				if isServer {
					serverCmd := "nomad server members"
					_, err := im.sshClient.ExecuteCommandWithOutput(serverCmd)
					if err != nil {
						time.Sleep(2 * time.Second)
						continue
					}
				}
				return nil
			}
		}

		time.Sleep(2 * time.Second)
	}

	return fmt.Errorf("nomad failed to become healthy after %d seconds", maxRetries*2)
}

func (im *InstallationManager) getSingleNodeConfig(nodeName string) string {
	return fmt.Sprintf(`
 data_dir = "/opt/nomad/data"
 
 log_level = "INFO"
 
 server {
	enabled = true
	bootstrap_expect = 1
 }
 
 client {
	enabled = true
	servers = ["127.0.0.1:4647"]
 }
 
 addresses {
	http = "0.0.0.0"
 }
 
 ports {
	http = 4646
	rpc  = 4647
	serf = 4648
 }
 
 consul {
	address = "127.0.0.1:8500"
	token = "%s"
	client_service_name = "%s"
	auto_advertise = true
	server_auto_join = true
	client_auto_join = true
 }
 
 plugin "docker" {
	config {
		allow_privileged = true
		volumes {
			enabled = true
		}
	}
 }
 
 telemetry {
	collection_interval = "1s"
	disable_hostname = true
	prometheus_metrics = true
	publish_allocation_metrics = true
	publish_node_metrics = true
 }`, im.config.ClusterConfig.ConsulConfig.Token, nodeName)
}
