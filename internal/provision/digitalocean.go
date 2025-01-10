package provision

import (
	"context"
	"fmt"
	"os"

	"github.com/brimblehq/migration/internal/db"
	"github.com/brimblehq/migration/internal/helpers"
	"github.com/brimblehq/migration/internal/ssh"
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

func (p *DigitalOceanProvisioner) getOrCreateVPC(ctx *pulumi.Context, vpcName, region string, provider *digitalocean.Provider) (*digitalocean.Vpc, error) {
	existingVPC, err := digitalocean.LookupVpc(ctx, &digitalocean.LookupVpcArgs{
		Name: &vpcName,
	}, pulumi.Provider(provider))

	if err == nil && existingVPC != nil {
		vpc, err := digitalocean.NewVpc(ctx, vpcName, &digitalocean.VpcArgs{
			Name:    pulumi.String(vpcName),
			Region:  pulumi.String(region),
			IpRange: pulumi.String(existingVPC.IpRange),
		},
			pulumi.Provider(provider),
			pulumi.RetainOnDelete(true),
			pulumi.Import(pulumi.ID(existingVPC.Id)))

		if err != nil {
			return nil, fmt.Errorf("failed to reference existing vpc: %v", err)
		}
		return vpc, nil
	}

	vpc, err := digitalocean.NewVpc(ctx, vpcName, &digitalocean.VpcArgs{
		Name:    pulumi.String(vpcName),
		Region:  pulumi.String(region),
		IpRange: pulumi.String("10.10.10.0/24"),
	}, pulumi.Provider(provider))

	if err != nil {
		return nil, fmt.Errorf("failed to create new vpc: %v", err)
	}

	return vpc, nil
}

func (p *DigitalOceanProvisioner) ProvisionServers(ctx *pulumi.Context, config types.ProvisionServerConfig, tempSSHManager *ssh.TempSSHManager, database *db.PostgresDB) (*types.ProvisionResult, error) {
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

	publicKey, keyPath, err := tempSSHManager.GenerateKeys(context.Background())

	if err != nil {
		return nil, fmt.Errorf("failed to generate keys: %v", err)
	}

	sshKey, err := digitalocean.NewSshKey(ctx, sshKeyName, &digitalocean.SshKeyArgs{
		Name:      pulumi.String(sshKeyName),
		PublicKey: pulumi.String(publicKey),
	}, pulumi.Provider(digitaloceanProvider))

	if err != nil {
		return nil, fmt.Errorf("failed to create SSH key: %v", err)
	}

	vpc, err := p.getOrCreateVPC(ctx, networkName, config.Region, digitaloceanProvider)
	if err != nil {
		return nil, fmt.Errorf("failed to get or create vpc: %v", err)
	}

	for i := 0; i < config.Count; i++ {
		reference, err := helpers.GenerateUniqueReference(database)

		if err != nil {
			return nil, fmt.Errorf("failed to generate unique reference: %v", err)
		}

		name := fmt.Sprintf("brimble-instance-%s-%d", reference, i+1)

		droplet, err := digitalocean.NewDroplet(ctx, name, &digitalocean.DropletArgs{
			Image:  pulumi.String(config.Image),
			Name:   pulumi.String(name),
			Region: pulumi.String(config.Region),
			Size:   pulumi.String(digitalocean.DropletSlugDropletS1VCPU1GB),
			SshKeys: pulumi.StringArray{
				sshKey.ID(),
			},
			Tags: pulumi.StringArray{
				pulumi.String("brimble"),
			},
			VpcUuid: vpc.ID(),
		}, pulumi.Provider(digitaloceanProvider), pulumi.RetainOnDelete(true))

		if err != nil {
			return nil, fmt.Errorf("failed to create droplet %s: %v", name, err)
		}

		serverOutput := types.ProvisionServerOutput{
			ID:               droplet.ID().ToStringOutput(),
			PublicIP:         droplet.Ipv4Address.ToStringOutput(),
			PrivateIP:        droplet.Ipv4AddressPrivate.ToStringOutput(),
			ProvisionKeyPath: keyPath,
		}

		result.Servers = append(result.Servers, serverOutput)
	}

	return result, nil
}
