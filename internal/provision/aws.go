package provision

import (
	"context"
	"fmt"

	"github.com/brimblehq/migration/internal/db"
	"github.com/brimblehq/migration/internal/helpers"
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

func (p *AWSProvisioner) ProvisionServers(ctx *pulumi.Context, config types.ProvisionServerConfig, tempSSHManager *ssh.TempSSHManager, database *db.PostgresDB) (*types.ProvisionResult, error) {
	var provisionedKeyPath string

	vpc, err := ec2.NewVpc(ctx, vpcName, &ec2.VpcArgs{
		CidrBlock:          pulumi.String("10.0.0.0/16"),
		EnableDnsHostnames: pulumi.Bool(true),
		EnableDnsSupport:   pulumi.Bool(true),
		Tags: pulumi.StringMap{
			"Name": pulumi.String(tagName),
		},
	}, pulumi.RetainOnDelete(true))

	if err != nil {
		return nil, fmt.Errorf("failed to create VPC: %v", err)
	}

	igw, err := ec2.NewInternetGateway(ctx, internetGatewayName, &ec2.InternetGatewayArgs{
		VpcId: vpc.ID(),
		Tags: pulumi.StringMap{
			"Name": pulumi.String(tagName),
		},
	})

	if err != nil {
		return nil, fmt.Errorf("failed to create Internet Gateway: %v", err)
	}

	subnet, err := ec2.NewSubnet(ctx, subnetName, &ec2.SubnetArgs{
		VpcId:               vpc.ID(),
		CidrBlock:           pulumi.String("10.0.1.0/24"),
		MapPublicIpOnLaunch: pulumi.Bool(true),
		Tags: pulumi.StringMap{
			"Name": pulumi.String(tagName),
		},
	}, pulumi.RetainOnDelete(true))

	if err != nil {
		return nil, fmt.Errorf("failed to create subnet: %v", err)
	}

	routeTable, err := ec2.NewRouteTable(ctx, routeTableName, &ec2.RouteTableArgs{
		VpcId: vpc.ID(),
		Routes: ec2.RouteTableRouteArray{
			&ec2.RouteTableRouteArgs{
				CidrBlock: pulumi.String("0.0.0.0/0"),
				GatewayId: igw.ID(),
			},
		},
		Tags: pulumi.StringMap{
			"Name": pulumi.String(tagName),
		},
	}, pulumi.ReplaceOnChanges([]string{
		"routes",
		"tags",
	}), pulumi.RetainOnDelete(true))

	if err != nil {
		return nil, fmt.Errorf("failed to create route table: %v", err)
	}

	_, err = ec2.NewRouteTableAssociation(ctx, routeTableAssociation, &ec2.RouteTableAssociationArgs{
		SubnetId:     subnet.ID(),
		RouteTableId: routeTable.ID(),
	}, pulumi.RetainOnDelete(true))
	if err != nil {
		return nil, fmt.Errorf("failed to associate route table: %v", err)
	}

	securityGroup, err := ec2.NewSecurityGroup(ctx, securityGroupName, &ec2.SecurityGroupArgs{
		VpcId:       vpc.ID(),
		Description: pulumi.String("Security group for Brimble instances"),
		Ingress: ec2.SecurityGroupIngressArray{
			&ec2.SecurityGroupIngressArgs{
				Description: pulumi.String("SSH access"),
				Protocol:    pulumi.String("tcp"),
				FromPort:    pulumi.Int(22),
				ToPort:      pulumi.Int(22),
				CidrBlocks:  pulumi.StringArray{pulumi.String("0.0.0.0/0")},
			},
		},
		Egress: ec2.SecurityGroupEgressArray{
			&ec2.SecurityGroupEgressArgs{
				Description: pulumi.String("Allow all outbound traffic"),
				Protocol:    pulumi.String("-1"),
				FromPort:    pulumi.Int(0),
				ToPort:      pulumi.Int(0),
				CidrBlocks:  pulumi.StringArray{pulumi.String("0.0.0.0/0")},
			},
		},
		Tags: pulumi.StringMap{
			"Name": pulumi.String(tagName),
		},
	}, pulumi.ReplaceOnChanges([]string{
		"ingress",
		"egress",
		"description",
		"tags",
	}))

	if err != nil {
		return nil, fmt.Errorf("failed to create security group: %v", err)
	}

	keyName := "brimble-provision-key"

	keyPair, err := ec2.LookupKeyPair(ctx, &ec2.LookupKeyPairArgs{
		KeyName: &keyName,
	})

	if err != nil || keyPair == nil {
		publicKey, keyPath, err := tempSSHManager.GenerateKeys(context.Background())
		if err != nil {
			return nil, fmt.Errorf("failed to generate keys: %v", err)
		}

		_, err = ec2.NewKeyPair(ctx, keyName, &ec2.KeyPairArgs{
			KeyName:   pulumi.String(keyName),
			PublicKey: pulumi.String(publicKey),
		})
		if err != nil {
			return nil, fmt.Errorf("failed to create key pair: %v", err)
		}

		provisionedKeyPath = keyPath
	}

	result := &types.ProvisionResult{
		Provider: p.GetProviderName(),
		Region:   config.Region,
		Servers:  make([]types.ProvisionServerOutput, 0),
	}

	for i := 0; i < config.Count; i++ {
		reference, err := helpers.GenerateUniqueReference(database)

		if err != nil {
			return nil, fmt.Errorf("failed to generate unique reference: %v", err)
		}

		name := fmt.Sprintf("%s-brimble-instance-%d", reference, i+1)
		instance, err := ec2.NewInstance(ctx, name, &ec2.InstanceArgs{
			InstanceType:             pulumi.String(config.Size),
			Ami:                      pulumi.String(config.Image),
			KeyName:                  pulumi.String(keyName),
			AssociatePublicIpAddress: pulumi.Bool(true),
			SubnetId:                 subnet.ID(),
			VpcSecurityGroupIds:      pulumi.StringArray{securityGroup.ID()},
			Tags: pulumi.StringMap{
				"Name":     pulumi.String(name),
				"Provider": pulumi.String("brimble"),
			},
		}, pulumi.RetainOnDelete(true))

		if err != nil {
			return nil, fmt.Errorf("failed to create EC2 instance %s: %v", name, err)
		}

		serverOutput := types.ProvisionServerOutput{
			ID:               instance.ID().ToStringOutput(),
			PublicIP:         instance.PublicIp.ToStringOutput(),
			PrivateIP:        instance.PrivateIp.ToStringOutput(),
			ProvisionKeyPath: provisionedKeyPath,
		}

		result.Servers = append(result.Servers, serverOutput)
	}

	return result, nil
}
