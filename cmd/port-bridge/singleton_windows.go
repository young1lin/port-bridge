//go:build windows
// +build windows

package main

import (
	"log"
	"syscall"
)

// singleInstanceMutex holds the named mutex handle for the lifetime of the process.
// Keeping it in a package-level var prevents the handle from being garbage-collected.
var singleInstanceMutex uintptr

// ensureSingleInstance creates a named Windows mutex.
// If another instance already holds the mutex, the process exits immediately.
func ensureSingleInstance() {
	h, err := createSingleInstance()
	if h == 0 {
		log.Printf("[ERROR] CreateMutexW failed: %v", err)
		exitProcess(1)
		return
	}

	// ERROR_ALREADY_EXISTS (183) means another instance is running
	if errno, ok := err.(syscall.Errno); ok && errno == 183 {
		log.Println("[INFO] Another instance is already running, exiting")
		exitProcess(0)
		return
	}

	singleInstanceMutex = h
}
