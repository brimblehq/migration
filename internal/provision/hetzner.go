package provision

import (
	"context"
	"fmt"
	"os"
	"strconv"

	"github.com/brimblehq/migration/internal/db"
	"github.com/brimblehq/migration/internal/helpers"
	"github.com/brimblehq/migration/internal/ssh"
	"github.com/brimblehq/migration/internal/types"
	"github.com/pulumi/pulumi-hcloud/sdk/go/hcloud"
	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"
)

type HetznerProvisioner struct{}

func (p *HetznerProvisioner) GetProviderName() string {
	return "hetzner"
}

func (p *HetznerProvisioner) ValidateConfig(config types.ProvisionServerConfig) error {
	if config.Size == "" || config.Region == "" || config.Image == "" {
		return fmt.Errorf("size, region, and image are required for Hetzner")
	}
	return nil
}

func (p *HetznerProvisioner) ProvisionServers(ctx *pulumi.Context, config types.ProvisionServerConfig, tempSSHManager *ssh.TempSSHManager, database *db.PostgresDB) (*types.ProvisionResult, error) {
	publicKey, keyPath, err := tempSSHManager.GenerateKeys(context.Background())

	if err != nil {
		return nil, fmt.Errorf("failed to generate keys: %v", err)
	}

	hcloudProvider, err := hcloud.NewProvider(ctx, "hcloud", &hcloud.ProviderArgs{
		Token: pulumi.String(os.Getenv("HCLOUD_TOKEN")),
	})

	if err != nil {
		return nil, fmt.Errorf("failed to create Hetzner provider: %v", err)
	}

	result := &types.ProvisionResult{
		Provider: p.GetProviderName(),
		Region:   config.Region,
		Servers:  make([]types.ProvisionServerOutput, 0),
	}

	sshKeyName := "brimble-ssh-key"

	sshKey, err := hcloud.NewSshKey(ctx, sshKeyName, &hcloud.SshKeyArgs{
		Name:      pulumi.String("brimble-key"),
		PublicKey: pulumi.String(publicKey),
		Labels: pulumi.StringMap{
			"Name":     pulumi.String(sshKeyName),
			"Provider": pulumi.String("brimble"),
		},
	}, pulumi.Provider(hcloudProvider))

	if err != nil {
		return nil, fmt.Errorf("failed to create SSH key: %v", err)
	}

	for i := 0; i < config.Count; i++ {
		reference, err := helpers.GenerateUniqueReference(database)

		if err != nil {
			return nil, fmt.Errorf("failed to generate unique reference: %v", err)
		}

		serverName := fmt.Sprintf("%s-brimble-instance-%d", reference, i+1)

		ipName := fmt.Sprintf("%s-brimble-ip", serverName)

		primaryIP, err := hcloud.NewPrimaryIp(ctx, ipName, &hcloud.PrimaryIpArgs{
			Name:         pulumi.String(ipName),
			Type:         pulumi.String("ipv4"),
			Datacenter:   pulumi.String(config.Region),
			AssigneeType: pulumi.String("server"),
			AutoDelete:   pulumi.Bool(true),
			Labels: pulumi.StringMap{
				"Name":     pulumi.String(serverName),
				"Provider": pulumi.String("brimble"),
				"RunID":    pulumi.String(reference),
			},
		}, pulumi.Provider(hcloudProvider))
		if err != nil {
			return nil, fmt.Errorf("failed to create primary IP for server %s: %v", serverName, err)
		}

		ipv4Address := primaryIP.ID().ToStringOutput().ApplyT(func(id string) *int { i, _ := strconv.Atoi(id); return &i }).(pulumi.IntPtrInput)

		server, err := hcloud.NewServer(ctx, serverName, &hcloud.ServerArgs{
			Name:       pulumi.String(serverName),
			ServerType: pulumi.String(config.Size),
			Image:      pulumi.String(config.Image),
			Datacenter: pulumi.String(config.Region),
			SshKeys: pulumi.StringArray{
				sshKey.ID().ToStringOutput(),
			},
			PublicNets: hcloud.ServerPublicNetArray{
				&hcloud.ServerPublicNetArgs{
					Ipv4Enabled: pulumi.Bool(true),
					Ipv4:        ipv4Address,
					Ipv6Enabled: pulumi.Bool(false),
				},
			},
			Labels: pulumi.StringMap{
				"Name":     pulumi.String(serverName),
				"Provider": pulumi.String("brimble"),
				"RunID":    pulumi.String(reference),
			},
		}, pulumi.Provider(hcloudProvider), pulumi.RetainOnDelete(true))
		if err != nil {
			return nil, fmt.Errorf("failed to create server %s: %v", serverName, err)
		}

		serverOutput := types.ProvisionServerOutput{
			ID:               server.ID().ToStringOutput(),
			PublicIP:         primaryIP.IpAddress.ToStringOutput(),
			PrivateIP:        server.Ipv4Address.ToStringOutput(),
			ProvisionKeyPath: keyPath,
		}

		result.Servers = append(result.Servers, serverOutput)
	}

	return result, nil
}
