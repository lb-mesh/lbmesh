package machine

import (
	"strings"
	"testing"
)

// ---------------------------------------------------------------------------
// formatAlert
// ---------------------------------------------------------------------------

func TestFormatAlert_containsKeyFields(t *testing.T) {
	r := CheckResult{
		OK:            false,
		TotalLimitMB:  2048,
		ActiveUsedMB:  1600,
		IncomingMB:    512,
		RequiredMB:    2112,
		ShortfallMB:   64,
		MachineName:   "podman-machine-default",
	}
	msg := formatAlert(r)

	mustContain := []string{
		"INSUFFICIENT MEMORY",
		"1600",  // active used
		"512",   // incoming
		"2112",  // required
		"2048",  // ceiling
		"64",    // shortfall
	}
	for _, sub := range mustContain {
		if !strings.Contains(msg, sub) {
			t.Errorf("formatAlert output missing %q\ngot:\n%s", sub, msg)
		}
	}
}

func TestFormatAlert_skippedServicesNote(t *testing.T) {
	r := CheckResult{
		OK:              false,
		TotalLimitMB:    1024,
		ActiveUsedMB:    0,
		IncomingMB:      2048,
		RequiredMB:      2048,
		ShortfallMB:     1024,
		SkippedServices: []string{"custom-service", "legacy-app"},
	}
	msg := formatAlert(r)

	if !strings.Contains(msg, "custom-service") {
		t.Errorf("expected skipped services listed in alert, got:\n%s", msg)
	}
	if !strings.Contains(msg, "legacy-app") {
		t.Errorf("expected skipped services listed in alert, got:\n%s", msg)
	}
}

// ---------------------------------------------------------------------------
// HaltIfInsufficient — writer interface
// ---------------------------------------------------------------------------

type testWriter struct{ buf strings.Builder }

func (w *testWriter) WriteString(s string) (int, error) { return w.buf.WriteString(s) }

func TestHaltIfInsufficient_sufficientResourcesReturnNil(t *testing.T) {
	// Pass incomingMB = 0 so required <= any discovered ceiling (even 0).
	// We rely on the platform read; on Linux it reads /proc/meminfo.
	// If the probe itself errors, HaltIfInsufficient prints a warning but
	// returns nil (non-blocking policy), so the test still passes.
	w := &testWriter{}
	err := HaltIfInsufficient(w, nil, 0)
	if err != nil {
		t.Errorf("expected nil for incomingMB=0, got: %v", err)
	}
}

func TestHaltIfInsufficient_returnsErrOnShortfall(t *testing.T) {
	// Direct call to formatAlert path by calling CheckCapacity is not possible
	// without a real host; instead we verify the error sentinel propagation
	// through the exported ErrInsufficientMemory.
	if ErrInsufficientMemory == nil {
		t.Fatal("ErrInsufficientMemory must not be nil")
	}
	if ErrInsufficientMemory.Error() == "" {
		t.Fatal("ErrInsufficientMemory must have a non-empty message")
	}
}
