package machine

import (
	"database/sql"
	"fmt"
	"runtime"
	"strings"
)

// ---------------------------------------------------------------------------
// Advisor public API
// ---------------------------------------------------------------------------

// CheckResult is returned by [CheckCapacity] describing the outcome of the
// resource validation.
type CheckResult struct {
	// OK is true when sufficient memory is available to start the incoming
	// recipe without exceeding the host or VM limit.
	OK bool

	// TotalLimitMB is the discovered memory ceiling (VM or host total).
	TotalLimitMB uint64

	// ActiveUsedMB is the sum of min_memory_mb for all currently running
	// services as recorded in the database.
	ActiveUsedMB int

	// IncomingMB is the min_memory_mb declared by the recipe being checked.
	IncomingMB int

	// RequiredMB is ActiveUsedMB + IncomingMB.
	RequiredMB int

	// ShortfallMB is the extra memory needed when OK is false (zero otherwise).
	ShortfallMB int

	// MachineName is set on macOS/Windows to the active Podman machine name
	// so the remediation message can reference it directly.
	MachineName string

	// SkippedServices lists running service names whose recipes could not be
	// resolved, meaning their memory was not counted in ActiveUsedMB.
	SkippedServices []string
}

// CheckCapacity evaluates whether the host or VM has enough memory to start a
// new recipe that requires incomingMinMemoryMB.
//
// On macOS and Windows it queries the active Podman machine via
// [InspectPodmanMachine].  On Linux it reads /proc/meminfo via
// [ReadLinuxMeminfo].
//
// The db parameter may be nil; when provided it is used to add the memory
// footprint of currently running services via [ActiveFootprint].
func CheckCapacity(db *sql.DB, incomingMinMemoryMB int) (CheckResult, error) {
	var result CheckResult
	result.IncomingMB = incomingMinMemoryMB

	// ------------------------------------------------------------------
	// 1. Discover host / VM memory ceiling.
	// ------------------------------------------------------------------
	switch runtime.GOOS {
	case "darwin", "windows":
		info, err := InspectPodmanMachine()
		if err != nil {
			return result, fmt.Errorf("reading podman machine capacity: %w", err)
		}
		result.TotalLimitMB = info.TotalMemoryMB
		result.MachineName = info.Name

	default: // linux
		meminfo, err := ReadLinuxMeminfo()
		if err != nil {
			return result, fmt.Errorf("reading host memory: %w", err)
		}
		// Use AvailableMemoryMB as the effective ceiling so the check reflects
		// memory the kernel considers actually free for new processes.
		result.TotalLimitMB = meminfo.AvailableMemoryMB
	}

	// ------------------------------------------------------------------
	// 2. Sum the footprint of currently running services.
	// ------------------------------------------------------------------
	if db != nil {
		_, totalActive, skipped, err := ActiveFootprint(db)
		if err != nil {
			// Non-fatal: surface the error as a warning but continue with
			// zero active usage so the check is conservative.
			_ = err
		}
		result.ActiveUsedMB = totalActive
		result.SkippedServices = skipped
	}

	// ------------------------------------------------------------------
	// 3. Evaluate whether the new recipe fits.
	// ------------------------------------------------------------------
	result.RequiredMB = result.ActiveUsedMB + result.IncomingMB
	if uint64(result.RequiredMB) <= result.TotalLimitMB {
		result.OK = true
		return result, nil
	}

	result.ShortfallMB = result.RequiredMB - int(result.TotalLimitMB)
	return result, nil
}

// ---------------------------------------------------------------------------
// Halt helper
// ---------------------------------------------------------------------------

// ErrInsufficientMemory is the sentinel returned by [HaltIfInsufficient] when
// memory limits are exceeded.
var ErrInsufficientMemory = fmt.Errorf("insufficient memory to start recipe")

// HaltIfInsufficient runs [CheckCapacity] and, when resources are constrained,
// prints a detailed diagnostic to the provided writer and returns
// [ErrInsufficientMemory].  When resources are sufficient it returns nil.
//
// The writer is typically os.Stderr so CLI callers can do:
//
//	if err := machine.HaltIfInsufficient(os.Stderr, db, recipe.Resources.MinMemoryMB); err != nil {
//	    return err
//	}
func HaltIfInsufficient(w interface{ WriteString(string) (int, error) }, db *sql.DB, incomingMinMemoryMB int) error {
	result, err := CheckCapacity(db, incomingMinMemoryMB)
	if err != nil {
		// Capacity check itself failed — surface the error but do not block
		// deployment on a probe failure (e.g. podman not installed yet).
		msg := fmt.Sprintf("⚠  capacity check skipped: %v\n", err)
		_, _ = w.WriteString(msg)
		return nil
	}

	if result.OK {
		return nil
	}

	_, _ = w.WriteString(formatAlert(result))
	return ErrInsufficientMemory
}

// ---------------------------------------------------------------------------
// Formatting
// ---------------------------------------------------------------------------

// formatAlert builds the human-readable capacity failure message including the
// exact remediation command the user should run.
func formatAlert(r CheckResult) string {
	var b strings.Builder

	b.WriteString("\n")
	b.WriteString("╔══════════════════════════════════════════════════════════╗\n")
	b.WriteString("║            ✗  INSUFFICIENT MEMORY — HALTED              ║\n")
	b.WriteString("╚══════════════════════════════════════════════════════════╝\n")
	b.WriteString("\n")

	b.WriteString(fmt.Sprintf("  Active services footprint : %4d MiB\n", r.ActiveUsedMB))
	b.WriteString(fmt.Sprintf("  Incoming recipe requires  : %4d MiB\n", r.IncomingMB))
	b.WriteString(fmt.Sprintf("  Total required            : %4d MiB\n", r.RequiredMB))
	b.WriteString(fmt.Sprintf("  Available ceiling         : %4d MiB\n", r.TotalLimitMB))
	b.WriteString(fmt.Sprintf("  Shortfall                 : %4d MiB\n", r.ShortfallMB))

	if len(r.SkippedServices) > 0 {
		b.WriteString(fmt.Sprintf("\n  Note: memory for the following services could not be resolved\n"))
		b.WriteString(fmt.Sprintf("  and was NOT counted: %s\n", strings.Join(r.SkippedServices, ", ")))
	}

	b.WriteString("\n")
	b.WriteString("  ── Remediation ──────────────────────────────────────────\n")
	b.WriteString("\n")

	switch runtime.GOOS {
	case "darwin", "windows":
		neededMB := uint64(r.RequiredMB) + uint64(r.ShortfallMB/10+128) // +10% headroom
		machineName := r.MachineName
		if machineName == "" {
			machineName = "podman-machine-default"
		}
		b.WriteString("  Increase your Podman machine VM memory and restart it:\n\n")
		b.WriteString(fmt.Sprintf("    podman machine stop %s\n", machineName))
		b.WriteString(fmt.Sprintf("    podman machine set  --memory %d %s\n", neededMB, machineName))
		b.WriteString(fmt.Sprintf("    podman machine start %s\n", machineName))
		b.WriteString("\n  Then re-run your lbmesh command.\n")

	default: // linux
		b.WriteString("  Your host does not have enough available memory.\n")
		b.WriteString("  Free up memory by stopping unused services:\n\n")
		b.WriteString("    lbmesh down <recipe>    # stop an active recipe\n\n")
		b.WriteString("  Or adjust system swappiness / add RAM before retrying.\n")
	}

	b.WriteString("\n")
	return b.String()
}
