package types

type ServerStep string

const (
	StepInitialized     ServerStep = "initialized"
	StepVerified        ServerStep = "verified"
	StepBaseInstalled   ServerStep = "base_installed"
	StepConsulSetup     ServerStep = "consul_setup"
	StepNomadSetup      ServerStep = "nomad_setup"
	StepMonitoringSetup ServerStep = "monitoring_setup"
	StepRunnerStarted   ServerStep = "runner_started"
	StepCompleted       ServerStep = "completed"
)

type ServerState struct {
	ID          string     `db:"id"`
	MachineID   string     `db:"machine_id"`
	PublicIP    string     `db:"public_ip"`
	PrivateIP   string     `db:"private_ip"`
	Role        string     `db:"role"`   // "server", "client", or "both"
	Status      string     `db:"status"` // "active", "inactive", "failed"
	Identifier  string     `db:"identifier"`
	CurrentStep ServerStep `db:"step"`
	CreatedAt   string     `db:"created_at"`
	UpdatedAt   string     `db:"updated_at"`
}
