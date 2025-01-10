package manager

import (
	"context"
	"fmt"
	"log"
	"strings"

	"github.com/brimblehq/migration/internal/core"
	"github.com/brimblehq/migration/internal/db"
	"github.com/brimblehq/migration/internal/notification"
	"github.com/brimblehq/migration/internal/ssh"
	"github.com/brimblehq/migration/internal/types"
	"github.com/brimblehq/migration/internal/ui"
)

type ServerSetup struct {
	Client    *ssh.SSHClient
	Server    types.Server
	MachineID string
	Hostname  string
}

func SetupServer(ctx context.Context, setup ServerSetup, config *types.Config, tempSSHManager *ssh.TempSSHManager, useTemp bool,
	decryptedTailScaleValue string, database *db.PostgresDB, licenseKey string,
	clusterRoles *ClusterManager, errorChan chan<- error, notifier *notification.DefaultNotifier) {

	terminalOutput := ui.NewTerminalOutput(setup.Server.Host)
	spinner := ui.NewStepSpinner(setup.Server.Host, terminalOutput)

	defer func() {
		if useTemp {
			if err := tempSSHManager.Cleanup(ctx, setup.Client); err != nil {
				log.Printf("Warning: Failed to cleanup SSH key on %s: %v", setup.Server.Host, err)
			}
		}
		setup.Client.Close()
	}()

	spinner.Start("Validating license")
	licenseResp, err := core.ValidateOrRegisterMachineLicenseKey(licenseKey, strings.TrimSpace(setup.MachineID), strings.TrimSpace(setup.Hostname))
	if err != nil || !licenseResp.Valid {
		spinner.Stop(false)
		errorChan <- fmt.Errorf("invalid license for server (%s)", setup.Server.Host)
		return
	}
	spinner.Stop(true)

	roles := clusterRoles.RoleMapping[setup.Server.Host]

	if err := handleServerRegistration(setup.MachineID, setup.Server, roles, licenseResp, database); err != nil {
		spinner.Stop(false)
		errorChan <- err
		return
	}

	currentStep, err := database.GetServerStep(setup.MachineID, licenseResp.Subscription.ID)
	if err != nil {
		spinner.Stop(false)
		errorChan <- err
		return
	}

	im := NewInstallationManager(setup.Client, setup.Server, roles, config, decryptedTailScaleValue, database, licenseResp)
	if err := executeInstallationSteps(ctx, im, currentStep, setup.MachineID, database, spinner, licenseKey, config.ClusterConfig.Runner.Instance, notifier); err != nil {
		errorChan <- err
		return
	}
}

func handleServerRegistration(machineID string, server types.Server, roles []types.ClusterRole, licenseResp *types.LicenseResponse, database *db.PostgresDB) error {
	role := "client"
	if len(roles) > 1 {
		role = "both"
	}

	return database.RegisterServer(
		machineID,
		server.PublicIP,
		server.PrivateIP,
		role,
		licenseResp.Subscription.ID,
		types.StepInitialized,
	)
}

func executeInstallationSteps(ctx context.Context, im *InstallationManager, currentStep types.ServerStep,
	machineID string, database *db.PostgresDB, spinner *ui.StepSpinner, licenseKey string, instances int, notifier *notification.DefaultNotifier) error {

	steps := []struct {
		name    string
		fn      func() error
		step    types.ServerStep
		require types.ServerStep
	}{
		{"Verifying machine requirements", im.VerifyMachineRequirement, types.StepVerified, types.StepInitialized},
		{"Installing base packages", im.InstallBasePackages, types.StepBaseInstalled, types.StepVerified},
		{"Setting up Consul client", im.SetupConsul, types.StepConsulSetup, types.StepBaseInstalled},
		{"Setting up Nomad", im.SetupNomad, types.StepNomadSetup, types.StepConsulSetup},
		{"Setting up monitoring", im.SetupMonitoring, types.StepMonitoringSetup, types.StepNomadSetup},
		{
			"Starting runner",
			func() error { return im.StartRunner(licenseKey, instances) },
			types.StepRunnerStarted,
			types.StepMonitoringSetup,
		},
	}

	stepOrder := map[types.ServerStep]int{
		types.StepInitialized: 0, types.StepVerified: 1, types.StepBaseInstalled: 2,
		types.StepConsulSetup: 3, types.StepNomadSetup: 4, types.StepMonitoringSetup: 5,
		types.StepRunnerStarted: 6, types.StepCompleted: 7,
	}

	currentStepOrder := stepOrder[currentStep]

	for _, step := range steps {
		select {
		case <-ctx.Done():
			return nil
		default:
			requiredStepOrder := stepOrder[step.require]
			currentLoopStepOrder := stepOrder[step.step]

			if currentStepOrder < currentLoopStepOrder && currentStepOrder >= requiredStepOrder {
				spinner.Start(step.name)
				if err := step.fn(); err != nil {
					spinner.Stop(false)
					if notifier != nil {
						_ = notifier.Send("Installation Error", fmt.Sprintf("Error during %s on %s: %v", step.name, im.server.Host, err))
					}
					return fmt.Errorf("error during (%s): %v", step.name, err)
				}
				if err := database.UpdateServerStep(machineID, step.step); err != nil {
					spinner.Stop(false)
					return fmt.Errorf("error updating step: %v", err)
				}
				currentStep = step.step
				currentStepOrder = stepOrder[currentStep]
				spinner.Stop(true)
				if notifier != nil {
					_ = notifier.Send(
						"Installation Progress",
						fmt.Sprintf("Completed %s on %s", step.name, im.server.Host),
					)
				}
			}
		}
	}

	return database.UpdateServerStep(machineID, types.StepCompleted)
}
