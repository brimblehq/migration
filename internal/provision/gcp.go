package provision

import (
	"context"
	"fmt"

	"github.com/brimblehq/migration/internal/ssh"
	"github.com/brimblehq/migration/internal/types"
	"github.com/pulumi/pulumi-gcp/sdk/v6/go/gcp"
	"github.com/pulumi/pulumi-gcp/sdk/v6/go/gcp/compute"
	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"
)

type GCPProvisioner struct{}

func (p *GCPProvisioner) GetProviderName() string {
	return "gcp"
}

func (p *GCPProvisioner) ValidateConfig(config types.ProvisionServerConfig) error {
	if config.Size == "" || config.Region == "" || config.Image == "" {
		return fmt.Errorf("size, region, and image are required for GCP")
	}
	return nil
}

func (p *GCPProvisioner) ProvisionServers(ctx *pulumi.Context, config types.ProvisionServerConfig, tempSSHManager *ssh.TempSSHManager) (*types.ProvisionResult, error) {
	publicKey, err := tempSSHManager.GenerateKeys(context.Background(), false)

	if err != nil {
		return nil, fmt.Errorf("failed to generate keys: %v", err)
	}

	gcpProvider, err := gcp.NewProvider(ctx, "gcp", &gcp.ProviderArgs{
		Credentials: pulumi.String(""),
		Region:      pulumi.String(config.Region),
	})

	if err != nil {
		return nil, fmt.Errorf("failed to create GCP provider: %v", err)
	}

	result := &types.ProvisionResult{
		Provider: p.GetProviderName(),
		Region:   config.Region,
		Servers:  make([]types.ProvisionServerOutput, 0),
	}

	network, err := compute.NewNetwork(ctx, fmt.Sprintf("%s-network", config.Name), &compute.NetworkArgs{
		Name:                  pulumi.String(fmt.Sprintf("%s-network", config.Name)),
		AutoCreateSubnetworks: pulumi.Bool(false),
	}, pulumi.Provider(gcpProvider))

	if err != nil {
		return nil, fmt.Errorf("failed to create network: %v", err)
	}

	subnet, err := compute.NewSubnetwork(ctx, fmt.Sprintf("%s-subnet", config.Name), &compute.SubnetworkArgs{
		Name:        pulumi.String(fmt.Sprintf("%s-subnet", config.Name)),
		IpCidrRange: pulumi.String("10.0.1.0/24"),
		Network:     network.ID(),
		Region:      pulumi.String(config.Region),
	}, pulumi.Provider(gcpProvider))
	if err != nil {
		return nil, fmt.Errorf("failed to create subnet: %v", err)
	}

	_, err = compute.NewFirewall(ctx, fmt.Sprintf("%s-firewall", config.Name), &compute.FirewallArgs{
		Network: network.SelfLink,
		Allows: compute.FirewallAllowArray{
			&compute.FirewallAllowArgs{
				Protocol: pulumi.String("tcp"),
				Ports: pulumi.StringArray{
					pulumi.String("22"),
				},
			},
		},
		SourceRanges: pulumi.StringArray{
			pulumi.String("0.0.0.0/0"),
		},
		TargetTags: pulumi.StringArray{
			pulumi.String(fmt.Sprintf("%s-server", config.Name)),
		},
	}, pulumi.Provider(gcpProvider))

	if err != nil {
		return nil, fmt.Errorf("failed to create firewall rules: %v", err)
	}

	for i := 0; i < config.Count; i++ {
		name := fmt.Sprintf("%s-%d", config.Name, i+1)

		staticIP, err := compute.NewAddress(ctx, fmt.Sprintf("%s-ip", name), &compute.AddressArgs{
			Name:   pulumi.String(fmt.Sprintf("%s-ip", name)),
			Region: pulumi.String(config.Region),
		}, pulumi.Provider(gcpProvider))

		if err != nil {
			return nil, fmt.Errorf("failed to create static IP for %s: %v", name, err)
		}

		internalIP, err := compute.NewAddress(ctx, fmt.Sprintf("%s-internal-ip", name), &compute.AddressArgs{
			Name:        pulumi.String(fmt.Sprintf("%s-internal-ip", name)),
			Subnetwork:  subnet.ID(),
			AddressType: pulumi.String("INTERNAL"),
			Region:      pulumi.String(config.Region),
			Purpose:     pulumi.String("GCE_ENDPOINT"),
		}, pulumi.Provider(gcpProvider))

		if err != nil {
			return nil, fmt.Errorf("failed to create internal IP for %s: %v", name, err)
		}

		sshMetadata := fmt.Sprintf("#!/bin/bash\necho '%s' >> /root/.ssh/authorized_keys", publicKey)

		instance, err := compute.NewInstance(ctx, name, &compute.InstanceArgs{
			Name:        pulumi.String(name),
			MachineType: pulumi.String(config.Size),
			Zone:        pulumi.String(fmt.Sprintf("%s-a", config.Region)), // e.g., us-central1-a
			BootDisk: &compute.InstanceBootDiskArgs{
				InitializeParams: &compute.InstanceBootDiskInitializeParamsArgs{
					Image: pulumi.String(config.Image),
				},
			},
			NetworkInterfaces: compute.InstanceNetworkInterfaceArray{
				&compute.InstanceNetworkInterfaceArgs{
					Network:    network.ID(),
					Subnetwork: subnet.ID(),
					NetworkIp:  internalIP.Address,
					AccessConfigs: compute.InstanceNetworkInterfaceAccessConfigArray{
						&compute.InstanceNetworkInterfaceAccessConfigArgs{
							NatIp: staticIP.Address,
						},
					},
				},
			},
			Tags: pulumi.StringArray{
				pulumi.String(fmt.Sprintf("%s-server", config.Name)),
			},
			Labels: pulumi.StringMap{
				"name":     pulumi.String(name),
				"provider": pulumi.String("brimble"),
			},
			MetadataStartupScript: pulumi.StringPtrInput(pulumi.String(sshMetadata)),
			ServiceAccount: &compute.InstanceServiceAccountArgs{
				Scopes: pulumi.StringArray{
					pulumi.String("cloud-platform"),
				},
			},
		}, pulumi.Provider(gcpProvider))

		if err != nil {
			return nil, fmt.Errorf("failed to create instance %s: %v", name, err)
		}

		serverOutput := types.ProvisionServerOutput{
			ID:        instance.ID().ToStringOutput(),
			PublicIP:  staticIP.Address.ToStringOutput(),
			PrivateIP: internalIP.Address.ToStringOutput(),
		}

		result.Servers = append(result.Servers, serverOutput)

		ctx.Export(fmt.Sprintf("%s-id", name), instance.ID().ToStringOutput())
		ctx.Export(fmt.Sprintf("%s-public-ip", name), staticIP.Address.ToStringOutput())
		ctx.Export(fmt.Sprintf("%s-private-ip", name), internalIP.Address.ToStringOutput())
	}

	return result, nil
}
