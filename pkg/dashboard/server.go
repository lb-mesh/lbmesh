// Package dashboard implements the embedded HTTP server that serves the
// compiled React + Carbon Design System SPA and exposes the REST / SSE API
// consumed by the browser dashboard.
//
// Static assets are served from the webdist package (module-root embed shim).
// The server uses only the Go standard library — no external router is needed.
package dashboard

import (
	"context"
	"database/sql"
	"fmt"
	"io/fs"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/docker/docker/client"
	webdist "github.com/lbmesh/lbmesh-v2"
)

// Config holds the runtime parameters for the dashboard server.
type Config struct {
	// Host is the interface address to bind (default "127.0.0.1").
	Host string
	// Port is the TCP port to listen on (default 8080).
	Port int
	// DB is the open SQLite database connection.  May be nil when the database
	// cannot be opened; endpoints that need it will return 503.
	DB *sql.DB
	// DockerClient is the active container engine client.  May be nil when no
	// engine is running; endpoints that need it will return 503.
	DockerClient *client.Client
}

// Server is the embedded dashboard HTTP server.
type Server struct {
	cfg    Config
	mux    *http.ServeMux
	distFS fs.FS // sub-rooted at web/dist
}

// New constructs a Server from cfg and wires all routes.
func New(cfg Config) (*Server, error) {
	sub, err := fs.Sub(webdist.FS, webdist.Root)
	if err != nil {
		return nil, fmt.Errorf("sub-rooting embedded dist FS: %w", err)
	}

	s := &Server{
		cfg:    cfg,
		mux:    http.NewServeMux(),
		distFS: sub,
	}
	s.routes()
	return s, nil
}

// Addr returns the TCP address the server will bind to.
func (s *Server) Addr() string {
	return fmt.Sprintf("%s:%d", s.cfg.Host, s.cfg.Port)
}

// Start begins listening and serving.  It blocks until ctx is cancelled or an
// unrecoverable accept error occurs.
func (s *Server) Start(ctx context.Context) error {
	ln, err := net.Listen("tcp", s.Addr())
	if err != nil {
		return fmt.Errorf("binding dashboard server on %s: %w", s.Addr(), err)
	}

	srv := &http.Server{
		Handler:           s.Handler(),
		ReadHeaderTimeout: 10 * time.Second,
		WriteTimeout:      0, // SSE streams must not timeout
		IdleTimeout:       60 * time.Second,
	}

	// Shut down gracefully when ctx is cancelled.
	go func() {
		<-ctx.Done()
		shutCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = srv.Shutdown(shutCtx)
	}()

	if err := srv.Serve(ln); err != nil && err != http.ErrServerClosed {
		return fmt.Errorf("dashboard server error: %w", err)
	}
	return nil
}

// ---------------------------------------------------------------------------
// Route wiring
// ---------------------------------------------------------------------------

// routes registers all API and static-asset handlers on s.mux.
func (s *Server) routes() {
	// REST API endpoints.
	s.mux.HandleFunc("/api/status", s.handleStatus)
	s.mux.HandleFunc("/api/projects", s.handleProjects)
	s.mux.HandleFunc("/api/recipes", s.handleRecipes)
	s.mux.HandleFunc("/api/ports", s.handlePorts)

	// Orchestration: POST /api/projects/{id}/up  |  /api/projects/{id}/down
	// The stdlib mux (Go 1.22+) supports {id} path parameters; older versions
	// need manual trimming — we use manual trimming for broader compatibility.
	s.mux.HandleFunc("/api/projects/", s.handleProjectAction)

	// SSE log stream: GET /api/logs/{containerID}  |  GET /api/logs?container={id}
	s.mux.HandleFunc("/api/logs", s.handleLogsSSE)
	s.mux.HandleFunc("/api/logs/", s.handleLogsSSE)

	// Static SPA assets + SPA fallback (must be last).
	s.mux.HandleFunc("/", s.handleSPA)
}

// Handler returns the server's root HTTP handler: the route mux wrapped in the
// local-origin guard.
func (s *Server) Handler() http.Handler {
	return s.localOnly(s.mux)
}

// localOnly rejects requests that a web page on another site could make
// through the user's browser. The dashboard has no authentication, so:
//
//   - The Host header must be an IP literal, "localhost", or the configured
//     bind host. This defeats DNS rebinding, where an attacker's domain is
//     re-pointed at 127.0.0.1 to read the API.
//   - Any request carrying an Origin header must come from this same host.
//     This stops cross-site POSTs to /api/projects/{id}/up|down (CSRF).
func (s *Server) localOnly(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !s.allowedHost(r.Host) {
			errJSON(w, http.StatusForbidden, "forbidden host")
			return
		}
		if origin := r.Header.Get("Origin"); origin != "" {
			u, err := url.Parse(origin)
			if err != nil || !strings.EqualFold(u.Host, r.Host) {
				errJSON(w, http.StatusForbidden, "cross-origin request rejected")
				return
			}
		}
		next.ServeHTTP(w, r)
	})
}

// allowedHost reports whether hostport (a Host header value) names this
// machine directly rather than through an arbitrary DNS name.
func (s *Server) allowedHost(hostport string) bool {
	host := hostport
	if h, _, err := net.SplitHostPort(hostport); err == nil {
		host = h
	}
	host = strings.Trim(host, "[]")
	if host == "" {
		return false
	}
	if net.ParseIP(host) != nil {
		return true
	}
	return strings.EqualFold(host, "localhost") || strings.EqualFold(host, s.cfg.Host)
}

// ---------------------------------------------------------------------------
// Static asset / SPA fallback handler
// ---------------------------------------------------------------------------

// handleSPA serves static files from the embedded dist FS.  Any path that
// does not match a real file falls back to index.html so that the React router
// can handle client-side navigation.
func (s *Server) handleSPA(w http.ResponseWriter, r *http.Request) {
	fileServer := http.FileServer(http.FS(s.distFS))

	// Attempt to open the requested path inside the embedded FS.
	path := strings.TrimPrefix(r.URL.Path, "/")
	if path == "" {
		path = "index.html"
	}

	if _, err := fs.Stat(s.distFS, path); err != nil {
		// File does not exist → serve index.html for client-side routing.
		r2 := r.Clone(r.Context())
		r2.URL.Path = "/"
		fileServer.ServeHTTP(w, r2)
		return
	}

	fileServer.ServeHTTP(w, r)
}
