package ssh

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/pem"
	"fmt"
	"io/ioutil"
	"log"
	"net"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/brimblehq/migration/internal/db"
	"github.com/brimblehq/migration/internal/types"
	"golang.org/x/crypto/ssh"
)

type ServerStatus struct {
	Host  string
	Ready bool
	Error error
}

type TempSSHManager struct {
	keyDir     string
	keyID      string
	privateKey *rsa.PrivateKey
	db         *db.PostgresDB
	publicKey  []byte
	servers    []string
	hostKeys   map[string][]byte
}

func NewTempSSHManager(db *db.PostgresDB, servers []string) (*TempSSHManager, error) {
	timestamp := time.Now().Unix()
	randomBytes := make([]byte, 4)
	if _, err := rand.Read(randomBytes); err != nil {
		return nil, fmt.Errorf("failed to generate random bytes: %w", err)
	}
	keyID := fmt.Sprintf("brimble-temp-%d-%x", timestamp, randomBytes)

	tmpDir := filepath.Join(os.TempDir(), "brimble-ssh")

	if err := os.MkdirAll(tmpDir, 0700); err != nil {
		return nil, fmt.Errorf("failed to create temp directory: %w", err)
	}

	return &TempSSHManager{
		keyDir:   tmpDir,
		keyID:    keyID,
		db:       db,
		servers:  servers,
		hostKeys: make(map[string][]byte),
	}, nil
}

func (m *TempSSHManager) GenerateKeys(ctx context.Context) error {
	privateKey, err := rsa.GenerateKey(rand.Reader, 4096)
	if err != nil {
		return fmt.Errorf("failed to generate private key: %w", err)
	}

	publicKey, err := ssh.NewPublicKey(&privateKey.PublicKey)
	if err != nil {
		return fmt.Errorf("failed to generate public key: %w", err)
	}

	m.publicKey = bytes.TrimSpace(ssh.MarshalAuthorizedKey(publicKey))
	m.publicKey = append(m.publicKey, []byte(" "+m.keyID)...)
	m.privateKey = privateKey

	if err := m.savePrivateKey(); err != nil {
		return fmt.Errorf("failed to save private key: %w", err)
	}
	_, err = m.db.CreateTempSSHKey(
		ctx,
		m.keyID,
		string(m.publicKey),
		m.servers,
	)
	if err != nil {
		return fmt.Errorf("failed to register key in database: %w", err)
	}

	return nil
}

func (m *TempSSHManager) ValidateKey(ctx context.Context) error {
	key, err := m.db.GetActiveKeyByID(ctx, m.keyID)
	if err != nil {
		return fmt.Errorf("failed to check key validity: %w", err)
	}

	if key == nil {
		return fmt.Errorf("key has expired or been invalidated")
	}

	return nil
}

func (m *TempSSHManager) GetPublicKeyWithInstructions() string {
	pubKey := string(m.publicKey)
	return fmt.Sprintf(`
🔑 Generated temporary SSH key: %s
📋 Please add this public key to your server:

%s

You can do this by running:
echo "%s" >> ~/.ssh/authorized_keys
`, m.keyID, pubKey, pubKey)
}

func (m *TempSSHManager) GetSSHConfig(host string) (*ssh.ClientConfig, error) {
	signer, err := ssh.NewSignerFromKey(m.privateKey)
	if err != nil {
		return nil, fmt.Errorf("failed to create signer: %w", err)
	}

	hostKey, err := m.getHostKey(host)
	if err != nil {
		return nil, fmt.Errorf("failed to get host key for %s: %w", host, err)
	}

	return &ssh.ClientConfig{
		Auth: []ssh.AuthMethod{
			ssh.PublicKeys(signer),
		},
		HostKeyCallback: ssh.FixedHostKey(hostKey),
		Timeout:         30 * time.Second,
	}, nil
}

func (m *TempSSHManager) getHostKey(host string) (ssh.PublicKey, error) {
	conn, err := net.Dial("tcp", host+":22")
	if err != nil {
		return nil, fmt.Errorf("failed to connect to %s: %w", host, err)
	}
	defer conn.Close()

	config := &ssh.ClientConfig{
		HostKeyCallback: func(hostname string, remote net.Addr, key ssh.PublicKey) error {
			// Store the host key
			m.hostKeys[host] = key.Marshal()
			return nil
		},
	}

	sshConn, chans, reqs, err := ssh.NewClientConn(conn, host, config)
	if err != nil {
		if !strings.Contains(err.Error(), "ssh: unable to authenticate") {
			return nil, fmt.Errorf("failed to get host key: %w", err)
		}
	}

	if sshConn != nil {
		sshConn.Close()
		go ssh.DiscardRequests(reqs)
		go func() {
			for c := range chans {
				c.Reject(ssh.ResourceShortage, "connection closed")
			}
		}()
	}

	hostKeyData, exists := m.hostKeys[host]
	if !exists {
		return nil, fmt.Errorf("no host key retrieved for %s", host)
	}

	hostKey, err := ssh.ParsePublicKey(hostKeyData)
	if err != nil {
		return nil, fmt.Errorf("failed to parse host key: %w", err)
	}

	return hostKey, nil
}

func (m *TempSSHManager) Cleanup(ctx context.Context, client *SSHClient) error {
	if err := m.db.MarkKeyAsExpired(ctx, m.keyID); err != nil {
		return fmt.Errorf("failed to mark key as expired: %w", err)
	}

	cleanupCmd := fmt.Sprintf("sed -i '/%s/d' ~/.ssh/authorized_keys", m.keyID)
	if err := client.ExecuteCommand(cleanupCmd); err != nil {
		return fmt.Errorf("failed to remove key from server: %w", err)
	}

	if err := m.db.MarkKeyAsCleaned(ctx, m.keyID); err != nil {
		return fmt.Errorf("failed to mark key as cleaned: %w", err)
	}

	keyPath := filepath.Join(m.keyDir, fmt.Sprintf("%s.pem", m.keyID))
	if err := os.Remove(keyPath); err != nil {
		return fmt.Errorf("failed to remove local private key: %w", err)
	}

	return nil
}

func (m *TempSSHManager) savePrivateKey() error {
	privateKeyPEM := &pem.Block{
		Type:  "RSA PRIVATE KEY",
		Bytes: x509.MarshalPKCS1PrivateKey(m.privateKey),
	}

	keyPath := filepath.Join(m.keyDir, fmt.Sprintf("%s.pem", m.keyID))
	if err := ioutil.WriteFile(keyPath, pem.EncodeToMemory(privateKeyPEM), 0600); err != nil {
		return fmt.Errorf("failed to save private key: %w", err)
	}

	return nil
}

func StartCleanupWorker(ctx context.Context, db *db.PostgresDB, config *types.Config) {
	ticker := time.NewTicker(15 * time.Minute)
	go func() {
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				if err := CleanupExpiredKeys(ctx, db, config); err != nil {
					log.Printf("Error cleaning up expired keys: %v", err)
				}
			}
		}
	}()
}

func CleanupExpiredKeys(ctx context.Context, db *db.PostgresDB, config *types.Config) error {
	keys, err := db.GetExpiredUncleaned(ctx)

	if err != nil {
		return fmt.Errorf("failed to get expired keys: %w", err)
	}

	serverDetails := make(map[string]types.Server)
	for _, server := range config.Servers {
		serverDetails[server.Host] = server
	}

	for _, key := range keys {
		for _, serverHost := range key.Servers {
			server, exists := serverDetails[serverHost]
			if !exists {
				log.Printf("Server %s not found in config, skipping cleanup", serverHost)
				continue
			}

			cleanupCmd := fmt.Sprintf("sed -i '/%s/d' ~/.ssh/authorized_keys", key.KeyID)
			client, err := NewSSHClient(server, nil)

			if err != nil {
				log.Printf("Failed to connect to %s: %v", serverHost, err)
				continue
			}

			if err := client.ExecuteCommand(cleanupCmd); err != nil {
				log.Printf("Failed to remove key from %s: %v", serverHost, err)
			}

			log.Printf("STALE KEYS REMOVED")

			client.Close()
		}

		if err := db.MarkKeyAsCleaned(ctx, key.KeyID); err != nil {
			log.Printf("Failed to mark key %s as cleaned: %v", key.KeyID, err)
		}
	}

	return nil
}

func WaitForSSHReadiness(ctx context.Context, servers []types.Server, sshManager *TempSSHManager) error {
	// spinner := ui.NewStepSpinner("SSH Setup")
	// spinner.Start("Waiting for SSH access...")

	statusChan := make(chan ServerStatus)
	doneChan := make(chan struct{})
	serverCount := len(servers)
	readyServers := make(map[string]bool)

	go func() {
		ticker := time.NewTicker(3 * time.Second)
		defer ticker.Stop()

		for {
			select {
			case <-ctx.Done():
				return
			case <-doneChan:
				return
			case <-ticker.C:
				checkServers(servers, sshManager, statusChan)
			}
		}
	}()

	for status := range statusChan {
		if status.Ready {
			readyServers[status.Host] = true
			remaining := serverCount - len(readyServers)
			if remaining > 0 {
				// spinner.Start(fmt.Sprintf("SSH access established for %s (%d servers remaining)",
				// 	status.Host, remaining))
			}
		}

		if len(readyServers) == serverCount {
			close(doneChan)
			// spinner.Stop(true)
			fmt.Println("✅ SSH setup complete! All servers are accessible.")
			return nil
		}
	}

	return nil
}

func checkServers(servers []types.Server, sshManager *TempSSHManager, statusChan chan<- ServerStatus) {
	var wg sync.WaitGroup

	for _, server := range servers {
		wg.Add(1)
		go func(server types.Server) {
			defer wg.Done()

			status := ServerStatus{
				Host: server.Host,
			}

			sshConfig, err := sshManager.GetSSHConfig(server.Host)
			if err != nil {
				status.Error = err
				statusChan <- status
				return
			}

			client, err := NewSSHClient(server, sshConfig)
			if err != nil {
				status.Error = err
				statusChan <- status
				return
			}
			defer client.Close()

			if _, err := client.ExecuteCommandWithOutput("echo 'test'"); err != nil {
				status.Error = err
				statusChan <- status
				return
			}

			status.Ready = true
			statusChan <- status
		}(server)
	}

	wg.Wait()
}
