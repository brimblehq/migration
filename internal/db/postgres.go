package db

import (
	"database/sql"
	"fmt"
	"time"

	"github.com/brimblehq/migration/internal/types"
	_ "github.com/lib/pq"
)

type PostgresDB struct {
	db *sql.DB
}

type Config struct {
	URI string
}

func NewPostgresDB(config Config) (*PostgresDB, error) {
	// fmt.Println(config.URI)
	// postgresql://postgres:xhGUnf75yy3Afyb@db.bwpuiyfchjkhezkypxpm.supabase.co:5432/postgres

	db, err := sql.Open("postgres", "postgresql://ileri:password@localhost:5411/defaultdb?sslmode=disable")
	if err != nil {
		return nil, fmt.Errorf("error connecting to database: %v", err)
	}

	if err := db.Ping(); err != nil {
		return nil, fmt.Errorf("error pinging database: %v", err)
	}

	return &PostgresDB{db: db}, nil
}

func (p *PostgresDB) Close() error {
	return p.db.Close()
}

func (p *PostgresDB) RegisterServer(machineID, publicIP, privateIP, role string, identifier string, step types.ServerStep) error {
	query := `
        INSERT INTO servers (machine_id, public_ip, private_ip, role, status, identifier, step, created_at, updated_at)
        VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $8)
        ON CONFLICT (machine_id) DO UPDATE
        SET status = $5, updated_at = $8
    `

	_, err := p.db.Exec(query,
		machineID,
		publicIP,
		privateIP,
		role,
		"active",
		identifier,
		step,
		time.Now(),
	)

	return err
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

func (p *PostgresDB) UpdateServerRole(machineID, role string) error {
	query := `
        UPDATE servers
        SET role = $1, updated_at = $2
        WHERE machine_id = $3
    `

	_, err := p.db.Exec(query, role, time.Now(), machineID)
	return err
}

func (p *PostgresDB) UpdateServerStep(machineID string, step types.ServerStep) error {
	query := `
        UPDATE servers 
        SET current_step = $1, updated_at = $2
        WHERE machine_id = $3
    `
	_, err := p.db.Exec(query, step, time.Now(), machineID)
	return err
}

func (p *PostgresDB) GetServerStep(machineID string) (types.ServerStep, error) {
	var step types.ServerStep
	query := `SELECT current_step FROM servers WHERE machine_id = $1`
	err := p.db.QueryRow(query, machineID).Scan(&step)
	return step, err
}
