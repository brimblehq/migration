package types

type NomadConfig struct {
	DataCenter     string
	BindAddr       string
	AdvertiseAddrs AdvertiseAddrs
	ServerConfig   *ServerConfig
	ClientConfig   ClientConfig
	ConsulConfig   ConsulConfig
	Plugins        map[string]interface{}
	Telemetry      TelemetryConfig
}

type AdvertiseAddrs struct {
	HTTP string
	RPC  string
	Serf string
}

type ServerConfig struct {
	Enabled         bool
	BootstrapExpect int
}

type ClientConfig struct {
	Enabled bool
	Servers []string
}

type TelemetryConfig struct {
	CollectionInterval       string
	DisableHostname          bool
	PrometheusMetrics        bool
	PublishAllocationMetrics bool
	PublishNodeMetrics       bool
}
