package provision

import (
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/brimblehq/migration/internal/db"
	"github.com/brimblehq/migration/internal/helpers"
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

func (p *GCPProvisioner) ProvisionServers(ctx *pulumi.Context, config types.ProvisionServerConfig, tempSSHManager *ssh.TempSSHManager, database *db.PostgresDB) (*types.ProvisionResult, error) {
	publicKey, keyPath, err := tempSSHManager.GenerateKeys(context.Background())

	if err != nil {
		return nil, fmt.Errorf("failed to generate keys: %v", err)
	}

	gcpProvider, err := gcp.NewProvider(ctx, "gcp", &gcp.ProviderArgs{
		Credentials: pulumi.String(os.Getenv("GOOGLE_APPLICATION_CREDENTIALS")),
		Project:     pulumi.String(os.Getenv("GOOGLE_CLOUD_PROJECT")),
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

	networkName := "network-brimble"

	network, err := compute.NewNetwork(ctx, networkName, &compute.NetworkArgs{
		Name:                  pulumi.String(networkName),
		AutoCreateSubnetworks: pulumi.Bool(true),
	}, pulumi.Provider(gcpProvider), pulumi.ReplaceOnChanges([]string{
		"Name", "AutoCreateSubnetworks",
	}))

	if err != nil {
		return nil, fmt.Errorf("failed to create network: %v", err)
	}

	_, err = compute.NewFirewall(ctx, "icmp-fw-in-brimble", &compute.FirewallArgs{
		Network: network.ID(),
		Allows: compute.FirewallAllowArray{
			&compute.FirewallAllowArgs{
				Protocol: pulumi.String("icmp"),
			},
		},
		Direction:    pulumi.String("INGRESS"),
		Priority:     pulumi.Int(65534),
		SourceRanges: pulumi.StringArray{pulumi.String("0.0.0.0/0")},
	})

	if err != nil {
		return nil, fmt.Errorf("failed to create icmp firewall rule: %v", err)
	}

	_, err = compute.NewFirewall(ctx, "internal-fw-in-brimble", &compute.FirewallArgs{
		Network: network.ID(),
		Allows: compute.FirewallAllowArray{
			&compute.FirewallAllowArgs{
				Protocol: pulumi.String("tcp"),
				Ports:    pulumi.StringArray{pulumi.String("0-65535")},
			},
			&compute.FirewallAllowArgs{
				Protocol: pulumi.String("udp"),
				Ports:    pulumi.StringArray{pulumi.String("0-65535")},
			},
			&compute.FirewallAllowArgs{
				Protocol: pulumi.String("icmp"),
			},
		},
		Direction:    pulumi.String("INGRESS"),
		Priority:     pulumi.Int(65534),
		SourceRanges: pulumi.StringArray{pulumi.String("10.128.0.0/9")},
	})

	if err != nil {
		return nil, fmt.Errorf("failed to create internal firewall rule: %v", err)
	}

	_, err = compute.NewFirewall(ctx, "rdp-fw-in-brimble", &compute.FirewallArgs{
		Network: network.ID(),
		Allows: compute.FirewallAllowArray{
			&compute.FirewallAllowArgs{
				Protocol: pulumi.String("tcp"),
				Ports:    pulumi.StringArray{pulumi.String("3389")},
			},
		},
		Direction:    pulumi.String("INGRESS"),
		Priority:     pulumi.Int(65534),
		SourceRanges: pulumi.StringArray{pulumi.String("0.0.0.0/0")},
	})

	if err != nil {
		return nil, fmt.Errorf("failed to create RDP firewall rule: %v", err)
	}

	_, err = compute.NewFirewall(ctx, "ssh-fw-in-brimble", &compute.FirewallArgs{
		Network: network.ID(),
		Allows: compute.FirewallAllowArray{
			&compute.FirewallAllowArgs{
				Protocol: pulumi.String("tcp"),
				Ports:    pulumi.StringArray{pulumi.String("22")},
			},
		},
		Direction:    pulumi.String("INGRESS"),
		Priority:     pulumi.Int(65534),
		SourceRanges: pulumi.StringArray{pulumi.String("0.0.0.0/0")},
	})

	if err != nil {
		return nil, fmt.Errorf("failed to create SSH firewall rule: %v", err)
	}

	for i := 0; i < config.Count; i++ {
		reference, err := helpers.GenerateUniqueReference(database)

		if err != nil {
			return nil, fmt.Errorf("failed to generate unique reference: %v", err)
		}

		name := fmt.Sprintf("instance-%s-brimble-%d", strings.ToLower(reference), i+1)

		staticIP, err := compute.NewAddress(ctx, fmt.Sprintf("ip-%s-ext-%d", name, i+1), &compute.AddressArgs{
			Name:   pulumi.String(fmt.Sprintf("ip-%s-ext-%d", name, i+1)),
			Region: pulumi.String(config.Region),
		}, pulumi.Provider(gcpProvider))

		if err != nil {
			return nil, fmt.Errorf("failed to create static IP for %s: %v", name, err)
		}

		sshMetadata := fmt.Sprintf("#!/bin/bash\necho '%s' >> /root/.ssh/authorized_keys", publicKey)

		instance, err := compute.NewInstance(ctx, name, &compute.InstanceArgs{
			Name:        pulumi.String(name),
			MachineType: pulumi.String(config.Size),
			Zone:        pulumi.String(fmt.Sprintf("%s-a", config.Region)),
			BootDisk: &compute.InstanceBootDiskArgs{
				InitializeParams: &compute.InstanceBootDiskInitializeParamsArgs{
					Image: pulumi.String(config.Image),
				},
			},
			NetworkInterfaces: compute.InstanceNetworkInterfaceArray{
				&compute.InstanceNetworkInterfaceArgs{
					Network: network.ID(),
					AccessConfigs: compute.InstanceNetworkInterfaceAccessConfigArray{
						&compute.InstanceNetworkInterfaceAccessConfigArgs{
							NatIp: staticIP.Address,
						},
					},
				},
			},
			Tags: pulumi.StringArray{
				pulumi.String(tagName),
			},
			Labels: pulumi.StringMap{
				"name":     pulumi.String(name),
				"provider": pulumi.String("brimble"),
			},
			Metadata: pulumi.StringMap{
				"enable-oslogin": pulumi.String("false"),
				"ssh-keys":       pulumi.String(fmt.Sprintf("root:%s", publicKey)),
			},
			MetadataStartupScript: pulumi.StringPtrInput(pulumi.String(sshMetadata)),
			ServiceAccount: &compute.InstanceServiceAccountArgs{
				Scopes: pulumi.StringArray{
					pulumi.String("cloud-platform"),
				},
			},
		}, pulumi.Provider(gcpProvider), pulumi.IgnoreChanges([]string{"name"}), pulumi.RetainOnDelete(true))

		if err != nil {
			return nil, fmt.Errorf("failed to create instance %s: %v", name, err)
		}

		serverOutput := types.ProvisionServerOutput{
			ID:               instance.ID().ToStringOutput(),
			PublicIP:         staticIP.Address.ToStringOutput(),
			PrivateIP:        staticIP.Address.ToStringOutput(),
			ProvisionKeyPath: keyPath,
		}

		result.Servers = append(result.Servers, serverOutput)
	}

	return result, nil
}
