//go:build windows
// +build windows

package main

// setupConsole sets up the console for proper UTF-8 display on Windows
func setupConsole() {
	setConsoleOutputCodePage(65001)
	setConsoleInputCodePage(65001)
}
