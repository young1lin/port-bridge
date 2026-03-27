//go:build windows
// +build windows

package main

import (
	"errors"
	"syscall"
	"testing"
)

func TestSetupConsole_UsesUTF8CodePage(t *testing.T) {
	origOutput := setConsoleOutputCodePage
	origInput := setConsoleInputCodePage
	t.Cleanup(func() {
		setConsoleOutputCodePage = origOutput
		setConsoleInputCodePage = origInput
	})

	var outputCP, inputCP uintptr
	setConsoleOutputCodePage = func(codePage uintptr) { outputCP = codePage }
	setConsoleInputCodePage = func(codePage uintptr) { inputCP = codePage }

	setupConsole()

	if outputCP != 65001 || inputCP != 65001 {
		t.Fatalf("expected UTF-8 code page 65001, got output=%d input=%d", outputCP, inputCP)
	}
}

func TestEnsureSingleInstance_SetsMutexOnSuccess(t *testing.T) {
	origCreate := createSingleInstance
	origExit := exitProcess
	singleInstanceMutex = 0
	t.Cleanup(func() {
		createSingleInstance = origCreate
		exitProcess = origExit
		singleInstanceMutex = 0
	})

	createSingleInstance = func() (uintptr, error) { return 123, syscall.Errno(0) }
	exitProcess = func(int) { t.Fatal("exit should not be called") }

	ensureSingleInstance()

	if singleInstanceMutex != 123 {
		t.Fatalf("expected mutex handle to be stored, got %d", singleInstanceMutex)
	}
}

func TestEnsureSingleInstance_ExitsWhenAlreadyRunning(t *testing.T) {
	origCreate := createSingleInstance
	origExit := exitProcess
	t.Cleanup(func() {
		createSingleInstance = origCreate
		exitProcess = origExit
	})

	createSingleInstance = func() (uintptr, error) { return 1, syscall.Errno(183) }

	exitCode := -1
	exitProcess = func(code int) { exitCode = code }

	ensureSingleInstance()

	if exitCode != 0 {
		t.Fatalf("expected exit code 0 for existing instance, got %d", exitCode)
	}
}

func TestEnsureSingleInstance_ExitsOnCreateFailure(t *testing.T) {
	origCreate := createSingleInstance
	origExit := exitProcess
	t.Cleanup(func() {
		createSingleInstance = origCreate
		exitProcess = origExit
	})

	createSingleInstance = func() (uintptr, error) { return 0, errors.New("boom") }

	exitCode := -1
	exitProcess = func(code int) { exitCode = code }

	ensureSingleInstance()

	if exitCode != 1 {
		t.Fatalf("expected exit code 1 on create failure, got %d", exitCode)
	}
}

func TestWindowsHooks_DirectCalls(t *testing.T) {
	windowsSetConsoleOutputCodePage(65001)
	windowsSetConsoleInputCodePage(65001)

	handle, err := windowsCreateSingleInstance()
	if errno, ok := err.(syscall.Errno); ok && errno != 0 && errno != 183 {
		t.Fatalf("unexpected mutex creation error: %v", err)
	}
	if handle == 0 {
		t.Fatal("expected non-zero mutex handle")
	}
}
