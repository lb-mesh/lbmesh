package dashboard

import (
	"net/http"
	"strings"
	"time"

	"github.com/docker/docker/api/types/container"
)

// ---------------------------------------------------------------------------
// POST /api/projects/{id}/up
// ---------------------------------------------------------------------------

// ProjectUpResponse is returned when a project is successfully brought up.
type ProjectUpResponse struct {
	ProjectID string    `json:"project_id"`
	Action    string    `json:"action"`
	Message   string    `json:"message"`
	Timestamp time.Time `json:"timestamp"`
}

// handleProjectUp starts all stopped containers belonging to the project.
// It queries the services table for container IDs, then issues a Docker
// ContainerStart for each one that is not already running.
func (s *Server) handleProjectUp(w http.ResponseWriter, r *http.Request, projectID string) {
	if s.cfg.DB == nil {
		errJSON(w, http.StatusServiceUnavailable, "database not available")
		return
	}
	if s.cfg.DockerClient == nil {
		errJSON(w, http.StatusServiceUnavailable, "container engine not available")
		return
	}

	containerIDs, err := s.containerIDsForProject(r, projectID)
	if err != nil {
		errJSON(w, http.StatusInternalServerError, "looking up project services: "+err.Error())
		return
	}
	if len(containerIDs) == 0 {
		errJSON(w, http.StatusNotFound, "no services found for project "+projectID)
		return
	}

	var started []string
	var errs []string

	for _, cid := range containerIDs {
		if cid == "" {
			continue
		}
		err := s.cfg.DockerClient.ContainerStart(r.Context(), cid, container.StartOptions{})
		if err != nil {
			// "already started" is not a real error; surface everything else.
			if !isAlreadyStarted(err) {
				errs = append(errs, cid+": "+err.Error())
			}
			continue
		}
		started = append(started, cid)
	}

	if len(errs) > 0 {
		errJSON(w, http.StatusInternalServerError,
			"errors starting containers: "+strings.Join(errs, "; "))
		return
	}

	writeJSON(w, http.StatusOK, ProjectUpResponse{
		ProjectID: projectID,
		Action:    "up",
		Message:   formatStarted(started),
		Timestamp: time.Now().UTC(),
	})
}

// ---------------------------------------------------------------------------
// POST /api/projects/{id}/down
// ---------------------------------------------------------------------------

// ProjectDownResponse is returned when a project is successfully brought down.
type ProjectDownResponse struct {
	ProjectID string    `json:"project_id"`
	Action    string    `json:"action"`
	Message   string    `json:"message"`
	Timestamp time.Time `json:"timestamp"`
}

// handleProjectDown stops all running containers belonging to the project.
// Containers are sent SIGTERM and given 10 seconds to stop before being killed.
func (s *Server) handleProjectDown(w http.ResponseWriter, r *http.Request, projectID string) {
	if s.cfg.DB == nil {
		errJSON(w, http.StatusServiceUnavailable, "database not available")
		return
	}
	if s.cfg.DockerClient == nil {
		errJSON(w, http.StatusServiceUnavailable, "container engine not available")
		return
	}

	containerIDs, err := s.containerIDsForProject(r, projectID)
	if err != nil {
		errJSON(w, http.StatusInternalServerError, "looking up project services: "+err.Error())
		return
	}
	if len(containerIDs) == 0 {
		errJSON(w, http.StatusNotFound, "no services found for project "+projectID)
		return
	}

	stopTimeout := 10 // seconds

	var stopped []string
	var errs []string

	for _, cid := range containerIDs {
		if cid == "" {
			continue
		}
		err := s.cfg.DockerClient.ContainerStop(r.Context(), cid,
			container.StopOptions{Timeout: &stopTimeout})
		if err != nil {
			if !isAlreadyStopped(err) {
				errs = append(errs, cid+": "+err.Error())
			}
			continue
		}
		stopped = append(stopped, cid)
	}

	if len(errs) > 0 {
		errJSON(w, http.StatusInternalServerError,
			"errors stopping containers: "+strings.Join(errs, "; "))
		return
	}

	writeJSON(w, http.StatusOK, ProjectDownResponse{
		ProjectID: projectID,
		Action:    "down",
		Message:   formatStopped(stopped),
		Timestamp: time.Now().UTC(),
	})
}

// ---------------------------------------------------------------------------
// Shared orchestration helpers
// ---------------------------------------------------------------------------

// containerIDsForProject queries the services table and returns all non-empty
// container IDs for the given project ID.
func (s *Server) containerIDsForProject(r *http.Request, projectID string) ([]string, error) {
	rows, err := s.cfg.DB.QueryContext(r.Context(),
		`SELECT container_id FROM services WHERE project_id = ?`, projectID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var ids []string
	for rows.Next() {
		var cid string
		if err := rows.Scan(&cid); err != nil {
			continue
		}
		if cid != "" {
			ids = append(ids, cid)
		}
	}
	return ids, rows.Err()
}

// isAlreadyStarted returns true when the Docker API error indicates the
// container is already in a running state.
func isAlreadyStarted(err error) bool {
	if err == nil {
		return false
	}
	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "already started") ||
		strings.Contains(msg, "container already running")
}

// isAlreadyStopped returns true when the Docker API error indicates the
// container is not running.
func isAlreadyStopped(err error) bool {
	if err == nil {
		return false
	}
	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "is not running") ||
		strings.Contains(msg, "already stopped")
}

func formatStarted(ids []string) string {
	if len(ids) == 0 {
		return "all containers already running"
	}
	return "started " + strings.Join(shortIDs(ids), ", ")
}

func formatStopped(ids []string) string {
	if len(ids) == 0 {
		return "all containers already stopped"
	}
	return "stopped " + strings.Join(shortIDs(ids), ", ")
}

// shortIDs returns 12-character prefixes of container IDs for human-readable
// output, falling back to the full ID when it is shorter than 12 characters.
func shortIDs(ids []string) []string {
	out := make([]string, len(ids))
	for i, id := range ids {
		if len(id) > 12 {
			out[i] = id[:12]
		} else {
			out[i] = id
		}
	}
	return out
}
