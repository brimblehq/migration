package provision

import (
	"fmt"

	"github.com/brimblehq/migration/internal/types"
	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"
)

func ProvisionInfrastructure(ctx *pulumi.Context, provider string, config *types.ProvisionServerConfig) error {
	provisioner, err := GetProvisioner(provider)
	if err != nil {
		return err
	}

	if err := provisioner.ValidateConfig(*config); err != nil {
		return err
	}

	result, err := provisioner.ProvisionServers(ctx, *config)
	if err != nil {
		return err
	}

	var serverIDs []pulumi.StringOutput
	var publicIPs []pulumi.StringOutput
	var privateIPs []pulumi.StringOutput

	for _, server := range result.Servers {
		serverIDs = append(serverIDs, server.ID.ToStringOutput())
		publicIPs = append(publicIPs, server.PublicIP)
		privateIPs = append(privateIPs, server.PrivateIP)
	}

	ctx.Export("serverIds", pulumi.ToStringArrayOutput(serverIDs))
	ctx.Export("publicIps", pulumi.ToStringArrayOutput(publicIPs))
	ctx.Export("privateIps", pulumi.ToStringArrayOutput(privateIPs))

	return nil
}

func GetProvisioner(provider string) (types.CloudProvisioner, error) {
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
