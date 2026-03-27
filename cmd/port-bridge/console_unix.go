//go:build !windows
// +build !windows

package main

// setupConsole is a no-op on non-Windows platforms
func setupConsole() {
	// Unix-like systems use UTF-8 by default
}
