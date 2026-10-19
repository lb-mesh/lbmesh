package version

import (
	"runtime/debug"
	"testing"
)

func TestApplyBuildInfo_FillsDevBuild(t *testing.T) {
	info := Info{Version: "dev", Commit: "none", Date: "unknown"}
	applyBuildInfo(&info, &debug.BuildInfo{
		Main: debug.Module{Version: "v1.0.2"},
		Settings: []debug.BuildSetting{
			{Key: "vcs.revision", Value: "bc0f727"},
			{Key: "vcs.time", Value: "2026-10-19T19:00:00Z"},
			{Key: "vcs.modified", Value: "true"},
		},
	})
	if info.Version != "v1.0.2" || info.Commit != "bc0f727-dirty" || info.Date != "2026-10-19T19:00:00Z" {
		t.Fatalf("got %+v", info)
	}
}

func TestApplyBuildInfo_KeepsReleaseValues(t *testing.T) {
	info := Info{Version: "1.0.2", Commit: "bc0f727", Date: "2026-10-19T19:00:00Z"}
	applyBuildInfo(&info, &debug.BuildInfo{
		Main:     debug.Module{Version: "(devel)"},
		Settings: []debug.BuildSetting{{Key: "vcs.revision", Value: "other"}},
	})
	if info.Version != "1.0.2" || info.Commit != "bc0f727" {
		t.Fatalf("release values overwritten: %+v", info)
	}
}
