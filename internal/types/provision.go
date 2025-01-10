package types

import (
	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"
)

type ProvisionServerConfig struct {
	Name   string
	Size   string
	Region string
	Image  string
	Tags   []string
	Count  int
	SSHKey string
}

type ProvisionResult struct {
	ServerIDs  []string
	PublicIPs  []string
	PrivateIPs []string
	Provider   string
	Region     string
	Servers    []ProvisionServerOutput
}

type ProvisionServerOutput struct {
	ID               pulumi.StringOutput
	PublicIP         pulumi.StringOutput
	PrivateIP        pulumi.StringOutput
	ProvisionKeyPath string
}

type Provider struct {
	ID       string    `json:"id"`
	Name     string    `json:"name"`
	Machines []Machine `json:"machines"`
}

type Machine struct {
	ID          string `json:"id"`
	Size        string `json:"size"`
	Region      Region `json:"region"`
	Image       string `json:"image"`
	Description string `json:"description"`
	UseCase     string `json:"use_case"`
	Role        string `json:"recommended_role"`
}

type Region struct {
	Name string `json:"name"`
	Type string `json:"type"`
}

type RegionOption struct {
	ID          string
	DisplayName string
}
