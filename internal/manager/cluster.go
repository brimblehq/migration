package manager

import (
	"fmt"
	"math"
	"strconv"
	"strings"

	"github.com/brimblehq/migration/internal/types"
)

type ClusterManager struct {
	TotalNodes  int
	ServerNodes int
	ServerHosts []string
	RoleMapping map[string][]types.ClusterRole
}

type StandardRequirement struct {
	Memory  int
	Cpu     int
	Storage float64
}

func NewClusterRoles(machines []types.Server) *ClusterManager {
	totalNodes := len(machines)
	var serverNodes int

	switch totalNodes {
	case 1, 2:
		serverNodes = 1
	default:
		serverNodes = totalNodes - 1
	}

	serverHosts := make([]string, serverNodes)
	for i := 0; i < serverNodes; i++ {
		serverHosts[i] = machines[i].Host
	}

	// log.Printf("Debug NewClusterRoles: Total nodes: %d, Server nodes: %d", totalNodes, serverNodes)
	// log.Printf("Debug NewClusterRoles: Server hosts: %v", serverHosts)

	return &ClusterManager{
		TotalNodes:  totalNodes,
		ServerNodes: serverNodes,
		ServerHosts: serverHosts,
		RoleMapping: make(map[string][]types.ClusterRole),
	}
}

func (cm *ClusterManager) CalculateRoles(machines []types.Server) {
	for i, machine := range machines {
		roles := []types.ClusterRole{types.RoleClient}
		if i < cm.ServerNodes {
			roles = append(roles, types.RoleServer)
		}
		cm.RoleMapping[machine.Host] = roles
		//log.Printf("Debug CalculateRoles: Server %s (index %d) assigned roles: %v", machine.Host, i, roles)
	}
}

func (cm *ClusterManager) GetServerRoles(serverHost string) []types.ClusterRole {
	return cm.RoleMapping[serverHost]
}

func (im *InstallationManager) VerifyMachineRequirement() error {
	var storageGB float64
	var cores int
	var memoryGB int

	// standardRequirement := &StandardRequirement{
	// 	Cpu:     2,
	// 	Memory:  32,
	// 	Storage: 20,
	// }

	standardRequirement := &StandardRequirement{
		Cpu:     1,
		Memory:  1,
		Storage: 10,
	}

	commands := []string{
		"nproc",
		"df -k / | awk 'NR==2{print $4}'",
		"free -k | awk '/^Mem:/ {print $2}'",
	}

	for _, command := range commands {
		result, err := im.sshClient.ExecuteCommandWithOutput(command)
		if err != nil {
			return fmt.Errorf("failed to execute command %s: %v", command, err)
		}

		switch command {
		case "nproc":
			cores, _ = strconv.Atoi(strings.TrimSpace(result))
		case "df -k / | awk 'NR==2{print $4}'":
			storage, _ := strconv.Atoi(strings.TrimSpace(result))
			storageGB = float64(storage) / 1024 / 1024
		case "free -k | awk '/^Mem:/ {print $2}'":
			memory, _ := strconv.Atoi(strings.TrimSpace(result))
			memoryGB = int(math.Round(float64(memory) / 1024 / 1024))
		}
	}

	if cores < standardRequirement.Cpu {
		return fmt.Errorf("insufficient CPU cores: have %d, need minimum of %d cores",
			cores, standardRequirement.Cpu)
	}

	if memoryGB < standardRequirement.Memory {
		return fmt.Errorf("insufficient memory: have %d GB, need minimum of %d GB",
			memoryGB, standardRequirement.Memory)
	}

	if storageGB < standardRequirement.Storage {
		return fmt.Errorf("insufficient storage: have %.2f GB, need minimum of %.2f GB",
			storageGB, standardRequirement.Storage)
	}

	return nil
}
