package types

import (
	"time"
)

type Config struct {
	Servers       []Server      `json:"servers"`
	ClusterConfig ClusterConfig `json:"cluster_config"`
}

type LicenseResponse struct {
	Valid        bool                 `json:"valid"`
	Key          string               `json:"key"`
	ExpireIn     *string              `json:"expireIn"`
	Tag          string               `json:"tag"`
	MaxDevices   int                  `json:"max_devices"`
	Subscription SubscriptionResponse `json:"subscription,omitempty"`
}

type SubscriptionResponse struct {
	ID             string    `json:"_id"`
	AdminID        string    `json:"admin_id"`
	BillableID     string    `json:"billable_id"`
	ProjectID      *string   `json:"project_id"`
	PlanType       string    `json:"plan_type"`
	Status         string    `json:"status"`
	DebitDate      time.Time `json:"debit_date"`
	StartDate      time.Time `json:"start_date"`
	ExpiryDate     time.Time `json:"expiry_date"`
	TriggerCreated bool      `json:"trigger_created"`
	CreatedAt      time.Time `json:"createdAt"`
	UpdatedAt      time.Time `json:"updatedAt"`
	Version        int       `json:"__v"`
	JobIdentifier  string    `json:"job_identifier"`
}

type FlagConfig struct {
	LicenseKey string
	Instances  string
	ConfigPath string
	UseTemp    bool
	Provision  bool
}

type Server struct {
	Host       string `json:"host"`
	Username   string `json:"username"`
	KeyPath    string `json:"key_path,omitempty"`
	Region     string `json:"region"`
	PublicIP   string `json:"public_ip"`
	PrivateIP  string `json:"private_ip"`
	AuthMethod string `json:"auth_method,omitempty"`
}

type ClusterConfig struct {
	ConsulConfig     ConsulConfig     `json:"consul"`
	MonitoringConfig MonitoringConfig `json:"monitoring"`
	Versions         Versions         `json:"versions"`
	Runner           Runner           `json:"runner"`
}

type ConsulConfig struct {
	ConsulImage string `json:"consul_image"`
}

type MonitoringConfig struct {
	GrafanaPassword string `json:"grafana_password"`
	MetricsPort     int    `json:"metrics_port"`
}

type Versions struct {
	Docker string `json:"docker"`
	NodeJS string `json:"nodejs"`
	Nomad  string `json:"nomad"`
}

type Docker struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

type Runner struct {
	Port     int `json:"port"`
	Instance int `json:"instance"`
}
