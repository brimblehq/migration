package infra

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/brimblehq/migration/internal/db"
	"github.com/brimblehq/migration/internal/helpers"
	"github.com/brimblehq/migration/internal/notification"
	"github.com/brimblehq/migration/internal/ssh"
	"github.com/brimblehq/migration/internal/ui"
	"github.com/pulumi/pulumi/sdk/v3/go/auto"
	"github.com/pulumi/pulumi/sdk/v3/go/auto/optup"
	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"

	provisioner "github.com/brimblehq/migration/internal/provision"
)

func ProvisionInfrastructure(licenseKey string, maxDevices int, database *db.PostgresDB, tempSSHManager *ssh.TempSSHManager, notifier *notification.DefaultNotifier) error {
	provider, setup, err := ui.InteractiveProvisioning(database, maxDevices)

	if err != nil {
		return fmt.Errorf("failed to provision servers: %v", err)
	}

	terminalOutput := ui.NewTerminalOutput("Server Provisioning")

	spinner := ui.NewStepSpinner("Server Provisioning", terminalOutput)

	spinner.Start("Provisioning your servers... This may take a few minutes, hang tight 🚀")

	var outputBuffer bytes.Buffer

	stackName := fmt.Sprintf("%s-%s", provider, licenseKey)

	s, err := auto.UpsertStackInlineSource(context.Background(), stackName, "brimble-provision", func(pulumiCtx *pulumi.Context) error {
		err := provisioner.ProvisionInfrastructure(pulumiCtx, provider, setup, tempSSHManager, database)
		if err != nil {
			return err
		}
		return nil
	})

	if err != nil {
		spinner.Stop(false)
		return fmt.Errorf("failed to initialize infrastructure: %v", err)
	}

	upRes, err := s.Up(context.Background(), optup.ProgressStreams(&outputBuffer))

	if err != nil {
		spinner.Stop(false)
		return handleProvisioningError(provider, err)
	}

	publicConfig, publicConfigExists := upRes.Outputs["publicIps"]
	privateIps, privateIpsExists := upRes.Outputs["privateIps"]
	keyPaths, keyPathsExists := upRes.Outputs["keyPaths"]

	if publicConfigExists && privateIpsExists && keyPathsExists {
		generatedConfig, err := helpers.GenerateProvisionedConfig(publicConfig, privateIps, keyPaths)

		if err != nil {
			fmt.Printf("Error generating config: %v\n", err)
			return err
		}

		jsonData, err := json.MarshalIndent(generatedConfig, "", "  ")

		if err != nil {
			fmt.Printf("Error marshaling to JSON: %v\n", err)
			return err
		}

		filePath, err := helpers.SaveConfigToFile(generatedConfig)

		if err != nil {
			fmt.Printf("Error saving config to file: %v\n", err)
			return err
		}

		fmt.Println(string(jsonData))

		helpers.PrintSuccessMessage(filePath, generatedConfig)
	}

	notifier.Send("Provision Complete", "Your machine instances have been provisioned successfully")

	spinner.Stop(true)

	return nil
}

func handleProvisioningError(provider string, err error) error {
	errorMsg := err.Error()

	if strings.Contains(errorMsg, "404") {
		return fmt.Errorf("resource not found. This might be due to a previous failed deployment")
	}

	if strings.Contains(errorMsg, "403") && provider == "gcp" {
		return fmt.Errorf("compute api has not been enabled or you have a forbidden access to this resource")
	}

	if strings.Contains(errorMsg, "Ask a project owner to grant you") && provider == "gcp" {
		return fmt.Errorf("forbidden access, ask the project owner to grant person to the iam.serviceAccountUser role on the service account")
	}

	if strings.Contains(errorMsg, "unauthorized") {
		return fmt.Errorf("authentication failed. Please check your credentials")
	}
	return fmt.Errorf("failed to provision servers: %v", helpers.GetCleanErrorMessage(err))
}
