# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Project Overview

SSH Port Forwarding GUI Tool - A lightweight cross-platform desktop application for managing SSH tunnel port forwarding, similar to XTerminal's port forwarding feature.

**Tech Stack:** Go 1.26, Fyne v2 (GUI), golang.org/x/crypto/ssh

**Requirements:** CGO enabled (requires GCC - use MSYS2 MinGW-w64 on Windows)

### Windows Environment Variables

When building or testing on Windows, set these environment variables (MSYS2 installed at `C:\msys64`):

```bash
export PATH="/c/msys64/mingw64/bin:$PATH"
export CGO_ENABLED=1
export GOCACHE="$(go env GOCACHE)"
```

If `make test` or `make build` reports a missing GOCACHE or GCC error, prepend these variables.

## Build Commands

```bash
make deps        # Install dependencies
make fmt         # Format code (auto-run before build)
make build       # Build release (no console window)
make build-debug # Build debug version (with console)
make run         # Run in development mode
make test        # Run tests
make clean       # Clean build artifacts
```

## Build Details

**Auto-format before build**: `make build` and `make build-debug` run `go fmt ./...` automatically before compiling.

**Release build** (`make build`):
- Output: `build/port-bridge.exe`
- Uses `-ldflags "-s -w -H=windowsgui"` to strip debug info and hide the console window
- Uses `-trimpath` to remove local file system paths from the binary

**Debug build** (`make build-debug`):
- Output: `build/port-bridge-debug.exe`
- Retains debug info and shows the console window for log output

## Architecture

```
	cmd/port-bridge/
├── main.go              # Entry point, UI setup, callbacks
├── console_windows.go   # Windows console control (hide/show)
└── console_unix.go      # Unix stub for console control

internal/
├── app/
│   └── app.go           # App lifecycle, managers container
├── models/
│   ├── connection.go    # SSHConnection, AuthType
│   ├── tunnel.go        # Tunnel struct
│   └── status.go        # TunnelStatus enum with Color()
├── ssh/
│   ├── client.go        # SSH client wrapper, ClientManager
│   ├── tunnel.go        # TunnelManager, tunnel lifecycle
│   └── port_check.go    # Local port availability check
├── storage/
│   └── json_store.go    # Config persistence (%APPDATA%)
├── logger/
│   └── logger.go        # Rotating log file (1MB max)
└── ui/
    ├── main_window.go   # Main window with tabs
    ├── dialogs/
    │   ├── connection_dialog.go  # SSH connection edit dialog
    │   └── tunnel_dialog.go      # Tunnel edit dialog
    ├── views/
    │   ├── connection_view.go    # Connection list view
    │   └── tunnel_view.go        # Tunnel list view
    └── theme/
        └── theme.go    # Chinese font support theme
```

### Key Components

**App** (`internal/app/app.go`): Central manager containing:
- `Store`: JSON file persistence
- `TunnelManager`: Tunnel lifecycle management
- `ClientManager`: SSH client connection pooling

**TunnelManager** (`internal/ssh/tunnel.go`):
- State machine: Disconnected → Connecting → Connected → Error/Reconnecting
- Monitors SSH connection health, auto-releases local port on disconnect
- Uses callbacks to notify UI of status changes

**Data Flow:**
1. User action → View callback → main.go handler
2. Presenter calls `internal/app` services
3. `TunnelManager` creates or reuses SSH client, starts local listener
4. Status changes flow back through callbacks → UI refresh

## Configuration

- Config file: `%APPDATA%\port-bridge\config.json`
- Log file: `%APPDATA%\port-bridge\logs\forward-port.log` (max 1MB, auto-rotating)

## Key Patterns

- **Mutex deadlock prevention**: Release locks before calling callbacks that may re-acquire them (see `StopTunnel`)
- **Async UI operations**: Tunnel start/stop run in goroutines to avoid blocking UI
- **Fyne widget lifecycle**: Widgets can only have one parent container; use container refresh pattern
