package dashboard

import (
	"context"
	"encoding/json"
	"net/http"
	"runtime"
	"sort"
	"strings"
	"time"

	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/filters"
	"github.com/lbmesh/lbmesh-v2/internal/version"
	"github.com/lbmesh/lbmesh-v2/pkg/ai"
	"github.com/lbmesh/lbmesh-v2/pkg/db"
	"github.com/lbmesh/lbmesh-v2/pkg/machine"
)

// ---------------------------------------------------------------------------
// Shared helpers
// ---------------------------------------------------------------------------

// writeJSON encodes v as JSON and writes it to w with the given HTTP status.
func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

// errJSON writes a JSON error body: {"error": "…"}.
func errJSON(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, map[string]string{"error": msg})
}

// ---------------------------------------------------------------------------
// GET /api/status
// ---------------------------------------------------------------------------

// StatusResponse is the JSON payload returned by GET /api/status.
type StatusResponse struct {
	OK                bool        `json:"ok"`
	Version           string      `json:"version"`
	OS                string      `json:"os"`
	Arch              string      `json:"arch"`
	Timestamp         time.Time   `json:"timestamp"`
	EngineOnline      bool        `json:"engine_online"`
	Engine            string      `json:"engine"`
	SocketPath        string      `json:"socket_path"`
	ActiveContainers  int         `json:"active_containers"`
	TotalMemoryMB     int64       `json:"total_memory_mb"`
	AvailableMemoryMB int64       `json:"available_memory_mb"`
	UsedMemoryMB      int64       `json:"used_memory_mb"`
	EngineInfo        *EngineInfo `json:"engine_info,omitempty"`
}

// EngineInfo holds container engine details surfaced by the status endpoint.
type EngineInfo struct {
	ServerVersion string `json:"server_version"`
	OS            string `json:"os"`
	Architecture  string `json:"architecture"`
	NCPU          int    `json:"ncpu"`
	MemTotal      int64  `json:"mem_total_bytes"`
}

func (s *Server) handleStatus(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		errJSON(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	resp := StatusResponse{
		OK:         true,
		Version:    version.Version,
		OS:         runtime.GOOS,
		Arch:       runtime.GOARCH,
		Timestamp:  time.Now().UTC(),
		SocketPath: s.cfg.SocketPath,
	}

	var activeContainers int
	var totalMemoryMB int64

	if s.cfg.DockerClient != nil {
		// Use a short timeout so status responds instantly
		infoCtx, cancel := context.WithTimeout(r.Context(), 2*time.Second)
		info, err := s.cfg.DockerClient.Info(infoCtx)
		cancel()
		if err == nil {
			resp.EngineOnline = true
			resp.EngineInfo = &EngineInfo{
				ServerVersion: info.ServerVersion,
				OS:            info.OSType,
				Architecture:  info.Architecture,
				NCPU:          info.NCPU,
				MemTotal:      info.MemTotal,
			}
			activeContainers = info.ContainersRunning
			if info.MemTotal > 0 {
				totalMemoryMB = info.MemTotal / (1024 * 1024)
			}

			// Identify engine display name
			engineName := "Docker Engine"
			sockLower := strings.ToLower(s.cfg.SocketPath)
			osLower := strings.ToLower(info.OperatingSystem)
			if strings.Contains(sockLower, "podman") || strings.Contains(osLower, "podman") {
				if strings.Contains(sockLower, "user") || strings.Contains(sockLower, ".local") || strings.Contains(sockLower, "run/user") {
					engineName = "Podman (Rootless)"
				} else {
					engineName = "Podman"
				}
			} else if strings.Contains(osLower, "docker desktop") || strings.Contains(sockLower, "docker-desktop") {
				engineName = "Docker Desktop"
			}
			resp.Engine = engineName
		}
	}

	if !resp.EngineOnline {
		if s.cfg.SocketPath != "" {
			resp.Engine = "Connecting..."
		} else {
			resp.Engine = "No Engine Detected"
		}
	}

	// If totalMemoryMB is not obtained from engine, inspect machine/host capacity
	if totalMemoryMB == 0 {
		switch runtime.GOOS {
		case "darwin", "windows":
			if machInfo, err := machine.InspectPodmanMachine(); err == nil && machInfo.TotalMemoryMB > 0 {
				totalMemoryMB = int64(machInfo.TotalMemoryMB)
				if resp.Engine == "" || resp.Engine == "No Engine Detected" {
					resp.Engine = "Podman Machine"
				}
			}
		default: // linux
			if linInfo, err := machine.ReadLinuxMeminfo(); err == nil && linInfo.TotalMemoryMB > 0 {
				totalMemoryMB = int64(linInfo.TotalMemoryMB)
			}
		}
	}

	// Active containers fallback to DB if engine reported 0
	if s.cfg.DB != nil {
		var dbRunning int
		_ = s.cfg.DB.QueryRowContext(r.Context(), `SELECT COUNT(*) FROM services WHERE status = 'running'`).Scan(&dbRunning)
		if dbRunning > activeContainers {
			activeContainers = dbRunning
		}

		// Calculate memory footprint from running services
		_, totalActiveMB, _, err := machine.ActiveFootprint(s.cfg.DB)
		if err == nil && totalActiveMB > 0 {
			resp.UsedMemoryMB = int64(totalActiveMB)
		}
	}

	resp.ActiveContainers = activeContainers
	resp.TotalMemoryMB = totalMemoryMB

	if resp.UsedMemoryMB > resp.TotalMemoryMB && resp.TotalMemoryMB > 0 {
		resp.UsedMemoryMB = resp.TotalMemoryMB
	}

	if resp.TotalMemoryMB > 0 {
		resp.AvailableMemoryMB = resp.TotalMemoryMB - resp.UsedMemoryMB
		if resp.AvailableMemoryMB < 0 {
			resp.AvailableMemoryMB = 0
		}
	}

	writeJSON(w, http.StatusOK, resp)
}

// ---------------------------------------------------------------------------
// GET /api/projects
// ---------------------------------------------------------------------------

// ProjectItem is an entry in the projects list response.
type ProjectItem struct {
	ID         string        `json:"id"`
	Name       string        `json:"name"`
	Path       string        `json:"path"`
	WorkingDir string        `json:"working_dir"`
	Scope      string        `json:"scope"`
	Network    string        `json:"network"`
	Status     string        `json:"status"`
	CreatedAt  time.Time     `json:"created_at"`
	UpdatedAt  time.Time     `json:"updated_at"`
	Services   []ServiceItem `json:"services"`
}

// ServiceItem is a container service belonging to a project.
type ServiceItem struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Scope       string `json:"scope"`
	ContainerID string `json:"container_id"`
	Image       string `json:"image"`
	Status      string `json:"status"`
	Ports       string `json:"ports"`
}

func (s *Server) handleProjects(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		errJSON(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	if s.cfg.DB == nil {
		errJSON(w, http.StatusServiceUnavailable, "database not available")
		return
	}

	rows, err := s.cfg.DB.QueryContext(r.Context(),
		`SELECT id, name, path, network, status, created_at, updated_at
		 FROM projects
		 ORDER BY updated_at DESC`)
	if err != nil {
		errJSON(w, http.StatusInternalServerError, "querying projects: "+err.Error())
		return
	}
	defer rows.Close()

	projects := []ProjectItem{}
	for rows.Next() {
		var p ProjectItem
		var createdAt, updatedAt any
		if err := rows.Scan(&p.ID, &p.Name, &p.Path, &p.Network, &p.Status, &createdAt, &updatedAt); err != nil {
			continue
		}
		p.CreatedAt = db.ParseTimestamp(createdAt)
		p.UpdatedAt = db.ParseTimestamp(updatedAt)
		p.WorkingDir = p.Path
		p.Scope = "project"

		// Attach services for this project.
		p.Services = s.servicesWhere(r, "s.project_id = ?", p.ID)
		projects = append(projects, p)
	}
	if err := rows.Err(); err != nil {
		errJSON(w, http.StatusInternalServerError, "iterating projects: "+err.Error())
		return
	}

	// Services deployed via `lbmesh up` are not attached to a project row.
	// Surface them as synthetic groups so running containers are visible.
	standalone := s.servicesWhere(r, "(s.project_id IS NULL OR s.project_id = '')")
	groups := map[string]*ProjectItem{}
	for _, svc := range standalone {
		g, ok := groups[svc.Scope]
		if !ok {
			g = &ProjectItem{
				ID:       "standalone-" + svc.Scope,
				Name:     "Standalone " + svc.Scope + " services",
				Scope:    svc.Scope,
				Status:   "active",
				Services: []ServiceItem{},
			}
			if svc.Scope == "system" {
				g.ID = "system-services"
				g.Name = "Global shared stacks"
			}
			groups[svc.Scope] = g
		}
		g.Services = append(g.Services, svc)
	}
	scopes := make([]string, 0, len(groups))
	for scope := range groups {
		scopes = append(scopes, scope)
	}
	sort.Strings(scopes)
	for _, scope := range scopes {
		projects = append(projects, *groups[scope])
	}

	writeJSON(w, http.StatusOK, projects)
}

// servicesWhere returns services matching the given SQL condition (which may
// reference the services table as "s"), including a "host:container" summary
// of their port allocations. The result is never nil.
func (s *Server) servicesWhere(r *http.Request, cond string, args ...any) []ServiceItem {
	items := []ServiceItem{}
	rows, err := s.cfg.DB.QueryContext(r.Context(),
		`SELECT s.id, s.name, s.scope, s.container_id, s.image, s.status,
		        COALESCE((SELECT group_concat(pa.host_port || ':' || pa.container_port, ', ')
		                  FROM port_allocations pa WHERE pa.service_id = s.id), '')
		 FROM services s WHERE `+cond+`
		 ORDER BY s.name`, args...)
	if err != nil {
		return items
	}
	defer rows.Close()

	for rows.Next() {
		var svc ServiceItem
		if err := rows.Scan(&svc.ID, &svc.Name, &svc.Scope, &svc.ContainerID, &svc.Image, &svc.Status, &svc.Ports); err != nil {
			continue
		}
		items = append(items, svc)
	}
	return items
}

// ---------------------------------------------------------------------------
// GET /api/recipes
// ---------------------------------------------------------------------------

// RecipeItem summarises a discovered recipe template.
type RecipeItem struct {
	Name        string `json:"name"`
	Category    string `json:"category"`
	Description string `json:"description"`
	Scope       string `json:"scope"`
	Version     string `json:"version"`
	Source      string `json:"source"`
}

func (s *Server) handleRecipes(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		errJSON(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	resolved, err := ai.DiscoverAllRecipes()
	if err != nil {
		errJSON(w, http.StatusInternalServerError, "listing recipes: "+err.Error())
		return
	}

	items := make([]RecipeItem, 0, len(resolved))
	for _, res := range resolved {
		items = append(items, RecipeItem{
			Name:        res.Recipe.Metadata.Name,
			Category:    res.Recipe.Metadata.Category,
			Description: res.Recipe.Metadata.Description,
			Scope:       res.Recipe.Metadata.Scope,
			Version:     res.Recipe.Metadata.Version,
			Source:      res.Source.String(),
		})
	}

	writeJSON(w, http.StatusOK, items)
}

// ---------------------------------------------------------------------------
// GET /api/ports
// ---------------------------------------------------------------------------

// PortItem describes an active host-to-container port allocation.
type PortItem struct {
	ID            int64     `json:"id"`
	ServiceID     string    `json:"service_id"`
	ServiceName   string    `json:"service_name"`
	ContainerPort int       `json:"container_port"`
	HostPort      int       `json:"host_port"`
	Protocol      string    `json:"protocol"`
	AllocatedAt   time.Time `json:"allocated_at"`
}

func (s *Server) handlePorts(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		errJSON(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	if s.cfg.DB == nil {
		errJSON(w, http.StatusServiceUnavailable, "database not available")
		return
	}

	rows, err := s.cfg.DB.QueryContext(r.Context(),
		`SELECT pa.id, pa.service_id, COALESCE(s.name, pa.service_id), pa.container_port, pa.host_port, pa.protocol, pa.allocated_at
		 FROM port_allocations pa
		 LEFT JOIN services s ON s.id = pa.service_id
		 ORDER BY pa.allocated_at DESC`)
	if err != nil {
		errJSON(w, http.StatusInternalServerError, "querying port allocations: "+err.Error())
		return
	}
	defer rows.Close()

	var ports []PortItem
	for rows.Next() {
		var p PortItem
		var allocatedAt any
		if err := rows.Scan(&p.ID, &p.ServiceID, &p.ServiceName, &p.ContainerPort, &p.HostPort, &p.Protocol, &allocatedAt); err != nil {
			continue
		}
		p.AllocatedAt = db.ParseTimestamp(allocatedAt)
		ports = append(ports, p)
	}
	if err := rows.Err(); err != nil {
		errJSON(w, http.StatusInternalServerError, "iterating port allocations: "+err.Error())
		return
	}

	if ports == nil {
		ports = []PortItem{}
	}
	writeJSON(w, http.StatusOK, ports)
}

// ---------------------------------------------------------------------------
// POST /api/projects/{id}/up  |  POST /api/projects/{id}/down
// ---------------------------------------------------------------------------

// handleProjectAction routes POST requests under /api/projects/{id}/up|down.
// It extracts the project ID and action verb from the URL path, then delegates
// to the corresponding orchestration helper.
func (s *Server) handleProjectAction(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		errJSON(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	// Path pattern: /api/projects/{id}/up  or  /api/projects/{id}/down
	// Strip the prefix and split.
	path := strings.TrimPrefix(r.URL.Path, "/api/projects/")
	parts := strings.SplitN(path, "/", 2)
	if len(parts) != 2 {
		errJSON(w, http.StatusNotFound, "not found")
		return
	}

	projectID := parts[0]
	action := parts[1]

	if projectID == "" {
		errJSON(w, http.StatusBadRequest, "missing project id")
		return
	}

	switch action {
	case "up":
		s.handleProjectUp(w, r, projectID)
	case "down":
		s.handleProjectDown(w, r, projectID)
	default:
		errJSON(w, http.StatusNotFound, "unknown action: "+action)
	}
}

// ---------------------------------------------------------------------------
// Container inspect helper used by orchestration handlers
// ---------------------------------------------------------------------------

// listContainersForProject returns active container summaries for the project
// by filtering running containers via the Docker API using the lbmesh.project label.
func (s *Server) listContainersForProject(r *http.Request, projectID string) ([]container.Summary, error) {
	f := filters.NewArgs()
	f.Add("label", "lbmesh.project="+projectID)
	return s.cfg.DockerClient.ContainerList(r.Context(), container.ListOptions{
		All:     true,
		Filters: f,
	})
}
