package mcp

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/docker/docker/api/types/container"

	"github.com/lbmesh/lbmesh-v2/pkg/db"
	"github.com/lbmesh/lbmesh-v2/pkg/orchestrate"
)

// fakeEngine records start/stop calls and fails for IDs listed in fail.
type fakeEngine struct {
	started, stopped []string
	fail             map[string]error
}

func (f *fakeEngine) ContainerStart(_ context.Context, id string, _ container.StartOptions) error {
	if err := f.fail[id]; err != nil {
		return err
	}
	f.started = append(f.started, id)
	return nil
}

func (f *fakeEngine) ContainerStop(_ context.Context, id string, _ container.StopOptions) error {
	if err := f.fail[id]; err != nil {
		return err
	}
	f.stopped = append(f.stopped, id)
	return nil
}

func engineFactory(eng orchestrate.Engine) EngineFactory {
	return func() (orchestrate.Engine, func(), error) { return eng, func() {}, nil }
}

// isolateHome points the lbmesh home directory at an empty temp dir.
func isolateHome(t *testing.T) string {
	t.Helper()
	home := t.TempDir()
	t.Setenv("HOME", home)
	return home
}

func newTestDB(t *testing.T) *sql.DB {
	t.Helper()
	database, err := db.OpenAndMigrate(filepath.Join(t.TempDir(), "state.db"))
	if err != nil {
		t.Fatalf("OpenAndMigrate: %v", err)
	}
	t.Cleanup(func() { database.Close() })
	return database
}

func seed(t *testing.T, database *sql.DB, stmts ...string) {
	t.Helper()
	for _, s := range stmts {
		if _, err := database.Exec(s); err != nil {
			t.Fatalf("seed %q: %v", s, err)
		}
	}
}

// seedProject registers project p1 "webapp" with two services: one with a
// container ID shorter than 12 characters and two ports, and one longer.
func seedProject(t *testing.T, database *sql.DB) {
	seed(t, database,
		`INSERT INTO projects (id, name, path) VALUES ('p1', 'webapp', '/src/webapp')`,
		`INSERT INTO services (id, project_id, name, scope, container_id, image, status)
		 VALUES ('s1', 'p1', 'api', 'project', 'abc', 'node:22', 'exited')`,
		`INSERT INTO services (id, project_id, name, scope, container_id, image, status)
		 VALUES ('s2', 'p1', 'db', 'project', '0123456789abcdef', 'postgres:17', 'exited')`,
		`INSERT INTO port_allocations (service_id, container_port, host_port) VALUES ('s1', 3000, 13000)`,
		`INSERT INTO port_allocations (service_id, container_port, host_port) VALUES ('s1', 9229, 19229)`,
	)
}

// session feeds lines to a fresh Serve loop and returns the decoded responses.
func session(t *testing.T, srv *Server, lines ...string) []JSONRPCResponse {
	t.Helper()
	var out bytes.Buffer
	in := strings.NewReader(strings.Join(lines, "\n"))
	if err := srv.Serve(context.Background(), in, &out); err != nil {
		t.Fatalf("Serve: %v", err)
	}
	var resps []JSONRPCResponse
	for _, l := range strings.Split(strings.TrimSpace(out.String()), "\n") {
		if l == "" {
			continue
		}
		var r JSONRPCResponse
		if err := json.Unmarshal([]byte(l), &r); err != nil {
			t.Fatalf("decoding response %q: %v", l, err)
		}
		resps = append(resps, r)
	}
	return resps
}

func one(t *testing.T, srv *Server, line string) JSONRPCResponse {
	t.Helper()
	resps := session(t, srv, line)
	if len(resps) != 1 {
		t.Fatalf("expected 1 response, got %d", len(resps))
	}
	return resps[0]
}

func callTool(t *testing.T, srv *Server, name, args string) (toolResult, *JSONRPCError) {
	t.Helper()
	if args == "" {
		args = "{}"
	}
	resp := one(t, srv, `{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"`+name+`","arguments":`+args+`}}`)
	if resp.Error != nil {
		return toolResult{}, resp.Error
	}
	raw, _ := json.Marshal(resp.Result)
	var res toolResult
	if err := json.Unmarshal(raw, &res); err != nil {
		t.Fatalf("decoding tool result: %v", err)
	}
	return res, nil
}

func text(r toolResult) string {
	var parts []string
	for _, c := range r.Content {
		parts = append(parts, c.Text)
	}
	return strings.Join(parts, "\n")
}

func TestInitialize_NegotiatesProtocolVersion(t *testing.T) {
	srv := NewServer(Options{Version: "1.2.3"})
	for requested, want := range map[string]string{
		"2024-11-05": "2024-11-05",
		"2025-06-18": "2025-06-18",
		"1999-01-01": supportedProtocolVersions[0],
	} {
		resp := one(t, srv, `{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"`+requested+`"}}`)
		result := resp.Result.(map[string]interface{})
		if got := result["protocolVersion"]; got != want {
			t.Errorf("requested %s: protocolVersion = %v, want %s", requested, got, want)
		}
		if v := result["serverInfo"].(map[string]interface{})["version"]; v != "1.2.3" {
			t.Errorf("serverInfo.version = %v, want 1.2.3", v)
		}
	}
}

func TestNotifications_NeverAnswered(t *testing.T) {
	srv := NewServer(Options{})
	resps := session(t, srv,
		`{"jsonrpc":"2.0","method":"notifications/initialized"}`,
		`{"jsonrpc":"2.0","method":"notifications/cancelled","params":{"requestId":1}}`,
		`{"jsonrpc":"2.0","method":"notifications/something-new"}`,
	)
	if len(resps) != 0 {
		t.Fatalf("expected no responses to notifications, got %+v", resps)
	}
}

func TestPing(t *testing.T) {
	resp := one(t, NewServer(Options{}), `{"jsonrpc":"2.0","id":"abc","method":"ping"}`)
	if resp.Error != nil {
		t.Fatalf("ping error: %+v", resp.Error)
	}
	if string(resp.ID) != `"abc"` {
		t.Errorf("ID = %s, want \"abc\"", resp.ID)
	}
}

func TestParseError_HasNullID(t *testing.T) {
	var out bytes.Buffer
	if err := NewServer(Options{}).Serve(context.Background(), strings.NewReader("{not json\n"), &out); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), `"id":null`) || !strings.Contains(out.String(), "-32700") {
		t.Errorf("unexpected parse error response: %s", out.String())
	}
}

func TestUnknownMethod(t *testing.T) {
	resp := one(t, NewServer(Options{}), `{"jsonrpc":"2.0","id":7,"method":"resources/list"}`)
	if resp.Error == nil || resp.Error.Code != codeMethodNotFound {
		t.Fatalf("expected method-not-found, got %+v", resp)
	}
}

func TestLongLine(t *testing.T) {
	pad := strings.Repeat("x", 200*1024)
	resp := one(t, NewServer(Options{}), `{"jsonrpc":"2.0","id":1,"method":"ping","params":{"pad":"`+pad+`"}}`)
	if resp.Error != nil {
		t.Fatalf("ping with large params failed: %+v", resp.Error)
	}
}

func TestToolsList(t *testing.T) {
	resp := one(t, NewServer(Options{}), `{"jsonrpc":"2.0","id":1,"method":"tools/list"}`)
	tools := resp.Result.(map[string]interface{})["tools"].([]interface{})
	var names []string
	for _, tool := range tools {
		names = append(names, tool.(map[string]interface{})["name"].(string))
	}
	want := "lbmesh_status,lbmesh_list_recipes,lbmesh_up,lbmesh_down"
	if got := strings.Join(names, ","); got != want {
		t.Errorf("tools = %s, want %s", got, want)
	}
}

func TestUnknownTool(t *testing.T) {
	_, rpcErr := callTool(t, NewServer(Options{}), "lbmesh_nope", "")
	if rpcErr == nil || rpcErr.Code != codeInvalidParams {
		t.Fatalf("expected invalid-params error, got %+v", rpcErr)
	}
}

func TestStatus(t *testing.T) {
	database := newTestDB(t)
	seedProject(t, database)
	seed(t, database,
		`INSERT INTO services (id, project_id, name, scope, container_id, image, status)
		 VALUES ('s3', NULL, 'gateway', 'system', '', 'traefik:3', 'created')`)

	res, rpcErr := callTool(t, NewServer(Options{DB: database}), "lbmesh_status", "")
	if rpcErr != nil || res.IsError {
		t.Fatalf("status failed: %+v %s", rpcErr, text(res))
	}
	out := text(res)
	for _, want := range []string{
		"## Project: webapp (ID: p1)",
		"`/src/webapp`",
		"| `abc` |",          // short container ID shown in full, no panic
		"| `0123456789ab` |", // long container ID truncated
		"127.0.0.1:13000 -> 3000/tcp",
		"127.0.0.1:19229 -> 9229/tcp", // every port, not just the first
		"## System Services",
		"**gateway**",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("status output missing %q:\n%s", want, out)
		}
	}
}

func TestStatus_SystemServicesOnly(t *testing.T) {
	database := newTestDB(t)
	seed(t, database,
		`INSERT INTO services (id, project_id, name, scope) VALUES ('s3', NULL, 'gateway', 'system')`)
	res, _ := callTool(t, NewServer(Options{DB: database}), "lbmesh_status", "")
	if !strings.Contains(text(res), "**gateway**") {
		t.Errorf("system service missing when no projects exist:\n%s", text(res))
	}
}

func TestStatus_DBOffline(t *testing.T) {
	res, rpcErr := callTool(t, NewServer(Options{}), "lbmesh_status", "")
	if rpcErr != nil || !res.IsError {
		t.Fatalf("expected tool error, got %+v %+v", rpcErr, res)
	}
}

func TestUp_StartsContainersAndRecordsStatus(t *testing.T) {
	database := newTestDB(t)
	seedProject(t, database)
	eng := &fakeEngine{}
	srv := NewServer(Options{DB: database, NewEngine: engineFactory(eng)})

	res, rpcErr := callTool(t, srv, "lbmesh_up", `{"project":"webapp"}`)
	if rpcErr != nil || res.IsError {
		t.Fatalf("up failed: %+v %s", rpcErr, text(res))
	}
	if len(eng.started) != 2 {
		t.Errorf("started %v, want both containers", eng.started)
	}
	var running int
	database.QueryRow(`SELECT COUNT(*) FROM services WHERE project_id = 'p1' AND status = 'running'`).Scan(&running)
	if running != 2 {
		t.Errorf("%d services marked running, want 2", running)
	}
}

func TestDown_ByIDWithPartialFailure(t *testing.T) {
	database := newTestDB(t)
	seedProject(t, database)
	eng := &fakeEngine{fail: map[string]error{"abc": errors.New("boom")}}
	srv := NewServer(Options{DB: database, NewEngine: engineFactory(eng)})

	res, _ := callTool(t, srv, "lbmesh_down", `{"project":"p1"}`)
	if !res.IsError || !strings.Contains(text(res), "abc: boom") {
		t.Fatalf("expected partial-failure tool error, got %s", text(res))
	}
	var status string
	database.QueryRow(`SELECT status FROM services WHERE id = 's2'`).Scan(&status)
	if status != "exited" {
		t.Errorf("stopped service status = %q, want exited", status)
	}
}

func TestDown_AlreadyStoppedIsNotAnError(t *testing.T) {
	database := newTestDB(t)
	seedProject(t, database)
	eng := &fakeEngine{fail: map[string]error{
		"abc":              errors.New("container abc is not running"),
		"0123456789abcdef": errors.New("container is not running"),
	}}
	res, _ := callTool(t, NewServer(Options{DB: database, NewEngine: engineFactory(eng)}), "lbmesh_down", `{"project":"p1"}`)
	if res.IsError || !strings.Contains(text(res), "all containers already stopped") {
		t.Fatalf("unexpected result: %s", text(res))
	}
}

func TestUp_Errors(t *testing.T) {
	home := isolateHome(t)
	// "redis" must be a known recipe; seed it as a synced catalog entry.
	if err := os.MkdirAll(filepath.Join(home, ".lbmesh", "catalog", "templates", "cache", "redis"), 0o755); err != nil {
		t.Fatal(err)
	}
	database := newTestDB(t)
	seedProject(t, database)
	seed(t, database, `INSERT INTO projects (id, name, path) VALUES ('p2', 'empty', '/src/empty')`)

	noEngine := func() (orchestrate.Engine, func(), error) { return nil, nil, errors.New("no socket") }
	srv := NewServer(Options{DB: database, NewEngine: noEngine})

	cases := map[string]string{
		`{}`:                    `"project" argument is required`,
		`{"project":"redis"}`:   "available recipe, but it has not been deployed",
		`{"project":"nothing"}`: "No registered project matches",
		`{"project":"empty"}`:   "has no containers registered",
		`{"project":"webapp"}`:  "No container engine available",
	}
	for args, want := range cases {
		res, rpcErr := callTool(t, srv, "lbmesh_up", args)
		if rpcErr != nil || !res.IsError || !strings.Contains(text(res), want) {
			t.Errorf("args %s: want tool error containing %q, got %+v %q", args, want, rpcErr, text(res))
		}
	}
}

func TestCollectRecipes_DedupesByPriority(t *testing.T) {
	home := isolateHome(t)
	if err := os.MkdirAll(filepath.Join(home, ".lbmesh", "custom", "templates", "redis"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(home, ".lbmesh", "catalog", "templates", "databases", "redis"), 0o755); err != nil {
		t.Fatal(err)
	}

	var redis *recipeEntry
	entries := collectRecipes()
	for i := range entries {
		if entries[i].name == "redis" {
			if redis != nil {
				t.Fatal("redis listed more than once")
			}
			redis = &entries[i]
		}
	}
	if redis == nil {
		t.Fatal("redis not listed")
	}
	if redis.source != "custom-override" {
		t.Errorf("redis source = %q, want custom-override", redis.source)
	}
	if got := strings.Join(redis.shadowed, ","); got != "catalog-cache" {
		t.Errorf("redis shadowed = %q, want catalog-cache", got)
	}
}
