package provision

import (
	"context"
	"fmt"

	"github.com/brimblehq/migration/internal/ssh"
	"github.com/brimblehq/migration/internal/types"
	"github.com/pulumi/pulumi-aws/sdk/v5/go/aws/ec2"
	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"
)

type AWSProvisioner struct{}

func (p *AWSProvisioner) GetProviderName() string {
	return "aws"
}

func (p *AWSProvisioner) ValidateConfig(config types.ProvisionServerConfig) error {
	if config.Size == "" || config.Region == "" || config.Image == "" {
		return fmt.Errorf("size, region, and image are required for AWS")
	}
	return nil
}

func (p *AWSProvisioner) ProvisionServers(ctx *pulumi.Context, config types.ProvisionServerConfig, tempSSHManager *ssh.TempSSHManager) (*types.ProvisionResult, error) {
	publicKey, err := tempSSHManager.GenerateKeys(context.Background(), false)
	if err != nil {
		return nil, fmt.Errorf("failed to generate keys: %v", err)
	}

	result := &types.ProvisionResult{
		Provider: p.GetProviderName(),
		Region:   config.Region,
		Servers:  make([]types.ProvisionServerOutput, 0),
	}

	vpc, err := ec2.NewVpc(ctx, fmt.Sprintf("%s-vpc", config.Name), &ec2.VpcArgs{
		CidrBlock:          pulumi.String("10.0.0.0/16"),
		EnableDnsSupport:   pulumi.Bool(true),
		EnableDnsHostnames: pulumi.Bool(true),
		Tags: pulumi.StringMap{
			"Name": pulumi.String(fmt.Sprintf("%s-brimble-vpc", config.Reference)),
		},
	})
	if err != nil {
		return nil, fmt.Errorf("failed to create VPC: %v", err)
	}

	gw, err := ec2.NewInternetGateway(ctx, fmt.Sprintf("%s-gw", config.Reference), &ec2.InternetGatewayArgs{
		VpcId: vpc.ID(),
	})
	if err != nil {
		return nil, fmt.Errorf("failed to create internet gateway: %v", err)
	}

	subnet, err := ec2.NewSubnet(ctx, fmt.Sprintf("%s-subnet", config.Name), &ec2.SubnetArgs{
		VpcId:               vpc.ID(),
		CidrBlock:           pulumi.String("10.0.1.0/24"),
		MapPublicIpOnLaunch: pulumi.Bool(true),
		Tags: pulumi.StringMap{
			"Name": pulumi.String(fmt.Sprintf("%s-brimble-subnet", config.Reference)),
		},
	})
	if err != nil {
		return nil, fmt.Errorf("failed to create subnet: %v", err)
	}

	routeTable, err := ec2.NewRouteTable(ctx, fmt.Sprintf("%s-rt", config.Reference), &ec2.RouteTableArgs{
		VpcId: vpc.ID(),
	})
	if err != nil {
		return nil, fmt.Errorf("failed to create route table: %v", err)
	}

	_, err = ec2.NewRoute(ctx, fmt.Sprintf("%s-rt-route", config.Reference), &ec2.RouteArgs{
		RouteTableId:         routeTable.ID(),
		DestinationCidrBlock: pulumi.String("0.0.0.0/0"),
		GatewayId:            gw.ID(),
	})
	if err != nil {
		return nil, fmt.Errorf("failed to create route: %v", err)
	}
	_, err = ec2.NewRouteTableAssociation(ctx, fmt.Sprintf("%s-rt-assoc", config.Reference), &ec2.RouteTableAssociationArgs{
		SubnetId:     subnet.ID(),
		RouteTableId: routeTable.ID(),
	})
	if err != nil {
		return nil, fmt.Errorf("failed to associate route table with subnet: %v", err)
	}

	securityGroup, err := ec2.NewSecurityGroup(ctx, fmt.Sprintf("%s-brimble-sg", config.Reference), &ec2.SecurityGroupArgs{
		VpcId: vpc.ID(),
		Ingress: ec2.SecurityGroupIngressArray{
			&ec2.SecurityGroupIngressArgs{
				Protocol:   pulumi.String("tcp"),
				FromPort:   pulumi.Int(80),
				ToPort:     pulumi.Int(80),
				CidrBlocks: pulumi.StringArray{pulumi.String("0.0.0.0/0")},
			},
		},
		Egress: ec2.SecurityGroupEgressArray{
			&ec2.SecurityGroupEgressArgs{
				Protocol:   pulumi.String("-1"),
				FromPort:   pulumi.Int(0),
				ToPort:     pulumi.Int(0),
				CidrBlocks: pulumi.StringArray{pulumi.String("0.0.0.0/0")},
			},
		},
	})
	if err != nil {
		return nil, fmt.Errorf("failed to create security group: %v", err)
	}

	keyPair, err := ec2.NewKeyPair(ctx, fmt.Sprintf("%s-brimble-key", config.Reference), &ec2.KeyPairArgs{
		KeyName:   pulumi.String(fmt.Sprintf("%s-brimble-key", config.Reference)),
		PublicKey: pulumi.String(publicKey),
	})
	if err != nil {
		return nil, fmt.Errorf("failed to create key pair: %v", err)
	}

	for i := 0; i < config.Count; i++ {
		name := fmt.Sprintf("%s-brimble-instance-%d", config.Reference, i+1)
		instance, err := ec2.NewInstance(ctx, name, &ec2.InstanceArgs{
			InstanceType:             pulumi.String(config.Size),
			Ami:                      pulumi.String(config.Image),
			KeyName:                  keyPair.KeyName,
			AssociatePublicIpAddress: pulumi.Bool(true),
			SubnetId:                 subnet.ID(),
			SecurityGroups:           pulumi.StringArray{securityGroup.ID()},
			Tags: pulumi.StringMap{
				"Name":     pulumi.String(name),
				"Provider": pulumi.String("brimble"),
			},
		})
		if err != nil {
			return nil, fmt.Errorf("failed to create EC2 instance %s: %v", name, err)
		}

		serverOutput := types.ProvisionServerOutput{
			ID:        instance.ID().ToStringOutput(),
			PublicIP:  instance.PublicIp.ToStringOutput(),
			PrivateIP: instance.PrivateIp.ToStringOutput(),
		}

		result.Servers = append(result.Servers, serverOutput)

		// Export instance details
		ctx.Export(fmt.Sprintf("%s-id", name), instance.ID().ToStringOutput())
		ctx.Export(fmt.Sprintf("%s-public-ip", name), instance.PublicIp.ToStringOutput())
		ctx.Export(fmt.Sprintf("%s-private-ip", name), instance.PrivateIp.ToStringOutput())
	}

	return result, nil
}
