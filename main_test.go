package main

import (
	"runtime/debug"
	"strings"
	"testing"
)

func TestResolvedVersionFromBuildInfoPrefersModuleVersion(t *testing.T) {
	info := &debug.BuildInfo{
		Main: debug.Module{
			Version: "v1.3.2",
		},
		Settings: []debug.BuildSetting{
			{Key: "vcs.revision", Value: "abcdef1234567890"},
			{Key: "vcs.modified", Value: "false"},
		},
	}

	got := resolvedVersionFromBuildInfo(info)
	if got != "v1.3.2" {
		t.Fatalf("expected module version to win, got %q", got)
	}
}

func TestResolvedVersionFromBuildInfoFallsBackToRevision(t *testing.T) {
	info := &debug.BuildInfo{
		Main: debug.Module{
			Version: "(devel)",
		},
		Settings: []debug.BuildSetting{
			{Key: "vcs.revision", Value: "abcdef1234567890"},
			{Key: "vcs.modified", Value: "true"},
		},
	}

	got := resolvedVersionFromBuildInfo(info)
	if got != "abcdef1-dirty" {
		t.Fatalf("expected short dirty revision fallback, got %q", got)
	}
}

// The notes shown in Settings → Changelog are the embedded CHANGELOG.md, so a
// build with an empty or unparsable file would silently ship a blank screen.
func TestEmbeddedChangelogHasReleaseNotes(t *testing.T) {
	for _, want := range []string{"## Unreleased", "## v1.2.1", "## v1.2.0"} {
		if !strings.Contains(changelog, want) {
			t.Fatalf("expected the embedded changelog to contain %q", want)
		}
	}
}
