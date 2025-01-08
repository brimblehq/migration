package provision

import (
	"fmt"
	"strconv"

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

func (p *HetznerProvisioner) ProvisionServers(ctx *pulumi.Context, config types.ProvisionServerConfig) (*types.ProvisionResult, error) {
	hcloudProvider, err := hcloud.NewProvider(ctx, "hcloud", &hcloud.ProviderArgs{
		Token: pulumi.String(""),
	})

	if err != nil {
		return nil, fmt.Errorf("failed to create Hetzner provider: %v", err)
	}

	result := &types.ProvisionResult{
		Provider: p.GetProviderName(),
		Region:   config.Region,
		Servers:  make([]types.ProvisionServerOutput, 0),
	}

	for i := 0; i < config.Count; i++ {
		name := fmt.Sprintf("%s-%d", config.Name, i+1)

		primaryIP, err := hcloud.NewPrimaryIp(ctx, fmt.Sprintf("%s-ip", name), &hcloud.PrimaryIpArgs{
			Name:         pulumi.String(fmt.Sprintf("%s-ip", name)),
			Type:         pulumi.String("ipv4"),
			Datacenter:   pulumi.String(config.Region),
			AssigneeType: pulumi.String("server"),
			AutoDelete:   pulumi.Bool(true),
			Labels: pulumi.StringMap{
				"Name":     pulumi.String(name),
				"Provider": pulumi.String("brimble"),
			},
		}, pulumi.Provider(hcloudProvider))
		if err != nil {
			return nil, fmt.Errorf("failed to create primary IP for server %s: %v", name, err)
		}

		ipv4Address := primaryIP.ID().ToStringOutput().ApplyT(func(id string) *int { i, _ := strconv.Atoi(id); return &i }).(pulumi.IntPtrInput)

		server, err := hcloud.NewServer(ctx, name, &hcloud.ServerArgs{
			Name:       pulumi.String(name),
			ServerType: pulumi.String(config.Size),
			Image:      pulumi.String(config.Image),
			Datacenter: pulumi.String(config.Region),
			SshKeys: pulumi.StringArray{
				pulumi.String(config.SSHKey),
			},
			PublicNets: hcloud.ServerPublicNetArray{
				&hcloud.ServerPublicNetArgs{
					Ipv4Enabled: pulumi.Bool(true),
					Ipv4:        ipv4Address,
					Ipv6Enabled: pulumi.Bool(false),
				},
			},
			Labels: pulumi.StringMap{
				"Name":     pulumi.String(name),
				"Provider": pulumi.String("brimble"),
			},
		}, pulumi.Provider(hcloudProvider))
		if err != nil {
			return nil, fmt.Errorf("failed to create server %s: %v", name, err)
		}

		serverOutput := types.ProvisionServerOutput{
			ID:        server.ID().ToStringOutput(),
			PublicIP:  primaryIP.IpAddress.ToStringOutput(),
			PrivateIP: server.Ipv4Address.ToStringOutput(),
		}

		result.Servers = append(result.Servers, serverOutput)

		ctx.Export(fmt.Sprintf("%s-id", name), server.ID().ToStringOutput())
		ctx.Export(fmt.Sprintf("%s-public-ip", name), primaryIP.IpAddress.ToStringOutput())
		ctx.Export(fmt.Sprintf("%s-private-ip", name), server.Ipv4Address.ToStringOutput())
	}

	return result, nil
}
