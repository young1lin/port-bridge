# Repository Guidelines

## Project Overview & Structure
PortBridge is a Go + Fyne desktop app for managing SSH port-forward tunnels. `cmd/port-bridge` contains the entry point, tray setup, platform-specific startup files, and embedded translations in `cmd/port-bridge/translations/`. Core code lives in `internal/`: `app` wires services together, `models` defines persisted types, `ssh` manages clients and tunnel lifecycle, `storage` saves JSON config, `ui` and `presenter` handle Fyne screens and callbacks, and `updater`, `i18n`, `logger`, `secure`, and `version` provide supporting services. The Go module path is `github.com/young1lin/port-bridge`, but runtime directory and binary are named `port-bridge`. Static assets live in `assets/icons`; release helpers live in `scripts/`; generated binaries go to `build/`.

## Build, Test, and Development Commands
Use the Makefile as the default workflow. `make deps` downloads and tidies modules. `make fmt` runs `go fmt ./...`. `make run` launches the app in development mode. `make build` produces `build/port-bridge.exe` with `windowsgui`, version metadata, and `-trimpath`; `make build-debug` keeps the console window for logs. `make test` runs the maintained package suite, including `-tags port_bridge` for storage/keyring tests. `make test-coverage` prints coverage for that suite. `go test -v ./...` matches CI and should be run before opening a PR.

CGO is required. On Windows, install MSYS2 MinGW-w64 GCC and ensure the path expected by `Makefile` is available.

## Architecture & Runtime Notes
The main flow is: UI action -> presenter/view callback -> `internal/app` service -> `TunnelManager` or `Store` -> UI refresh callback. `App` owns the `Store`, `ClientManager`, and `TunnelManager`. `ClientManager` uses reference counting and a single-flight connection path to avoid duplicate SSH dials. `TunnelManager` tracks runtime state such as connecting, connected, error, and reconnecting, and runs tunnel start/stop work in goroutines so the UI stays responsive.

## Coding Style & Naming Conventions
Follow standard Go conventions and let `gofmt` define formatting. Keep packages lowercase, exported identifiers in `CamelCase`, and tests in `*_test.go`. Prefer small files with focused responsibilities, for example `internal/ssh/tunnel.go` or `internal/ui/dialogs/update_dialog.go`. Keep platform-specific behavior split by suffix such as `*_windows.go` and `*_unix.go`.

## Testing & Safety Guidelines
Tests use Go’s `testing` package; `testify` is available when assertions help readability. Name tests like `TestType_Action` or `TestFunction_Scenario`, and keep them next to the code they verify. Add regression coverage for tunnel lifecycle, storage, updater, and presenter changes. Be careful with lock ordering and callbacks: release mutexes before invoking callbacks that may re-enter managed state. In Fyne code, avoid reusing a widget in multiple containers; rebuild or refresh the container instead. When initializing `widget.Select`, do not call `SetSelected(...)` with a live `OnChanged` handler unless you want the handler to fire immediately; use the helper in `internal/ui/main_window.go` or temporarily clear `OnChanged`. On Windows, secondary Fyne windows can still nudge the parent window size; route those cases through `internal/ui/windowguard` instead of ad-hoc `Resize` calls.

## Configuration, Commits, and PRs
Runtime data is stored under `%APPDATA%\\port-bridge\\config.json` on Windows, with a home-directory fallback on other platforms. Logs are written to `%APPDATA%\\port-bridge\\logs\\forward-port.log` and rotated at 1 MB. Recent history follows Conventional Commit prefixes such as `feat:` and `fix:`; keep subjects short and imperative. Pull requests should summarize behavior changes, list verification steps such as `make test` and manual UI checks, include screenshots for UI updates, and keep release-related edits aligned with `.github/workflows/` and `.goreleaser.yml`.

## Additional Reference
`CLAUDE.md` contains longer assistant-facing notes about the same codebase. Use it as supplemental context, but prefer the current code, Makefile, and CI workflow when details differ.
