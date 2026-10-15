package db

import (
	"database/sql"
	"fmt"
)

// Migration represents a single versioned database migration step.
type Migration struct {
	Version int
	Name    string
	SQL     string
}

// migrations is the ordered list of all schema migrations.
var migrations = []Migration{
	{
		Version: 1,
		Name:    "001_initial_schema",
		SQL: `
CREATE TABLE IF NOT EXISTS projects (
    id TEXT PRIMARY KEY,
    name TEXT NOT NULL,
    path TEXT NOT NULL UNIQUE,
    network TEXT NOT NULL DEFAULT '',
    status TEXT NOT NULL DEFAULT 'active',
    created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE IF NOT EXISTS services (
    id TEXT PRIMARY KEY,
    project_id TEXT,
    name TEXT NOT NULL,
    scope TEXT NOT NULL,
    container_id TEXT NOT NULL DEFAULT '',
    image TEXT NOT NULL DEFAULT '',
    status TEXT NOT NULL DEFAULT 'created',
    created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    FOREIGN KEY (project_id) REFERENCES projects(id) ON DELETE CASCADE
);

CREATE TABLE IF NOT EXISTS port_allocations (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    service_id TEXT NOT NULL,
    container_port INTEGER NOT NULL,
    host_port INTEGER NOT NULL,
    protocol TEXT NOT NULL DEFAULT 'tcp',
    allocated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    FOREIGN KEY (service_id) REFERENCES services(id) ON DELETE CASCADE
);

CREATE INDEX IF NOT EXISTS idx_services_project_id ON services(project_id);
CREATE INDEX IF NOT EXISTS idx_services_scope ON services(scope);
CREATE INDEX IF NOT EXISTS idx_port_allocations_service_id ON port_allocations(service_id);
CREATE INDEX IF NOT EXISTS idx_port_allocations_host_port ON port_allocations(host_port, protocol);
`,
	},
	{
		Version: 2,
		Name:    "002_networks",
		SQL: `
-- Tracks every bridge network managed by lbmesh, keyed by the Docker/Podman
-- network ID.  Rows are inserted when a network is created and deleted when it
-- is torn down so that the table always reflects live state.
CREATE TABLE IF NOT EXISTS networks (
    id TEXT PRIMARY KEY,            -- Docker/Podman network ID
    name TEXT NOT NULL UNIQUE,      -- e.g. "lbmesh-global", "lbmesh-project-myapp"
    scope TEXT NOT NULL,            -- "system" | "project" | "custom"
    driver TEXT NOT NULL DEFAULT 'bridge',
    subnet TEXT NOT NULL DEFAULT '',
    created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);

-- Associates a running container with the network it joined.  Multiple rows
-- can exist for the same network when several containers share it.
CREATE TABLE IF NOT EXISTS network_containers (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    network_id TEXT NOT NULL,
    container_id TEXT NOT NULL,
    service_id TEXT NOT NULL,
    connected_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    FOREIGN KEY (network_id) REFERENCES networks(id) ON DELETE CASCADE,
    FOREIGN KEY (service_id) REFERENCES services(id) ON DELETE CASCADE,
    UNIQUE (network_id, container_id)
);

CREATE INDEX IF NOT EXISTS idx_networks_scope ON networks(scope);
CREATE INDEX IF NOT EXISTS idx_network_containers_network_id ON network_containers(network_id);
CREATE INDEX IF NOT EXISTS idx_network_containers_service_id ON network_containers(service_id);
`,
	},
}

// Migrate executes all pending database migrations in sequential order.
func Migrate(db *sql.DB) error {
	// Ensure migration tracking table exists
	initMigrationsTable := `
CREATE TABLE IF NOT EXISTS schema_migrations (
    version INTEGER PRIMARY KEY,
    name TEXT NOT NULL,
    applied_at DATETIME DEFAULT CURRENT_TIMESTAMP
);`
	if _, err := db.Exec(initMigrationsTable); err != nil {
		return fmt.Errorf("creating schema_migrations table: %w", err)
	}

	for _, m := range migrations {
		var exists bool
		err := db.QueryRow("SELECT EXISTS(SELECT 1 FROM schema_migrations WHERE version = ?)", m.Version).Scan(&exists)
		if err != nil {
			return fmt.Errorf("checking migration status for version %d: %w", m.Version, err)
		}

		if exists {
			continue
		}

		// Execute migration within a transaction
		tx, err := db.Begin()
		if err != nil {
			return fmt.Errorf("beginning transaction for migration %d (%s): %w", m.Version, m.Name, err)
		}

		if _, err := tx.Exec(m.SQL); err != nil {
			_ = tx.Rollback()
			return fmt.Errorf("executing migration %d (%s): %w", m.Version, m.Name, err)
		}

		if _, err := tx.Exec("INSERT INTO schema_migrations (version, name) VALUES (?, ?)", m.Version, m.Name); err != nil {
			_ = tx.Rollback()
			return fmt.Errorf("recording migration %d (%s): %w", m.Version, m.Name, err)
		}

		if err := tx.Commit(); err != nil {
			return fmt.Errorf("committing migration %d (%s): %w", m.Version, m.Name, err)
		}
	}

	return nil
}

// OpenAndMigrate opens the SQLite database at dbPath and runs all pending migrations.
func OpenAndMigrate(dbPath string) (*sql.DB, error) {
	database, err := Open(dbPath)
	if err != nil {
		return nil, err
	}

	if err := Migrate(database); err != nil {
		_ = database.Close()
		return nil, fmt.Errorf("running database migrations: %w", err)
	}

	return database, nil
}

// OpenDefaultAndMigrate opens ~/.lbmesh/state.db and runs all pending migrations.
func OpenDefaultAndMigrate() (*sql.DB, error) {
	path, err := DefaultPath()
	if err != nil {
		return nil, err
	}
	return OpenAndMigrate(path)
}
