// Package db provides SQLite database connection management and migrations
// using the pure-Go modernc.org/sqlite driver.
package db

import (
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"time"

	_ "modernc.org/sqlite"

	"github.com/lbmesh/lbmesh-v2/pkg/dirs"
)

// DefaultPath returns the default absolute path to ~/.lbmesh/state.db.
func DefaultPath() (string, error) {
	root, err := dirs.LocalRoot()
	if err != nil {
		return "", fmt.Errorf("resolving local root directory: %w", err)
	}
	return filepath.Join(root, "state.db"), nil
}

// Open initializes and configures a SQLite database connection with WAL mode,
// busy timeout, foreign key enforcement, and safe connection pool settings.
func Open(dbPath string) (*sql.DB, error) {
	if dbPath == "" {
		var err error
		dbPath, err = DefaultPath()
		if err != nil {
			return nil, err
		}
	}

	// Ensure the parent directory exists
	dir := filepath.Dir(dbPath)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, fmt.Errorf("creating database directory %s: %w", dir, err)
	}

	// Open database connection with modernc.org/sqlite driver
	dsn := fmt.Sprintf("file:%s?_pragma=busy_timeout(5000)&_pragma=journal_mode(WAL)&_pragma=synchronous(NORMAL)&_pragma=foreign_keys(ON)", dbPath)
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, fmt.Errorf("opening sqlite database at %s: %w", dbPath, err)
	}

	// Configure connection pool for safe concurrency in WAL mode
	db.SetMaxOpenConns(10)
	db.SetMaxIdleConns(5)
	db.SetConnMaxLifetime(time.Hour)

	// Explicitly verify and enforce essential PRAGMAs
	pragmas := []string{
		"PRAGMA journal_mode = WAL;",
		"PRAGMA busy_timeout = 5000;",
		"PRAGMA synchronous = NORMAL;",
		"PRAGMA foreign_keys = ON;",
	}

	for _, p := range pragmas {
		if _, err := db.Exec(p); err != nil {
			_ = db.Close()
			return nil, fmt.Errorf("setting pragma %q: %w", p, err)
		}
	}

	// Verify database connectivity
	if err := db.Ping(); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("pinging sqlite database: %w", err)
	}

	return db, nil
}

// OpenDefault opens the default state database (~/.lbmesh/state.db).
func OpenDefault() (*sql.DB, error) {
	path, err := DefaultPath()
	if err != nil {
		return nil, err
	}
	return Open(path)
}
