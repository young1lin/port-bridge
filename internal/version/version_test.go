package version

import (
	"strings"
	"testing"
)

func TestShortVersion(t *testing.T) {
	orig := Version
	Version = "1.2.3"
	defer func() { Version = orig }()

	got := ShortVersion()
	if got != "v1.2.3" {
		t.Errorf("ShortVersion() = %q, want %q", got, "v1.2.3")
	}
}

func TestFullVersionString(t *testing.T) {
	origV, origC, origB := Version, GitCommit, BuildDate
	Version, GitCommit, BuildDate = "1.0.0", "abc1234", "2026-01-01"
	defer func() { Version, GitCommit, BuildDate = origV, origC, origB }()

	got := FullVersionString()
	if !strings.Contains(got, "v1.0.0") {
		t.Errorf("FullVersionString() should contain version, got: %s", got)
	}
	if !strings.Contains(got, "abc1234") {
		t.Errorf("FullVersionString() should contain commit, got: %s", got)
	}
	if !strings.Contains(got, "2026-01-01") {
		t.Errorf("FullVersionString() should contain build date, got: %s", got)
	}
}

func TestRepoConstants(t *testing.T) {
	if RepoOwner == "" {
		t.Error("RepoOwner should not be empty")
	}
	if RepoName == "" {
		t.Error("RepoName should not be empty")
	}
}
