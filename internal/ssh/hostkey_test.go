//go:build integration
// +build integration

package ssh

import (
	"os"
	"path/filepath"
	"sync"
	"testing"
)

// injectConfigDir overrides configDirFunc for the duration of a test, resetting the
// host-key singleton so each test gets a clean state.
func injectConfigDir(t *testing.T, dir string) {
	t.Helper()
	origFn := configDirFunc
	t.Cleanup(func() {
		configDirFunc = origFn
		hostKeyOnce = sync.Once{}
		hostKeyInitErr = nil
		hostKeyCallback = nil
	})
	configDirFunc = func() (string, error) { return dir, nil }
	hostKeyOnce = sync.Once{}
	hostKeyInitErr = nil
	hostKeyCallback = nil
}

func TestGetConfigDir_ReturnsValidPath(t *testing.T) {
	dir, err := getConfigDir()
	if err != nil {
		t.Fatalf("getConfigDir: %v", err)
	}
	if dir == "" {
		t.Fatal("getConfigDir returned empty path")
	}
}

func TestGetHostKeyCallback(t *testing.T) {
	dir := t.TempDir()
	injectConfigDir(t, dir)

	cb, err := GetHostKeyCallback()
	if err != nil {
		t.Fatalf("GetHostKeyCallback: %v", err)
	}
	if cb == nil {
		t.Fatal("GetHostKeyCallback returned nil callback")
	}

	// known_hosts file should be created inside dir
	if _, statErr := os.Stat(filepath.Join(dir, "known_hosts")); os.IsNotExist(statErr) {
		t.Errorf("known_hosts file should be created at %s", filepath.Join(dir, "known_hosts"))
	}
}

func TestGetHostKeyCallback_ExistingKnownHosts(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "known_hosts"), []byte(""), 0644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	injectConfigDir(t, dir)

	cb, err := GetHostKeyCallback()
	if err != nil {
		t.Fatalf("GetHostKeyCallback: %v", err)
	}
	if cb == nil {
		t.Fatal("GetHostKeyCallback returned nil callback")
	}
}

func TestGetHostKeyCallback_FallbackReturnsError(t *testing.T) {
	tmpDir := t.TempDir()
	blocker := filepath.Join(tmpDir, "blocked")
	if err := os.WriteFile(blocker, []byte("x"), 0644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	// blocker is a file; MkdirAll on it will fail, triggering the fallback path
	injectConfigDir(t, blocker)

	cb, err := GetHostKeyCallback()
	if err == nil {
		t.Fatal("expected fallback initialization error, got nil")
	}
	if cb == nil {
		t.Fatal("expected insecure fallback callback, got nil")
	}
}
