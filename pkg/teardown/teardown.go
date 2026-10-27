// Package teardown implements the end-to-end teardown sequence for a single
// recipe: look up running services by name → stop containers → remove
// containers → release port allocations → deregister network membership →
// remove the bridge network when empty.
package teardown

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"os"
	"time"

	"strings"

	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/client"
	"github.com/docker/docker/errdefs"

	"github.com/lbmesh/lbmesh-v2/pkg/db"
	"github.com/lbmesh/lbmesh-v2/pkg/socket"
)

const stopTimeout = 10
const containerOpTimeout = 30 * time.Second

// Options controls a single teardown invocation.
type Options struct {
	// RecipeName is the service name to tear down (matches services.name).
	RecipeName string

	// Scope narrows the lookup to a specific scope (project | system | custom).
	// When empty all matching services regardless of scope are torn down.
	Scope string

	// ProjectDir is used to derive the project-scoped network name.
	// Defaults to os.Getwd() when Scope is "project" and ProjectDir is empty.
	ProjectDir string
}

// Run executes the full teardown sequence and prints progress to stdout.
// It is the implementation behind `lbmesh down`.
func Run(opts Options) error {
	if opts.RecipeName == "" {
		return fmt.Errorf("recipe name is required")
	}

	// ── 1. Open state DB ─────────────────────────────────────────────────────
	database, err := db.OpenDefaultAndMigrate()
	if err != nil {
		return fmt.Errorf("opening state database: %w", err)
	}
	defer database.Close()

	// ── 2. Look up services by name ──────────────────────────────────────────
	services, err := servicesByName(database, opts.RecipeName, opts.Scope)
	if err != nil {
		return fmt.Errorf("looking up services for %q: %w", opts.RecipeName, err)
	}
	if len(services) == 0 {
		fmt.Printf("No running services found for recipe %q.\n", opts.RecipeName)
		return nil
	}

	// ── 3. Probe container engine ────────────────────────────────────────────
	fmt.Println("Detecting container engine...")
	c, host, err := socket.Detect()
	if err != nil {
		return err
	}
	defer c.Close()
	fmt.Printf("  engine: %s\n", host)

	// ── 4. Stop + remove each service's container ────────────────────────────
	var containerIDs []string
	var scope socket.Scope

	for _, svc := range services {
		if svc.ContainerID == "" {
			continue
		}
		containerIDs = append(containerIDs, svc.ContainerID)

		fmt.Printf("Stopping container %s (%s)...\n", svc.Name, svc.ContainerID[:12])
		if err := stopContainer(c, svc.ContainerID); err != nil {
			fmt.Fprintf(os.Stderr, "  warning: stopping %s: %v\n", svc.ContainerID[:12], err)
		}

		fmt.Printf("Removing container %s...\n", svc.ContainerID[:12])
		if err := removeContainer(c, svc.ContainerID); err != nil {
			fmt.Fprintf(os.Stderr, "  warning: removing %s: %v\n", svc.ContainerID[:12], err)
		}

		// Capture the scope from the last service (all services for a recipe
		// share the same scope).
		scope = socket.Scope(svc.Scope)
	}

	// ── 5. Release port allocations and delete service rows ──────────────────
	for _, svc := range services {
		if err := deleteService(database, svc.ID); err != nil {
			fmt.Fprintf(os.Stderr, "  warning: removing state for service %s: %v\n", svc.Name, err)
		}
	}

	// ── 6. Tear down the bridge network if now empty ─────────────────────────
	if len(containerIDs) > 0 && scope != "" {
		projectDir := opts.ProjectDir
		if scope == socket.ScopeProject && projectDir == "" {
			projectDir, err = os.Getwd()
			if err != nil {
				return fmt.Errorf("resolving project directory: %w", err)
			}
		}

		fmt.Println("Checking network...")
		if err := socket.TeardownScope(c, database, scope, projectDir, "", containerIDs); err != nil {
			fmt.Fprintf(os.Stderr, "  warning: network teardown: %v\n", err)
		}
	}

	fmt.Printf("Done. Recipe %q is down.\n", opts.RecipeName)
	return nil
}

// ---------------------------------------------------------------------------
// Container helpers
// ---------------------------------------------------------------------------

func stopContainer(c *client.Client, cid string) error {
	ctx, cancel := context.WithTimeout(context.Background(), containerOpTimeout)
	defer cancel()
	timeout := stopTimeout
	err := c.ContainerStop(ctx, cid, container.StopOptions{Timeout: &timeout})
	if err != nil && !isAlreadyStopped(err) && !errdefs.IsNotFound(err) {
		return err
	}
	return nil
}

func removeContainer(c *client.Client, cid string) error {
	ctx, cancel := context.WithTimeout(context.Background(), containerOpTimeout)
	defer cancel()
	err := c.ContainerRemove(ctx, cid, container.RemoveOptions{Force: true})
	if err != nil && !errdefs.IsNotFound(err) {
		return err
	}
	return nil
}

func isAlreadyStopped(err error) bool {
	if err == nil {
		return false
	}
	msg := err.Error()
	return strings.Contains(msg, "is not running") || strings.Contains(msg, "already stopped")
}

// ---------------------------------------------------------------------------
// Database helpers
// ---------------------------------------------------------------------------

// serviceRow is a minimal projection of the services table.
type serviceRow struct {
	ID          string
	Name        string
	Scope       string
	ContainerID string
}

// servicesByName returns all non-stopped service rows matching name.
// When scope is non-empty only rows with that scope are returned.
func servicesByName(database *sql.DB, name, scope string) ([]serviceRow, error) {
	var rows *sql.Rows
	var err error

	if scope != "" {
		rows, err = database.QueryContext(context.Background(),
			`SELECT id, name, scope, container_id FROM services
			 WHERE name = ? AND scope = ? AND status != 'stopped'
			 ORDER BY created_at`,
			name, scope)
	} else {
		rows, err = database.QueryContext(context.Background(),
			`SELECT id, name, scope, container_id FROM services
			 WHERE name = ? AND status != 'stopped'
			 ORDER BY created_at`,
			name)
	}
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var services []serviceRow
	for rows.Next() {
		var s serviceRow
		if err := rows.Scan(&s.ID, &s.Name, &s.Scope, &s.ContainerID); err != nil {
			return nil, err
		}
		services = append(services, s)
	}
	return services, rows.Err()
}

// deleteService removes the service row and cascades to port_allocations.
func deleteService(database *sql.DB, serviceID string) error {
	_, err := database.ExecContext(context.Background(),
		`DELETE FROM services WHERE id = ?`, serviceID)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return fmt.Errorf("deleting service %s: %w", serviceID, err)
	}
	return nil
}
