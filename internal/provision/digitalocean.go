package provision

import (
	"fmt"
	"os"

	"github.com/brimblehq/migration/internal/types"
	"github.com/pulumi/pulumi-digitalocean/sdk/v4/go/digitalocean"
	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"
)

type DigitalOceanProvisioner struct{}

func (p *DigitalOceanProvisioner) GetProviderName() string {
	return "digitalocean"
}

func (p *DigitalOceanProvisioner) ValidateConfig(config types.ProvisionServerConfig) error {
	if config.Size == "" || config.Region == "" || config.Image == "" {
		return fmt.Errorf("size, region, and image are required for DigitalOcean")
	}
	return nil
}

func (p *DigitalOceanProvisioner) ProvisionServers(ctx *pulumi.Context, config types.ProvisionServerConfig) (*types.ProvisionResult, error) {
	digitaloceanProvider, err := digitalocean.NewProvider(ctx, "digitalocean", &digitalocean.ProviderArgs{
		Token: pulumi.String(os.Getenv("DIGITALOCEAN_ACCESS_TOKEN")),
	})

	if err != nil {
		return nil, fmt.Errorf("failed to create DigitalOcean provider: %v", err)
	}

	result := &types.ProvisionResult{
		Provider: p.GetProviderName(),
		Region:   config.Region,
		Servers:  make([]types.ProvisionServerOutput, 0),
	}

	sshKey, err := digitalocean.NewSshKey(ctx, fmt.Sprintf("%s-key", config.Name), &digitalocean.SshKeyArgs{
		Name:      pulumi.String(fmt.Sprintf("%s-key", config.Name)),
		PublicKey: pulumi.String(config.SSHKey),
	}, pulumi.Provider(digitaloceanProvider))

	if err != nil {
		return nil, fmt.Errorf("failed to create SSH key: %v", err)
	}

	vpc, err := digitalocean.NewVpc(ctx, fmt.Sprintf("%s-vpc", config.Name), &digitalocean.VpcArgs{
		Name:    pulumi.String("brimble-network"),
		Region:  pulumi.String(config.Region),
		IpRange: pulumi.String("10.10.10.0/24"),
	},
		pulumi.Provider(digitaloceanProvider),
		pulumi.IgnoreChanges([]string{"name"}))

	if err != nil {
		return nil, fmt.Errorf("failed to create vpc network : %v", err)
	}

	for i := 0; i < config.Count; i++ {
		name := fmt.Sprintf("%s-%d", config.Name, i+1)
		droplet, err := digitalocean.NewDroplet(ctx, name, &digitalocean.DropletArgs{
			Image:  pulumi.String(config.Image),
			Name:   pulumi.String(fmt.Sprintf("brimble-%s", name)),
			Region: pulumi.String(config.Region),
			Size:   pulumi.String(digitalocean.DropletSlugDropletS1VCPU1GB),
			SshKeys: pulumi.StringArray{
				sshKey.ID(),
			},
			Tags: pulumi.StringArray{
				pulumi.String("brimble"),
			},
			VpcUuid: vpc.ID(),
		}, pulumi.Provider(digitaloceanProvider))

		if err != nil {
			return nil, fmt.Errorf("failed to create droplet %s: %v", name, err)
		}

		serverOutput := types.ProvisionServerOutput{
			ID:        droplet.ID().ToStringOutput(),
			PublicIP:  droplet.Ipv4Address.ToStringOutput(),
			PrivateIP: droplet.Ipv4AddressPrivate.ToStringOutput(),
		}

		result.Servers = append(result.Servers, serverOutput)

		ctx.Export(fmt.Sprintf("%s-id", name), droplet.ID().ToStringOutput())
		ctx.Export(fmt.Sprintf("%s-public-ip", name), droplet.Ipv4Address.ToStringOutput())
		ctx.Export(fmt.Sprintf("%s-private-ip", name), droplet.Ipv4AddressPrivate.ToStringOutput())
	}

	return result, nil
}
