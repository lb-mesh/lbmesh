package socket

import (
	"database/sql"
	"fmt"

	"github.com/docker/docker/client"
	"github.com/lbmesh/lbmesh-v2/pkg/db"
)

// TeardownScope is the entry-point for `lbmesh down`.
//
// It performs the full teardown sequence for a single scope instance:
//
//  1. Deregisters each containerID from the network_containers table.
//  2. Checks whether any other containers remain on the network (SQLite).
//  3. Double-checks with a live NetworkInspect against the daemon.
//  4. Removes the bridge network from the daemon and deletes the networks row
//     — but ONLY when the scope is "project" or "custom".
//     The "system" (lbmesh-global) network is intentionally preserved so that
//     other system-scoped services that are still running are not disrupted.
//
// Parameters:
//   - c           – verified Docker/Podman client (from socket.Detect).
//   - database    – open *sql.DB with migrations applied.
//   - scope       – the scope that is being torn down.
//   - projectDir  – the project directory path (required for ScopeProject).
//   - instance    – the custom instance name (required for ScopeCustom).
//   - containerIDs – the container IDs that were started for this scope instance.
func TeardownScope(
	c *client.Client,
	database *sql.DB,
	scope Scope,
	projectDir string,
	instance string,
	containerIDs []string,
) error {
	networkName, err := NetworkNameForScope(scope, projectDir, instance)
	if err != nil {
		return err
	}

	// Look up the network record so we have the daemon network ID.
	netRecord, err := db.GetNetwork(database, networkName)
	if err == sql.ErrNoRows {
		// Network was never registered — nothing to clean up.
		return nil
	}
	if err != nil {
		return fmt.Errorf("looking up network %q in state: %w", networkName, err)
	}

	// Step 1: Deregister each container from the SQLite association table.
	for _, cid := range containerIDs {
		if err := db.DeregisterContainerFromNetwork(database, netRecord.ID, cid); err != nil {
			// Non-fatal: log but continue so we don't leave the network dangling.
			fmt.Printf("warning: deregistering container %s from network %s: %v\n", cid, networkName, err)
		}
	}

	// Step 2: Check whether the network is still in use (SQLite view).
	dbCount, err := db.ActiveContainerCountOnNetwork(database, netRecord.ID)
	if err != nil {
		return fmt.Errorf("counting active containers on network %q: %w", networkName, err)
	}

	// Step 3: Corroborate with a live daemon count to guard against stale state.
	daemonCount, err := ActiveContainerCount(c, netRecord.ID)
	if err != nil {
		return fmt.Errorf("live container count for network %q: %w", networkName, err)
	}

	stillInUse := dbCount > 0 || daemonCount > 0

	// Step 4: Remove the network only for isolated scopes and when no containers
	// remain.  The global system network is never removed automatically.
	if scope == ScopeSystem {
		// The lbmesh-global network outlives any single service teardown.
		return nil
	}

	if stillInUse {
		// Other containers are still connected — leave the network alive.
		return nil
	}

	// No remaining containers: remove the bridge from the daemon.
	if err := RemoveNetwork(c, netRecord.ID); err != nil {
		return fmt.Errorf("removing network %q from daemon: %w", networkName, err)
	}

	// Remove the state record from SQLite (CASCADE deletes network_containers).
	if err := db.DeleteNetwork(database, netRecord.ID); err != nil {
		return fmt.Errorf("deleting network state for %q: %w", networkName, err)
	}

	return nil
}

// RegisterScopeNetwork is the counter-part called during `lbmesh up`.
//
// After EnsureNetwork / ResolveNetwork has returned a live NetworkInfo, this
// function persists that network to SQLite and records each containerID as a
// member of it.  Callers should invoke this once per service start, passing a
// single-element containerIDs slice for each container.
func RegisterScopeNetwork(
	database *sql.DB,
	info NetworkInfo,
	scope Scope,
	containerID string,
	serviceID string,
) error {
	// Persist the network record (idempotent upsert).
	record := db.Network{
		ID:     info.ID,
		Name:   info.Name,
		Scope:  string(scope),
		Driver: info.Driver,
		Subnet: info.Subnet,
	}
	if err := db.UpsertNetwork(database, record); err != nil {
		return err
	}

	// Record the container association.
	if err := db.RegisterContainerOnNetwork(database, info.ID, containerID, serviceID); err != nil {
		return err
	}

	return nil
}
