//go:build !windows
// +build !windows

package main

// ensureSingleInstance is a no-op on non-Windows platforms.
func ensureSingleInstance() {}
