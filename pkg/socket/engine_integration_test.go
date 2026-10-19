package socket_test

import (
	"bytes"
	"context"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/pkg/stdcopy"

	"github.com/lbmesh/lbmesh-v2/pkg/orchestrate"
	"github.com/lbmesh/lbmesh-v2/pkg/socket"
)

// TestEngineIntegration exercises the Docker API calls lbmesh relies on
// against whatever engine Detect finds (Docker or Podman). It needs a running
// engine and a locally available image, so it is opt-in:
//
//	LBMESH_ENGINE_TEST=1 LBMESH_TEST_IMAGE=alpine:3 go test ./pkg/socket -run Integration -v
func TestEngineIntegration(t *testing.T) {
	if os.Getenv("LBMESH_ENGINE_TEST") == "" {
		t.Skip("set LBMESH_ENGINE_TEST=1 to run against a live engine")
	}
	image := os.Getenv("LBMESH_TEST_IMAGE")
	if image == "" {
		image = "alpine:3"
	}

	c, host, err := socket.Detect()
	if err != nil {
		t.Fatalf("Detect: %v", err)
	}
	defer c.Close()
	ctx := context.Background()
	ver, err := c.ServerVersion(ctx)
	if err != nil {
		t.Fatalf("ServerVersion: %v", err)
	}
	engine := "Docker"
	for _, comp := range ver.Components {
		if strings.Contains(comp.Name, "Podman") {
			engine = "Podman"
		}
	}
	t.Logf("engine: %s %s (API %s) via %s", engine, ver.Version, c.ClientVersion(), host)

	// Networks: create, idempotent re-ensure, remove.
	netName := "lbmesh-custom-itest-" + time.Now().Format("150405")
	info, err := socket.EnsureNetwork(c, netName)
	if err != nil {
		t.Fatalf("EnsureNetwork: %v", err)
	}
	defer func() {
		if err := socket.RemoveNetwork(c, info.ID); err != nil {
			t.Errorf("RemoveNetwork: %v", err)
		}
		if err := socket.RemoveNetwork(c, info.ID); err != nil {
			t.Errorf("RemoveNetwork (already gone) should be a no-op: %v", err)
		}
	}()
	again, err := socket.EnsureNetwork(c, netName)
	if err != nil || again.ID != info.ID {
		t.Fatalf("EnsureNetwork second call = %+v, %v; want same ID %s", again, err, info.ID)
	}

	// A container that writes to both streams and then idles.
	created, err := c.ContainerCreate(ctx, &container.Config{
		Image:      image,
		Entrypoint: []string{"sh", "-c", "echo out-line; echo err-line >&2; sleep 300"},
		Labels:     map[string]string{"lbmesh.managed": "true"},
	}, nil, nil, nil, netName+"-ctr")
	if err != nil {
		t.Fatalf("ContainerCreate (is %s available locally?): %v", image, err)
	}
	id := created.ID
	defer func() {
		_ = c.ContainerRemove(ctx, id, container.RemoveOptions{Force: true})
	}()

	// Start twice: the second pass must be treated as "already running".
	if res := orchestrate.StartContainers(ctx, c, []string{id}); len(res.Errors) > 0 || len(res.Changed) != 1 {
		t.Fatalf("first start: %+v", res)
	}
	if res := orchestrate.StartContainers(ctx, c, []string{id}); len(res.Errors) > 0 {
		t.Fatalf("second start should not error on %s: %+v", engine, res)
	}

	// Connect twice: the second must be tolerated.
	if err := socket.ConnectContainer(c, info.ID, id); err != nil {
		t.Fatalf("ConnectContainer: %v", err)
	}
	if err := socket.ConnectContainer(c, info.ID, id); err != nil {
		t.Fatalf("ConnectContainer (already connected) on %s: %v", engine, err)
	}
	if n, err := socket.ActiveContainerCount(c, info.ID); err != nil || n != 1 {
		t.Fatalf("ActiveContainerCount = %d, %v; want 1", n, err)
	}

	// Logs: non-TTY stream must demultiplex into clean stdout/stderr.
	time.Sleep(time.Second)
	rc, err := c.ContainerLogs(ctx, id, container.LogsOptions{ShowStdout: true, ShowStderr: true})
	if err != nil {
		t.Fatalf("ContainerLogs: %v", err)
	}
	var stdout, stderr bytes.Buffer
	if _, err := stdcopy.StdCopy(&stdout, &stderr, rc); err != nil {
		t.Fatalf("demuxing logs: %v", err)
	}
	rc.Close()
	if strings.TrimSpace(stdout.String()) != "out-line" || strings.TrimSpace(stderr.String()) != "err-line" {
		t.Fatalf("logs stdout=%q stderr=%q", stdout.String(), stderr.String())
	}

	// Stop twice: the second pass must be treated as "already stopped".
	if res := orchestrate.StopContainers(ctx, c, []string{id}, 1); len(res.Errors) > 0 || len(res.Changed) != 1 {
		t.Fatalf("first stop: %+v", res)
	}
	if res := orchestrate.StopContainers(ctx, c, []string{id}, 1); len(res.Errors) > 0 {
		t.Fatalf("second stop should not error on %s: %+v", engine, res)
	}

	_ = c.ContainerRemove(ctx, id, container.RemoveOptions{Force: true})
}
