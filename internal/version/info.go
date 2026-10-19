package version

import (
	"runtime"
	"runtime/debug"
)

// Info describes the running lbmesh binary.
type Info struct {
	Version   string `json:"version"`
	Commit    string `json:"commit"`
	Date      string `json:"date"`
	GoVersion string `json:"go_version"`
	Platform  string `json:"platform"`
}

// Get returns the build information for this binary. Release builds carry
// the values GoReleaser links in; for plain `go build` / `go install` builds
// the commit and date fall back to the VCS stamp Go embeds in the binary.
func Get() Info {
	info := Info{
		Version:   Version,
		Commit:    Commit,
		Date:      Date,
		GoVersion: runtime.Version(),
		Platform:  runtime.GOOS + "/" + runtime.GOARCH,
	}
	if bi, ok := debug.ReadBuildInfo(); ok {
		applyBuildInfo(&info, bi)
	}
	return info
}

// applyBuildInfo fills fields still at their defaults from Go's build info.
func applyBuildInfo(info *Info, bi *debug.BuildInfo) {
	if info.Version == "dev" && bi.Main.Version != "" && bi.Main.Version != "(devel)" {
		info.Version = bi.Main.Version // set by `go install module@vX.Y.Z`
	}
	var modified bool
	for _, s := range bi.Settings {
		switch s.Key {
		case "vcs.revision":
			if info.Commit == "none" {
				info.Commit = s.Value
			}
		case "vcs.time":
			if info.Date == "unknown" {
				info.Date = s.Value
			}
		case "vcs.modified":
			modified = s.Value == "true"
		}
	}
	if modified && info.Commit != "none" && Commit == "none" {
		info.Commit += "-dirty"
	}
}
