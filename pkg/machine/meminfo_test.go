package machine

import (
	"os"
	"testing"
)

// ---------------------------------------------------------------------------
// readMeminfoFile
// ---------------------------------------------------------------------------

func TestReadMeminfoFile_standard(t *testing.T) {
	// Write a minimal fixture that mirrors the real /proc/meminfo format.
	fixture := `MemTotal:       16384000 kB
MemFree:         8192000 kB
MemAvailable:   12288000 kB
Buffers:          512000 kB
Cached:          2048000 kB
`
	f, err := os.CreateTemp(t.TempDir(), "meminfo")
	if err != nil {
		t.Fatalf("creating temp file: %v", err)
	}
	if _, err := f.WriteString(fixture); err != nil {
		t.Fatalf("writing fixture: %v", err)
	}
	f.Close()

	info, err := readMeminfoFile(f.Name())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// 16384000 kB / 1024 = 16000 MiB
	if info.TotalMemoryMB != 16000 {
		t.Errorf("TotalMemoryMB = %d, want 16000", info.TotalMemoryMB)
	}
	// 12288000 kB / 1024 = 12000 MiB
	if info.AvailableMemoryMB != 12000 {
		t.Errorf("AvailableMemoryMB = %d, want 12000", info.AvailableMemoryMB)
	}
}

// When MemAvailable is absent the function still succeeds, returning 0 for
// AvailableMemoryMB (older kernels < 3.14 do not emit this field).
func TestReadMeminfoFile_noMemAvailable(t *testing.T) {
	fixture := `MemTotal:       8192000 kB
MemFree:        4096000 kB
`
	f, _ := os.CreateTemp(t.TempDir(), "meminfo")
	_, _ = f.WriteString(fixture)
	f.Close()

	info, err := readMeminfoFile(f.Name())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if info.TotalMemoryMB != 8000 {
		t.Errorf("TotalMemoryMB = %d, want 8000", info.TotalMemoryMB)
	}
	if info.AvailableMemoryMB != 0 {
		t.Errorf("AvailableMemoryMB = %d, want 0", info.AvailableMemoryMB)
	}
}

// Missing MemTotal should return an error.
func TestReadMeminfoFile_missingMemTotal(t *testing.T) {
	fixture := `MemFree:        4096000 kB
MemAvailable:   4096000 kB
`
	f, _ := os.CreateTemp(t.TempDir(), "meminfo")
	_, _ = f.WriteString(fixture)
	f.Close()

	_, err := readMeminfoFile(f.Name())
	if err == nil {
		t.Fatal("expected error when MemTotal is absent, got nil")
	}
}

// Unreadable file path should return an error.
func TestReadMeminfoFile_notFound(t *testing.T) {
	_, err := readMeminfoFile("/nonexistent/path/meminfo")
	if err == nil {
		t.Fatal("expected error for non-existent file, got nil")
	}
}

// ---------------------------------------------------------------------------
// parseMeminfoLine
// ---------------------------------------------------------------------------

func TestParseMeminfoLine(t *testing.T) {
	cases := []struct {
		line      string
		wantKey   string
		wantValue uint64
		wantOK    bool
	}{
		{"MemTotal:       16384000 kB", "MemTotal", 16384000, true},
		{"MemAvailable:    4096 kB", "MemAvailable", 4096, true},
		{"HugePages_Total:       0", "HugePages_Total", 0, true},
		{"", "", 0, false},
		{"NoColon", "", 0, false},
		{"BadValue:  notanumber kB", "", 0, false},
	}

	for _, tc := range cases {
		key, val, ok := parseMeminfoLine(tc.line)
		if ok != tc.wantOK {
			t.Errorf("parseMeminfoLine(%q) ok = %v, want %v", tc.line, ok, tc.wantOK)
			continue
		}
		if !ok {
			continue
		}
		if key != tc.wantKey {
			t.Errorf("parseMeminfoLine(%q) key = %q, want %q", tc.line, key, tc.wantKey)
		}
		if val != tc.wantValue {
			t.Errorf("parseMeminfoLine(%q) val = %d, want %d", tc.line, val, tc.wantValue)
		}
	}
}

// ---------------------------------------------------------------------------
// kbToMiB
// ---------------------------------------------------------------------------

func TestKbToMiB(t *testing.T) {
	cases := []struct{ in, want uint64 }{
		{0, 0},
		{1024, 1},
		{2048, 2},
		{16384000, 16000},
	}
	for _, tc := range cases {
		got := kbToMiB(tc.in)
		if got != tc.want {
			t.Errorf("kbToMiB(%d) = %d, want %d", tc.in, got, tc.want)
		}
	}
}
