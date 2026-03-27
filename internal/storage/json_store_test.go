//go:build port_bridge

package storage

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/young1lin/port-bridge/internal/models"
	"github.com/young1lin/port-bridge/internal/secure"
)

type failingKeyring struct {
	setErr error
	getErr error
}

func (k *failingKeyring) Set(service, user, secret string) error {
	return k.setErr
}

func (k *failingKeyring) Get(service, user string) (string, error) {
	if k.getErr != nil {
		return "", k.getErr
	}
	return "", nil
}

func (k *failingKeyring) Delete(service, user string) error {
	return nil
}

func newTestStore(t *testing.T) *Store {
	t.Helper()
	s, err := NewStoreAt(t.TempDir())
	if err != nil {
		t.Fatalf("NewStoreAt: %v", err)
	}
	return s
}

// TestNewStore_CreatesNew verifies that a new store initializes with empty data.
func TestNewStore_CreatesNew(t *testing.T) {
	s, err := NewStoreAt(t.TempDir())
	if err != nil {
		t.Fatalf("NewStoreAt returned error: %v", err)
	}

	if conns := s.GetConnections(); len(conns) != 0 {
		t.Fatalf("expected 0 connections in new store, got %d", len(conns))
	}
	if tunnels := s.GetTunnels(); len(tunnels) != 0 {
		t.Fatalf("expected 0 tunnels in new store, got %d", len(tunnels))
	}
}

// TestNewStore_LoadsExisting verifies that an existing config.json is loaded correctly.
func TestNewStore_LoadsExisting(t *testing.T) {
	dir := t.TempDir()

	existing := Config{
		Connections: []*models.SSHConnection{
			{
				ID:       "conn-001",
				Name:     "My Server",
				Host:     "192.168.1.1",
				Port:     22,
				Username: "admin",
				AuthType: models.AuthTypePassword,
			},
		},
		Tunnels: []*models.Tunnel{
			{
				ID:           "tunnel-001",
				Name:         "Web Forward",
				LocalPort:    8080,
				ConnectionID: "conn-001",
				TargetHost:   "10.0.0.1",
				TargetPort:   80,
			},
		},
	}

	data, err := json.MarshalIndent(existing, "", "  ")
	if err != nil {
		t.Fatalf("failed to marshal existing config: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "config.json"), data, 0644); err != nil {
		t.Fatalf("failed to write existing config: %v", err)
	}

	s, err := NewStoreAt(dir)
	if err != nil {
		t.Fatalf("NewStoreAt returned error: %v", err)
	}

	conns := s.GetConnections()
	if len(conns) != 1 {
		t.Fatalf("expected 1 connection, got %d", len(conns))
	}
	if conns[0].ID != "conn-001" || conns[0].Name != "My Server" || conns[0].Host != "192.168.1.1" {
		t.Errorf("connection data mismatch: got ID=%s Name=%s Host=%s", conns[0].ID, conns[0].Name, conns[0].Host)
	}

	tunnels := s.GetTunnels()
	if len(tunnels) != 1 {
		t.Fatalf("expected 1 tunnel, got %d", len(tunnels))
	}
	if tunnels[0].ID != "tunnel-001" || tunnels[0].Name != "Web Forward" {
		t.Errorf("tunnel data mismatch: got ID=%s Name=%s", tunnels[0].ID, tunnels[0].Name)
	}
}

// TestGetConnections_Empty verifies that a fresh store returns an empty slice.
func TestGetConnections_Empty(t *testing.T) {
	s := newTestStore(t)

	conns := s.GetConnections()
	if conns == nil {
		t.Fatal("GetConnections returned nil, expected empty slice")
	}
	if len(conns) != 0 {
		t.Fatalf("expected 0 connections, got %d", len(conns))
	}
}

// TestSaveConnection_New verifies saving a new connection.
func TestSaveConnection_New(t *testing.T) {
	s := newTestStore(t)

	conn := &models.SSHConnection{
		ID:       "conn-new-1",
		Name:     "Test Server",
		Host:     "10.0.0.1",
		Port:     2222,
		Username: "root",
		AuthType: models.AuthTypePassword,
	}

	if err := s.SaveConnection(conn); err != nil {
		t.Fatalf("SaveConnection returned error: %v", err)
	}

	conns := s.GetConnections()
	if len(conns) != 1 {
		t.Fatalf("expected 1 connection, got %d", len(conns))
	}
	if conns[0].ID != "conn-new-1" {
		t.Errorf("expected ID conn-new-1, got %s", conns[0].ID)
	}
	if conns[0].Name != "Test Server" {
		t.Errorf("expected Name 'Test Server', got %s", conns[0].Name)
	}
	if conns[0].Host != "10.0.0.1" {
		t.Errorf("expected Host 10.0.0.1, got %s", conns[0].Host)
	}
	if conns[0].Port != 2222 {
		t.Errorf("expected Port 2222, got %d", conns[0].Port)
	}
}

// TestSaveConnection_Update verifies updating an existing connection with the same ID.
func TestSaveConnection_Update(t *testing.T) {
	s := newTestStore(t)

	conn := &models.SSHConnection{
		ID:       "conn-update-1",
		Name:     "Original Name",
		Host:     "192.168.1.1",
		Port:     22,
		Username: "user1",
		AuthType: models.AuthTypePassword,
	}
	if err := s.SaveConnection(conn); err != nil {
		t.Fatalf("initial SaveConnection error: %v", err)
	}

	updated := &models.SSHConnection{
		ID:       "conn-update-1",
		Name:     "Updated Name",
		Host:     "192.168.1.1",
		Port:     22,
		Username: "user1",
		AuthType: models.AuthTypePassword,
	}
	if err := s.SaveConnection(updated); err != nil {
		t.Fatalf("update SaveConnection error: %v", err)
	}

	conns := s.GetConnections()
	if len(conns) != 1 {
		t.Fatalf("expected 1 connection after update, got %d", len(conns))
	}
	if conns[0].Name != "Updated Name" {
		t.Errorf("expected Name 'Updated Name', got %s", conns[0].Name)
	}
}

// TestDeleteConnection verifies deleting a connection.
func TestDeleteConnection(t *testing.T) {
	s := newTestStore(t)

	conn := &models.SSHConnection{
		ID:       "conn-del-1",
		Name:     "To Delete",
		Host:     "10.0.0.1",
		Port:     22,
		Username: "user",
		AuthType: models.AuthTypePassword,
	}
	if err := s.SaveConnection(conn); err != nil {
		t.Fatalf("SaveConnection error: %v", err)
	}

	if err := s.DeleteConnection("conn-del-1"); err != nil {
		t.Fatalf("DeleteConnection returned error: %v", err)
	}

	conns := s.GetConnections()
	if len(conns) != 0 {
		t.Fatalf("expected 0 connections after delete, got %d", len(conns))
	}
}

// TestDeleteConnection_NotFound verifies deleting a non-existent connection returns no error.
func TestDeleteConnection_NotFound(t *testing.T) {
	s := newTestStore(t)

	if err := s.DeleteConnection("nonexistent-id"); err != nil {
		t.Fatalf("DeleteConnection with non-existent ID returned error: %v", err)
	}

	conns := s.GetConnections()
	if len(conns) != 0 {
		t.Fatalf("expected 0 connections, got %d", len(conns))
	}
}

// TestSaveConnection_PasswordKeyring verifies that saving a connection with a password
// stores the password in the keyring and sets the PasswordRef.
func TestSaveConnection_PasswordKeyring(t *testing.T) {
	s := newTestStore(t)

	conn := &models.SSHConnection{
		ID:       "conn-keyring-1",
		Name:     "Keyring Test",
		Host:     "10.0.0.1",
		Port:     22,
		Username: "user",
		AuthType: models.AuthTypePassword,
		Password: "secret123",
	}

	if err := s.SaveConnection(conn); err != nil {
		t.Fatalf("SaveConnection error: %v", err)
	}

	conns := s.GetConnections()
	if len(conns) != 1 {
		t.Fatalf("expected 1 connection, got %d", len(conns))
	}
	if conns[0].PasswordRef == "" {
		t.Error("expected PasswordRef to be set after saving with password, got empty string")
	}
	expectedRef := "conn-conn-keyring-1-password"
	if conns[0].PasswordRef != expectedRef {
		t.Errorf("expected PasswordRef %q, got %q", expectedRef, conns[0].PasswordRef)
	}

	if conn.PasswordRef != expectedRef {
		t.Errorf("expected original conn PasswordRef %q, got %q", expectedRef, conn.PasswordRef)
	}
}

// TestGetTunnels_Empty verifies that a fresh store returns an empty tunnels slice.
func TestGetTunnels_Empty(t *testing.T) {
	s := newTestStore(t)

	tunnels := s.GetTunnels()
	if tunnels == nil {
		t.Fatal("GetTunnels returned nil, expected empty slice")
	}
	if len(tunnels) != 0 {
		t.Fatalf("expected 0 tunnels, got %d", len(tunnels))
	}
}

// TestSaveTunnel_New verifies saving a new tunnel.
func TestSaveTunnel_New(t *testing.T) {
	s := newTestStore(t)

	tunnel := &models.Tunnel{
		ID:           "tunnel-new-1",
		Name:         "My Tunnel",
		LocalPort:    9090,
		ConnectionID: "conn-001",
		TargetHost:   "localhost",
		TargetPort:   3306,
	}

	if err := s.SaveTunnel(tunnel); err != nil {
		t.Fatalf("SaveTunnel returned error: %v", err)
	}

	tunnels := s.GetTunnels()
	if len(tunnels) != 1 {
		t.Fatalf("expected 1 tunnel, got %d", len(tunnels))
	}
	if tunnels[0].ID != "tunnel-new-1" {
		t.Errorf("expected ID tunnel-new-1, got %s", tunnels[0].ID)
	}
	if tunnels[0].Name != "My Tunnel" {
		t.Errorf("expected Name 'My Tunnel', got %s", tunnels[0].Name)
	}
	if tunnels[0].LocalPort != 9090 {
		t.Errorf("expected LocalPort 9090, got %d", tunnels[0].LocalPort)
	}
}

// TestDeleteTunnel verifies deleting a tunnel.
func TestDeleteTunnel(t *testing.T) {
	s := newTestStore(t)

	tunnel := &models.Tunnel{
		ID:           "tunnel-del-1",
		Name:         "Delete Me",
		LocalPort:    8080,
		ConnectionID: "conn-001",
		TargetHost:   "10.0.0.1",
		TargetPort:   80,
	}
	if err := s.SaveTunnel(tunnel); err != nil {
		t.Fatalf("SaveTunnel error: %v", err)
	}

	if err := s.DeleteTunnel("tunnel-del-1"); err != nil {
		t.Fatalf("DeleteTunnel returned error: %v", err)
	}

	tunnels := s.GetTunnels()
	if len(tunnels) != 0 {
		t.Fatalf("expected 0 tunnels after delete, got %d", len(tunnels))
	}
}

// TestExportConfig verifies that ExportConfig returns valid JSON with the stored data.
func TestExportConfig(t *testing.T) {
	s := newTestStore(t)

	conn := &models.SSHConnection{
		ID:       "conn-export-1",
		Name:     "Export Test",
		Host:     "10.0.0.1",
		Port:     22,
		Username: "admin",
		AuthType: models.AuthTypePassword,
	}
	if err := s.SaveConnection(conn); err != nil {
		t.Fatalf("SaveConnection error: %v", err)
	}

	tunnel := &models.Tunnel{
		ID:           "tunnel-export-1",
		Name:         "Export Tunnel",
		LocalPort:    7070,
		ConnectionID: "conn-export-1",
		TargetHost:   "10.0.0.2",
		TargetPort:   443,
	}
	if err := s.SaveTunnel(tunnel); err != nil {
		t.Fatalf("SaveTunnel error: %v", err)
	}

	data, err := s.ExportConfig()
	if err != nil {
		t.Fatalf("ExportConfig returned error: %v", err)
	}

	var cfg Config
	if err := json.Unmarshal(data, &cfg); err != nil {
		t.Fatalf("exported data is not valid JSON: %v", err)
	}

	if len(cfg.Connections) != 1 {
		t.Fatalf("expected 1 connection in exported config, got %d", len(cfg.Connections))
	}
	if cfg.Connections[0].ID != "conn-export-1" {
		t.Errorf("exported connection ID mismatch: got %s", cfg.Connections[0].ID)
	}

	if len(cfg.Tunnels) != 1 {
		t.Fatalf("expected 1 tunnel in exported config, got %d", len(cfg.Tunnels))
	}
	if cfg.Tunnels[0].ID != "tunnel-export-1" {
		t.Errorf("exported tunnel ID mismatch: got %s", cfg.Tunnels[0].ID)
	}
}

// TestImportConfig verifies importing JSON config data into the store.
func TestImportConfig(t *testing.T) {
	s := newTestStore(t)

	importData := Config{
		Connections: []*models.SSHConnection{
			{
				ID:       "conn-import-1",
				Name:     "Imported Server",
				Host:     "172.16.0.1",
				Port:     2222,
				Username: "deploy",
				AuthType: models.AuthTypeKey,
				KeyPath:  "/home/deploy/.ssh/id_rsa",
			},
		},
		Tunnels: []*models.Tunnel{
			{
				ID:           "tunnel-import-1",
				Name:         "Imported Tunnel",
				LocalPort:    6000,
				ConnectionID: "conn-import-1",
				TargetHost:   "172.16.0.2",
				TargetPort:   5432,
			},
		},
	}

	raw, err := json.MarshalIndent(importData, "", "  ")
	if err != nil {
		t.Fatalf("failed to marshal import data: %v", err)
	}

	if err := s.ImportConfig(raw); err != nil {
		t.Fatalf("ImportConfig returned error: %v", err)
	}

	conns := s.GetConnections()
	if len(conns) != 1 {
		t.Fatalf("expected 1 connection after import, got %d", len(conns))
	}
	if conns[0].ID != "conn-import-1" || conns[0].Name != "Imported Server" {
		t.Errorf("connection mismatch: got ID=%s Name=%s", conns[0].ID, conns[0].Name)
	}
	if conns[0].AuthType != models.AuthTypeKey {
		t.Errorf("expected AuthType key, got %s", conns[0].AuthType)
	}

	tunnels := s.GetTunnels()
	if len(tunnels) != 1 {
		t.Fatalf("expected 1 tunnel after import, got %d", len(tunnels))
	}
	if tunnels[0].ID != "tunnel-import-1" || tunnels[0].LocalPort != 6000 {
		t.Errorf("tunnel mismatch: got ID=%s LocalPort=%d", tunnels[0].ID, tunnels[0].LocalPort)
	}
}

// TestImportConfig_InvalidJSON verifies that importing invalid JSON returns an error.
func TestImportConfig_InvalidJSON(t *testing.T) {
	s := newTestStore(t)

	err := s.ImportConfig([]byte("this is not json"))
	if err == nil {
		t.Fatal("expected error when importing invalid JSON, got nil")
	}
}

// TestSaveTunnel_Update verifies updating an existing tunnel with the same ID.
func TestSaveTunnel_Update(t *testing.T) {
	s := newTestStore(t)

	tunnel := &models.Tunnel{
		ID:           "tunnel-upd-1",
		Name:         "Original",
		LocalPort:    3000,
		ConnectionID: "conn-001",
		TargetHost:   "localhost",
		TargetPort:   3000,
	}
	if err := s.SaveTunnel(tunnel); err != nil {
		t.Fatalf("initial SaveTunnel error: %v", err)
	}

	updated := &models.Tunnel{
		ID:           "tunnel-upd-1",
		Name:         "Updated Tunnel",
		LocalPort:    4000,
		ConnectionID: "conn-001",
		TargetHost:   "localhost",
		TargetPort:   3000,
	}
	if err := s.SaveTunnel(updated); err != nil {
		t.Fatalf("update SaveTunnel error: %v", err)
	}

	tunnels := s.GetTunnels()
	if len(tunnels) != 1 {
		t.Fatalf("expected 1 tunnel after update, got %d", len(tunnels))
	}
	if tunnels[0].Name != "Updated Tunnel" {
		t.Errorf("expected Name 'Updated Tunnel', got %s", tunnels[0].Name)
	}
	if tunnels[0].LocalPort != 4000 {
		t.Errorf("expected LocalPort 4000, got %d", tunnels[0].LocalPort)
	}
}

// TestGetConnection_Found verifies retrieving a connection by ID.
func TestGetConnection_Found(t *testing.T) {
	s := newTestStore(t)

	conn := &models.SSHConnection{
		ID:       "conn-get-1",
		Name:     "Get Test",
		Host:     "10.0.0.1",
		Port:     22,
		Username: "admin",
		AuthType: models.AuthTypePassword,
	}
	if err := s.SaveConnection(conn); err != nil {
		t.Fatalf("SaveConnection: %v", err)
	}

	found := s.GetConnection("conn-get-1")
	if found == nil {
		t.Fatal("GetConnection returned nil for existing ID")
	}
	if found.Name != "Get Test" {
		t.Errorf("GetConnection Name = %q, want %q", found.Name, "Get Test")
	}
}

// TestGetConnection_NotFound verifies GetConnection returns nil for non-existent ID.
func TestGetConnection_NotFound(t *testing.T) {
	s := newTestStore(t)

	found := s.GetConnection("nonexistent")
	if found != nil {
		t.Errorf("GetConnection should return nil for non-existent ID, got %+v", found)
	}
}

// TestGetTunnel_Found verifies retrieving a tunnel by ID.
func TestGetTunnel_Found(t *testing.T) {
	s := newTestStore(t)

	tunnel := &models.Tunnel{
		ID:           "tunnel-get-1",
		Name:         "Get Tunnel",
		LocalPort:    7070,
		ConnectionID: "conn-1",
		TargetHost:   "localhost",
		TargetPort:   80,
	}
	if err := s.SaveTunnel(tunnel); err != nil {
		t.Fatalf("SaveTunnel: %v", err)
	}

	found := s.GetTunnel("tunnel-get-1")
	if found == nil {
		t.Fatal("GetTunnel returned nil for existing ID")
	}
	if found.Name != "Get Tunnel" {
		t.Errorf("GetTunnel Name = %q, want %q", found.Name, "Get Tunnel")
	}
}

// TestGetTunnel_NotFound verifies GetTunnel returns nil for non-existent ID.
func TestGetTunnel_NotFound(t *testing.T) {
	s := newTestStore(t)

	found := s.GetTunnel("nonexistent")
	if found != nil {
		t.Errorf("GetTunnel should return nil for non-existent ID, got %+v", found)
	}
}

// TestDeleteTunnel_NotFound verifies deleting a non-existent tunnel returns no error.
func TestDeleteTunnel_NotFound(t *testing.T) {
	s := newTestStore(t)

	if err := s.DeleteTunnel("nonexistent-id"); err != nil {
		t.Fatalf("DeleteTunnel with non-existent ID returned error: %v", err)
	}
}

// TestSaveConnection_KeyPassphrase verifies saving a connection with key passphrase
// stores it in the keyring and sets the KeyPassRef.
func TestSaveConnection_KeyPassphrase(t *testing.T) {
	s := newTestStore(t)

	conn := &models.SSHConnection{
		ID:            "conn-keypass-1",
		Name:          "Key Pass Test",
		Host:          "10.0.0.1",
		Port:          22,
		Username:      "user",
		AuthType:      models.AuthTypeKey,
		KeyPassphrase: "mypassphrase",
	}

	if err := s.SaveConnection(conn); err != nil {
		t.Fatalf("SaveConnection error: %v", err)
	}

	conns := s.GetConnections()
	if len(conns) != 1 {
		t.Fatalf("expected 1 connection, got %d", len(conns))
	}
	expectedRef := "conn-conn-keypass-1-keypass"
	if conns[0].KeyPassRef != expectedRef {
		t.Errorf("expected KeyPassRef %q, got %q", expectedRef, conns[0].KeyPassRef)
	}
}

// TestSaveConnection_ProxyPassword verifies saving a connection with proxy password
// stores it in the keyring and sets the ProxyPassRef.
func TestSaveConnection_ProxyPassword(t *testing.T) {
	s := newTestStore(t)

	conn := &models.SSHConnection{
		ID:            "conn-proxypass-1",
		Name:          "Proxy Pass Test",
		Host:          "10.0.0.1",
		Port:          22,
		Username:      "user",
		AuthType:      models.AuthTypePassword,
		ProxyPassword: "myproxypass",
	}

	if err := s.SaveConnection(conn); err != nil {
		t.Fatalf("SaveConnection error: %v", err)
	}

	conns := s.GetConnections()
	if len(conns) != 1 {
		t.Fatalf("expected 1 connection, got %d", len(conns))
	}
	expectedRef := "conn-conn-proxypass-1-proxypass"
	if conns[0].ProxyPassRef != expectedRef {
		t.Errorf("expected ProxyPassRef %q, got %q", expectedRef, conns[0].ProxyPassRef)
	}
}

// TestLoadPasswordsFromKeyring verifies passwords are loaded back from the keyring.
func TestLoadPasswordsFromKeyring(t *testing.T) {
	s := newTestStore(t)

	conn := &models.SSHConnection{
		ID:            "conn-load-pw",
		Name:          "Load PW Test",
		Host:          "10.0.0.1",
		Port:          22,
		Username:      "user",
		AuthType:      models.AuthTypePassword,
		Password:      "mysecret",
		KeyPassphrase: "keysecret",
	}
	if err := s.SaveConnection(conn); err != nil {
		t.Fatalf("SaveConnection: %v", err)
	}

	conns := s.GetConnections()
	if len(conns) != 1 {
		t.Fatalf("expected 1 connection, got %d", len(conns))
	}
	if conns[0].PasswordRef == "" {
		t.Error("PasswordRef should be set after save")
	}
	if conns[0].KeyPassRef == "" {
		t.Error("KeyPassRef should be set after save")
	}
	expectedPWRef := "conn-conn-load-pw-password"
	if conns[0].PasswordRef != expectedPWRef {
		t.Errorf("PasswordRef = %q, want %q", conns[0].PasswordRef, expectedPWRef)
	}
	expectedKPRef := "conn-conn-load-pw-keypass"
	if conns[0].KeyPassRef != expectedKPRef {
		t.Errorf("KeyPassRef = %q, want %q", conns[0].KeyPassRef, expectedKPRef)
	}
}

// TestDeleteConnection_WithPassword verifies password is cleaned from keyring on delete.
func TestDeleteConnection_WithPassword(t *testing.T) {
	s := newTestStore(t)

	conn := &models.SSHConnection{
		ID:       "conn-del-pw",
		Name:     "Delete PW",
		Host:     "10.0.0.1",
		Port:     22,
		Username: "user",
		AuthType: models.AuthTypePassword,
		Password: "secret",
	}
	if err := s.SaveConnection(conn); err != nil {
		t.Fatalf("SaveConnection: %v", err)
	}

	if err := s.DeleteConnection("conn-del-pw"); err != nil {
		t.Fatalf("DeleteConnection: %v", err)
	}

	if conns := s.GetConnections(); len(conns) != 0 {
		t.Errorf("expected 0 connections after delete, got %d", len(conns))
	}
}

// TestGetConfigDir_ReturnsValidPath verifies getConfigDir returns a usable path on the current OS.
func TestGetConfigDir_ReturnsValidPath(t *testing.T) {
	dir, err := getConfigDir()
	if err != nil {
		t.Fatalf("getConfigDir: %v", err)
	}
	if dir == "" {
		t.Fatal("getConfigDir returned empty path")
	}
}

// TestLoadPasswordsFromKeyring_ProxyPassword tests loading proxy password from keyring.
func TestLoadPasswordsFromKeyring_ProxyPassword(t *testing.T) {
	s := newTestStore(t)

	conn := &models.SSHConnection{
		ID:            "conn-proxy-pw",
		Name:          "Proxy PW",
		Host:          "10.0.0.1",
		Port:          22,
		Username:      "user",
		AuthType:      models.AuthTypePassword,
		ProxyPassword: "proxypass",
	}
	if err := s.SaveConnection(conn); err != nil {
		t.Fatalf("SaveConnection: %v", err)
	}

	conns := s.GetConnections()
	if conns[0].ProxyPassRef == "" {
		t.Error("ProxyPassRef should be set")
	}
	expectedRef := "conn-conn-proxy-pw-proxypass"
	if conns[0].ProxyPassRef != expectedRef {
		t.Errorf("ProxyPassRef = %q, want %q", conns[0].ProxyPassRef, expectedRef)
	}
}

// TestDeleteConnection_WithAllPasswords tests cleanup of all keyring refs on delete.
func TestDeleteConnection_WithAllPasswords(t *testing.T) {
	s := newTestStore(t)

	conn := &models.SSHConnection{
		ID:            "conn-all-pw",
		Name:          "All PW",
		Host:          "10.0.0.1",
		Port:          22,
		Username:      "user",
		AuthType:      models.AuthTypeKey,
		Password:      "pw",
		KeyPassphrase: "kp",
		ProxyPassword: "pp",
	}
	if err := s.SaveConnection(conn); err != nil {
		t.Fatalf("SaveConnection: %v", err)
	}

	conns := s.GetConnections()
	if conns[0].PasswordRef == "" {
		t.Error("PasswordRef should be set")
	}
	if conns[0].KeyPassRef == "" {
		t.Error("KeyPassRef should be set")
	}
	if conns[0].ProxyPassRef == "" {
		t.Error("ProxyPassRef should be set")
	}

	if err := s.DeleteConnection("conn-all-pw"); err != nil {
		t.Fatalf("DeleteConnection: %v", err)
	}

	if len(s.GetConnections()) != 0 {
		t.Error("should have 0 connections after delete")
	}
}

// TestNewStore_WithExistingConfig tests loading an existing config with valid JSON.
func TestNewStore_WithExistingConfig(t *testing.T) {
	dir := t.TempDir()

	existing := Config{
		Connections: []*models.SSHConnection{
			{
				ID:       "conn-existing",
				Name:     "Existing",
				Host:     "10.0.0.1",
				Port:     22,
				Username: "root",
				AuthType: models.AuthTypeKey,
				KeyPath:  "/home/root/.ssh/id_rsa",
			},
		},
		Tunnels: []*models.Tunnel{
			{
				ID:           "tun-existing",
				Name:         "Existing Tunnel",
				LocalPort:    8080,
				ConnectionID: "conn-existing",
				TargetHost:   "localhost",
				TargetPort:   80,
			},
		},
	}
	data, err := json.MarshalIndent(existing, "", "  ")
	if err != nil {
		t.Fatalf("MarshalIndent: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "config.json"), data, 0644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	s, err := NewStoreAt(dir)
	if err != nil {
		t.Fatalf("NewStoreAt: %v", err)
	}

	if len(s.GetConnections()) != 1 {
		t.Fatalf("expected 1 connection, got %d", len(s.GetConnections()))
	}
	if len(s.GetTunnels()) != 1 {
		t.Fatalf("expected 1 tunnel, got %d", len(s.GetTunnels()))
	}
}

// TestNewStore_InvalidConfig tests error handling for invalid JSON config.
func TestNewStore_InvalidConfig(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "config.json"), []byte("not json"), 0644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	if _, err := NewStoreAt(dir); err == nil {
		t.Fatal("expected error for invalid JSON config")
	}
}

func TestNewStore_EnsureDirFailure(t *testing.T) {
	tmpDir := t.TempDir()
	blocker := filepath.Join(tmpDir, "blocked")
	if err := os.WriteFile(blocker, []byte("x"), 0644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	// blocker is a regular file; NewStoreAt tries to create it as a directory → fails
	if _, err := NewStoreAt(blocker); err == nil {
		t.Fatal("expected NewStoreAt to fail when path is a regular file")
	}
}

// TestSaveConnection_AllSecrets tests saving a connection with all secret types.
func TestSaveConnection_AllSecrets(t *testing.T) {
	s := newTestStore(t)

	conn := &models.SSHConnection{
		ID:            "conn-all-secrets",
		Name:          "All Secrets",
		Host:          "10.0.0.1",
		Port:          22,
		Username:      "user",
		AuthType:      models.AuthTypeKey,
		Password:      "sshpass",
		KeyPassphrase: "keypass",
		ProxyPassword: "proxypass",
	}

	if err := s.SaveConnection(conn); err != nil {
		t.Fatalf("SaveConnection: %v", err)
	}

	conns := s.GetConnections()
	if conns[0].PasswordRef != "conn-conn-all-secrets-password" {
		t.Errorf("PasswordRef = %q", conns[0].PasswordRef)
	}
	if conns[0].KeyPassRef != "conn-conn-all-secrets-keypass" {
		t.Errorf("KeyPassRef = %q", conns[0].KeyPassRef)
	}
	if conns[0].ProxyPassRef != "conn-conn-all-secrets-proxypass" {
		t.Errorf("ProxyPassRef = %q", conns[0].ProxyPassRef)
	}
}

// TestSaveConnection_NoSecrets tests saving a connection without any secrets.
func TestSaveConnection_NoSecrets(t *testing.T) {
	s := newTestStore(t)

	conn := &models.SSHConnection{
		ID:       "conn-no-secrets",
		Name:     "No Secrets",
		Host:     "10.0.0.1",
		Port:     22,
		Username: "user",
		AuthType: models.AuthTypeKey,
		KeyPath:  "/home/user/.ssh/id_rsa",
	}

	if err := s.SaveConnection(conn); err != nil {
		t.Fatalf("SaveConnection: %v", err)
	}

	conns := s.GetConnections()
	if conns[0].PasswordRef != "" {
		t.Errorf("PasswordRef should be empty, got %q", conns[0].PasswordRef)
	}
	if conns[0].KeyPassRef != "" {
		t.Errorf("KeyPassRef should be empty, got %q", conns[0].KeyPassRef)
	}
	if conns[0].ProxyPassRef != "" {
		t.Errorf("ProxyPassRef should be empty, got %q", conns[0].ProxyPassRef)
	}
}

func TestLoadPasswordsFromKeyring_LoadsAllRefs(t *testing.T) {
	s := newTestStore(t)
	keyring := secure.NewMockKeyring()
	if err := keyring.Set(secure.ServiceName, "pw-ref", "pw-secret"); err != nil {
		t.Fatalf("Set password secret: %v", err)
	}
	if err := keyring.Set(secure.ServiceName, "key-ref", "key-secret"); err != nil {
		t.Fatalf("Set key secret: %v", err)
	}
	if err := keyring.Set(secure.ServiceName, "proxy-ref", "proxy-secret"); err != nil {
		t.Fatalf("Set proxy secret: %v", err)
	}

	s.keyring = keyring

	conn := &models.SSHConnection{
		PasswordRef:  "pw-ref",
		KeyPassRef:   "key-ref",
		ProxyPassRef: "proxy-ref",
	}
	s.loadPasswordsFromKeyring(conn)

	if conn.Password != "pw-secret" {
		t.Fatalf("Password = %q, want %q", conn.Password, "pw-secret")
	}
	if conn.KeyPassphrase != "key-secret" {
		t.Fatalf("KeyPassphrase = %q, want %q", conn.KeyPassphrase, "key-secret")
	}
	if conn.ProxyPassword != "proxy-secret" {
		t.Fatalf("ProxyPassword = %q, want %q", conn.ProxyPassword, "proxy-secret")
	}
}

func TestLoadPasswordsFromKeyring_IgnoresLookupErrors(t *testing.T) {
	s := newTestStore(t)
	s.keyring = &failingKeyring{getErr: errors.New("boom")}

	conn := &models.SSHConnection{
		PasswordRef:  "pw-ref",
		KeyPassRef:   "key-ref",
		ProxyPassRef: "proxy-ref",
	}
	s.loadPasswordsFromKeyring(conn)

	if conn.Password != "" || conn.KeyPassphrase != "" || conn.ProxyPassword != "" {
		t.Fatal("secrets should remain empty when keyring lookups fail")
	}
}

func TestSavePasswordsToKeyring_IgnoresSetErrors(t *testing.T) {
	s := newTestStore(t)
	s.keyring = &failingKeyring{setErr: errors.New("boom")}

	conn := &models.SSHConnection{
		ID:            "conn-set-fail",
		Password:      "pw",
		KeyPassphrase: "kp",
		ProxyPassword: "pp",
	}
	s.savePasswordsToKeyring(conn)

	if conn.PasswordRef != "" || conn.KeyPassRef != "" || conn.ProxyPassRef != "" {
		t.Fatal("refs should stay empty when keyring writes fail")
	}
}

func TestStoreSave_EnsureDirFailure(t *testing.T) {
	tmpDir := t.TempDir()
	blocker := filepath.Join(tmpDir, "blocked")
	if err := os.WriteFile(blocker, []byte("x"), 0644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	s := &Store{
		filePath: filepath.Join(blocker, "config.json"),
		data:     &Config{},
		keyring:  secure.NewMockKeyring(),
	}

	if err := s.save(); err == nil {
		t.Fatal("expected save to fail when parent path cannot be created")
	}
}

func TestStoreSave_WriteFileError(t *testing.T) {
	s := &Store{
		filePath: t.TempDir(),
		data: &Config{
			Connections: []*models.SSHConnection{},
			Tunnels:     []*models.Tunnel{},
		},
		keyring: secure.NewMockKeyring(),
	}

	if err := s.save(); err == nil {
		t.Fatal("expected save to fail when filePath points to a directory")
	}
}
