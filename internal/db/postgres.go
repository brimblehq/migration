package db

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"hash/fnv"
	"time"

	"github.com/brimblehq/migration/internal/types"
	_ "github.com/lib/pq"
)

type PostgresDB struct {
	db *sql.DB
}

type DbConfig struct {
	URI string
}

type TempSSHKey struct {
	ID                 int64      `json:"id"`
	KeyID              string     `json:"key_id"`
	PublicKey          string     `json:"public_key"`
	CreatedAt          time.Time  `json:"created_at"`
	ExpiresAt          time.Time  `json:"expires_at"`
	Status             string     `json:"status"`
	CleanupAttemptedAt *time.Time `json:"cleanup_attempted_at,omitempty"`
	Servers            []string   `json:"servers"`
}

func NewPostgresDB(config DbConfig) (*PostgresDB, error) {
	db, err := sql.Open("postgres", fmt.Sprintf(
		"host=%s port=%d user=%s password=%s dbname=%s sslmode=%s",
		"aws-0-eu-west-2.pooler.supabase.com", 6543, "postgres.bwpuiyfchjkhezkypxpm", "xhGUnf75yy3Afyb#", "postgres", "require",
	))

	if err != nil {
		return nil, fmt.Errorf("error connecting to database: %v", err)
	}

	db.SetMaxOpenConns(10)
	db.SetMaxIdleConns(5)
	db.SetConnMaxLifetime(time.Hour)
	db.SetConnMaxIdleTime(30 * time.Minute)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := db.PingContext(ctx); err != nil {
		return nil, fmt.Errorf("error pinging database: %v", err)
	}

	return &PostgresDB{db: db}, nil
}

func (p *PostgresDB) Close() error {
	return p.db.Close()
}

func (p *PostgresDB) RegisterServer(machineID, publicIP, privateIP, role string, identifier string, step types.ServerStep) error {
	fmt.Printf("Input parameters: machineID=%s, publicIP=%s, privateIP=%s, role=%s, identifier=%s, step=%v\n",
		machineID, publicIP, privateIP, role, identifier, step)

	tx, err := p.db.Begin()
	if err != nil {
		return fmt.Errorf("failed to begin transaction: %v", err)
	}
	defer tx.Rollback()

	_, err = tx.Exec("SELECT pg_advisory_xact_lock($1)", hashString(machineID))
	if err != nil {
		return fmt.Errorf("failed to acquire lock: %v", err)
	}

	query := `
        INSERT INTO servers (machine_id, public_ip, private_ip, role, status, identifier, step, created_at, updated_at)
        VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $8)
        ON CONFLICT (machine_id) DO UPDATE
        SET status = $5, updated_at = $8
    `
	_, err = tx.Exec(query,
		machineID,
		publicIP,
		privateIP,
		role,
		"active",
		identifier,
		step,
		time.Now(),
	)

	fmt.Printf("ERROR: %v", err)

	if err != nil {
		return fmt.Errorf("failed to execute query: %v", err)
	}

	return tx.Commit()
}

func (p *PostgresDB) UpdateServerRole(machineID, role string) error {
	tx, err := p.db.Begin()
	if err != nil {
		return fmt.Errorf("failed to begin transaction: %v", err)
	}
	defer tx.Rollback()

	_, err = tx.Exec("SELECT pg_advisory_xact_lock($1)", hashString(machineID))
	if err != nil {
		return fmt.Errorf("failed to acquire lock: %v", err)
	}

	query := `
        UPDATE servers
        SET role = $1, updated_at = $2
        WHERE machine_id = $3
    `

	_, err = tx.Exec(query, role, time.Now(), machineID)
	if err != nil {
		return fmt.Errorf("failed to update role: %v", err)
	}

	return tx.Commit()
}

func (p *PostgresDB) UpdateServerStep(machineID string, step types.ServerStep) error {
	tx, err := p.db.Begin()
	if err != nil {
		return fmt.Errorf("failed to begin transaction: %v", err)
	}
	defer tx.Rollback()

	_, err = tx.Exec("SELECT pg_advisory_xact_lock($1)", hashString(machineID))
	if err != nil {
		return fmt.Errorf("failed to acquire lock: %v", err)
	}

	query := `
        UPDATE servers 
        SET step = $1, updated_at = $2
        WHERE machine_id = $3
    `

	_, err = tx.Exec(query, step, time.Now(), machineID)
	if err != nil {
		return fmt.Errorf("failed to update step: %v", err)
	}

	return tx.Commit()
}

func hashString(s string) int64 {
	h := fnv.New64a()
	h.Write([]byte(s))
	return int64(h.Sum64())
}

func (p *PostgresDB) IsServerRegistered(machineID string) (bool, error) {
	var exists bool
	query := `
        SELECT EXISTS(
            SELECT 1 FROM servers 
            WHERE machine_id = $1 AND status = 'active'
        )
    `

	err := p.db.QueryRow(query, machineID).Scan(&exists)
	if err != nil {
		return false, fmt.Errorf("error checking server registration: %v", err)
	}

	return exists, nil
}

func (p *PostgresDB) GetAllServers() ([]types.ServerState, error) {
	query := `
        SELECT id, machine_id, public_ip, private_ip, role, status, created_at, updated_at
        FROM servers
        WHERE status = 'active'
        ORDER BY created_at ASC
    `

	rows, err := p.db.Query(query)
	if err != nil {
		return nil, fmt.Errorf("error querying servers: %v", err)
	}
	defer rows.Close()

	var servers []types.ServerState
	for rows.Next() {
		var server types.ServerState
		err := rows.Scan(
			&server.ID,
			&server.MachineID,
			&server.PublicIP,
			&server.PrivateIP,
			&server.Role,
			&server.Status,
			&server.CreatedAt,
			&server.UpdatedAt,
		)
		if err != nil {
			return nil, fmt.Errorf("error scanning server row: %v", err)
		}
		servers = append(servers, server)
	}

	return servers, nil
}

func (p *PostgresDB) GetServerStep(machineID string, identifier string) (types.ServerStep, error) {
	var step types.ServerStep
	query := `SELECT step FROM servers WHERE machine_id = $1 AND identifier = $2`
	err := p.db.QueryRow(query, machineID, identifier).Scan(&step)
	return step, err
}

func (p *PostgresDB) CreateTempSSHKey(ctx context.Context, keyID, publicKey string, servers []string) (*TempSSHKey, error) {
	serversJSON, err := json.Marshal(servers)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal servers: %w", err)
	}

	var key TempSSHKey
	var serversBytes []byte

	err = p.db.QueryRowContext(ctx, `
        INSERT INTO temp_ssh_keys (
            key_id, 
            public_key, 
            expires_at, 
            servers,
            status
        ) VALUES (
            $1, 
            $2, 
            $3, 
            $4,
            'active'
        ) RETURNING id, key_id, public_key, created_at, expires_at, status, cleanup_attempted_at, servers`,
		keyID,
		publicKey,
		time.Now().Add(2*time.Hour),
		serversJSON,
	).Scan(
		&key.ID,
		&key.KeyID,
		&key.PublicKey,
		&key.CreatedAt,
		&key.ExpiresAt,
		&key.Status,
		&key.CleanupAttemptedAt,
		&serversBytes,
	)

	if err != nil {
		return nil, fmt.Errorf("failed to create temp SSH key: %w", err)
	}

	if err := json.Unmarshal(serversBytes, &key.Servers); err != nil {
		return nil, fmt.Errorf("failed to unmarshal servers: %w", err)
	}

	return &key, nil
}

func (p *PostgresDB) GetActiveKeyByID(ctx context.Context, keyID string) (*TempSSHKey, error) {
	var key TempSSHKey
	var serversBytes []byte

	err := p.db.QueryRowContext(ctx, `
        SELECT 
            id, 
            key_id, 
            public_key, 
            created_at, 
            expires_at, 
            status, 
            cleanup_attempted_at, 
            servers
        FROM temp_ssh_keys
        WHERE key_id = $1 
        AND status = 'active'
        AND expires_at > NOW()`,
		keyID,
	).Scan(
		&key.ID,
		&key.KeyID,
		&key.PublicKey,
		&key.CreatedAt,
		&key.ExpiresAt,
		&key.Status,
		&key.CleanupAttemptedAt,
		&serversBytes,
	)

	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("failed to get temp SSH key: %w", err)
	}

	if err := json.Unmarshal(serversBytes, &key.Servers); err != nil {
		return nil, fmt.Errorf("failed to unmarshal servers: %w", err)
	}

	return &key, nil
}

func (p *PostgresDB) MarkKeyAsExpired(ctx context.Context, keyID string) error {
	_, err := p.db.ExecContext(ctx, `
        UPDATE temp_ssh_keys 
        SET status = 'expired' 
        WHERE key_id = $1 
        AND status = 'active'`,
		keyID,
	)
	return err
}

func (p *PostgresDB) MarkKeyAsCleaned(ctx context.Context, keyID string) error {
	_, err := p.db.ExecContext(ctx, `
        UPDATE temp_ssh_keys 
        SET 
            status = 'cleaned',
            cleanup_attempted_at = NOW()
        WHERE key_id = $1`,
		keyID,
	)
	return err
}

func (p *PostgresDB) GetExpiredUncleaned(ctx context.Context) ([]TempSSHKey, error) {
	rows, err := p.db.QueryContext(ctx, `
        SELECT 
            id, 
            key_id, 
            public_key, 
            created_at, 
            expires_at, 
            status, 
            cleanup_attempted_at, 
            servers::text  -- Convert JSONB to text for scanning
        FROM temp_ssh_keys
        WHERE (status = 'active' AND expires_at <= NOW())
        OR (status = 'expired' AND cleanup_attempted_at IS NULL)`,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to query expired keys: %w", err)
	}
	defer rows.Close()

	var keys []TempSSHKey
	for rows.Next() {
		var key TempSSHKey
		var serversJSON string

		err := rows.Scan(
			&key.ID,
			&key.KeyID,
			&key.PublicKey,
			&key.CreatedAt,
			&key.ExpiresAt,
			&key.Status,
			&key.CleanupAttemptedAt,
			&serversJSON,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan key: %w", err)
		}

		if err := json.Unmarshal([]byte(serversJSON), &key.Servers); err != nil {
			return nil, fmt.Errorf("failed to parse servers JSON: %w", err)
		}

		keys = append(keys, key)
	}

	if err = rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating rows: %w", err)
	}

	fmt.Printf("Total keys found: %d\n", len(keys))
	return keys, nil
}

func (p *PostgresDB) SaveConsulAddress(address, machineID string) error {
	tx, err := p.db.Begin()
	if err != nil {
		return fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer tx.Rollback()

	query := `
        UPDATE servers 
        SET consul_address = $1,
            updated_at = NOW()
        WHERE machine_id = $2`

	fmt.Printf("Executing SQL: %s with params [address=%s, machineID=%s]\n",
		query, address, machineID)

	result, err := tx.Exec(query, address, machineID)
	if err != nil {
		return fmt.Errorf("failed to update consul address: %w", err)
	}

	rows, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("failed to get rows affected: %w", err)
	}

	if rows == 0 {
		return fmt.Errorf("no server found with machine_id: %s", machineID)
	}

	if err = tx.Commit(); err != nil {
		return fmt.Errorf("failed to commit transaction: %w", err)
	}

	return nil
}
func (p *PostgresDB) GetConsulAddress() (string, error) {
	var addr string
	err := p.db.QueryRow(`
        SELECT consul_address 
        FROM servers 
        WHERE consul_address IS NOT NULL 
        LIMIT 1`).Scan(&addr)

	if err == sql.ErrNoRows {
		return "", fmt.Errorf("no consul server address found")
	}
	return addr, err
}
