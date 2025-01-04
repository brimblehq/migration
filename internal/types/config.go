package types

type Config struct {
	Servers       []Server      `json:"servers"`
	ClusterConfig ClusterConfig `json:"cluster_config"`
}

type Server struct {
	Host       string `json:"host"`
	Username   string `json:"username"`
	KeyPath    string `json:"key_path"`
	DataCenter string `json:"datacenter"`
	PublicIP   string `json:"public_ip"`
	PrivateIP  string `json:"private_ip"`
}

type ClusterConfig struct {
	ConsulConfig     ConsulConfig     `json:"consul"`
	MonitoringConfig MonitoringConfig `json:"monitoring"`
	Versions         Versions         `json:"versions"`
}

type ConsulConfig struct {
	ServerAddress string `json:"server_address"`
	Token         string `json:"token"`
	DataCenter    string `json:"datacenter"`
	ConsulImage   string `json:"consul_image"`
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
