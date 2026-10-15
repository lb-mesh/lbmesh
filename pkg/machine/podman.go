// Package machine inspects hypervisor and host capacities to prevent
// Out-Of-Memory container failures before orchestration begins.
//
// On macOS and Windows, capacity is read from the active Podman machine VM by
// running `podman machine inspect` and parsing its JSON output.
// On Linux, host native memory is read directly from /proc/meminfo.
package machine

import (
	"encoding/json"
	"fmt"
	"os/exec"
	"strings"
)

// ---------------------------------------------------------------------------
// Podman machine JSON model
// ---------------------------------------------------------------------------

// podmanMachineList is the top-level array returned by `podman machine inspect`.
type podmanMachineList []podmanMachineEntry

// podmanMachineEntry is the relevant subset of a single machine entry.
// Only the fields needed for capacity inspection are decoded; the rest are
// ignored via the catch-all JSON decoder behaviour.
type podmanMachineEntry struct {
	Name   string               `json:"Name"`
	State  string               `json:"State"`
	Config podmanMachineConfig  `json:"Config"`
}

// podmanMachineConfig holds the VM resource allocation configuration.
type podmanMachineConfig struct {
	// Memory is the VM memory limit in MiB as set by `podman machine set`.
	Memory uint64 `json:"Memory"`
	// CPUs is the number of virtual CPUs allocated to the machine.
	CPUs uint64 `json:"CPUs"`
}

// ---------------------------------------------------------------------------
// MachineInfo – unified result type
// ---------------------------------------------------------------------------

// MachineInfo reports the discovered capacity of the active Podman VM.
type MachineInfo struct {
	// Name is the machine name (e.g. "podman-machine-default").
	Name string
	// State is the current machine state (e.g. "running").
	State string
	// TotalMemoryMB is the total VM memory limit in MiB.
	TotalMemoryMB uint64
	// CPUs is the number of virtual CPUs assigned to the VM.
	CPUs uint64
}

// ---------------------------------------------------------------------------
// InspectPodmanMachine – public entry point
// ---------------------------------------------------------------------------

// InspectPodmanMachine runs `podman machine inspect` and returns capacity
// information for the first running machine found.
//
// It is intended for use on macOS and Windows where the container runtime
// executes inside a lightweight VM managed by Podman Desktop / podman-machine.
// On Linux this function is not needed; use [ReadLinuxMeminfo] instead.
//
// The function executes the podman binary resolved via PATH; no hard-coded path
// is assumed so that both Homebrew and official installer layouts are supported.
func InspectPodmanMachine() (MachineInfo, error) {
	raw, err := runPodmanInspect()
	if err != nil {
		return MachineInfo{}, err
	}
	return parsePodmanMachineJSON(raw)
}

// ---------------------------------------------------------------------------
// Internal helpers
// ---------------------------------------------------------------------------

// runPodmanInspect executes `podman machine inspect` and returns its stdout.
// A dedicated helper keeps exec.Command usage isolated for easier testing via
// [parsePodmanMachineJSON].
func runPodmanInspect() ([]byte, error) {
	cmd := exec.Command("podman", "machine", "inspect")
	out, err := cmd.Output()
	if err != nil {
		// exec.ExitError carries stderr; surface it for actionable diagnostics.
		var exitErr *exec.ExitError
		if asExitErr(err, &exitErr) && len(exitErr.Stderr) > 0 {
			return nil, fmt.Errorf(
				"podman machine inspect failed: %w\nstderr: %s",
				err, strings.TrimSpace(string(exitErr.Stderr)),
			)
		}
		return nil, fmt.Errorf("running podman machine inspect: %w", err)
	}
	return out, nil
}

// asExitErr is a thin shim so that errors.As-style type assertion compiles
// across all supported Go versions without importing errors.
func asExitErr(err error, target **exec.ExitError) bool {
	e, ok := err.(*exec.ExitError)
	if ok {
		*target = e
	}
	return ok
}

// parsePodmanMachineJSON decodes the raw JSON bytes emitted by
// `podman machine inspect` and returns a [MachineInfo] for the first machine
// whose State is "running".  If no running machine is found it falls back to
// the first entry in the list so callers always receive useful capacity data.
func parsePodmanMachineJSON(data []byte) (MachineInfo, error) {
	var list podmanMachineList
	if err := json.Unmarshal(data, &list); err != nil {
		return MachineInfo{}, fmt.Errorf("parsing podman machine inspect output: %w", err)
	}
	if len(list) == 0 {
		return MachineInfo{}, fmt.Errorf("podman machine inspect returned no machines; run `podman machine init` first")
	}

	// Prefer a running machine; fall back to the first entry.
	entry := list[0]
	for _, m := range list {
		if strings.EqualFold(m.State, "running") {
			entry = m
			break
		}
	}

	return MachineInfo{
		Name:          entry.Name,
		State:         entry.State,
		TotalMemoryMB: entry.Config.Memory,
		CPUs:          entry.Config.CPUs,
	}, nil
}
