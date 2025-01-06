package manager

import (
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
	commands := []string{
		"sudo useradd -r -s /bin/false brimble",
		"sudo mkdir -p /opt/brimble/runner",
		"sudo mkdir -p /var/run/brimble",
		"sudo mkdir -p /home/brimble/.pm2",
		"sudo chown -R brimble:brimble /opt/brimble",
		"sudo chown -R brimble:brimble /var/run/brimble",
		"sudo chown -R brimble:brimble /home/brimble/.pm2",
	}

	for _, cmd := range commands {
		if err := im.sshClient.ExecuteCommand(cmd); err != nil {
			return fmt.Errorf("failed to execute command %q: %v", cmd, err)
		}
	}

	if err := createEnvFile(im, envVars); err != nil {
		return err
	}

	if err := createSystemdService(im, licenseKey, instances); err != nil {
		return err
	}

	return nil
}

func createEnvFile(im *InstallationManager, envVars map[string]string) error {
	var envContent strings.Builder

	for key, value := range envVars {
		envContent.WriteString(fmt.Sprintf("%s=%s\n", key, value))
	}

	cmd := fmt.Sprintf("sudo bash -c 'cat > /opt/brimble/runner/.env << EOL\n%sEOL'", envContent.String())
	if err := im.sshClient.ExecuteCommand(cmd); err != nil {
		return fmt.Errorf("failed to create environment file: %v", err)
	}

	if err := im.sshClient.ExecuteCommand("sudo chown brimble:brimble /opt/brimble/runner/.env"); err != nil {
		return fmt.Errorf("failed to set environment file permissions: %v", err)
	}

	return nil
}

func createSystemdService(im *InstallationManager, licenseKey string, instances int) error {
	serviceContent := `[Unit]
Description=Brimble Runner Service
After=network.target

[Service]
Type=forking
User=brimble
Group=brimble
WorkingDirectory=/opt/brimble/runner
EnvironmentFile=/opt/brimble/runner/.env
Environment=NODE_ENV=production
Environment=PM2_HOME=/home/brimble/.pm2
Environment=RUNNER_MODE=service
ExecStart=/usr/local/bin/runner --license-key %s --instances %d --service
ExecStop=/usr/local/bin/pm2 delete runner
Restart=always
RestartSec=10

[Install]
WantedBy=multi-user.target`

	cmd := fmt.Sprintf("sudo bash -c 'cat > /etc/systemd/system/brimble-runner.service << EOL\n%s\nEOL'",
		fmt.Sprintf(serviceContent, licenseKey, instances))
	if err := im.sshClient.ExecuteCommand(cmd); err != nil {
		return fmt.Errorf("failed to create service file: %v", err)
	}

	commands := []string{
		"sudo systemctl daemon-reload",
		"sudo systemctl enable brimble-runner.service",
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
		"AUTH_TOKEN":                   "",
		"BRIMBLE_BUILD_ID":             "",
		"BRIMBLE_BUILD_WEBHOOK_SECRET": "",
		"CENTRIFUGO_API_KEY":           "",
		"CENTRIFUGO_SUBSCRIBER_TOKEN":  "",
		"CONSUL_TOKEN":                 "",
		"DEPLOY_URL":                   "",
		"DOCKER_BASEURL":               "",
		"DOCKER_REGISTRY":              "",
		"DOCKER_HUB_PASSWORD":          "",
		"DOCKER_HUB_USERNAME":          "",
		"ENCRYPTION_KEY":               "",
		"NOMAD_ACL_TOKEN":              "",
		"REDIS_HOST":                   "",
		"REDIS_PASSWORD":               "",
		"REDIS_PORT":                   "",
		"REDIS_TLS":                    "",
		"SENTRY_DSN":                   "",
	}

	return envVars
}
