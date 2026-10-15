package machine

import (
	"bufio"
	"fmt"
	"os"
	"strconv"
	"strings"
)

// HostMemInfo holds the subset of /proc/meminfo values needed for capacity
// checks.  All values are in MiB (converted from the kB units in the file).
type HostMemInfo struct {
	// TotalMemoryMB is MemTotal converted to MiB.
	TotalMemoryMB uint64
	// AvailableMemoryMB is MemAvailable converted to MiB.
	// MemAvailable is a kernel-provided estimate of memory available for new
	// processes without swapping (present since Linux 3.14).  It is preferred
	// over the naive MemFree + Buffers + Cached calculation.
	AvailableMemoryMB uint64
}

// ReadLinuxMeminfo reads /proc/meminfo and returns a [HostMemInfo].
// It is intentionally CGO-free: the file is parsed with pure Go text scanning
// to keep cross-compilation targets clean.
//
// This function is only meaningful on Linux; on other platforms the Podman VM
// approach ([InspectPodmanMachine]) should be used instead.
func ReadLinuxMeminfo() (HostMemInfo, error) {
	return readMeminfoFile("/proc/meminfo")
}

// readMeminfoFile is the testable inner implementation parameterised on the
// file path so unit tests can supply a fixture without needing /proc.
func readMeminfoFile(path string) (HostMemInfo, error) {
	f, err := os.Open(path)
	if err != nil {
		return HostMemInfo{}, fmt.Errorf("opening %s: %w", path, err)
	}
	defer f.Close()

	var totalKB, availKB uint64
	var foundTotal, foundAvail bool

	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := scanner.Text()
		key, valKB, ok := parseMeminfoLine(line)
		if !ok {
			continue
		}
		switch key {
		case "MemTotal":
			totalKB = valKB
			foundTotal = true
		case "MemAvailable":
			availKB = valKB
			foundAvail = true
		}
		// Stop early once both keys are found; they appear near the top of the
		// file so this avoids scanning the full ~50-line document on every call.
		if foundTotal && foundAvail {
			break
		}
	}
	if err := scanner.Err(); err != nil {
		return HostMemInfo{}, fmt.Errorf("scanning %s: %w", path, err)
	}
	if !foundTotal {
		return HostMemInfo{}, fmt.Errorf("%s: MemTotal field not found", path)
	}

	return HostMemInfo{
		TotalMemoryMB:     kbToMiB(totalKB),
		AvailableMemoryMB: kbToMiB(availKB),
	}, nil
}

// parseMeminfoLine extracts the key and numeric kB value from a single
// /proc/meminfo line in the format:
//
//	MemTotal:       16384000 kB
//
// It returns ok=false for any line that does not match this pattern.
func parseMeminfoLine(line string) (key string, valueKB uint64, ok bool) {
	// Split on ':' to separate key from value+unit.
	colon := strings.IndexByte(line, ':')
	if colon < 0 {
		return "", 0, false
	}
	key = strings.TrimSpace(line[:colon])
	rest := strings.TrimSpace(line[colon+1:])

	// The value field is "<number> kB"; split on whitespace and parse the first
	// token as the numeric part.
	fields := strings.Fields(rest)
	if len(fields) == 0 {
		return "", 0, false
	}
	n, err := strconv.ParseUint(fields[0], 10, 64)
	if err != nil {
		return "", 0, false
	}
	return key, n, true
}

// kbToMiB converts a value in kibibytes to mebibytes, truncating the result.
func kbToMiB(kb uint64) uint64 {
	return kb / 1024
}
