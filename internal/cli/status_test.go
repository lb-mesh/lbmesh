package cli

import (
	"bytes"
	"context"
	"strings"
	"testing"

	"github.com/docker/docker/api/types/container"

	"github.com/lbmesh/lbmesh-v2/pkg/db"
)

type mockContainerLister struct {
	containers []container.Summary
	err        error
}

func (m *mockContainerLister) ContainerList(ctx context.Context, options container.ListOptions) ([]container.Summary, error) {
	if m.err != nil {
		return nil, m.err
	}
	return m.containers, nil
}

func TestRenderStatus_Empty(t *testing.T) {
	var out bytes.Buffer
	if err := renderStatus(&out, nil, false); err != nil {
		t.Fatal(err)
	}
	want := "status: no running instances found.\n"
	if out.String() != want {
		t.Fatalf("got %q, want %q", out.String(), want)
	}
}

func TestRenderStatus_Running(t *testing.T) {
	var out bytes.Buffer
	rows := []instanceRow{
		{
			ContainerID: "6b38353a75f6",
			Name:        "portkey",
			Scope:       "project",
			Status:      "running",
			Ports:       "0.0.0.0:8787->8787/tcp",
			Image:       "docker.io/portkeyai/gateway:latest",
			IsRunning:   true,
		},
	}

	if err := renderStatus(&out, rows, false); err != nil {
		t.Fatal(err)
	}

	output := out.String()
	for _, want := range []string{"CONTAINER ID", "NAME", "SCOPE", "STATUS", "PORTS", "IMAGE", "6b38353a75f6", "portkey", "project", "running", "0.0.0.0:8787->8787/tcp", "docker.io/portkeyai/gateway:latest"} {
		if !strings.Contains(output, want) {
			t.Errorf("output missing %q:\n%s", want, output)
		}
	}
}

func TestCollectInstances_FiltersUnrelatedContainers(t *testing.T) {
	ctx := context.Background()

	mock := &mockContainerLister{
		containers: []container.Summary{
			{
				ID:      "1003a1c1ce6c123456",
				Names:   []string{"/telemetry-server_mysql_1"},
				Image:   "docker.io/library/mysql:8.4",
				Status:  "Up 3 weeks (healthy)",
				State:   "running",
				Ports:   []container.Port{{IP: "127.0.0.1", PublicPort: 3306, PrivatePort: 3306, Type: "tcp"}},
				Labels:  map[string]string{},
			},
			{
				ID:      "6b38353a75f6123456",
				Names:   []string{"/lbmesh-portkey"},
				Image:   "docker.io/portkeyai/gateway:latest",
				Status:  "Up 29 seconds",
				State:   "running",
				Ports:   []container.Port{{IP: "0.0.0.0", PublicPort: 8787, PrivatePort: 8787, Type: "tcp"}},
				Labels:  map[string]string{},
			},
		},
	}

	rows, err := collectInstances(ctx, nil, mock)
	if err != nil {
		t.Fatal(err)
	}

	if len(rows) != 1 {
		t.Fatalf("got %d rows, want 1", len(rows))
	}

	row := rows[0]
	if row.ContainerID != "6b38353a75f6" {
		t.Errorf("ContainerID = %q, want %q", row.ContainerID, "6b38353a75f6")
	}
	if row.Name != "portkey" {
		t.Errorf("Name = %q, want %q", row.Name, "portkey")
	}
	if row.Scope != "project" {
		t.Errorf("Scope = %q, want %q", row.Scope, "project")
	}
	if row.Status != "running" {
		t.Errorf("Status = %q, want %q", row.Status, "running")
	}
	if row.Ports != "0.0.0.0:8787->8787/tcp" {
		t.Errorf("Ports = %q, want %q", row.Ports, "0.0.0.0:8787->8787/tcp")
	}
	if row.Image != "docker.io/portkeyai/gateway:latest" {
		t.Errorf("Image = %q, want %q", row.Image, "docker.io/portkeyai/gateway:latest")
	}
	if !row.IsRunning {
		t.Errorf("IsRunning = false, want true")
	}
}

func TestCollectInstances_EnrichesFromDatabase(t *testing.T) {
	ctx := context.Background()

	database, err := db.Open(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()

	if err := db.Migrate(database); err != nil {
		t.Fatal(err)
	}

	_, err = database.Exec(`
		INSERT INTO projects (id, name, path, status)
		VALUES ('proj-1', 'my-proj', '/path/to/my-proj', 'active');
		INSERT INTO services (id, project_id, name, scope, container_id, image, status)
		VALUES ('svc-1', 'proj-1', 'portkey', 'system', '6b38353a75f6123456', 'portkeyai/gateway:latest', 'running');
		INSERT INTO port_allocations (service_id, container_port, host_port, protocol)
		VALUES ('svc-1', 8787, 8787, 'tcp');
	`)
	if err != nil {
		t.Fatal(err)
	}

	mock := &mockContainerLister{
		containers: []container.Summary{
			{
				ID:      "6b38353a75f6123456",
				Names:   []string{"/custom-name"},
				Image:   "portkeyai/gateway:latest",
				Status:  "Up 10 minutes",
				State:   "running",
				Ports:   nil, // Test fallback to DB port allocation
				Labels:  map[string]string{"lbmesh.managed": "true"},
			},
		},
	}

	rows, err := collectInstances(ctx, database, mock)
	if err != nil {
		t.Fatal(err)
	}

	if len(rows) != 1 {
		t.Fatalf("got %d rows, want 1", len(rows))
	}

	row := rows[0]
	if row.Name != "portkey" {
		t.Errorf("Name = %q, want %q", row.Name, "portkey")
	}
	if row.Scope != "system" {
		t.Errorf("Scope = %q, want %q", row.Scope, "system")
	}
	if row.Ports != "127.0.0.1:8787->8787/tcp" {
		t.Errorf("Ports = %q, want %q", row.Ports, "127.0.0.1:8787->8787/tcp")
	}
}
