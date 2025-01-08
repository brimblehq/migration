package infra

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/brimblehq/migration/internal/db"
	"github.com/brimblehq/migration/internal/helpers"
	"github.com/brimblehq/migration/internal/ssh"
	"github.com/brimblehq/migration/internal/ui"
	"github.com/pulumi/pulumi/sdk/v3/go/auto"
	"github.com/pulumi/pulumi/sdk/v3/go/auto/optup"
	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"

	provisioner "github.com/brimblehq/migration/internal/provision"
)

func ProvisionInfrastructure(licenseKey string, database *db.PostgresDB, tempSSHManager *ssh.TempSSHManager) error {
	provider, setup, err := ui.InteractiveProvisioning(database)
	if err != nil {
		return fmt.Errorf("failed to provision servers: %v", err)
	}

	terminalOutput := ui.NewTerminalOutput("Server Provisioning")
	spinner := ui.NewStepSpinner("Server Provisioning", terminalOutput)
	spinner.Start("Provisioning your servers... This may take a few minutes")

	var outputBuffer bytes.Buffer
	workspaceOpts := []auto.LocalWorkspaceOption{
		auto.WorkDir("."),
		auto.PulumiHome("./.pulumi"),
	}

	os.Setenv("PULUMI_HOME", "./.pulumi")

	stackName := fmt.Sprintf("%s-%s", provider, licenseKey)

	s, err := auto.UpsertStackInlineSource(context.Background(), stackName, "brimble-provision",
		func(pulumiCtx *pulumi.Context) error {
			return provisioner.ProvisionInfrastructure(pulumiCtx, provider, setup, tempSSHManager)
		},
		workspaceOpts[0],
	)

	if err != nil {
		spinner.Stop(false)
		return fmt.Errorf("failed to initialize infrastructure: %v", err)
	}

	upRes, err := s.Up(context.Background(), optup.ProgressStreams(&outputBuffer))
	if err != nil {
		spinner.Stop(false)
		return handleProvisioningError(err)
	}

	spinner.Stop(true)
	helpers.PrintSuccessMessage(upRes)
	return nil
}

func handleProvisioningError(err error) error {
	errorMsg := err.Error()
	if strings.Contains(errorMsg, "404") {
		return fmt.Errorf("resource not found. This might be due to a previous failed deployment")
	}
	if strings.Contains(errorMsg, "unauthorized") {
		return fmt.Errorf("authentication failed. Please check your credentials")
	}
	return fmt.Errorf("failed to provision servers: %v", helpers.GetCleanErrorMessage(err))
}
