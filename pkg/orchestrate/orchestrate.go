// Package orchestrate holds the container start/stop logic shared by the
// dashboard REST API and the MCP server.
//
// It operates on projects that are already registered in the state database:
// it looks up their container IDs and starts or stops those containers via the
// active Docker/Podman engine. Deploying a recipe from scratch is not handled
// here.
package orchestrate

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"

	"github.com/docker/docker/api/types/container"
)

// DefaultStopTimeout is the number of seconds a container is given to exit
// after SIGTERM before it is killed.
const DefaultStopTimeout = 10

// Engine is the subset of the Docker/Podman client used for orchestration.
// *client.Client from github.com/docker/docker/client satisfies it.
type Engine interface {
	ContainerStart(ctx context.Context, containerID string, options container.StartOptions) error
	ContainerStop(ctx context.Context, containerID string, options container.StopOptions) error
}

// ErrProjectNotFound is returned by ResolveProject when no project matches.
var ErrProjectNotFound = errors.New("project not found")

// Project is a minimal projects-table row.
type Project struct {
	ID   string
	Name string
	Path string
}

// Result reports the outcome of a start or stop pass over a set of containers.
type Result struct {
	// Changed holds the container IDs whose state was changed.
	Changed []string
	// Errors holds "<id>: <message>" entries for containers that failed.
	Errors []string
}

// ResolveProject finds a project by exact ID, or else by exact name. A name
// shared by several projects is reported as an error listing their IDs.
func ResolveProject(ctx context.Context, db *sql.DB, ref string) (Project, error) {
	var p Project
	err := db.QueryRowContext(ctx,
		`SELECT id, name, path FROM projects WHERE id = ?`, ref).Scan(&p.ID, &p.Name, &p.Path)
	if err == nil {
		return p, nil
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return Project{}, err
	}

	rows, err := db.QueryContext(ctx,
		`SELECT id, name, path FROM projects WHERE name = ? ORDER BY id`, ref)
	if err != nil {
		return Project{}, err
	}
	defer rows.Close()

	var matches []Project
	for rows.Next() {
		var m Project
		if err := rows.Scan(&m.ID, &m.Name, &m.Path); err != nil {
			return Project{}, err
		}
		matches = append(matches, m)
	}
	if err := rows.Err(); err != nil {
		return Project{}, err
	}

	switch len(matches) {
	case 0:
		return Project{}, ErrProjectNotFound
	case 1:
		return matches[0], nil
	default:
		ids := make([]string, len(matches))
		for i, m := range matches {
			ids[i] = m.ID
		}
		return Project{}, fmt.Errorf("project name %q is ambiguous; use one of the IDs: %s",
			ref, strings.Join(ids, ", "))
	}
}

// ContainerIDsForProject returns all non-empty container IDs registered for
// the given project ID.
func ContainerIDsForProject(ctx context.Context, db *sql.DB, projectID string) ([]string, error) {
	rows, err := db.QueryContext(ctx,
		`SELECT container_id FROM services WHERE project_id = ?`, projectID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var ids []string
	for rows.Next() {
		var cid string
		if err := rows.Scan(&cid); err != nil {
			return nil, err
		}
		if cid != "" {
			ids = append(ids, cid)
		}
	}
	return ids, rows.Err()
}

// SetServiceStatus records status for the services running the given
// containers, so the state database reflects a start or stop pass.
func SetServiceStatus(ctx context.Context, db *sql.DB, containerIDs []string, status string) error {
	for _, cid := range containerIDs {
		if _, err := db.ExecContext(ctx,
			`UPDATE services SET status = ?, updated_at = CURRENT_TIMESTAMP WHERE container_id = ?`,
			status, cid); err != nil {
			return fmt.Errorf("updating status for container %s: %w", ShortID(cid), err)
		}
	}
	return nil
}

// StartContainers starts each container that is not already running.
func StartContainers(ctx context.Context, eng Engine, ids []string) Result {
	var res Result
	for _, cid := range ids {
		if cid == "" {
			continue
		}
		if err := eng.ContainerStart(ctx, cid, container.StartOptions{}); err != nil {
			// "already started" is not a real error; surface everything else.
			if !IsAlreadyStarted(err) {
				res.Errors = append(res.Errors, cid+": "+err.Error())
			}
			continue
		}
		res.Changed = append(res.Changed, cid)
	}
	return res
}

// StopContainers stops each running container, giving it timeoutSec seconds
// to exit before it is killed.
func StopContainers(ctx context.Context, eng Engine, ids []string, timeoutSec int) Result {
	var res Result
	for _, cid := range ids {
		if cid == "" {
			continue
		}
		err := eng.ContainerStop(ctx, cid, container.StopOptions{Timeout: &timeoutSec})
		if err != nil {
			if !IsAlreadyStopped(err) {
				res.Errors = append(res.Errors, cid+": "+err.Error())
			}
			continue
		}
		res.Changed = append(res.Changed, cid)
	}
	return res
}

// IsAlreadyStarted returns true when the Docker API error indicates the
// container is already in a running state.
func IsAlreadyStarted(err error) bool {
	if err == nil {
		return false
	}
	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "already started") ||
		strings.Contains(msg, "container already running")
}

// IsAlreadyStopped returns true when the Docker API error indicates the
// container is not running.
func IsAlreadyStopped(err error) bool {
	if err == nil {
		return false
	}
	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "is not running") ||
		strings.Contains(msg, "already stopped")
}

// FormatStarted renders a human-readable summary of a start pass.
func FormatStarted(ids []string) string {
	if len(ids) == 0 {
		return "all containers already running"
	}
	return "started " + strings.Join(ShortIDs(ids), ", ")
}

// FormatStopped renders a human-readable summary of a stop pass.
func FormatStopped(ids []string) string {
	if len(ids) == 0 {
		return "all containers already stopped"
	}
	return "stopped " + strings.Join(ShortIDs(ids), ", ")
}

// ShortID returns the 12-character prefix of a container ID, or the full ID
// when it is shorter than 12 characters.
func ShortID(id string) string {
	if len(id) > 12 {
		return id[:12]
	}
	return id
}

// ShortIDs applies ShortID to every element of ids.
func ShortIDs(ids []string) []string {
	out := make([]string, len(ids))
	for i, id := range ids {
		out[i] = ShortID(id)
	}
	return out
}
