//go:build windows
// +build windows

package main

import (
	"os"
	"syscall"
	"unsafe"
)

var (
	setConsoleOutputCodePage = windowsSetConsoleOutputCodePage
	setConsoleInputCodePage  = windowsSetConsoleInputCodePage
	createSingleInstance     = windowsCreateSingleInstance
	exitProcess              = os.Exit
)

func windowsSetConsoleOutputCodePage(codePage uintptr) {
	kernel32 := syscall.NewLazyDLL("kernel32.dll")
	kernel32.NewProc("SetConsoleOutputCP").Call(codePage)
}

func windowsSetConsoleInputCodePage(codePage uintptr) {
	kernel32 := syscall.NewLazyDLL("kernel32.dll")
	kernel32.NewProc("SetConsoleCP").Call(codePage)
}

func windowsCreateSingleInstance() (uintptr, error) {
	name, _ := syscall.UTF16PtrFromString("Local\\port-bridge-single-instance")
	kernel32 := syscall.NewLazyDLL("kernel32.dll")
	createMutex := kernel32.NewProc("CreateMutexW")

	h, _, err := createMutex.Call(0, 0, uintptr(unsafe.Pointer(name)))
	return h, err
}
