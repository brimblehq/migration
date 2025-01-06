package manager

import (
	"crypto/sha256"
	"fmt"
	"strconv"
	"strings"
)

func (im *InstallationManager) StartRunner(licenseKey string, instances string) error {
	instanceCount, err := strconv.Atoi(instances)
	if err != nil {
		return fmt.Errorf("invalid instances value: %v", err)
	}

	envVars := enivornmentVariablesBuilder()

	err = createSystemDaemonSetup(im, envVars, licenseKey, instanceCount)
	if err != nil {
		return fmt.Errorf("unable to setup runner on machine")
	}

	command := fmt.Sprintf("runner --license-key %s --instances %d", licenseKey, instanceCount)
	fmt.Println(command)
	return im.sshClient.ExecuteCommand(command)
}

func createSystemDaemonSetup(im *InstallationManager, envVars map[string]string, licenseKey string, instances int) error {
	hash := sha256.Sum256([]byte("brimble-runner"))

	hashedName := fmt.Sprintf("service-%x", hash[:8])

	commands := []string{
		"sudo useradd -r -s /bin/false brimble",
		fmt.Sprintf("sudo mkdir -p /opt/%s/runner", hashedName),
		fmt.Sprintf("sudo mkdir -p /var/run/%s", hashedName),
		"sudo mkdir -p /home/brimble/.pm2",
		fmt.Sprintf("sudo chown -R brimble:brimble /opt/%s", hashedName),
		fmt.Sprintf("sudo chown -R brimble:brimble /var/run/%s", hashedName),
		"sudo chown -R brimble:brimble /home/brimble/.pm2",
	}

	for _, cmd := range commands {
		if err := im.sshClient.ExecuteCommand(cmd); err != nil {
			return fmt.Errorf("failed to execute command %q: %v", cmd, err)
		}
	}

	if err := createEnvFile(im, envVars, hashedName); err != nil {
		return err
	}

	if err := createSystemdService(im, licenseKey, instances, hashedName); err != nil {
		return err
	}

	return nil
}

func createEnvFile(im *InstallationManager, envVars map[string]string, hashedName string) error {
	var envContent strings.Builder

	for key, value := range envVars {
		envContent.WriteString(fmt.Sprintf("%s=%s\n", key, value))
	}

	cmd := fmt.Sprintf("sudo bash -c 'cat > /opt/%s/runner/.env << EOL\n%sEOL'",
		hashedName, envContent.String())
	if err := im.sshClient.ExecuteCommand(cmd); err != nil {
		return fmt.Errorf("failed to create environment file: %v", err)
	}

	if err := im.sshClient.ExecuteCommand(fmt.Sprintf("sudo chown brimble:brimble /opt/%s/runner/.env", hashedName)); err != nil {
		return fmt.Errorf("failed to set environment file permissions: %v", err)
	}

	return nil
}

func createSystemdService(im *InstallationManager, licenseKey string, instances int, hashedName string) error {
	serviceContent := `[Unit]
Description=Brimble Runner Service (%s)
After=network.target

[Service]
Type=forking
User=brimble
Group=brimble
WorkingDirectory=/opt/%s/runner
EnvironmentFile=/opt/%s/runner/.env
Environment=NODE_ENV=production
Environment=PM2_HOME=/home/brimble/.pm2
Environment=RUNNER_MODE=service
ExecStart=/usr/local/bin/runner --license-key %s --instances %d --service
ExecStop=/usr/local/bin/pm2 delete runner
Restart=always
RestartSec=10

[Install]
WantedBy=multi-user.target`

	serviceContent = fmt.Sprintf(serviceContent,
		hashedName,
		hashedName,
		hashedName,
		licenseKey,
		instances)

	cmd := fmt.Sprintf("sudo bash -c 'cat > /etc/systemd/system/%s.service << EOL\n%s\nEOL'",
		hashedName, serviceContent)
	if err := im.sshClient.ExecuteCommand(cmd); err != nil {
		return fmt.Errorf("failed to create service file: %v", err)
	}

	commands := []string{
		"sudo systemctl daemon-reload",
		fmt.Sprintf("sudo systemctl enable %s.service", hashedName),
	}

	for _, cmd := range commands {
		if err := im.sshClient.ExecuteCommand(cmd); err != nil {
			return fmt.Errorf("failed to execute command %q: %v", cmd, err)
		}
	}

	return nil
}

func enivornmentVariablesBuilder() map[string]string {
	envVars := map[string]string{
		"DEPLOY_URL":          "",
		"DOCKER_BASEURL":      "",
		"DOCKER_REGISTRY":     "",
		"DOCKER_HUB_PASSWORD": "",
		"DOCKER_HUB_USERNAME": "",
		"ENCRYPTION_KEY":      "",
		"NOMAD_ACL_TOKEN":     "",
		"REDIS_HOST":          "",
		"REDIS_PASSWORD":      "",
		"REDIS_PORT":          "",
		"REDIS_TLS":           "",
		"SENTRY_DSN":          "",
	}

	return envVars
}
