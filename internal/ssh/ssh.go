package ssh

import (
	"context"
	"fmt"
	"time"

	"github.com/brimblehq/migration/internal/db"
	"github.com/brimblehq/migration/internal/helpers"
	"github.com/brimblehq/migration/internal/types"
)

type SSHSetupManager struct {
	database *db.PostgresDB
	config   *types.Config
}

func NewSSHSetupManager(database *db.PostgresDB, config *types.Config) *SSHSetupManager {
	return &SSHSetupManager{
		database: database,
		config:   config,
	}
}

func (m *SSHSetupManager) ValidateAndInitializeSSH(ctx context.Context, useTemp bool, maxDevices int) (*TempSSHManager, error) {
	noOfServers := len(m.config.Servers)

	if noOfServers > maxDevices {
		return nil, fmt.Errorf("\033[31mthis license key only supports a number: %d devices\033[0m", maxDevices)
	}

	err := ValidateServerAuth(m.config.Servers, useTemp)
	if err != nil {
		if _, ok := err.(*SingleServerTempKeyError); ok {
			return m.initializeTempSSH(ctx)
		}
		return nil, fmt.Errorf("invalid server configuration: %w", err)
	}

	if useTemp {
		return m.initializeTempSSH(ctx)
	}

	keyID, err := helpers.GenerateKeyID()

	if err != nil {
		return nil, fmt.Errorf("failed to generate key ID: %w", err)
	}

	sshManager := &TempSSHManager{
		db:       m.database,
		keyID:    keyID,
		keyDir:   "~/.ssh",
		servers:  make([]string, 0),
		hostKeys: make(map[string][]byte),
	}

	return sshManager, nil

}

func (m *SSHSetupManager) initializeTempSSH(ctx context.Context) (*TempSSHManager, error) {
	cleanupCtx, cleanupCancel := context.WithCancel(ctx)

	defer cleanupCancel()

	CleanupExpiredKeys(cleanupCtx, m.database, m.config)

	servers := make([]string, len(m.config.Servers))
	for i, server := range m.config.Servers {
		servers[i] = server.Host
	}

	sshManager, err := NewTempSSHManager(m.database, servers)
	if err != nil {
		return nil, fmt.Errorf("failed to create SSH manager: %w", err)
	}

	_, err = sshManager.GenerateKeys(ctx, true)

	if err != nil {
		return nil, fmt.Errorf("failed to generate SSH keys: %w", err)
	}

	fmt.Println("\n🔐 Temporary SSH Setup Required")
	fmt.Println(sshManager.GetPublicKeyWithInstructions())

	checkCtx, cancel := context.WithTimeout(ctx, 10*time.Minute)
	defer cancel()

	if err := WaitForSSHReadiness(checkCtx, m.config.Servers, sshManager); err != nil {
		return nil, fmt.Errorf("SSH setup failed: %w", err)
	}

	return sshManager, nil
}
