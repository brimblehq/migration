package manager

import (
	"encoding/base64"
	"fmt"
	"path/filepath"
	"sort"
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

	jobOrder := map[string]int{
		"loki.nomad":          1,
		"cadvisor.nomad":      2,
		"node-exporter.nomad": 3,
		"promtail.nomad":      4,
		"prometheus.nomad":    5,
		"grafana.nomad":       6,
	}

	var orderedJobs []string
	for _, entry := range entries {
		if !strings.HasSuffix(entry.Name(), ".nomad") {
			continue
		}
		orderedJobs = append(orderedJobs, entry.Name())
	}

	sort.Slice(orderedJobs, func(i, j int) bool {
		orderI := jobOrder[orderedJobs[i]]
		orderJ := jobOrder[orderedJobs[j]]
		if orderI == 0 {
			orderI = len(jobOrder) + 1
		}
		if orderJ == 0 {
			orderJ = len(jobOrder) + 1
		}
		return orderI < orderJ
	})

	for _, jobName := range orderedJobs {
		fmt.Printf("Deploying %s...\n", jobName)

		jobContent, err := im.files.ReadFile(filepath.Join("monitoring", jobName))
		if err != nil {
			return fmt.Errorf("failed to read job file %s: %v", jobName, err)
		}

		modifiedJob := im.modifyServiceName(string(jobContent), machineID)

		encodedContent := base64.StdEncoding.EncodeToString([]byte(modifiedJob))
		tempFile := fmt.Sprintf("/tmp/%s", jobName)

		createFileCmd := fmt.Sprintf(`echo '%s' | base64 -d > %s`, encodedContent, tempFile)
		if err := im.sshClient.ExecuteCommand(createFileCmd); err != nil {
			return fmt.Errorf("failed to create job file: %v", err)
		}

		if err := im.sshClient.ExecuteCommand(fmt.Sprintf("nomad job run %s", tempFile)); err != nil {
			return fmt.Errorf("failed to run job %s: %v", jobName, err)
		}

		// if err := im.waitForJobHealth(jobName); err != nil {
		// 	return fmt.Errorf("job %s failed to become healthy: %v", jobName, err)
		// }

		fmt.Printf("%s deployed successfully\n", jobName)
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
