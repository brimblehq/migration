package db

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"os"

	"github.com/brimblehq/migration/internal/types"
	"github.com/google/uuid"
	_ "github.com/lib/pq"
)

// const createTablesSQL = `
// -- Enable UUID extension
// CREATE EXTENSION IF NOT EXISTS "uuid-ossp";

// -- Create servers table
// CREATE TABLE servers (
// 	id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
//     machine_id VARCHAR(255) PRIMARY KEY,
//     public_ip VARCHAR(45),
//     private_ip VARCHAR(45),
//     role VARCHAR(50),
//     status VARCHAR(20),
//     identifier VARCHAR(255),
//     step VARCHAR(50),
// 	consul_address VARCHAR(60)
//     created_at TIMESTAMP NOT NULL,
//     updated_at TIMESTAMP NOT NULL
// );

// -- Create ssh table
// CREATE TABLE temp_ssh_keys (
//     id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
//     key_id VARCHAR(255) NOT NULL,
//     public_key TEXT NOT NULL,
//     created_at TIMESTAMP NOT NULL DEFAULT NOW(),
//     expires_at TIMESTAMP NOT NULL,
//     status VARCHAR(20) NOT NULL,
//     cleanup_attempted_at TIMESTAMP,
//     servers JSONB NOT NULL,
//     UNIQUE(key_id)
// );

// -- Create providers table
// CREATE TABLE IF NOT EXISTS providers (
//     id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
//     provider_name VARCHAR(50) NOT NULL UNIQUE,
//     created_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP
// );

// -- Create regions table
// CREATE TABLE IF NOT EXISTS regions (
//     id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
//     name VARCHAR(50) NOT NULL,
//     type VARCHAR(50)  NOT NULL,
//     provider_id UUID REFERENCES providers(id),
//     created_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP,
//     UNIQUE(name, provider_id)
// );

// -- Create machines table
// CREATE TABLE IF NOT EXISTS machines (
//     id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
//     size VARCHAR(50) NOT NULL,
//     image VARCHAR(100) NOT NULL,
//     description TEXT,
//     use_case TEXT,
//     recommended_role VARCHAR(50),
//     provider_id UUID REFERENCES providers(id),
//     region_id UUID REFERENCES regions(id),
//     created_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP,
//     UNIQUE(size, provider_id, region_id)
// );
// `

func (p *PostgresDB) SeedCloudProviders(jsonPath string) error {
	jsonData, err := os.ReadFile(jsonPath)
	if err != nil {
		return fmt.Errorf("error reading JSON file: %v", err)
	}

	var providers []types.Provider
	if err := json.Unmarshal(jsonData, &providers); err != nil {
		return fmt.Errorf("error unmarshaling JSON: %v", err)
	}

	tx, err := p.db.Begin()
	if err != nil {
		return fmt.Errorf("error starting transaction: %v", err)
	}
	defer tx.Rollback()

	insertProvider, err := tx.Prepare(`
        INSERT INTO providers (id, provider_name)
        VALUES ($1, $2)
        ON CONFLICT (provider_name) DO UPDATE 
        SET provider_name = EXCLUDED.provider_name
        RETURNING id
    `)

	if err != nil {
		return fmt.Errorf("error preparing provider statement: %v", err)
	}

	insertRegion, err := tx.Prepare(`
        INSERT INTO regions (id, name, type, provider_id)
        VALUES ($1, $2, $3, $4)
        ON CONFLICT (name, provider_id) DO UPDATE 
        SET type = EXCLUDED.type
        RETURNING id
    `)
	if err != nil {
		return fmt.Errorf("error preparing region statement: %v", err)
	}

	insertMachine, err := tx.Prepare(`
        INSERT INTO machines (
            id,
            size, 
            image, 
            description, 
            use_case, 
            recommended_role, 
            provider_id, 
            region_id
        )
        VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
        ON CONFLICT (size, provider_id, region_id) 
        DO UPDATE SET 
            image = EXCLUDED.image,
            description = EXCLUDED.description,
            use_case = EXCLUDED.use_case,
            recommended_role = EXCLUDED.recommended_role
    `)
	if err != nil {
		return fmt.Errorf("error preparing machine statement: %v", err)
	}

	for _, provider := range providers {
		providerID := uuid.New()

		err := insertProvider.QueryRow(providerID, provider.Name).Scan(&providerID)
		if err != nil && err != sql.ErrNoRows {
			return fmt.Errorf("error inserting provider %s: %v", provider.Name, err)
		}

		processedRegions := make(map[string]uuid.UUID)

		for _, machine := range provider.Machines {
			machineID := uuid.New()

			regionKey := machine.Region.Name + provider.ID
			var regionID uuid.UUID

			if id, exists := processedRegions[regionKey]; exists {
				regionID = id
			} else {
				regionID = uuid.New()
				err := insertRegion.QueryRow(
					regionID,
					machine.Region.Name,
					machine.Region.Type,
					providerID,
				).Scan(&regionID)
				if err != nil {
					return fmt.Errorf("error inserting region %s: %v", machine.Region.Name, err)
				}
				processedRegions[regionKey] = regionID
			}

			_, err = insertMachine.Exec(
				machineID,
				machine.Size,
				machine.Image,
				machine.Description,
				machine.UseCase,
				machine.Role,
				providerID,
				regionID,
			)
			if err != nil {
				return fmt.Errorf("error inserting machine %s: %v", machine.Size, err)
			}
		}
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("error committing transaction: %v", err)
	}

	return nil
}

func (p *PostgresDB) GetMachinesByProviderAndRegion(providerName string, regionType string) ([]types.Machine, error) {
	query := `
        SELECT 
            m.id,
            m.size,
            m.image,
            m.description,
            m.use_case,
            m.recommended_role,
            r.name as region_name,
            r.type as region_type
        FROM machines m
        JOIN providers p ON m.provider_id = p.id
        JOIN regions r ON m.region_id = r.id
        WHERE p.provider_name = $1
        AND r.type = $2
    `

	rows, err := p.db.Query(query, providerName, regionType)
	if err != nil {
		return nil, fmt.Errorf("error querying machines: %v", err)
	}
	defer rows.Close()

	var machines []types.Machine
	for rows.Next() {
		var m types.Machine
		var region types.Region
		if err := rows.Scan(
			&m.ID,
			&m.Size,
			&m.Image,
			&m.Description,
			&m.UseCase,
			&m.Role,
			&region.Name,
			&region.Type,
		); err != nil {
			return nil, fmt.Errorf("error scanning machine row: %v", err)
		}
		m.Region = region
		machines = append(machines, m)
	}

	return machines, nil
}

// in db/postgres.go
func (p *PostgresDB) GetProviderRegions(providerID string) ([]types.RegionOption, error) {
	query := `
	SELECT DISTINCT 
    r.type AS region_type,
    CASE 
        WHEN r.type = 'america' THEN 'America (US)'
        WHEN r.type = 'europe' THEN 'Europe (EU)'
        ELSE r.type
    END AS display_name
FROM regions r
JOIN providers p ON r.provider_id = p.id
WHERE p.provider_name = $1
ORDER BY r.type;
    `

	rows, err := p.db.Query(query, providerID)
	if err != nil {
		return nil, fmt.Errorf("error querying regions: %v", err)
	}
	defer rows.Close()

	var regions []types.RegionOption
	for rows.Next() {
		var region types.RegionOption
		if err := rows.Scan(&region.ID, &region.DisplayName); err != nil {
			return nil, fmt.Errorf("error scanning region row: %v", err)
		}
		regions = append(regions, region)
	}

	if err = rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating region rows: %v", err)
	}

	return regions, nil
}

func (p *PostgresDB) GetProviderConfigs() ([]types.Provider, error) {
	query := `
        WITH provider_data AS (
            SELECT 
                p.id as provider_id,
                p.provider_name,
                m.id as machine_id,
                m.size,
                m.image,
                m.description,
                m.use_case,
                m.recommended_role as role,
                r.name as region_name,
                r.type as region_type
            FROM providers p
            LEFT JOIN machines m ON m.provider_id = p.id
            LEFT JOIN regions r ON m.region_id = r.id
            ORDER BY p.provider_name, r.type, m.size
        )
        SELECT 
            provider_name,
            COALESCE(
                jsonb_agg(
                    CASE 
                        WHEN machine_id IS NOT NULL THEN
                            jsonb_build_object(
                                'id', machine_id,
                                'size', size,
                                'image', image,
                                'description', description,
                                'use_case', use_case,
                                'recommended_role', role,
                                'region', jsonb_build_object(
                                    'name', region_name,
                                    'type', region_type
                                )
                            )
                        ELSE NULL
                    END
                ) FILTER (WHERE machine_id IS NOT NULL),
                '[]'::jsonb
            ) as machines
        FROM provider_data
        GROUP BY provider_name;
    `

	rows, err := p.db.Query(query)
	if err != nil {
		return nil, fmt.Errorf("error querying providers: %v", err)
	}
	defer rows.Close()

	var providers []types.Provider
	for rows.Next() {
		var provider types.Provider
		var machinesJSON []byte

		if err := rows.Scan(&provider.ID, &machinesJSON); err != nil {
			return nil, fmt.Errorf("error scanning provider row: %v", err)
		}

		if err := json.Unmarshal(machinesJSON, &provider.Machines); err != nil {
			return nil, fmt.Errorf("error unmarshaling machines for provider %s: %v", provider.ID, err)
		}

		providers = append(providers, provider)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating provider rows: %v", err)
	}

	return providers, nil
}

func (p *PostgresDB) GetProviderConfig(providerName string) (*types.Provider, error) {
	query := `
        WITH provider_data AS (
            SELECT 
                p.id as provider_id,
                p.provider_name,
                m.id as machine_id,
                m.size,
                m.image,
                m.description,
                m.use_case,
                m.recommended_role as role,
                r.name as region_name,
                r.type as region_type
            FROM providers p
            LEFT JOIN machines m ON m.provider_id = p.id
            LEFT JOIN regions r ON m.region_id = r.id
            WHERE p.provider_name = $1
            ORDER BY r.type, m.size
        )
        SELECT 
            provider_name,
            COALESCE(
                jsonb_agg(
                    CASE 
                        WHEN machine_id IS NOT NULL THEN
                            jsonb_build_object(
                                'id', machine_id,
                                'size', size,
                                'image', image,
                                'description', description,
                                'use_case', use_case,
                                'recommended_role', role,
                                'region', jsonb_build_object(
                                    'name', region_name,
                                    'type', region_type
                                )
                            )
                        ELSE NULL
                    END
                ) FILTER (WHERE machine_id IS NOT NULL),
                '[]'::jsonb
            ) as machines
        FROM provider_data
        GROUP BY provider_name;
    `

	var provider types.Provider
	var machinesJSON []byte

	err := p.db.QueryRow(query, providerName).Scan(&provider.ID, &machinesJSON)
	if err == sql.ErrNoRows {
		return nil, fmt.Errorf("provider %s not found", providerName)
	}
	if err != nil {
		return nil, fmt.Errorf("error querying provider %s: %v", providerName, err)
	}

	if err := json.Unmarshal(machinesJSON, &provider.Machines); err != nil {
		return nil, fmt.Errorf("error unmarshaling machines for provider %s: %v", providerName, err)
	}

	return &provider, nil
}

func (p *PostgresDB) GetMachinesByProviderAndRegionType(providerName, regionType string) ([]types.Machine, error) {
	query := `
        SELECT 
            m.id,
            m.size,
            m.image,
            m.description,
            m.use_case,
            m.recommended_role,
            r.name,
            r.type
        FROM machines m
        JOIN providers p ON m.provider_id = p.id
        JOIN regions r ON m.region_id = r.id
        WHERE p.provider_name = $1 AND r.type = $2
        ORDER BY m.size;
    `

	rows, err := p.db.Query(query, providerName, regionType)
	if err != nil {
		return nil, fmt.Errorf("error querying machines: %v", err)
	}
	defer rows.Close()

	var machines []types.Machine
	for rows.Next() {
		var m types.Machine
		var region types.Region
		if err := rows.Scan(
			&m.ID,
			&m.Size,
			&m.Image,
			&m.Description,
			&m.UseCase,
			&m.Role,
			&region.Name,
			&region.Type,
		); err != nil {
			return nil, fmt.Errorf("error scanning machine row: %v", err)
		}
		m.Region = region
		machines = append(machines, m)
	}

	return machines, nil
}
