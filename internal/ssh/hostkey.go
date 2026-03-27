package ssh

import (
	"errors"
	"fmt"
	"log"
	"net"
	"os"
	"path/filepath"
	"sync"

	"golang.org/x/crypto/ssh"
	"golang.org/x/crypto/ssh/knownhosts"
)

var (
	hostKeyCallback ssh.HostKeyCallback
	hostKeyOnce     sync.Once
	hostKeyInitErr  error
	knownHostsPath  string
	fileWriteMu     sync.Mutex // Protect concurrent writes to known_hosts

	// configDirFunc returns the directory for known_hosts. Override in tests
	// with a t.TempDir() function to avoid touching real config or env vars.
	configDirFunc = getConfigDir
)

// getConfigDir returns the application config directory (same logic as storage package)
func getConfigDir() (string, error) {
	appData := os.Getenv("APPDATA")
	if appData == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", err
		}
		return filepath.Join(home, ".port-bridge"), nil
	}
	return filepath.Join(appData, "port-bridge"), nil
}

// HostKeyError represents errors related to host key verification
type HostKeyError struct {
	Op       string // Operation that failed
	Err      error  // Underlying error
	Hostname string // Hostname being verified
}

func (e *HostKeyError) Error() string {
	return fmt.Sprintf("host key error: %s: %v", e.Op, e.Err)
}

func (e *HostKeyError) Unwrap() error {
	return e.Err
}

// ErrInsecureFallback is returned when falling back to insecure mode
var ErrInsecureFallback = errors.New("insecure host key verification fallback")

// GetHostKeyCallback returns a singleton HostKeyCallback using known_hosts (TOFU mode).
// Returns an error if known_hosts cannot be initialized - caller should decide whether
// to fall back to insecure mode with proper user warning.
func GetHostKeyCallback() (ssh.HostKeyCallback, error) {
	hostKeyOnce.Do(func() {
		configDir, err := configDirFunc()
		if err != nil {
			log.Printf("[ERROR] Failed to get config dir for known_hosts: %v", err)
			hostKeyInitErr = &HostKeyError{Op: "get_config_dir", Err: err}
			return
		}

		knownHostsPath = filepath.Join(configDir, "known_hosts")

		// Ensure directory exists with secure permissions
		if err := os.MkdirAll(configDir, 0700); err != nil {
			log.Printf("[ERROR] Failed to create config dir: %v", err)
			hostKeyInitErr = &HostKeyError{Op: "create_config_dir", Err: err}
			return
		}

		// Check if known_hosts file exists
		if _, err := os.Stat(knownHostsPath); os.IsNotExist(err) {
			// First run: create empty known_hosts file with secure permissions
			f, err := os.OpenFile(knownHostsPath, os.O_CREATE|os.O_WRONLY, 0600)
			if err != nil {
				log.Printf("[ERROR] Failed to create known_hosts: %v", err)
				hostKeyInitErr = &HostKeyError{Op: "create_known_hosts", Err: err}
				return
			}
			f.Close()
		}

		// Load known_hosts — use TOFU callback for new hosts
		cb, err := knownhosts.New(knownHostsPath)
		if err != nil {
			log.Printf("[ERROR] Failed to load known_hosts: %v", err)
			hostKeyInitErr = &HostKeyError{Op: "load_known_hosts", Err: err}
			return
		}

		hostKeyCallback = func(hostname string, remote net.Addr, key ssh.PublicKey) error {
			err := cb(hostname, remote, key)
			if err == nil {
				return nil
			}

			// Check if it's a knownhosts error
			var keyErr *knownhosts.KeyError
			if errors.As(err, &keyErr) {
				if len(keyErr.Want) > 0 {
					// Key was found but doesn't match — possible MITM attack
					log.Printf("[ERROR] HOST KEY CHANGED for %s! Possible MITM attack. Rejecting.", hostname)
					return &HostKeyError{
						Op:       "key_mismatch",
						Err:      err,
						Hostname: hostname,
					}
				}
			}

			// Key not found — first time seeing this host, add it (TOFU)
			log.Printf("[INFO] First time connecting to %s, adding host key to known_hosts", hostname)

			// Use mutex to protect concurrent writes
			fileWriteMu.Lock()
			defer fileWriteMu.Unlock()

			f, ferr := os.OpenFile(knownHostsPath, os.O_APPEND|os.O_WRONLY, 0600)
			if ferr != nil {
				log.Printf("[ERROR] Failed to open known_hosts for writing: %v", ferr)
				// Still accept the connection but log the error
				return nil
			}
			defer f.Close()

			line := knownhosts.Line([]string{knownhosts.Normalize(hostname)}, key)
			if _, ferr := f.WriteString(line + "\n"); ferr != nil {
				log.Printf("[ERROR] Failed to write host key: %v", ferr)
			}

			return nil
		}
	})

	// On init failure, return an insecure fallback so the caller always has a
	// usable callback; the non-nil error signals to the caller to warn the user.
	if hostKeyInitErr != nil && hostKeyCallback == nil {
		return ssh.InsecureIgnoreHostKey(), hostKeyInitErr
	}
	return hostKeyCallback, hostKeyInitErr
}

// GetInsecureCallback returns an insecure host key callback.
// WARNING: This should only be used after user confirmation, as it disables
// host key verification and makes the connection vulnerable to MITM attacks.
func GetInsecureCallback() ssh.HostKeyCallback {
	log.Printf("[WARN] Using INSECURE host key verification - MITM attacks possible!")
	return ssh.InsecureIgnoreHostKey()
}
