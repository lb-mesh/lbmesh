package dashboard

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"
	"time"

	"github.com/docker/docker/pkg/stdcopy"

	"github.com/lbmesh/lbmesh-v2/pkg/db"
)

func newTestServer(t *testing.T, cfg Config) *Server {
	t.Helper()
	if cfg.Host == "" {
		cfg.Host = "127.0.0.1"
	}
	s, err := New(cfg)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	return s
}

func TestLocalOnly(t *testing.T) {
	s := newTestServer(t, Config{})
	h := s.Handler()

	tests := []struct {
		name   string
		method string
		host   string
		origin string
		want   int
	}{
		{"loopback IP", http.MethodGet, "127.0.0.1:8080", "", http.StatusOK},
		{"localhost", http.MethodGet, "localhost:8080", "", http.StatusOK},
		{"IPv6 loopback", http.MethodGet, "[::1]:8080", "", http.StatusOK},
		{"LAN IP", http.MethodGet, "192.168.1.20:8080", "", http.StatusOK},
		{"same-origin POST", http.MethodPost, "127.0.0.1:8080", "http://127.0.0.1:8080", http.StatusServiceUnavailable},
		{"DNS rebinding host", http.MethodGet, "evil.example:8080", "", http.StatusForbidden},
		{"cross-site POST", http.MethodPost, "127.0.0.1:8080", "https://evil.example", http.StatusForbidden},
		{"cross-port origin", http.MethodPost, "127.0.0.1:8080", "http://127.0.0.1:3000", http.StatusForbidden},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			path := "/api/status"
			if tc.method == http.MethodPost {
				path = "/api/projects/p1/up"
			}
			req := httptest.NewRequest(tc.method, path, nil)
			req.Host = tc.host
			if tc.origin != "" {
				req.Header.Set("Origin", tc.origin)
			}
			rec := httptest.NewRecorder()
			h.ServeHTTP(rec, req)
			if rec.Code != tc.want {
				t.Fatalf("status = %d, want %d (body %s)", rec.Code, tc.want, rec.Body.String())
			}
		})
	}
}

func TestProjectsTimestamps(t *testing.T) {
	database, err := db.OpenAndMigrate(filepath.Join(t.TempDir(), "state.db"))
	if err != nil {
		t.Fatalf("OpenAndMigrate: %v", err)
	}
	defer database.Close()
	if _, err := database.Exec(`INSERT INTO projects (id, name, path) VALUES ('p1', 'demo', '/tmp/demo')`); err != nil {
		t.Fatalf("insert: %v", err)
	}

	s := newTestServer(t, Config{DB: database})
	req := httptest.NewRequest(http.MethodGet, "/api/projects", nil)
	req.Host = "127.0.0.1:8080"
	rec := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d: %s", rec.Code, rec.Body.String())
	}

	var projects []ProjectItem
	if err := json.Unmarshal(rec.Body.Bytes(), &projects); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(projects) != 1 {
		t.Fatalf("got %d projects, want 1", len(projects))
	}
	if time.Since(projects[0].CreatedAt) > time.Minute || time.Since(projects[0].UpdatedAt) > time.Minute {
		t.Fatalf("timestamps not parsed: created=%v updated=%v", projects[0].CreatedAt, projects[0].UpdatedAt)
	}
}

func TestLogsContainerID(t *testing.T) {
	for url, want := range map[string]string{
		"/api/logs/abc123":           "abc123",
		"/api/logs/abc123/":          "abc123",
		"/api/logs?container=abc123": "abc123",
		"/api/logs/":                 "",
	} {
		req := httptest.NewRequest(http.MethodGet, url, nil)
		if got := logsContainerID(req); got != want {
			t.Errorf("logsContainerID(%q) = %q, want %q", url, got, want)
		}
	}
}

func TestLogLines_DemuxesFrames(t *testing.T) {
	// A payload size of 10 puts a newline byte (0x0a) inside the frame
	// header, which broke the old line-based header stripping.
	var buf bytes.Buffer
	stdout := stdcopy.NewStdWriter(&buf, stdcopy.Stdout)
	stderr := stdcopy.NewStdWriter(&buf, stdcopy.Stderr)
	_, _ = stdout.Write([]byte("line one\n"))
	_, _ = stderr.Write([]byte("err: two\n"))
	_, _ = stdout.Write([]byte("three\nfour\n"))

	got, err := io.ReadAll(logLines(&buf, false))
	if err != nil {
		t.Fatalf("ReadAll: %v", err)
	}
	want := "line one\nerr: two\nthree\nfour\n"
	if string(got) != want {
		t.Fatalf("logLines = %q, want %q", got, want)
	}
}

func TestLogLines_TTYPassthrough(t *testing.T) {
	got, _ := io.ReadAll(logLines(bytes.NewBufferString("raw tty\n"), true))
	if string(got) != "raw tty\n" {
		t.Fatalf("logLines(tty) = %q", got)
	}
}

func TestStatusEndpoint(t *testing.T) {
	s := newTestServer(t, Config{
		SocketPath: "unix:///tmp/podman.sock",
	})
	req := httptest.NewRequest(http.MethodGet, "/api/status", nil)
	req.Host = "127.0.0.1:8080"
	rec := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d: %s", rec.Code, rec.Body.String())
	}

	var status StatusResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &status); err != nil {
		t.Fatalf("decode: %v", err)
	}

	if !status.OK {
		t.Errorf("status.OK = false, want true")
	}
	if status.SocketPath != "unix:///tmp/podman.sock" {
		t.Errorf("status.SocketPath = %q, want %q", status.SocketPath, "unix:///tmp/podman.sock")
	}
	if status.Engine == "" {
		t.Errorf("status.Engine is empty, want non-empty")
	}
}
