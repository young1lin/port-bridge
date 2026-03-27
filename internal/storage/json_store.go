package storage

import (
	"encoding/json"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"sync"

	"github.com/young1lin/port-bridge/internal/models"
	"github.com/young1lin/port-bridge/internal/secure"
)

// Store manages configuration persistence
type Store struct {
	mu       sync.RWMutex
	filePath string
	data     *Config
	keyring  secure.Keyring
}

// Config holds all application configuration
type Config struct {
	Connections []*models.SSHConnection `json:"connections"`
	Tunnels     []*models.Tunnel        `json:"tunnels"`
}

// NewStore creates a new store instance using the platform config directory.
func NewStore() (*Store, error) {
	configDir, err := getConfigDir()
	if err != nil {
		log.Printf("[ERROR] Failed to get config directory: %v", err)
		return nil, err
	}
	return NewStoreAt(configDir)
}

// NewStoreAt creates a store rooted at the given directory.
// The config file is placed directly inside dir as config.json.
// Tests use this with t.TempDir() to avoid touching real config or env vars.
func NewStoreAt(dir string) (*Store, error) {
	filePath := filepath.Join(dir, "config.json")
	log.Printf("[DEBUG] Config file path: %s", filePath)

	s := &Store{
		filePath: filePath,
		data: &Config{
			Connections: make([]*models.SSHConnection, 0),
			Tunnels:     make([]*models.Tunnel, 0),
		},
		keyring: secure.NewKeyring(),
	}

	if err := s.load(); err != nil {
		// If file doesn't exist, create empty config
		if os.IsNotExist(err) {
			log.Printf("[DEBUG] Config file doesn't exist, creating new one")
			if err := s.ensureDir(); err != nil {
				log.Printf("[ERROR] Failed to create config directory: %v", err)
				return nil, err
			}
			return s, nil
		}
		log.Printf("[ERROR] Failed to load config: %v", err)
		return nil, err
	}

	log.Printf("[DEBUG] Loaded %d connections, %d tunnels from config",
		len(s.data.Connections), len(s.data.Tunnels))
	return s, nil
}

// getConfigDir returns the application config directory
func getConfigDir() (string, error) {
	// Use AppData/Roaming on Windows
	appData := os.Getenv("APPDATA")
	if appData == "" {
		// Fallback to home directory
		home, err := os.UserHomeDir()
		if err != nil {
			return "", err
		}
		log.Printf("[DEBUG] Using home directory for config: %s", home)
		return filepath.Join(home, ".port-bridge"), nil
	}
	log.Printf("[DEBUG] Using AppData for config: %s", appData)
	return filepath.Join(appData, "port-bridge"), nil
}

// ensureDir creates the config directory if it doesn't exist
func (s *Store) ensureDir() error {
	dir := filepath.Dir(s.filePath)
	log.Printf("[DEBUG] Ensuring config directory exists: %s", dir)
	return os.MkdirAll(dir, 0755)
}

// load reads the configuration from disk
func (s *Store) load() error {
	log.Printf("[DEBUG] Loading config from: %s", s.filePath)
	data, err := os.ReadFile(s.filePath)
	if err != nil {
		return err
	}

	if err := json.Unmarshal(data, s.data); err != nil {
		return err
	}

	// Load passwords from keyring
	for _, conn := range s.data.Connections {
		s.loadPasswordsFromKeyring(conn)
	}

	return nil
}

// loadPasswordsFromKeyring loads passwords from system keyring
func (s *Store) loadPasswordsFromKeyring(conn *models.SSHConnection) {
	if conn.PasswordRef != "" {
		if secret, err := s.keyring.Get(secure.ServiceName, conn.PasswordRef); err == nil {
			conn.Password = secret
		} else {
			log.Printf("[DEBUG] Failed to load password from keyring for %s: %v", conn.ID, err)
		}
	}
	if conn.KeyPassRef != "" {
		if secret, err := s.keyring.Get(secure.ServiceName, conn.KeyPassRef); err == nil {
			conn.KeyPassphrase = secret
		}
	}
	if conn.ProxyPassRef != "" {
		if secret, err := s.keyring.Get(secure.ServiceName, conn.ProxyPassRef); err == nil {
			conn.ProxyPassword = secret
		}
	}
}

// savePasswordsToKeyring saves passwords to system keyring and sets references.
// If a password is cleared (empty), the stale keyring entry is deleted.
func (s *Store) savePasswordsToKeyring(conn *models.SSHConnection) {
	// Save SSH password
	if conn.Password != "" {
		ref := fmt.Sprintf("conn-%s-password", conn.ID)
		if err := s.keyring.Set(secure.ServiceName, ref, conn.Password); err != nil {
			log.Printf("[ERROR] Failed to save password to keyring: %v", err)
		} else {
			conn.PasswordRef = ref
		}
	} else if conn.PasswordRef != "" {
		s.keyring.Delete(secure.ServiceName, conn.PasswordRef)
		conn.PasswordRef = ""
	}
	// Save key passphrase
	if conn.KeyPassphrase != "" {
		ref := fmt.Sprintf("conn-%s-keypass", conn.ID)
		if err := s.keyring.Set(secure.ServiceName, ref, conn.KeyPassphrase); err != nil {
			log.Printf("[ERROR] Failed to save key passphrase to keyring: %v", err)
		} else {
			conn.KeyPassRef = ref
		}
	} else if conn.KeyPassRef != "" {
		s.keyring.Delete(secure.ServiceName, conn.KeyPassRef)
		conn.KeyPassRef = ""
	}
	// Save proxy password
	if conn.ProxyPassword != "" {
		ref := fmt.Sprintf("conn-%s-proxypass", conn.ID)
		if err := s.keyring.Set(secure.ServiceName, ref, conn.ProxyPassword); err != nil {
			log.Printf("[ERROR] Failed to save proxy password to keyring: %v", err)
		} else {
			conn.ProxyPassRef = ref
		}
	} else if conn.ProxyPassRef != "" {
		s.keyring.Delete(secure.ServiceName, conn.ProxyPassRef)
		conn.ProxyPassRef = ""
	}
}

// deletePasswordsFromKeyring removes passwords from system keyring
func (s *Store) deletePasswordsFromKeyring(conn *models.SSHConnection) {
	if conn.PasswordRef != "" {
		s.keyring.Delete(secure.ServiceName, conn.PasswordRef)
	}
	if conn.KeyPassRef != "" {
		s.keyring.Delete(secure.ServiceName, conn.KeyPassRef)
	}
	if conn.ProxyPassRef != "" {
		s.keyring.Delete(secure.ServiceName, conn.ProxyPassRef)
	}
}

// save writes the configuration to disk with secure permissions (0600)
// Uses atomic write pattern: write to temp file first, then rename
func (s *Store) save() error {
	if err := s.ensureDir(); err != nil {
		return err
	}

	data, err := json.MarshalIndent(s.data, "", "  ")
	if err != nil {
		return err
	}

	// Write to temporary file first (atomic write pattern)
	tmpPath := s.filePath + ".tmp"
	if err := os.WriteFile(tmpPath, data, 0600); err != nil {
		log.Printf("[ERROR] Failed to write temp config file: %v", err)
		return err
	}

	// Atomic rename - on Windows this replaces the target file
	log.Printf("[DEBUG] Saving config to: %s", s.filePath)
	if err := os.Rename(tmpPath, s.filePath); err != nil {
		log.Printf("[ERROR] Failed to rename config file: %v", err)
		// Try to clean up temp file
		os.Remove(tmpPath)
		return err
	}

	return nil
}

// GetConnections returns all saved connections
func (s *Store) GetConnections() []*models.SSHConnection {
	s.mu.RLock()
	defer s.mu.RUnlock()

	result := make([]*models.SSHConnection, len(s.data.Connections))
	copy(result, s.data.Connections)
	return result
}

// GetConnection returns a connection by ID
func (s *Store) GetConnection(id string) *models.SSHConnection {
	s.mu.RLock()
	defer s.mu.RUnlock()

	for _, conn := range s.data.Connections {
		if conn.ID == id {
			return conn
		}
	}
	return nil
}

// SaveConnection saves or updates a connection
func (s *Store) SaveConnection(conn *models.SSHConnection) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	log.Printf("[DEBUG] Saving connection: id=%s, name=%s", conn.ID, conn.Name)

	// Save passwords to keyring
	s.savePasswordsToKeyring(conn)

	// Update existing or add new
	for i, c := range s.data.Connections {
		if c.ID == conn.ID {
			s.data.Connections[i] = conn
			log.Printf("[DEBUG] Updated existing connection: id=%s", conn.ID)
			return s.save()
		}
	}

	s.data.Connections = append(s.data.Connections, conn)
	log.Printf("[DEBUG] Added new connection: id=%s", conn.ID)
	return s.save()
}

// DeleteConnection removes a connection by ID
func (s *Store) DeleteConnection(id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	log.Printf("[DEBUG] Deleting connection: id=%s", id)

	for i, conn := range s.data.Connections {
		if conn.ID == id {
			// Delete passwords from keyring
			s.deletePasswordsFromKeyring(conn)
			s.data.Connections = append(s.data.Connections[:i], s.data.Connections[i+1:]...)
			log.Printf("[DEBUG] Connection deleted: id=%s", id)
			return s.save()
		}
	}
	log.Printf("[DEBUG] Connection not found for deletion: id=%s", id)
	return nil
}

// GetTunnels returns all saved tunnels
func (s *Store) GetTunnels() []*models.Tunnel {
	s.mu.RLock()
	defer s.mu.RUnlock()

	result := make([]*models.Tunnel, len(s.data.Tunnels))
	copy(result, s.data.Tunnels)
	return result
}

// GetTunnel returns a tunnel by ID
func (s *Store) GetTunnel(id string) *models.Tunnel {
	s.mu.RLock()
	defer s.mu.RUnlock()

	for _, t := range s.data.Tunnels {
		if t.ID == id {
			return t
		}
	}
	return nil
}

// SaveTunnel saves or updates a tunnel
func (s *Store) SaveTunnel(tunnel *models.Tunnel) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	log.Printf("[DEBUG] Saving tunnel: id=%s, name=%s", tunnel.ID, tunnel.Name)

	// Update existing or add new
	for i, t := range s.data.Tunnels {
		if t.ID == tunnel.ID {
			s.data.Tunnels[i] = tunnel
			log.Printf("[DEBUG] Updated existing tunnel: id=%s", tunnel.ID)
			return s.save()
		}
	}

	s.data.Tunnels = append(s.data.Tunnels, tunnel)
	log.Printf("[DEBUG] Added new tunnel: id=%s", tunnel.ID)
	return s.save()
}

// DeleteTunnel removes a tunnel by ID
func (s *Store) DeleteTunnel(id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	log.Printf("[DEBUG] Deleting tunnel: id=%s", id)

	for i, t := range s.data.Tunnels {
		if t.ID == id {
			s.data.Tunnels = append(s.data.Tunnels[:i], s.data.Tunnels[i+1:]...)
			log.Printf("[DEBUG] Tunnel deleted: id=%s", id)
			return s.save()
		}
	}
	log.Printf("[DEBUG] Tunnel not found for deletion: id=%s", id)
	return nil
}

// ExportConfig returns the full configuration as JSON
func (s *Store) ExportConfig() ([]byte, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	log.Printf("[DEBUG] Exporting config")
	return json.MarshalIndent(s.data, "", "  ")
}

// ImportConfig imports configuration from JSON
func (s *Store) ImportConfig(data []byte) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	log.Printf("[DEBUG] Importing config")

	var config Config
	if err := json.Unmarshal(data, &config); err != nil {
		log.Printf("[ERROR] Failed to parse imported config: %v", err)
		return err
	}

	s.data = &config
	log.Printf("[DEBUG] Imported %d connections, %d tunnels",
		len(s.data.Connections), len(s.data.Tunnels))
	return s.save()
}
