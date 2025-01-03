package types

type Config struct {
	Servers       []Server      `yaml:"servers"`
	ClusterConfig ClusterConfig `yaml:"cluster_config"`
}

type Server struct {
	Host       string `yaml:"host"`
	Username   string `yaml:"username"`
	KeyPath    string `yaml:"key_path"`
	DataCenter string `yaml:"datacenter"`
	PublicIP   string `yaml:"public_ip"`
	PrivateIP  string `yaml:"private_ip"`
}

type ClusterConfig struct {
	ConsulConfig     ConsulConfig     `yaml:"consul"`
	MonitoringConfig MonitoringConfig `yaml:"monitoring"`
	Versions         Versions         `yaml:"versions"`
}

type ConsulConfig struct {
	ServerAddress string `yaml:"server_address"`
	Token         string `yaml:"token"`
	DataCenter    string `yaml:"datacenter"`
	ConsulImage   string `yaml:"consul_image"`
}

type MonitoringConfig struct {
	GrafanaPassword string `yaml:"grafana_password"`
	MetricsPort     int    `yaml:"metrics_port"`
}

type Versions struct {
	Docker string `yaml:"docker"`
	NodeJS string `yaml:"nodejs"`
	Nomad  string `yaml:"nomad"`
}
