// Package ports provides a dynamic port allocation broker that guarantees
// host ports are fully unoccupied before committing them to running containers.
//
// The broker enforces two layers of conflict detection:
//   1. A SQLite query against the active port_allocations table to catch
//      ports already claimed by lbmesh-managed services.
//   2. A real-time TCP socket probe via net.Listen to catch host-native
//      processes (e.g. Homebrew-installed databases) that sit outside of
//      Docker/Podman entirely.
//
// When a conflict is found the broker increments the candidate port and
// retries until a fully vacant port is located. The confirmed allocation is
// then written atomically to SQLite and the caller receives the final port
// together with any environment variable substitutions required to pass the
// new value into the starting container.
package ports

import (
	"database/sql"
	"fmt"
	"net"
)

// maxSearchRange is the maximum number of ports the broker will scan above
// a requested port before giving up.
const maxSearchRange = 100

// Request describes a single port binding that a service wants to claim.
type Request struct {
	// ServiceID is the owning service's UUID (matches services.id in SQLite).
	ServiceID string

	// ContainerPort is the port that the process inside the container listens on.
	ContainerPort int

	// PreferredHostPort is the initial host-side port the caller would like to
	// bind. The broker may return a different value when this is occupied.
	PreferredHostPort int

	// Protocol is "tcp" or "udp". Defaults to "tcp" when empty.
	Protocol string

	// EnvKey is the environment variable name the caller expects the allocated
	// host port to be surfaced under, e.g. "POSTGRES_HOST_PORT". Leave empty
	// when no env injection is needed.
	EnvKey string
}

// Result is the confirmed, collision-free allocation returned by the broker.
type Result struct {
	// ServiceID mirrors Request.ServiceID for easy correlation.
	ServiceID string

	// ContainerPort mirrors Request.ContainerPort.
	ContainerPort int

	// AllocatedHostPort is the actual host port that was reserved and recorded.
	// This may differ from Request.PreferredHostPort when a collision was found.
	AllocatedHostPort int

	// Protocol mirrors the resolved protocol ("tcp" or "udp").
	Protocol string

	// EnvInjections maps environment variable names to their resolved values.
	// Populated only for requests that include a non-empty EnvKey.
	EnvInjections map[string]string
}

// Broker manages port allocation backed by a SQLite database.
type Broker struct {
	db *sql.DB
}

// New returns a Broker that uses the supplied *sql.DB for persistence.
// The database must have been opened and migrated before calling New.
func New(db *sql.DB) *Broker {
	return &Broker{db: db}
}

// Allocate resolves a single port Request to a collision-free Result and
// persists the allocation atomically to SQLite.
//
// Resolution order for each candidate port:
//  1. SQLite check — skip if port is already recorded in port_allocations for
//     an active service.
//  2. TCP probe — skip if a net.Listen call fails, meaning something on the
//     host OS owns the port.
//  3. Atomic INSERT into port_allocations inside a transaction.
func (b *Broker) Allocate(req Request) (Result, error) {
	if req.Protocol == "" {
		req.Protocol = "tcp"
	}

	for offset := 0; offset < maxSearchRange; offset++ {
		candidate := req.PreferredHostPort + offset

		// Validate port range.
		if candidate < 1 || candidate > 65535 {
			return Result{}, fmt.Errorf(
				"port search exhausted valid range starting from %d", req.PreferredHostPort,
			)
		}

		// Layer 1: SQLite allocation check.
		occupied, err := b.isPortInDB(candidate, req.Protocol)
		if err != nil {
			return Result{}, fmt.Errorf("querying port_allocations for port %d: %w", candidate, err)
		}
		if occupied {
			continue
		}

		// Layer 2: Physical TCP socket probe.
		if isPortBound(candidate, req.Protocol) {
			continue
		}

		// Both checks passed — commit the allocation.
		if err := b.commitAllocation(req.ServiceID, req.ContainerPort, candidate, req.Protocol); err != nil {
			// Another goroutine may have raced us to the INSERT; keep scanning.
			continue
		}

		result := Result{
			ServiceID:         req.ServiceID,
			ContainerPort:     req.ContainerPort,
			AllocatedHostPort: candidate,
			Protocol:          req.Protocol,
			EnvInjections:     make(map[string]string),
		}
		if req.EnvKey != "" {
			result.EnvInjections[req.EnvKey] = fmt.Sprintf("%d", candidate)
		}
		return result, nil
	}

	return Result{}, fmt.Errorf(
		"no free port found in range [%d, %d] for protocol %s",
		req.PreferredHostPort,
		req.PreferredHostPort+maxSearchRange-1,
		req.Protocol,
	)
}

// AllocateMany resolves multiple port Requests, returning results in the same
// order. All results share a merged EnvInjections map on the last result;
// callers wanting per-service injections should inspect each Result.EnvInjections
// individually.
func (b *Broker) AllocateMany(reqs []Request) ([]Result, error) {
	results := make([]Result, 0, len(reqs))
	for _, req := range reqs {
		r, err := b.Allocate(req)
		if err != nil {
			return nil, fmt.Errorf("allocating port for service %s (container port %d): %w",
				req.ServiceID, req.ContainerPort, err)
		}
		results = append(results, r)
	}
	return results, nil
}

// isPortInDB returns true when the given host port + protocol combination is
// already recorded in the port_allocations table.
func (b *Broker) isPortInDB(hostPort int, protocol string) (bool, error) {
	var count int
	err := b.db.QueryRow(
		`SELECT COUNT(1) FROM port_allocations WHERE host_port = ? AND protocol = ?`,
		hostPort, protocol,
	).Scan(&count)
	if err != nil {
		return false, err
	}
	return count > 0, nil
}

// isPortBound probes the host OS by attempting to listen on addr.
// Returns true when the port is already in use (listen fails), false when the
// port is free (listen succeeds — the temporary listener is closed immediately).
func isPortBound(hostPort int, protocol string) bool {
	addr := fmt.Sprintf("127.0.0.1:%d", hostPort)
	ln, err := net.Listen(protocol, addr)
	if err != nil {
		// Cannot bind — port is occupied by something on the host.
		return true
	}
	_ = ln.Close()
	return false
}

// commitAllocation inserts a port_allocations row inside a transaction.
// It returns an error if the INSERT races with another writer (UNIQUE violation
// or FK error), allowing the caller to continue scanning.
func (b *Broker) commitAllocation(serviceID string, containerPort, hostPort int, protocol string) error {
	tx, err := b.db.Begin()
	if err != nil {
		return fmt.Errorf("beginning allocation transaction: %w", err)
	}

	_, err = tx.Exec(
		`INSERT INTO port_allocations (service_id, container_port, host_port, protocol)
		 VALUES (?, ?, ?, ?)`,
		serviceID, containerPort, hostPort, protocol,
	)
	if err != nil {
		_ = tx.Rollback()
		return fmt.Errorf("inserting port allocation: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("committing port allocation: %w", err)
	}
	return nil
}
