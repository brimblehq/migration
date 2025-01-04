package manager

import (
	"fmt"
	"path/filepath"
	"strings"
	"time"
)

func (im *InstallationManager) SetupMonitoring() error {
	if err := im.waitForNomadCluster(); err != nil {
		return fmt.Errorf("nomad not ready: %v", err)
	}

	machineID, err := im.getMachineID()
	if err != nil {
		return fmt.Errorf("failed to get machine-id: %v", err)
	}

	entries, err := im.files.ReadDir("monitoring")
	if err != nil {
		return fmt.Errorf("failed to read monitoring directory: %v", err)
	}

	for _, entry := range entries {
		if !strings.HasSuffix(entry.Name(), ".nomad") {
			continue
		}

		jobContent, err := im.files.ReadFile(filepath.Join("monitoring", entry.Name()))
		if err != nil {
			return fmt.Errorf("failed to read job file %s: %v", entry.Name(), err)
		}

		modifiedJob := im.modifyServiceName(string(jobContent), machineID)

		tempFile := fmt.Sprintf("/tmp/%s", entry.Name())
		if err := im.sshClient.ExecuteCommand(fmt.Sprintf(`echo '%s' > %s`, modifiedJob, tempFile)); err != nil {
			return fmt.Errorf("failed to create temporary job file: %v", err)
		}

		if err := im.sshClient.ExecuteCommand(fmt.Sprintf("nomad job run %s", tempFile)); err != nil {
			return fmt.Errorf("failed to run job %s: %v", entry.Name(), err)
		}

		im.sshClient.ExecuteCommand(fmt.Sprintf("rm %s", tempFile))
	}

	return nil
}

func (im *InstallationManager) modifyServiceName(jobContent string, machineID string) string {
	lines := strings.Split(jobContent, "\n")
	for i, line := range lines {
		if strings.Contains(line, "name = ") && strings.Contains(line, "service") {
			parts := strings.Split(line, "=")
			if len(parts) != 2 {
				continue
			}

			serviceName := strings.Trim(strings.TrimSpace(parts[1]), `"'`)

			newServiceName := fmt.Sprintf(`      name = "%s-%s"`, serviceName, machineID)

			lines[i] = newServiceName
		}
	}

	return strings.Join(lines, "\n")
}

func (im *InstallationManager) waitForNomadCluster() error {
	maxRetries := 30
	for i := 0; i < maxRetries; i++ {
		if err := im.sshClient.ExecuteCommand("nomad status"); err == nil {
			return nil
		}
		time.Sleep(10 * time.Second)
	}
	return fmt.Errorf("nomad not ready after %d attempts", maxRetries)
}

func (im *InstallationManager) getMachineID() (string, error) {
	cmd := "cat /etc/machine-id"

	session, err := im.sshClient.Client.NewSession()
	if err != nil {
		return "", err
	}
	defer session.Close()

	output, err := session.Output(cmd)
	if err != nil {
		return "", err
	}

	return strings.TrimSpace(string(output)), nil
}
