package manager

import (
	"github.com/brimblehq/migration/internal/types"
)

type ClusterManager struct {
	TotalNodes  int
	ServerNodes int
	ServerHosts []string
	RoleMapping map[string][]types.ClusterRole
}

func NewClusterRoles(servers []types.Server) *ClusterManager {
	totalNodes := len(servers)
	var serverNodes int

	switch totalNodes {
	case 1, 2:
		serverNodes = 1
	default:
		serverNodes = totalNodes - 1
	}

	serverHosts := make([]string, serverNodes)
	for i := 0; i < serverNodes; i++ {
		serverHosts[i] = servers[i].Host
	}

	return &ClusterManager{
		TotalNodes:  totalNodes,
		ServerNodes: serverNodes,
		ServerHosts: serverHosts,
		RoleMapping: make(map[string][]types.ClusterRole),
	}
}

func (cm *ClusterManager) CalculateRoles(servers []types.Server) {
	for i, server := range servers {
		roles := []types.ClusterRole{types.RoleClient}
		if i < cm.ServerNodes {
			roles = append(roles, types.RoleServer)
		}
		cm.RoleMapping[server.Host] = roles
	}
}

func (cm *ClusterManager) GetServerRoles(serverHost string) []types.ClusterRole {
	return cm.RoleMapping[serverHost]
}
