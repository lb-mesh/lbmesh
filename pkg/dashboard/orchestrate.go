package dashboard

import (
	"net/http"
	"strings"
	"time"

	"github.com/lbmesh/lbmesh-v2/pkg/orchestrate"
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

	containerIDs, err := orchestrate.ContainerIDsForProject(r.Context(), s.cfg.DB, projectID)
	if err != nil {
		errJSON(w, http.StatusInternalServerError, "looking up project services: "+err.Error())
		return
	}
	if len(containerIDs) == 0 {
		errJSON(w, http.StatusNotFound, "no services found for project "+projectID)
		return
	}

	res := orchestrate.StartContainers(r.Context(), s.cfg.DockerClient, containerIDs)
	if len(res.Errors) > 0 {
		errJSON(w, http.StatusInternalServerError,
			"errors starting containers: "+strings.Join(res.Errors, "; "))
		return
	}
	if err := orchestrate.SetServiceStatus(r.Context(), s.cfg.DB, res.Changed, "running"); err != nil {
		errJSON(w, http.StatusInternalServerError, err.Error())
		return
	}

	writeJSON(w, http.StatusOK, ProjectUpResponse{
		ProjectID: projectID,
		Action:    "up",
		Message:   orchestrate.FormatStarted(res.Changed),
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

	containerIDs, err := orchestrate.ContainerIDsForProject(r.Context(), s.cfg.DB, projectID)
	if err != nil {
		errJSON(w, http.StatusInternalServerError, "looking up project services: "+err.Error())
		return
	}
	if len(containerIDs) == 0 {
		errJSON(w, http.StatusNotFound, "no services found for project "+projectID)
		return
	}

	res := orchestrate.StopContainers(r.Context(), s.cfg.DockerClient, containerIDs,
		orchestrate.DefaultStopTimeout)
	if len(res.Errors) > 0 {
		errJSON(w, http.StatusInternalServerError,
			"errors stopping containers: "+strings.Join(res.Errors, "; "))
		return
	}
	if err := orchestrate.SetServiceStatus(r.Context(), s.cfg.DB, res.Changed, "exited"); err != nil {
		errJSON(w, http.StatusInternalServerError, err.Error())
		return
	}

	writeJSON(w, http.StatusOK, ProjectDownResponse{
		ProjectID: projectID,
		Action:    "down",
		Message:   orchestrate.FormatStopped(res.Changed),
		Timestamp: time.Now().UTC(),
	})
}
