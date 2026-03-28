# Makefile for PortBridge
# Windows build with MSYS2 MinGW-w64 GCC

APP_NAME = port-bridge
BUILD_DIR = build
MAIN_PATH = ./cmd/port-bridge
COVERAGE_PROFILE = coverage_core.cov
CORE_PACKAGES = ./internal/storage/ ./internal/secure/ ./internal/models/ ./internal/version/ ./internal/logger/ ./internal/i18n/
UNIT_PACKAGES = ./internal/models/ ./internal/version/ ./internal/updater/ ./internal/logger/ ./internal/i18n/ ./internal/app/
GUI_PACKAGES = ./cmd/port-bridge/ ./internal/presenter/ ./internal/ui/ ./internal/ui/dialogs/ ./internal/ui/theme/ ./internal/ui/views/
COVERAGE_PACKAGES = ./internal/storage/ ./internal/secure/ ./internal/models/ ./internal/version/ ./internal/logger/ ./internal/i18n/
TEST_TMP_DIR_WIN = C:\Temp\port-bridge-go
MINGW_PATH_WIN = C:\msys64\mingw64\bin

# MinGW-w64 GCC path (installed via MSYS2)
MINGW_PATH = /c/msys64/mingw64/bin

ifeq ($(OS),Windows_NT)
RAW_VERSION = $(shell git describe --tags --abbrev=0 --match "v*" 2>NUL)
BUILD_DATE = $(shell powershell -NoProfile -Command "(Get-Date).ToUniversalTime().ToString('yyyy-MM-ddTHH:mm:ssZ')")
GIT_COMMIT = $(shell git rev-parse --short HEAD 2>NUL)
BUILD_PREP = if not exist "$(TEST_TMP_DIR_WIN)" mkdir "$(TEST_TMP_DIR_WIN)"
BUILD_DIR_PREP = if not exist "$(BUILD_DIR)" mkdir "$(BUILD_DIR)"
BUILD_ENV_PREFIX = set TEMP=$(TEST_TMP_DIR_WIN) && set TMP=$(TEST_TMP_DIR_WIN) &&
BUILD_OUTPUT = "$(BUILD_DIR)\$(APP_NAME).exe"
BUILD_DEBUG_OUTPUT = "$(BUILD_DIR)\$(APP_NAME)-debug.exe"
else
RAW_VERSION = $(shell git describe --tags --abbrev=0 --match 'v*' 2>/dev/null)
BUILD_DATE = $(shell date -u +%Y-%m-%dT%H:%M:%SZ)
GIT_COMMIT = $(shell git rev-parse --short HEAD 2>/dev/null)
BUILD_PREP = mkdir -p "$(TEST_TMP_DIR_WIN)"
BUILD_DIR_PREP = mkdir -p "$(BUILD_DIR)"
BUILD_ENV_PREFIX = TEMP="$(TEST_TMP_DIR_WIN)" TMP="$(TEST_TMP_DIR_WIN)"
BUILD_OUTPUT = "$(BUILD_DIR)/$(APP_NAME).exe"
BUILD_DEBUG_OUTPUT = "$(BUILD_DIR)/$(APP_NAME)-debug.exe"
endif

VERSION = $(patsubst v%,%,$(RAW_VERSION))
ifeq ($(strip $(VERSION)),)
VERSION = 0.0.1
endif

# Build flags
VERSION_PKG = github.com/young1lin/port-bridge/internal/version
LDFLAGS_COMMON = -s -w -X $(VERSION_PKG).Version=$(VERSION) -X $(VERSION_PKG).BuildDate=$(BUILD_DATE) -X $(VERSION_PKG).GitCommit=$(GIT_COMMIT)
# -H=windowsgui hides the console window; only valid for Windows targets
LDFLAGS = $(LDFLAGS_COMMON) -H=windowsgui

.PHONY: all build run clean test test-unit test-gui test-integration test-all test-coverage deps fmt build-debug compress run-debug icon
.PHONY: build-windows build-darwin-amd64 build-darwin-arm64 build-linux build-all

# Default target
all: deps build

# Install dependencies
deps:
	go mod download
	go mod tidy

# Development run (with console window for debug output)
run:
	@export PATH="$(MINGW_PATH):$$PATH" && go run $(MAIN_PATH)

# Build release version (no console window)
build: fmt
	@echo "Building $(APP_NAME)..."
	@$(BUILD_PREP)
	@$(BUILD_DIR_PREP)
	@$(BUILD_ENV_PREFIX) go build -ldflags "$(LDFLAGS)" -trimpath -o $(BUILD_OUTPUT) $(MAIN_PATH)
	@echo "Build complete: $(BUILD_DIR)/$(APP_NAME).exe"

# Build debug version (with console window)
build-debug: fmt
	@echo "Building $(APP_NAME) (debug)..."
	@$(BUILD_PREP)
	@$(BUILD_DIR_PREP)
	@$(BUILD_ENV_PREFIX) go build -trimpath -o $(BUILD_DEBUG_OUTPUT) $(MAIN_PATH)
	@echo "Build complete: $(BUILD_DIR)/$(APP_NAME)-debug.exe"

# Run unit tests (-tags port_bridge activates MockKeyring for storage).
# Keep this target pure Go and OS-independent: no Fyne/CGO packages here.
test: test-unit

test-unit:
	@powershell -NoProfile -Command "$$env:TEMP='$(TEST_TMP_DIR_WIN)'; $$env:TMP='$(TEST_TMP_DIR_WIN)'; New-Item -ItemType Directory -Force '$(TEST_TMP_DIR_WIN)' | Out-Null; go test -short -v -tags port_bridge ./internal/storage/ ./internal/secure/"
	@powershell -NoProfile -Command "$$env:TEMP='$(TEST_TMP_DIR_WIN)'; $$env:TMP='$(TEST_TMP_DIR_WIN)'; New-Item -ItemType Directory -Force '$(TEST_TMP_DIR_WIN)' | Out-Null; go test -short -v $(UNIT_PACKAGES)"

# GUI tests depend on Fyne and therefore on CGO / a working compiler toolchain.
test-gui:
	@powershell -NoProfile -Command "$$env:PATH='$(MINGW_PATH_WIN);' + $$env:PATH; $$env:TEMP='$(TEST_TMP_DIR_WIN)'; $$env:TMP='$(TEST_TMP_DIR_WIN)'; New-Item -ItemType Directory -Force '$(TEST_TMP_DIR_WIN)' | Out-Null; go test -short -v $(GUI_PACKAGES)"

# Run integration tests explicitly.
test-integration:
	@powershell -NoProfile -Command "$$env:TEMP='$(TEST_TMP_DIR_WIN)'; $$env:TMP='$(TEST_TMP_DIR_WIN)'; New-Item -ItemType Directory -Force '$(TEST_TMP_DIR_WIN)' | Out-Null; go test -short -v -tags integration ./internal/ssh/"

test-all: test-unit test-gui test-integration

# Run pure unit tests, then enforce 95%+ coverage for deterministic core packages.
# GUI, SSH integration, and self-update process replacement are exercised separately.
test-coverage: test-unit
	@powershell -NoProfile -Command "$$env:TEMP='$(TEST_TMP_DIR_WIN)'; $$env:TMP='$(TEST_TMP_DIR_WIN)'; New-Item -ItemType Directory -Force '$(TEST_TMP_DIR_WIN)' | Out-Null; go test -short -v -tags port_bridge -covermode=atomic -coverprofile='$(COVERAGE_PROFILE)' $(COVERAGE_PACKAGES)"
	@go tool cover -func='$(COVERAGE_PROFILE)'
	@powershell -NoProfile -Command "$$line = go tool cover -func='$(COVERAGE_PROFILE)' | Select-String '^total:'; $$last = $$line.ToString().Split()[-1]; $$total = [double]$$last.Substring(0, $$last.Length - 1); if ($$total -lt 95) { Write-Host ('Coverage ' + $$total + ' is below 95'); exit 1 }"

# Clean
clean:
	go clean
	rm -rf $(BUILD_DIR)

# Format code
fmt:
	go fmt ./...

# Regenerate Windows icon and resource file.
# Requires: windres (part of MinGW-w64, available via MSYS2 or the cross-toolchain)
# Outputs:  cmd/port-bridge/icon.ico  and  cmd/port-bridge/portbridge_amd64.syso
icon:
	@echo "Generating icon.ico..."
	@go run ./cmd/port-bridge/gen_ico/main.go
	@echo "Compiling Windows resource (portbridge_amd64.syso)..."
	@windres cmd/port-bridge/portbridge.rc -O coff -o cmd/port-bridge/portbridge_amd64.syso
	@echo "Done."

# Run the debug build
run-debug: build-debug
	./$(BUILD_DIR)/$(APP_NAME)-debug.exe

# Compress release build with UPX (requires: scoop install upx / choco install upx)
# Reduces binary from ~19MB to ~6MB
compress: build
	@echo "Compressing $(APP_NAME).exe with UPX..."
	@upx --best $(BUILD_DIR)/$(APP_NAME).exe
	@echo "Compressed size:"
	@ls -lh $(BUILD_DIR)/$(APP_NAME).exe

# Cross-platform builds
build-windows: fmt
	@echo "Building $(APP_NAME) for Windows amd64..."
	@mkdir -p $(BUILD_DIR)
	GOOS=windows GOARCH=amd64 CGO_ENABLED=1 CC=x86_64-w64-mingw32-gcc \
		go build -ldflags "$(LDFLAGS)" -trimpath -o $(BUILD_DIR)/$(APP_NAME)-windows-amd64.exe $(MAIN_PATH)
	@echo "Build complete: $(BUILD_DIR)/$(APP_NAME)-windows-amd64.exe"

build-darwin-amd64: fmt
	@echo "Building $(APP_NAME) for macOS amd64..."
	@mkdir -p $(BUILD_DIR)
	GOOS=darwin GOARCH=amd64 CGO_ENABLED=1 \
		go build -ldflags "$(LDFLAGS_COMMON)" -trimpath -o $(BUILD_DIR)/$(APP_NAME)-darwin-amd64 $(MAIN_PATH)
	@echo "Build complete: $(BUILD_DIR)/$(APP_NAME)-darwin-amd64"

build-darwin-arm64: fmt
	@echo "Building $(APP_NAME) for macOS arm64..."
	@mkdir -p $(BUILD_DIR)
	GOOS=darwin GOARCH=arm64 CGO_ENABLED=1 \
		go build -ldflags "$(LDFLAGS_COMMON)" -trimpath -o $(BUILD_DIR)/$(APP_NAME)-darwin-arm64 $(MAIN_PATH)
	@echo "Build complete: $(BUILD_DIR)/$(APP_NAME)-darwin-arm64"

build-linux: fmt
	@echo "Building $(APP_NAME) for Linux amd64..."
	@mkdir -p $(BUILD_DIR)
	GOOS=linux GOARCH=amd64 CGO_ENABLED=1 \
		go build -ldflags "$(LDFLAGS_COMMON)" -trimpath -o $(BUILD_DIR)/$(APP_NAME)-linux-amd64 $(MAIN_PATH)
	@echo "Build complete: $(BUILD_DIR)/$(APP_NAME)-linux-amd64"

build-all: build-windows build-darwin-amd64 build-darwin-arm64 build-linux
	@echo "All builds complete in $(BUILD_DIR)/"
