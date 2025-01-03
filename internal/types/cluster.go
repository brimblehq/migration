package types

type ClusterRole string

const (
	RoleServer ClusterRole = "server"
	RoleClient ClusterRole = "client"
)

type ClusterMember struct {
	Server Server
	Roles  []ClusterRole
}
