package manager

import (
	"fmt"
	"strings"

	"github.com/brimblehq/migration/internal/ssh"
	"github.com/brimblehq/migration/internal/types"
)

type InstallationManager struct {
	sshClient *ssh.SSHClient
	server    types.Server
	roles     []types.ClusterRole
	config    *types.Config
}

func NewInstallationManager(client *ssh.SSHClient, server types.Server, roles []types.ClusterRole, config *types.Config) *InstallationManager {
	return &InstallationManager{
		sshClient: client,
		server:    server,
		roles:     roles,
		config:    config,
	}
}

func (im *InstallationManager) InstallBasePackages() error {
	commands := []string{
		"sudo apt-get update",
		"sudo apt-get upgrade -y",

		"curl -fsSL https://get.docker.com -o get-docker.sh",
		"sudo sh get-docker.sh",
		"sudo usermod -aG docker $USER",

		fmt.Sprintf("curl -fsSL https://deb.nodesource.com/setup_%s | sudo -E bash -", im.config.ClusterConfig.Versions.NodeJS),
		"sudo apt-get install -y nodejs",

		"sudo npm install -g pm2",

		"sudo apt-get install -y redis-server",
		"sudo systemctl enable redis-server",
		"sudo systemctl start redis-server",

		fmt.Sprintf(`curl -fsSL https://releases.hashicorp.com/nomad/%s/nomad_%s_linux_amd64.zip -o nomad.zip`,
			im.config.ClusterConfig.Versions.Nomad,
			im.config.ClusterConfig.Versions.Nomad),
		"unzip nomad.zip",
		"sudo mv nomad /usr/local/bin/",
		"sudo chmod +x /usr/local/bin/nomad",

		"curl -fsSL https://cdn.brimble.io/runner.sh -o runner.sh",
		"sudo chmod +x runner.sh",
		"sudo mv runner.sh /usr/local/bin/runner",
	}

	for _, cmd := range commands {
		if err := im.sshClient.ExecuteCommand(cmd); err != nil {
			return fmt.Errorf("failed to execute command %s: %v", cmd, err)
		}
	}
	return nil
}

func (im *InstallationManager) SetupConsulClient() error {
	consulCmd := fmt.Sprintf(`docker run -d \
       --name consul-client \
       --network host \
       --restart unless-stopped \
       %s agent \
       -node=nomad-client-%d \
       -retry-join=%s \
       -client=0.0.0.0 \
       -bind=%s \
       -datacenter=%s \
       -token=%s`,
		im.config.ClusterConfig.ConsulConfig.ConsulImage,
		im.getMachineNumber(),
		im.config.ClusterConfig.ConsulConfig.ServerAddress,
		im.server.PrivateIP,
		im.config.ClusterConfig.ConsulConfig.DataCenter,
		im.config.ClusterConfig.ConsulConfig.Token,
	)

	return im.sshClient.ExecuteCommand(consulCmd)
}

func (im *InstallationManager) getMachineNumber() int {
	for i, server := range im.config.Servers {
		if server.Host == im.server.Host {
			return i + 1
		}
	}
	return 1
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
		serverIP := im.config.Servers[i].PrivateIP
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
	isServer := false
	for _, role := range im.roles {
		if role == types.RoleServer {
			isServer = true
			break
		}
	}

	nomadConfig := fmt.Sprintf(`
datacenter = "%s"
data_dir = "/opt/nomad/data"
bind_addr = "%s"

advertise {
   http = "%s:4646"
   rpc = "%s:4647"
   serf = "%s:4648"
}

server {
   enabled = %t
   bootstrap_expect = %d
}

client {
   enabled = true
   servers = [%s]
}

consul {
   address = "127.0.0.1:8500"
   token = "%s"
   client_service_name = "nomad-client-%d"
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
		im.server.PrivateIP,
		im.server.PrivateIP,
		im.server.PrivateIP,
		im.server.PrivateIP,
		isServer,
		im.getServerCount(),
		strings.Join(quoteServerAddresses(im.getNomadServerAddresses()), ", "),
		im.config.ClusterConfig.ConsulConfig.Token,
		im.getMachineNumber(),
	)

	commands := []string{
		"sudo mkdir -p /etc/nomad.d",
		fmt.Sprintf(`echo '%s' | sudo tee /etc/nomad.d/nomad.hcl`, nomadConfig),
		"sudo systemctl enable nomad",
		"sudo systemctl start nomad",
	}

	for _, cmd := range commands {
		if err := im.sshClient.ExecuteCommand(cmd); err != nil {
			return err
		}
	}

	return nil
}

func (im *InstallationManager) StartRunner(licenseToken string) error {
	return im.sshClient.ExecuteCommand(fmt.Sprintf("/usr/local/bin/runner  --license-key=%s", licenseToken))
}
