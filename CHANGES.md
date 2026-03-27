# Go GUI Development Notes

Lessons learned from building PortBridge (SSH port forwarding tool).

---

## 1. Technology Choices

### GUI Framework Comparison

| Framework | Pros | Cons | Best for |
|-----------|------|------|----------|
| **Fyne** | Pure Go, cross-platform, clean API | Requires CGO, less native look | Cross-platform desktop apps |
| **Wails** | Web tech, modern UI | Requires WebView, large binary | Projects with web frontend |
| **Gio** | Pure Go, no CGO | Complex API, small community | Dependency-sensitive projects |
| **Walk** | Native Windows look | Windows only | Windows-only tools |

**This project uses Fyne**: cross-platform requirement + pure Go stack + mature community.

### CGO Dependency Setup

```bash
# Windows: install MSYS2 MinGW-w64
pacman -S mingw-w64-x86_64-gcc

# Set environment variables
export PATH="/c/msys64/mingw64/bin:$PATH"
export CGO_ENABLED=1
```

---

## 2. Project Structure

### Recommended Layout

```
project/
├── cmd/
│   └── appname/           # main entry point
│       ├── main.go        # entrypoint
│       ├── singleton.go   # single-instance guard
│       └── tray.go        # system tray
├── internal/              # private packages
│   ├── app/               # app core, lifecycle management
│   ├── models/            # data models
│   ├── storage/           # config persistence
│   ├── ssh/               # SSH client wrapper
│   ├── secure/            # security (keyring storage)
│   └── ui/                # UI layer
│       ├── dialogs/       # dialogs
│       ├── views/         # views/pages
│       └── theme/         # theme customization
├── .github/
│   └── workflows/         # CI/CD
├── go.mod
├── Makefile
└── README.md
```

### Layered Architecture

```
┌─────────────────────────────────────┐
│           cmd/appname               │  ← entry: init, lifecycle
├─────────────────────────────────────┤
│              UI Layer               │  ← presentation: dialogs, views, theme
├─────────────────────────────────────┤
│           Presenter Layer           │  ← logic: user interaction handling
├─────────────────────────────────────┤
│           Service Layer             │  ← services: SSH, tunnel management
├─────────────────────────────────────┤
│           Storage Layer             │  ← persistence: config serialization
├─────────────────────────────────────┤
│           Models Layer              │  ← data structures
└─────────────────────────────────────┘
```

---

## 3. Core Features

### 1. Sensitive Data Storage

**Problem**: Storing passwords in plain text config files is a security risk.

**Solution**: Use the system keyring

```go
// Interface abstraction for testability
type Keyring interface {
    Set(service, user, secret string) error
    Get(service, user string) (string, error)
    Delete(service, user string) error
}

// Model uses a reference field instead of the raw secret
type SSHConnection struct {
    Password    string `json:"-"`                              // never serialized
    PasswordRef string `json:"password_ref,omitempty"`         // keyring reference
}

// Store saves the secret to the keyring, keeps only the ref in JSON
func (s *Store) SaveConnection(conn *SSHConnection) error {
    if conn.Password != "" {
        ref := fmt.Sprintf("conn-%s-password", conn.ID)
        s.keyring.Set(ServiceName, ref, conn.Password)
        conn.PasswordRef = ref
    }
    return s.save()
}
```

**Platform support**:
- Windows: DPAPI
- macOS: Keychain
- Linux: Secret Service / GNOME Keyring

### 2. Proxy Support

**Problem**: Some networks require routing SSH through a proxy.

**Solution**: SOCKS5 proxy support

```go
func (c *Client) dialThroughSOCKS5(config *ssh.ClientConfig) (*ssh.Client, error) {
    proxyAddr := fmt.Sprintf("%s:%d", c.conn.ProxyHost, c.conn.ProxyPort)

    var auth proxy.Auth
    if c.conn.ProxyUsername != "" {
        auth = proxy.Auth{User: c.conn.ProxyUsername, Password: c.conn.ProxyPassword}
    }

    dialer, err := proxy.SOCKS5("tcp", proxyAddr, &auth, proxy.Direct)
    if err != nil {
        return nil, err
    }

    conn, err := dialer.Dial("tcp", c.conn.Address())
    if err != nil {
        return nil, err
    }

    sshConn, chans, reqs, err := ssh.NewClientConn(conn, c.conn.Address(), config)
    if err != nil {
        return nil, err
    }

    return ssh.NewClient(sshConn, chans, reqs), nil
}
```

### 3. Port Forwarding Listen Address

**Problem**: Binding to 127.0.0.1 only prevents LAN access.

**Solution**: AllowLAN toggle

```go
func (t *Tunnel) LocalAddress() string {
    if t.AllowLAN {
        return fmt.Sprintf("0.0.0.0:%d", t.LocalPort)
    }
    return fmt.Sprintf("127.0.0.1:%d", t.LocalPort)
}
```

### 4. Single Instance Guard

**Windows implementation**:
```go
func ensureSingleInstance() {
    name, _ := syscall.UTF16PtrFromString("Local\\portbridge-single-instance")
    kernel32 := syscall.NewLazyDLL("kernel32.dll")
    createMutex := kernel32.NewProc("CreateMutexW")

    h, _, err := createMutex.Call(0, 0, uintptr(unsafe.Pointer(name)))
    if err.(syscall.Errno) == 183 { // ERROR_ALREADY_EXISTS
        os.Exit(0)
    }
    singleInstanceMutex = h
}
```

### 5. Connection Pool

**Problem**: Multiple tunnels may share one SSH connection.

**Solution**: Reference counting + connection pool

```go
type ClientManager struct {
    clients  map[string]*ssh.Client
    refCount map[string]int
}

func (cm *ClientManager) GetOrCreateClient(conn *SSHConnection) (*ssh.Client, error) {
    if client, exists := cm.clients[conn.ID]; exists && client.IsConnected() {
        cm.refCount[conn.ID]++
        return client, nil
    }
    // create new connection...
}

func (cm *ClientManager) ReleaseClient(connID string) {
    cm.refCount[connID]--
    if cm.refCount[connID] <= 0 {
        cm.clients[connID].Disconnect()
        delete(cm.clients, connID)
    }
}
```

---

## 4. Concurrency and Thread Safety

### Locking Rules

1. **Avoid deadlocks**: calling callbacks while holding a lock can deadlock
   ```go
   // Wrong: callback may re-acquire the lock
   func (m *Manager) Stop(id string) {
       m.mu.Lock()
       defer m.mu.Unlock()
       m.notifyStatus(id) // potential deadlock!
   }

   // Correct: release the lock before notifying
   func (m *Manager) Stop(id string) {
       m.mu.Lock()
       // ... stop logic
       m.mu.Unlock()
       m.notifyStatus(id) // lock already released
   }
   ```

2. **UI updates on the main thread**: Fyne requires UI changes on the main goroutine
   ```go
   go func() {
       err := tunnel.Start()
       // Fyne handles thread-safety internally on Refresh
       view.Refresh()
   }()
   ```

### Context for Cancellation

```go
ctx, cancel := context.WithCancel(context.Background())

select {
case <-ctx.Done():
    return // cancelled
default:
    // continue
}
```

---

## 5. Build and Release

### Makefile Template

```makefile
APP_NAME = myapp
VERSION = 1.0.0
BUILD_DIR = build
MAIN_PATH = ./cmd/myapp

LDFLAGS = -s -w -H=windowsgui -X main.Version=$(VERSION)

.PHONY: build build-debug clean

build:
    @mkdir -p $(BUILD_DIR)
    go build -ldflags "$(LDFLAGS)" -trimpath -o $(BUILD_DIR)/$(APP_NAME).exe $(MAIN_PATH)

build-debug:
    go build -trimpath -o $(BUILD_DIR)/$(APP_NAME)-debug.exe $(MAIN_PATH)

clean:
    rm -rf $(BUILD_DIR)
```

### GitHub Actions Multi-Platform Build

```yaml
jobs:
  build-windows:
    runs-on: windows-latest
    steps:
      - uses: actions/checkout@v4
      - uses: actions/setup-go@v5
        with:
          go-version: '1.21'
      - uses: msys2/setup-msys2@v2
        with:
          msystem: MINGW64
          install: mingw-w64-x86_64-gcc
      - run: go build -ldflags "-s -w -H=windowsgui" -o build/app.exe ./cmd/app

  build-linux:
    runs-on: ubuntu-latest
    steps:
      - run: sudo apt-get install -y gcc libgl1-mesa-dev xorg-dev
      - run: go build -ldflags "-s -w" -o build/app ./cmd/app
```

---

## 6. Common Issues

### 1. CGO Compile Error

**Problem**: `C compiler "gcc" not found`

**Fix**: Install MinGW-w64 (Windows) or gcc (Linux/macOS)

### 2. CJK Font Rendering

**Problem**: Default Fyne font does not include CJK glyphs.

**Fix**: Custom Theme with a bundled CJK font

```go
func (t *PortBridgeTheme) Font(s fyne.TextStyle) fyne.Resource {
    if s.Monospace {
        return resourceMonospace
    }
    return resourceCJKFont
}
```

### 3. System Tray

Fyne has built-in systray support:

```go
fyneApp.NewWindow("Main")
// tray icon and menu run alongside the main window
```

### 4. Config File Location

Follow platform conventions:
- Windows: `%APPDATA%/appname/`
- macOS: `~/Library/Application Support/appname/`
- Linux: `~/.config/appname/` (XDG_CONFIG_HOME)

---

## 7. Dependencies

### Key Packages

```go
// GUI
fyne.io/fyne/v2

// SSH
golang.org/x/crypto/ssh

// Proxy
golang.org/x/net/proxy

// Keyring
github.com/zalando/go-keyring

// UUID
github.com/google/uuid
```

### Version Management

```bash
go mod tidy
go mod vendor  # optional: vendor mode
```

---

## 8. Testing Recommendations

1. **Unit tests**: keep business logic separate from UI for easy testing
2. **Mock interfaces**: use mocks for external dependencies (Keyring, SSH Client)
3. **Build tags**: separate test and production implementations
4. **Cross-platform**: use `t.TempDir()` instead of OS-specific env vars (e.g., `APPDATA`)

```go
//go:build test
// +build test

package secure

func NewKeyring() Keyring {
    return NewMockKeyring()
}
```

---

## 9. Performance Tips

1. **Connection reuse**: SSH connection pool avoids repeated handshakes
2. **Async operations**: run network operations in goroutines
3. **Resource cleanup**: close listeners and connections promptly
4. **Keep-alive**: prevent idle SSH connections from dropping

```go
func (c *Client) keepAlive() {
    ticker := time.NewTicker(55 * time.Second)
    for {
        select {
        case <-ticker.C:
            client.SendRequest("keepalive@golang.org", true, nil)
        case <-c.stopChan:
            return
        }
    }
}
```

---

## 10. Checklist

- [ ] Use full module path (`github.com/user/project`)
- [ ] Never serialize secrets to config files
- [ ] Abstract external dependencies behind interfaces
- [ ] Be careful with lock ordering in concurrent code
- [ ] Follow platform config directory conventions
- [ ] Provide a Makefile to simplify the build
- [ ] Configure CI/CD for automated releases
- [ ] Bundle a CJK font if the UI needs it
- [ ] Implement single-instance guard
- [ ] Add structured logging for debugging

### Further Reading

- [Fyne documentation](https://developer.fyne.io/)
- [Go SSH library](https://pkg.go.dev/golang.org/x/crypto/ssh)
- [go-keyring](https://github.com/zalando/go-keyring)
