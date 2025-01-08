package helpers

import (
	"crypto/rand"
	"fmt"
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
