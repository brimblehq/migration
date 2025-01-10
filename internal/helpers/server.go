package helpers

import (
	"crypto/rand"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/brimblehq/migration/internal/db"
	"github.com/brimblehq/migration/internal/types"
)

func GetServerList(database *db.PostgresDB, config types.Config) []types.Server {
	existingServers, _ := database.GetAllServers()
	var allServers []types.Server
	existingIPs := make(map[string]struct {
		step      types.ServerStep
		publicIP  string
		privateIP string
	})

	for _, srv := range existingServers {
		server := types.Server{
			Host:      srv.MachineID,
			PublicIP:  srv.PublicIP,
			PrivateIP: srv.PrivateIP,
		}
		allServers = append(allServers, server)
		existingIPs[srv.PrivateIP] = struct {
			step      types.ServerStep
			publicIP  string
			privateIP string
		}{
			step:      srv.CurrentStep,
			publicIP:  srv.PublicIP,
			privateIP: srv.PrivateIP,
		}
	}

	for _, configServer := range config.Servers {
		if existingInfo, exists := existingIPs[configServer.PrivateIP]; exists {
			if existingInfo.step != types.StepCompleted {
				allServers = append(allServers, configServer)
			}
			continue
		}
		allServers = append(allServers, configServer)
	}

	return allServers
}

func ProcessErrors(errorChan chan error) error {
	var errors []string
	for err := range errorChan {
		if err != nil {
			errors = append(errors, err.Error())
		}
	}

	if len(errors) > 0 {
		return fmt.Errorf("setup failed with errors:\n%s", strings.Join(errors, "\n"))
	}

	return nil
}

func GenerateKeyID() (string, error) {
	timestamp := time.Now().Unix()
	randomBytes := make([]byte, 4)
	if _, err := rand.Read(randomBytes); err != nil {
		return "", fmt.Errorf("failed to generate random bytes: %w", err)
	}
	keyID := fmt.Sprintf("brimble-temp-%d-%x", timestamp, randomBytes)

	return keyID, nil
}

func GenerateConfig(result *types.ProvisionResult) (string, error) {
	servers := make([]types.Server, len(result.Servers))
	for i, server := range result.Servers {
		publicIP := ""
		server.PublicIP.ApplyT(func(s string) string {
			publicIP = s
			return s
		})

		privateIP := ""
		server.PrivateIP.ApplyT(func(s string) string {
			privateIP = s
			return s
		})

		servers[i] = types.Server{
			Host:       fmt.Sprintf("instance-%d", i+1),
			Username:   "root",
			KeyPath:    server.ProvisionKeyPath,
			Region:     result.Region,
			PublicIP:   publicIP,
			PrivateIP:  privateIP,
			AuthMethod: "key_path",
		}
	}

	config := &types.Config{
		Servers: servers,
		ClusterConfig: types.ClusterConfig{
			ConsulConfig: types.ConsulConfig{
				ConsulImage: "hashicorp/consul:1.16",
			},
			MonitoringConfig: types.MonitoringConfig{
				GrafanaPassword: "password",
				MetricsPort:     9100,
			},
			Versions: types.Versions{
				Docker: "latest",
				NodeJS: "20.x",
				Nomad:  "1.6.3",
			},
			Runner: types.Runner{
				Port:     3000,
				Instance: 4,
			},
		},
	}

	fmt.Printf("GENERATED CONFIG: %v", config)

	jsonData, err := json.MarshalIndent(config, "", "  ")

	if err != nil {
		return "", fmt.Errorf("failed to marshal config to JSON: %v", err)
	}

	err = os.WriteFile("setup-config.json", []byte(string(jsonData)), 0644)

	if err != nil {
		return "", fmt.Errorf("failed to write config file: %v", err)
	}

	return string(jsonData), nil
}

type ServerOutput struct {
	ID        string `json:"id"`
	PublicIP  string `json:"publicIp"`
	PrivateIP string `json:"privateIp"`
	KeyPath   string `json:"keyPath"`
}
