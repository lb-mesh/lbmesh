package machine

import (
	"encoding/json"
	"testing"
)

// ---------------------------------------------------------------------------
// parsePodmanMachineJSON
// ---------------------------------------------------------------------------

func TestParsePodmanMachineJSON_running(t *testing.T) {
	list := podmanMachineList{
		{
			Name:  "podman-machine-default",
			State: "running",
			Config: podmanMachineConfig{
				Memory: 4096,
				CPUs:   4,
			},
		},
	}
	data, err := json.Marshal(list)
	if err != nil {
		t.Fatalf("marshal test fixture: %v", err)
	}

	info, err := parsePodmanMachineJSON(data)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if info.Name != "podman-machine-default" {
		t.Errorf("Name = %q, want %q", info.Name, "podman-machine-default")
	}
	if info.State != "running" {
		t.Errorf("State = %q, want %q", info.State, "running")
	}
	if info.TotalMemoryMB != 4096 {
		t.Errorf("TotalMemoryMB = %d, want 4096", info.TotalMemoryMB)
	}
	if info.CPUs != 4 {
		t.Errorf("CPUs = %d, want 4", info.CPUs)
	}
}

// When no machine is "running", the first entry is returned as fallback.
func TestParsePodmanMachineJSON_fallbackToFirst(t *testing.T) {
	list := podmanMachineList{
		{Name: "first", State: "stopped", Config: podmanMachineConfig{Memory: 2048, CPUs: 2}},
		{Name: "second", State: "stopped", Config: podmanMachineConfig{Memory: 8192, CPUs: 8}},
	}
	data, _ := json.Marshal(list)

	info, err := parsePodmanMachineJSON(data)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if info.Name != "first" {
		t.Errorf("Name = %q, want %q", info.Name, "first")
	}
}

func TestParsePodmanMachineJSON_empty(t *testing.T) {
	_, err := parsePodmanMachineJSON([]byte(`[]`))
	if err == nil {
		t.Fatal("expected error for empty machine list, got nil")
	}
}

func TestParsePodmanMachineJSON_invalidJSON(t *testing.T) {
	_, err := parsePodmanMachineJSON([]byte(`not json`))
	if err == nil {
		t.Fatal("expected error for invalid JSON, got nil")
	}
}

// Multiple machines: prefer the running one even when it isn't first.
func TestParsePodmanMachineJSON_prefersRunning(t *testing.T) {
	list := podmanMachineList{
		{Name: "idle", State: "stopped", Config: podmanMachineConfig{Memory: 1024}},
		{Name: "active", State: "running", Config: podmanMachineConfig{Memory: 6144}},
	}
	data, _ := json.Marshal(list)

	info, err := parsePodmanMachineJSON(data)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if info.Name != "active" {
		t.Errorf("Name = %q, want %q (running machine)", info.Name, "active")
	}
	if info.TotalMemoryMB != 6144 {
		t.Errorf("TotalMemoryMB = %d, want 6144", info.TotalMemoryMB)
	}
}
