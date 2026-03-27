//go:build integration
// +build integration

package ssh

import (
	"fmt"
	"io"
	"net"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/young1lin/port-bridge/internal/models"
	gossh "golang.org/x/crypto/ssh"
)

type fakeKeepAliveTicker struct {
	ch chan time.Time
}

func newFakeKeepAliveTicker() *fakeKeepAliveTicker {
	return &fakeKeepAliveTicker{ch: make(chan time.Time, 4)}
}

func (t *fakeKeepAliveTicker) Chan() <-chan time.Time { return t.ch }
func (t *fakeKeepAliveTicker) Stop()                  {}

type fakeTargetConn struct {
	closed bool
	mu     sync.Mutex
}

func (c *fakeTargetConn) Read(_ []byte) (int, error)         { return 0, io.EOF }
func (c *fakeTargetConn) Write(p []byte) (int, error)        { return len(p), nil }
func (c *fakeTargetConn) LocalAddr() net.Addr                { return fakeAddr("local-target") }
func (c *fakeTargetConn) RemoteAddr() net.Addr               { return fakeAddr("remote-target") }
func (c *fakeTargetConn) SetDeadline(_ time.Time) error      { return nil }
func (c *fakeTargetConn) SetReadDeadline(_ time.Time) error  { return nil }
func (c *fakeTargetConn) SetWriteDeadline(_ time.Time) error { return nil }
func (c *fakeTargetConn) Close() error {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.closed = true
	return nil
}

func (c *fakeTargetConn) isClosed() bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.closed
}

func TestNewClient(t *testing.T) {
	conn := &models.SSHConnection{
		ID:       "conn-1",
		Name:     "Test Server",
		Host:     "192.168.1.1",
		Port:     22,
		Username: "admin",
		AuthType: models.AuthTypePassword,
		Password: "pass",
	}

	c := NewClient(conn)
	if c == nil {
		t.Fatal("NewClient returned nil")
	}
	if c.conn != conn {
		t.Error("conn not set correctly")
	}
	if c.client != nil {
		t.Error("client should be nil before Connect")
	}
}

func TestClient_IsConnected_False(t *testing.T) {
	c := NewClient(&models.SSHConnection{ID: "conn-1"})
	if c.IsConnected() {
		t.Error("IsConnected should be false before Connect")
	}
}

func TestClient_GetClient_Nil(t *testing.T) {
	c := NewClient(&models.SSHConnection{ID: "conn-1"})
	if c.GetClient() != nil {
		t.Error("GetClient should return nil before Connect")
	}
}

func TestClient_Dial_NotConnected(t *testing.T) {
	c := NewClient(&models.SSHConnection{ID: "conn-1"})
	_, err := c.Dial("tcp", "localhost:8080")
	if err == nil {
		t.Fatal("expected error when dialing without connection")
	}
	if err.Error() != "not connected" {
		t.Errorf("Dial error = %q, want %q", err.Error(), "not connected")
	}
}

func TestClient_Dial_UsesInjectedDialer(t *testing.T) {
	c := NewClient(&models.SSHConnection{ID: "conn-1"})
	c.client = &gossh.Client{}

	var gotNetwork, gotAddr string
	conn := &fakeTargetConn{}
	c.dialFunc = func(_ *gossh.Client, network, addr string) (net.Conn, error) {
		gotNetwork = network
		gotAddr = addr
		return conn, nil
	}

	gotConn, err := c.Dial("tcp", "localhost:8080")
	if err != nil {
		t.Fatalf("Dial: %v", err)
	}
	if gotConn != conn {
		t.Fatal("Dial should return the injected connection")
	}
	if gotNetwork != "tcp" || gotAddr != "localhost:8080" {
		t.Fatalf("Dial used wrong args: %s %s", gotNetwork, gotAddr)
	}
}

func TestClient_TestTargetPort_NotConnected(t *testing.T) {
	c := NewClient(&models.SSHConnection{ID: "conn-1"})
	err := c.TestTargetPort("localhost", 8080)
	if err == nil {
		t.Fatal("expected error when testing target port without connection")
	}
	if err.Error() != "not connected" {
		t.Errorf("TestTargetPort error = %q, want %q", err.Error(), "not connected")
	}
}

func TestClient_TestTargetPort_Success(t *testing.T) {
	c := NewClient(&models.SSHConnection{ID: "conn-1"})
	c.client = &gossh.Client{}

	targetConn := &fakeTargetConn{}
	c.dialFunc = func(_ *gossh.Client, network, addr string) (net.Conn, error) {
		if network != "tcp" {
			t.Fatalf("network = %s, want tcp", network)
		}
		if addr != "localhost:8080" {
			t.Fatalf("addr = %s, want localhost:8080", addr)
		}
		return targetConn, nil
	}

	if err := c.TestTargetPort("localhost", 8080); err != nil {
		t.Fatalf("TestTargetPort: %v", err)
	}
	if !targetConn.isClosed() {
		t.Fatal("target connection should be closed after successful port test")
	}
}

func TestClient_TestTargetPort_DialFailure(t *testing.T) {
	c := NewClient(&models.SSHConnection{ID: "conn-1"})
	c.client = &gossh.Client{}
	c.dialFunc = func(_ *gossh.Client, network, addr string) (net.Conn, error) {
		return nil, fmt.Errorf("boom")
	}

	err := c.TestTargetPort("localhost", 8080)
	if err == nil {
		t.Fatal("expected target port test to fail")
	}
	if err.Error() != "target port localhost:8080 unreachable: boom" {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestClient_Disconnect_NotConnected(t *testing.T) {
	c := NewClient(&models.SSHConnection{ID: "conn-1"})
	// Should not panic
	err := c.Disconnect()
	if err != nil {
		t.Errorf("Disconnect on unconnected client: %v", err)
	}
}

func TestClient_Wait_NotConnected(t *testing.T) {
	c := NewClient(&models.SSHConnection{ID: "conn-1"})
	// Should not block
	c.Wait()
}

func TestClient_WaitForDisconnect_NotConnected(t *testing.T) {
	c := NewClient(&models.SSHConnection{ID: "conn-1"})
	// Should return immediately when client is nil
	c.WaitForDisconnect()
}

func TestClient_WaitForDisconnectContext_NotConnected(t *testing.T) {
	c := NewClient(&models.SSHConnection{ID: "conn-1"})
	// Should return immediately when client is nil
	done := make(chan struct{})
	go func() {
		c.WaitForDisconnectContext(nil)
		close(done)
	}()
	// Wait with a reasonable timeout
	select {
	case <-done:
		// Good: returned immediately
	case <-time.After(2 * time.Second):
		t.Fatal("WaitForDisconnectContext should return immediately for nil client")
	}
}

func TestClient_KeepAlive_StopsOnSignal(t *testing.T) {
	c := NewClient(&models.SSHConnection{ID: "conn-1", Host: "localhost", Port: 22})
	c.client = &gossh.Client{}
	c.keepAliveStop = make(chan struct{})
	ticker := newFakeKeepAliveTicker()
	c.newTicker = func(d time.Duration) keepAliveTicker { return ticker }

	done := make(chan struct{})
	go func() {
		c.keepAlive()
		close(done)
	}()

	close(c.keepAliveStop)

	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("keepAlive did not stop after stop signal")
	}
}

func TestClient_KeepAlive_StopsOnRequestError(t *testing.T) {
	c := NewClient(&models.SSHConnection{ID: "conn-1", Host: "localhost", Port: 22})
	c.client = &gossh.Client{}
	c.keepAliveStop = make(chan struct{})
	ticker := newFakeKeepAliveTicker()
	c.newTicker = func(d time.Duration) keepAliveTicker { return ticker }
	c.sendRequest = func(_ *gossh.Client) error { return fmt.Errorf("closed network connection") }
	closeCalled := make(chan struct{}, 1)
	c.closeConn = func(_ *gossh.Client) error { closeCalled <- struct{}{}; return nil }

	done := make(chan struct{})
	go func() {
		c.keepAlive()
		close(done)
	}()

	ticker.ch <- time.Now()

	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("keepAlive did not stop after request error")
	}

	select {
	case <-closeCalled:
	default:
		t.Fatal("keepAlive did not call closeConn after request error")
	}
}

func TestLoadPrivateKey_FileNotFound(t *testing.T) {
	c := NewClient(&models.SSHConnection{
		ID:      "conn-key",
		KeyPath: "/nonexistent/path/id_rsa",
	})

	_, err := c.loadPrivateKey()
	if err == nil {
		t.Fatal("expected error for non-existent key file")
	}
}

func TestLoadPrivateKey_DefaultPath(t *testing.T) {
	// When KeyPath is empty, it falls back to ~/.ssh/id_rsa
	// In a test environment, that file likely doesn't exist
	c := NewClient(&models.SSHConnection{
		ID:       "conn-defaultkey",
		KeyPath:  "",
		AuthType: models.AuthTypeKey,
	})

	_, err := c.loadPrivateKey()
	if err == nil {
		t.Log("Default key path exists in test environment (unexpected)")
	}
}

func TestLoadPrivateKey_InvalidKey(t *testing.T) {
	tmpDir := t.TempDir()
	keyFile := filepath.Join(tmpDir, "bad_key")

	// Write invalid key data
	if err := os.WriteFile(keyFile, []byte("not a valid ssh key"), 0644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	c := NewClient(&models.SSHConnection{
		ID:      "conn-badkey",
		KeyPath: keyFile,
	})

	_, err := c.loadPrivateKey()
	if err == nil {
		t.Fatal("expected error for invalid key data")
	}
}

func TestClient_TestConnection_Fail(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping network-dependent test in short mode")
	}
	// TestConnection calls Connect which will fail to a non-existent host
	c := NewClient(&models.SSHConnection{
		ID:       "conn-test",
		Host:     "192.0.2.1", // TEST-NET, should not be reachable
		Port:     22,
		Username: "test",
		AuthType: models.AuthTypePassword,
		Password: "test",
	})

	err := c.TestConnection()
	if err == nil {
		c.Disconnect()
		t.Fatal("expected error when connecting to unreachable host")
	}
}

func TestClient_Connect_Password(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping network-dependent test in short mode")
	}
	// Connect to unreachable host with password auth
	c := NewClient(&models.SSHConnection{
		ID:       "conn-pw",
		Host:     "192.0.2.1",
		Port:     22,
		Username: "test",
		AuthType: models.AuthTypePassword,
		Password: "test",
	})

	err := c.Connect()
	if err == nil {
		c.Disconnect()
		t.Log("Unexpectedly connected to 192.0.2.1")
	}
}

func TestClient_Connect_UnknownAuthType(t *testing.T) {
	c := NewClient(&models.SSHConnection{
		ID:       "conn-unknown",
		Host:     "192.0.2.1",
		Port:     22,
		Username: "test",
		AuthType: models.AuthType("unknown"),
	})

	err := c.Connect()
	if err == nil {
		t.Fatal("expected unsupported auth type error")
	}
	if err.Error() != "unsupported auth type: unknown" {
		t.Fatalf("Connect error = %q, want %q", err.Error(), "unsupported auth type: unknown")
	}
}

func TestClient_Connect_KeyAuth(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping network-dependent test in short mode")
	}
	c := NewClient(&models.SSHConnection{
		ID:       "conn-keyauth",
		Host:     "192.0.2.1",
		Port:     22,
		Username: "test",
		AuthType: models.AuthTypeKey,
		KeyPath:  "/nonexistent/id_rsa",
	})

	err := c.Connect()
	if err == nil {
		c.Disconnect()
		t.Log("Unexpectedly connected")
	}
}

func TestClient_Connect_WithProxy(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping network-dependent test in short mode")
	}
	// Connect with SOCKS5 proxy to unreachable proxy should fail
	c := NewClient(&models.SSHConnection{
		ID:        "conn-socks",
		Host:      "192.168.1.1",
		Port:      22,
		Username:  "test",
		AuthType:  models.AuthTypePassword,
		Password:  "test",
		UseProxy:  true,
		ProxyHost: "192.0.2.1",
		ProxyPort: 1080,
	})

	err := c.Connect()
	if err == nil {
		c.Disconnect()
		t.Log("Unexpectedly connected through proxy")
	}
}

func TestClient_Connect_ProxyWithAuth(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping network-dependent test in short mode")
	}
	c := NewClient(&models.SSHConnection{
		ID:            "conn-socks-auth",
		Host:          "192.168.1.1",
		Port:          22,
		Username:      "test",
		AuthType:      models.AuthTypePassword,
		Password:      "test",
		UseProxy:      true,
		ProxyHost:     "192.0.2.1",
		ProxyPort:     1080,
		ProxyUsername: "proxyuser",
		ProxyPassword: "proxypass",
	})

	err := c.Connect()
	if err == nil {
		c.Disconnect()
		t.Log("Unexpectedly connected through proxy with auth")
	}
}

func TestClient_Address_Helper(t *testing.T) {
	tests := []struct {
		host string
		port int
		want string
	}{
		{"192.168.1.1", 22, "192.168.1.1:22"},
		{"localhost", 2222, "localhost:2222"},
		{"example.com", 0, "example.com:0"},
	}

	for _, tt := range tests {
		c := NewClient(&models.SSHConnection{
			ID:       fmt.Sprintf("conn-%s-%d", tt.host, tt.port),
			Host:     tt.host,
			Port:     tt.port,
			AuthType: models.AuthTypePassword,
		})
		if addr := c.conn.Address(); addr != tt.want {
			t.Errorf("Address() = %q, want %q", addr, tt.want)
		}
	}
}
