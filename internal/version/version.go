// Package version exposes the lbmesh build version.
package version

// GoReleaser overrides these at link time via
// -X github.com/lbmesh/lbmesh-v2/internal/version.<Name>=<value>.
var (
	// Version is the lbmesh release version.
	Version = "dev"
	// Commit is the git commit the binary was built from.
	Commit = "none"
	// Date is the build timestamp (RFC 3339).
	Date = "unknown"
)
