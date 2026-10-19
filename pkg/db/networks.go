package db

import (
	"database/sql"
	"fmt"
	"time"
)

// ---------------------------------------------------------------------------
// Network persistence helpers
// ---------------------------------------------------------------------------

// UpsertNetwork inserts a new network record or replaces it when a network
// with the same ID already exists (e.g. after a binary restart where the
// daemon still owns the network).
func UpsertNetwork(db *sql.DB, n Network) error {
	if n.CreatedAt.IsZero() {
		n.CreatedAt = time.Now().UTC()
	}
	_, err := db.Exec(
		`INSERT INTO networks (id, name, scope, driver, subnet, created_at)
		 VALUES (?, ?, ?, ?, ?, ?)
		 ON CONFLICT(id) DO UPDATE SET
		     name   = excluded.name,
		     scope  = excluded.scope,
		     driver = excluded.driver,
		     subnet = excluded.subnet`,
		n.ID, n.Name, n.Scope, n.Driver, n.Subnet, n.CreatedAt,
	)
	if err != nil {
		return fmt.Errorf("upserting network %q: %w", n.Name, err)
	}
	return nil
}

// GetNetwork returns the Network record for the given ID or name.
// It first tries to match by ID, then by name, so callers can use either.
// Returns sql.ErrNoRows when no matching record is found.
func GetNetwork(db *sql.DB, idOrName string) (Network, error) {
	var n Network
	var createdAt any

	row := db.QueryRow(
		`SELECT id, name, scope, driver, subnet, created_at
		 FROM networks
		 WHERE id = ? OR name = ?
		 LIMIT 1`,
		idOrName, idOrName,
	)
	err := row.Scan(&n.ID, &n.Name, &n.Scope, &n.Driver, &n.Subnet, &createdAt)
	if err != nil {
		return Network{}, err
	}

	n.CreatedAt = ParseTimestamp(createdAt)
	return n, nil
}

// DeleteNetwork removes the network record and cascades to network_containers.
func DeleteNetwork(db *sql.DB, networkID string) error {
	_, err := db.Exec(`DELETE FROM networks WHERE id = ?`, networkID)
	if err != nil {
		return fmt.Errorf("deleting network %s: %w", networkID, err)
	}
	return nil
}

// ---------------------------------------------------------------------------
// Container–network association helpers
// ---------------------------------------------------------------------------

// RegisterContainerOnNetwork records that containerID (belonging to serviceID)
// has been connected to networkID.  The operation is idempotent: a duplicate
// (network_id, container_id) pair is silently ignored.
func RegisterContainerOnNetwork(db *sql.DB, networkID, containerID, serviceID string) error {
	_, err := db.Exec(
		`INSERT OR IGNORE INTO network_containers
		     (network_id, container_id, service_id, connected_at)
		 VALUES (?, ?, ?, ?)`,
		networkID, containerID, serviceID, time.Now(),
	)
	if err != nil {
		return fmt.Errorf(
			"registering container %s on network %s: %w",
			containerID, networkID, err,
		)
	}
	return nil
}

// DeregisterContainerFromNetwork removes a single container→network
// association.  It is a no-op when the row does not exist.
func DeregisterContainerFromNetwork(db *sql.DB, networkID, containerID string) error {
	_, err := db.Exec(
		`DELETE FROM network_containers
		 WHERE network_id = ? AND container_id = ?`,
		networkID, containerID,
	)
	if err != nil {
		return fmt.Errorf(
			"deregistering container %s from network %s: %w",
			containerID, networkID, err,
		)
	}
	return nil
}

// ActiveContainerCountOnNetwork returns the number of container rows still
// recorded for the given network ID.  Used during teardown to decide whether
// the bridge is safe to remove.
func ActiveContainerCountOnNetwork(db *sql.DB, networkID string) (int, error) {
	var count int
	err := db.QueryRow(
		`SELECT COUNT(1) FROM network_containers WHERE network_id = ?`,
		networkID,
	).Scan(&count)
	if err != nil {
		return 0, fmt.Errorf("counting containers on network %s: %w", networkID, err)
	}
	return count, nil
}
