package db

import (
	"database/sql"
	"path/filepath"
	"sync"
	"testing"
)

func setupTestDB(t *testing.T) (*sql.DB, string) {
	t.Helper()
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test_state.db")

	database, err := OpenAndMigrate(dbPath)
	if err != nil {
		t.Fatalf("OpenAndMigrate failed: %v", err)
	}

	return database, dbPath
}

func TestOpenAndMigrate(t *testing.T) {
	db, _ := setupTestDB(t)
	defer db.Close()

	// Verify WAL mode
	var journalMode string
	if err := db.QueryRow("PRAGMA journal_mode;").Scan(&journalMode); err != nil {
		t.Fatalf("failed to query journal_mode: %v", err)
	}
	if journalMode != "wal" {
		t.Errorf("expected journal_mode 'wal', got %q", journalMode)
	}

	// Verify tables exist
	tables := []string{"projects", "services", "port_allocations", "schema_migrations"}
	for _, table := range tables {
		var name string
		err := db.QueryRow("SELECT name FROM sqlite_master WHERE type='table' AND name=?", table).Scan(&name)
		if err != nil {
			t.Errorf("table %q not found in sqlite_master: %v", table, err)
		}
	}

	// Run migration a second time to ensure idempotence
	if err := Migrate(db); err != nil {
		t.Fatalf("second Migrate call failed (not idempotent): %v", err)
	}
}

func TestForeignKeyCascades(t *testing.T) {
	db, _ := setupTestDB(t)
	defer db.Close()

	// Insert project
	_, err := db.Exec(`
		INSERT INTO projects (id, name, path, network, status)
		VALUES ('proj-1', 'My Project', '/tmp/proj-1', 'lbmesh-project-proj-1', 'active')
	`)
	if err != nil {
		t.Fatalf("failed to insert project: %v", err)
	}

	// Insert service referencing project
	_, err = db.Exec(`
		INSERT INTO services (id, project_id, name, scope, container_id, image, status)
		VALUES ('svc-1', 'proj-1', 'postgres', 'project', 'cont-123', 'postgres:16', 'running')
	`)
	if err != nil {
		t.Fatalf("failed to insert service: %v", err)
	}

	// Insert port allocation referencing service
	_, err = db.Exec(`
		INSERT INTO port_allocations (service_id, container_port, host_port, protocol)
		VALUES ('svc-1', 5432, 5432, 'tcp')
	`)
	if err != nil {
		t.Fatalf("failed to insert port allocation: %v", err)
	}

	// Verify insertion
	var portCount int
	if err := db.QueryRow("SELECT COUNT(*) FROM port_allocations WHERE service_id = 'svc-1'").Scan(&portCount); err != nil {
		t.Fatalf("failed to query port_allocations: %v", err)
	}
	if portCount != 1 {
		t.Fatalf("expected 1 port allocation, got %d", portCount)
	}

	// Delete project and verify cascade deletes service and port_allocation
	_, err = db.Exec("DELETE FROM projects WHERE id = 'proj-1'")
	if err != nil {
		t.Fatalf("failed to delete project: %v", err)
	}

	var svcCount int
	if err := db.QueryRow("SELECT COUNT(*) FROM services WHERE id = 'svc-1'").Scan(&svcCount); err != nil {
		t.Fatalf("failed to query services: %v", err)
	}
	if svcCount != 0 {
		t.Errorf("expected 0 services after cascade delete, got %d", svcCount)
	}

	if err := db.QueryRow("SELECT COUNT(*) FROM port_allocations WHERE service_id = 'svc-1'").Scan(&portCount); err != nil {
		t.Fatalf("failed to query port_allocations: %v", err)
	}
	if portCount != 0 {
		t.Errorf("expected 0 port_allocations after cascade delete, got %d", portCount)
	}
}

func TestConcurrentAccess(t *testing.T) {
	db, dbPath := setupTestDB(t)
	defer db.Close()

	const numWorkers = 10
	const opsPerWorker = 20

	var wg sync.WaitGroup
	wg.Add(numWorkers)

	for i := 0; i < numWorkers; i++ {
		workerID := i
		go func() {
			defer wg.Done()
			conn, err := Open(dbPath)
			if err != nil {
				t.Errorf("worker %d failed to open connection: %v", workerID, err)
				return
			}
			defer conn.Close()

			for j := 0; j < opsPerWorker; j++ {
				projID := filepath.Join("proj", string(rune('A'+workerID)), string(rune('0'+j)))
				_, err := conn.Exec(`
					INSERT INTO projects (id, name, path, network, status)
					VALUES (?, ?, ?, ?, 'active')
				`, projID, "Worker Project", "/tmp/"+projID, "net-"+projID)
				if err != nil {
					t.Errorf("worker %d insert %d failed: %v", workerID, j, err)
				}

				var count int
				if err := conn.QueryRow("SELECT COUNT(*) FROM projects").Scan(&count); err != nil {
					t.Errorf("worker %d read failed: %v", workerID, err)
				}
			}
		}()
	}

	wg.Wait()

	var totalProjects int
	if err := db.QueryRow("SELECT COUNT(*) FROM projects").Scan(&totalProjects); err != nil {
		t.Fatalf("failed to query total projects: %v", err)
	}
	expected := numWorkers * opsPerWorker
	if totalProjects != expected {
		t.Errorf("expected %d total projects, got %d", expected, totalProjects)
	}
}
