package types

type ServerState struct {
	ID        int    `db:"id"`
	MachineID string `db:"machine_id"`
	PublicIP  string `db:"public_ip"`
	PrivateIP string `db:"private_ip"`
	Role      string `db:"role"`   // "server", "client", or "both"
	Status    string `db:"status"` // "active", "inactive", "failed"
	CreatedAt string `db:"created_at"`
	UpdatedAt string `db:"updated_at"`
}
