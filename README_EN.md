# PortBridge

[![Go Version](https://img.shields.io/badge/Go-1.26-00ADD8?style=flat&logo=go)](https://go.dev/)
[![CI](https://github.com/young1lin/port-bridge/actions/workflows/ci.yml/badge.svg)](https://github.com/young1lin/port-bridge/actions/workflows/ci.yml)
[![Coverage](https://codecov.io/gh/young1lin/port-bridge/branch/main/graph/badge.svg)](https://codecov.io/gh/young1lin/port-bridge)
[![Go Report Card](https://goreportcard.com/badge/github.com/young1lin/port-bridge)](https://goreportcard.com/report/github.com/young1lin/port-bridge)
[![License](https://img.shields.io/badge/License-MIT-blue.svg)](LICENSE)
[![Release](https://img.shields.io/github/v/release/young1lin/port-bridge)](https://github.com/young1lin/port-bridge/releases)
[![Platform](https://img.shields.io/badge/platform-Windows%20%7C%20macOS%20%7C%20Linux-blue)](https://github.com/young1lin/port-bridge/releases)
[![Downloads](https://img.shields.io/github/downloads/young1lin/port-bridge/total)](https://github.com/young1lin/port-bridge/releases)

English | [简体中文](README.md)

PortBridge is a Go + Fyne desktop application for managing SSH connections and local port-forward tunnels. It targets common development and remote-access workflows with a simple GUI, connection reuse, and reconnect handling.

## Features

- Manage SSH connections with password or private-key authentication
- Create and monitor local port-forward tunnels
- Reuse SSH clients and recover from disconnects
- System tray integration, rotating logs, and in-app updates
- Built-in English and Simplified Chinese UI translations

## Project Layout

- `cmd/port-bridge`: entry point, tray setup, platform startup files, embedded translations
- `internal/app`: application services
- `internal/ssh`: SSH client management, tunnel lifecycle, known_hosts
- `internal/storage`: config persistence and keyring integration
- `internal/ui`, `internal/presenter`: Fyne views and interaction flow
- `internal/updater`, `internal/logger`, `internal/i18n`: updates, logging, localization

## Platform Notes

### Windows

Download and run the `.exe` directly — no extra setup required.

### Linux (Ubuntu Desktop and other GUI distros)

Requires OpenGL and X11/Wayland libraries, which Ubuntu Desktop includes by default. If you see OpenGL errors, install the missing packages:

```bash
sudo apt install libgl1-mesa-dev xorg-dev
```

### macOS

macOS Gatekeeper blocks unsigned third-party GUI apps. If you see **"cannot be opened because Apple cannot check it for malicious software"** or **"is damaged"**, run the following in Terminal to remove the quarantine flag:

```bash
xattr -cr /path/to/PortBridge.app
```

Or right-click the app in Finder → Open → Open anyway.

> Note: Full code signing and notarization requires an Apple Developer account ($99/year). Current builds are unsigned.

## Development

```bash
make deps
make run
make test-unit
make test-gui
make test-all
make test-coverage
```

- `make build` builds `build/port-bridge.exe`
- `make build-debug` keeps the console window for debugging
- `make test-unit` runs pure unit tests without Fyne / CGO / GUI runtime dependencies
- `make test-gui` runs GUI-focused tests that require Fyne and a CGO toolchain
- `make test-all` runs unit, GUI, and SSH integration tests
- `make test-coverage` enforces a 95%+ gate on core packages

The repository uses Go `1.26` and the module path `github.com/young1lin/port-bridge`.

## Releases

Pushing a tag like `v1.2.3` triggers the GitHub Release workflow. Prebuilt binaries are published on the [Releases](https://github.com/young1lin/port-bridge/releases) page.

## License

[MIT](LICENSE)
