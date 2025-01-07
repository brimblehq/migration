package manager

import (
	"fmt"
	"strings"
	"time"
)

func (im *InstallationManager) SetupConsul() error {
	serverAddr, err := im.DB.GetConsulAddress()

	isServerExists := err == nil && serverAddr != ""

	shouldBeServer := false
	if !isServerExists {
		for _, role := range im.roles {
			if role == "server" || role == "both" {
				shouldBeServer = true
				break
			}
		}
	}

	if shouldBeServer {
		serverAddr, err := im.setupConsulServer()
		if err != nil {
			return err
		}

		address := strings.Split(serverAddr, ":")[0]

		machineID, _ := im.getMachineID()

		if err := im.DB.SaveConsulAddress(address, machineID); err != nil {
			return err
		}
	}

	return im.setupConsulClient()
}

func (c *InstallationManager) setupConsulServer() (string, error) {
	ports := []string{"8300", "8301", "8302", "8500", "8600"}

	for _, port := range ports {
		killCmds := []string{
			fmt.Sprintf("sudo lsof -t -i:%s | xargs -r sudo kill -9", port),
			fmt.Sprintf("sudo pkill -f 'consul.*%s'", port),
			"sudo killall -9 consul || true",
		}
		for _, cmd := range killCmds {
			_ = c.sshClient.ExecuteCommand(cmd)
		}
	}

	cleanup := []string{
		"docker stop consul-server || true",
		"docker rm -f consul-server || true",
		"docker network rm -f consul-net || true",
		"docker ps -aq --filter name=consul | xargs -r docker rm -f || true",
		"sudo rm -rf /opt/consul/*",
	}
	for _, cmd := range cleanup {
		_ = c.sshClient.ExecuteCommand(cmd)
	}

	dirs := []string{
		"/opt/consul/data",
		"/opt/consul/config",
	}
	for _, dir := range dirs {
		if err := c.sshClient.ExecuteCommand(fmt.Sprintf("sudo mkdir -p %s", dir)); err != nil {
			return "", fmt.Errorf("failed to create directory %s: %w", dir, err)
		}
	}

	nodeName, err := c.getMachineNodeName()
	if err != nil {
		return "", err
	}

	if err := c.sshClient.ExecuteCommand("docker network create consul-net"); err != nil {
		return "", fmt.Errorf("failed to create network: %w", err)
	}

	runCmd := fmt.Sprintf("docker run -d --name consul-server --restart unless-stopped -p 8500:8500 -p 8600:8600/tcp -p 8600:8600/udp -p 8301:8301/tcp -p 8301:8301/udp -p 8302:8302/tcp -p 8302:8302/udp -p 8300:8300 -v /opt/consul/data:/consul/data -v /opt/consul/config:/consul/config %s agent -server -ui -bootstrap-expect=1 -node=%s -client=0.0.0.0 -bind=0.0.0.0 -advertise=%s -serf-wan-port=8302 -serf-lan-port=8301 -server-port=8300 -datacenter=dc1",
		c.config.ClusterConfig.ConsulConfig.ConsulImage,
		nodeName,
		c.server.PublicIP,
	)

	fmt.Println("runCmd", runCmd)

	if err := c.sshClient.ExecuteCommand(runCmd); err != nil {
		return "", fmt.Errorf("failed to start consul server: %w", err)
	}

	serverAddr := fmt.Sprintf("%s:8500", c.server.PublicIP)
	for i := 0; i < 30; i++ {
		if output, err := c.sshClient.ExecuteCommandWithOutput(fmt.Sprintf("curl -s http://%s/v1/status/leader", serverAddr)); err == nil && output != "" {
			return serverAddr, nil
		}
		time.Sleep(2 * time.Second)
	}

	return "", fmt.Errorf("consul server failed to become ready")
}

func (im *InstallationManager) setupConsulClient() error {
	serverAddr, err := im.DB.GetConsulAddress()
	if err != nil {
		return fmt.Errorf("failed to get consul server address: %w", err)
	}

	checkCmd := "docker ps -a --format '{{.Names}}' | grep -w consul-client || true"
	output, err := im.sshClient.ExecuteCommandWithOutput(checkCmd)
	if err != nil {
		return fmt.Errorf("failed to check consul client container: %w", err)
	}

	if strings.Contains(output, "consul-client") {
		stopCommands := []string{
			"docker stop consul-client",
			"docker rm consul-client",
		}
		for _, cmd := range stopCommands {
			if err := im.sshClient.ExecuteCommand(cmd); err != nil {
				return fmt.Errorf("failed to execute command %q: %w", cmd, err)
			}
		}
	}

	nodeName, err := im.getMachineNodeName()
	if err != nil {
		return fmt.Errorf("failed to get node name: %w", err)
	}

	serverHost := strings.Split(serverAddr, ":")[0]

	runCmd := fmt.Sprintf(`docker run -d \
    --name consul-client \
    --network host \
    --restart unless-stopped \
    %s agent \
    -node=%s \
    -retry-join=%s \
    -client=0.0.0.0 \
    -bind=%s \
    -serf-lan-port=8311 \
    -serf-wan-port=8312 \
    -server-port=8310 \
    -dns-port=8610 \
    -http-port=8510 \
    -datacenter=dc1`,
		im.config.ClusterConfig.ConsulConfig.ConsulImage,
		nodeName,
		serverHost,
		im.server.PublicIP,
	)

	if err := im.sshClient.ExecuteCommand(runCmd); err != nil {
		return fmt.Errorf("failed to start consul client: %w", err)
	}

	return nil
}
