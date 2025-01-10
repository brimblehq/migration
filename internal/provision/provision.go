package provision

import (
	"fmt"

	"github.com/brimblehq/migration/internal/db"
	"github.com/brimblehq/migration/internal/ssh"
	"github.com/brimblehq/migration/internal/types"
	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"
)

type CloudProvisioner interface {
	ProvisionServers(ctx *pulumi.Context, config types.ProvisionServerConfig, tempSSHManager *ssh.TempSSHManager, database *db.PostgresDB) (*types.ProvisionResult, error)
	ValidateConfig(config types.ProvisionServerConfig) error
	GetProviderName() string
}

func ProvisionInfrastructure(ctx *pulumi.Context, provider string, config *types.ProvisionServerConfig, tempSSHManager *ssh.TempSSHManager, database *db.PostgresDB) error {
	provisioner, err := GetProvisioner(provider)
	if err != nil {
		return err
	}

	if err := provisioner.ValidateConfig(*config); err != nil {
		return err
	}

	result, err := provisioner.ProvisionServers(ctx, *config, tempSSHManager, database)
	if err != nil {
		return err
	}

	var serverIDs []pulumi.StringOutput
	var publicIPs []pulumi.StringOutput
	var privateIPs []pulumi.StringOutput
	var keyPaths []string

	for _, server := range result.Servers {
		serverIDs = append(serverIDs, server.ID.ToStringOutput())
		publicIPs = append(publicIPs, server.PublicIP.ToStringOutput())
		privateIPs = append(privateIPs, server.PrivateIP.ToStringOutput())
		keyPaths = append(keyPaths, server.ProvisionKeyPath)
	}

	ctx.Export("serverIds", pulumi.ToStringArrayOutput(serverIDs))
	ctx.Export("publicIps", pulumi.ToStringArrayOutput(publicIPs))
	ctx.Export("privateIps", pulumi.ToStringArrayOutput(privateIPs))
	ctx.Export("keyPaths", pulumi.Any(keyPaths))

	return nil
}

func GetProvisioner(provider string) (CloudProvisioner, error) {
	switch provider {
	case "digitalocean":
		return &DigitalOceanProvisioner{}, nil
	case "aws":
		return &AWSProvisioner{}, nil
	case "gcp":
		return &GCPProvisioner{}, nil
	case "hetzner":
		return &HetznerProvisioner{}, nil
	default:
		return nil, fmt.Errorf("unsupported provider: %s", provider)
	}
}
