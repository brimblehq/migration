package manager

import (
	"fmt"
	"time"
)

func (im *InstallationManager) SetupMonitoring() error {
	if im.server.Host == im.config.Servers[0].Host {
		return nil
	}
	return im.setupMonitoringServer()
}

func (im *InstallationManager) setupMonitoringServer() error {
	prometheusCmd := `docker run -d \
        --name prometheus \
        --restart=unless-stopped \
        -v /etc/prometheus-config.yml:/etc/prometheus/prometheus.yml \
        -p 9090:9090 \
        prom/prometheus:latest`

	grafanaCmd := `docker run -d \
        --name grafana \
        --restart=unless-stopped \
        -p 3000:3000 \
        grafana/grafana:latest`

	commands := []string{
		im.generatePrometheusConfig(),
		prometheusCmd,
		grafanaCmd,
	}

	for _, cmd := range commands {
		if err := im.sshClient.ExecuteCommand(cmd); err != nil {
			return fmt.Errorf("monitoring server setup failed: %v", err)
		}
	}

	if err := im.waitForNomadCluster(); err != nil {
		return err
	}

	return im.deployMonitoringJobs()
}

func (im *InstallationManager) waitForNomadCluster() error {
	maxRetries := 30
	retryInterval := 10 * time.Second

	for i := 0; i < maxRetries; i++ {
		if err := im.checkNomadStatus(); err == nil {
			return nil
		}
		time.Sleep(retryInterval)
	}
	return fmt.Errorf("nomad cluster not ready after %v seconds", maxRetries*int(retryInterval.Seconds()))
}

func (im *InstallationManager) checkNomadStatus() error {
	cmd := "nomad status"
	return im.sshClient.ExecuteCommand(cmd)
}

func (im *InstallationManager) deployMonitoringJobs() error {
	jobs := []string{
		im.generateNodeExporterJob(),
		im.generateCadvisorJob(),
		im.generateLokiJob(),
		im.generatePromtailJob(),
	}

	for _, jobSpec := range jobs {
		if err := im.deployNomadJob(jobSpec); err != nil {
			return fmt.Errorf("job deployment failed: %v", err)
		}
	}
	return nil
}

func (im *InstallationManager) deployNomadJob(jobSpec string) error {
	tempFile := fmt.Sprintf("/tmp/job-%d.hcl", time.Now().Unix())

	writeCmd := fmt.Sprintf(`cat << 'EOF' > %s
%s
EOF`, tempFile, jobSpec)

	if err := im.sshClient.ExecuteCommand(writeCmd); err != nil {
		return err
	}

	deployCmd := fmt.Sprintf("nomad job run %s", tempFile)

	if err := im.sshClient.ExecuteCommand(deployCmd); err != nil {
		return err
	}

	return im.sshClient.ExecuteCommand(fmt.Sprintf("rm %s", tempFile))
}

func (im *InstallationManager) generatePrometheusConfig() string {
	return ``
}

func (im *InstallationManager) generateNodeExporterJob() string {
	return ``
}

func (im *InstallationManager) generateCadvisorJob() string {
	return ``
}

func (im *InstallationManager) generateLokiJob() string {
	return ``
}

func (im *InstallationManager) generatePromtailJob() string {
	return ``
}
