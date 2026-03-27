//go:build integration
// +build integration

package ssh

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"sync"
	"testing"
	"time"

	"github.com/young1lin/port-bridge/internal/models"
)

type fakeAddr string

func (a fakeAddr) Network() string { return "tcp" }
func (a fakeAddr) String() string  { return string(a) }

type acceptResult struct {
	conn net.Conn
	err  error
}

type fakeListener struct {
	mu       sync.Mutex
	addr     net.Addr
	acceptCh chan acceptResult
	closedCh chan struct{}
	closed   bool
}

func newFakeListener() *fakeListener {
	return &fakeListener{
		addr:     fakeAddr("127.0.0.1:0"),
		acceptCh: make(chan acceptResult),
		closedCh: make(chan struct{}),
	}
}

func (l *fakeListener) Accept() (net.Conn, error) {
	select {
	case <-l.closedCh:
		return nil, net.ErrClosed
	case result := <-l.acceptCh:
		return result.conn, result.err
	}
}

func (l *fakeListener) Close() error {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.closed {
		return nil
	}
	l.closed = true
	close(l.closedCh)
	return nil
}

func (l *fakeListener) Addr() net.Addr {
	return l.addr
}

func (l *fakeListener) queueConn(conn net.Conn) {
	l.acceptCh <- acceptResult{conn: conn}
}

func (l *fakeListener) queueError(err error) {
	l.acceptCh <- acceptResult{err: err}
}

type fakeConn struct {
	mu         sync.Mutex
	localAddr  net.Addr
	remoteAddr net.Addr
	closed     bool
	closeCount int
}

func newFakeConn(name string) *fakeConn {
	return &fakeConn{
		localAddr:  fakeAddr("local-" + name),
		remoteAddr: fakeAddr("remote-" + name),
	}
}

func (c *fakeConn) Read(_ []byte) (int, error)         { return 0, io.EOF }
func (c *fakeConn) Write(p []byte) (int, error)        { return len(p), nil }
func (c *fakeConn) LocalAddr() net.Addr                { return c.localAddr }
func (c *fakeConn) RemoteAddr() net.Addr               { return c.remoteAddr }
func (c *fakeConn) SetDeadline(_ time.Time) error      { return nil }
func (c *fakeConn) SetReadDeadline(_ time.Time) error  { return nil }
func (c *fakeConn) SetWriteDeadline(_ time.Time) error { return nil }

func (c *fakeConn) Close() error {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.closed = true
	c.closeCount++
	return nil
}

func (c *fakeConn) isClosed() bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.closed
}

func waitForCondition(t *testing.T, timeout time.Duration, fn func() bool) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if fn() {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("condition not met before timeout")
}

func newTestActiveTunnel(interval int) (*activeTunnel, context.CancelFunc) {
	ctx, cancel := context.WithCancel(context.Background())
	at := &activeTunnel{
		tunnel: &models.Tunnel{
			ReconnectInterval: interval,
			AutoReconnect:     true,
		},
		ctx:    ctx,
		cancel: cancel,
	}
	return at, cancel
}

// TestCalcReconnectDuration verifies backoff calculation without sleeping.
func TestCalcReconnectDuration(t *testing.T) {
	m := &TunnelManager{
		tunnels: make(map[string]*activeTunnel),
	}

	at, cancel := newTestActiveTunnel(1)
	defer cancel()

	tests := []struct {
		name        string
		attempt     int
		minDuration time.Duration
		maxDuration time.Duration
	}{
		{"attempt_0_base", 0, 750 * time.Millisecond, 1250 * time.Millisecond},
		{"attempt_1_doubled", 1, 1500 * time.Millisecond, 2500 * time.Millisecond},
		{"attempt_2_4x", 2, 3 * time.Second, 5 * time.Second},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			at.reconnectAttempts = tt.attempt
			d := m.calcReconnectDuration(at)

			if d < tt.minDuration || d > tt.maxDuration {
				t.Errorf("calcReconnectDuration(attempt=%d) = %v, want [%v, %v]",
					tt.attempt, d, tt.minDuration, tt.maxDuration)
			}
		})
	}
}

// TestCalcReconnectDuration_Capped verifies the 5-minute base cap (jitter may slightly exceed).
func TestCalcReconnectDuration_Capped(t *testing.T) {
	m := &TunnelManager{
		tunnels: make(map[string]*activeTunnel),
	}

	at, cancel := newTestActiveTunnel(1)
	defer cancel()

	at.reconnectAttempts = 20 // base should be capped at 5min
	d := m.calcReconnectDuration(at)

	// With base capped at 300s and ±25% jitter, max is 300*1.25=375s
	maxExpected := 375 * time.Second
	if d > maxExpected {
		t.Errorf("calcReconnectDuration(attempt=20) = %v, should be <= %v", d, maxExpected)
	}
}

// TestCalcReconnectDuration_Increases verifies each attempt has a longer base.
func TestCalcReconnectDuration_Increases(t *testing.T) {
	m := &TunnelManager{
		tunnels: make(map[string]*activeTunnel),
	}

	at, cancel := newTestActiveTunnel(1)
	defer cancel()

	var prev time.Duration
	for i := 0; i < 5; i++ {
		at.reconnectAttempts = i
		d := m.calcReconnectDuration(at)
		if i > 0 && d <= prev {
			t.Errorf("attempt %d duration %v should be > previous %v", i, d, prev)
		}
		prev = d
	}
}

// TestReconnectWait_ContextCancellation verifies sleep respects context cancellation.
func TestReconnectWait_ContextCancellation(t *testing.T) {
	m := &TunnelManager{
		tunnels: make(map[string]*activeTunnel),
	}

	sleepCalled := make(chan time.Duration, 1)
	m.sleep = func(ctx context.Context, d time.Duration) {
		sleepCalled <- d
		<-ctx.Done()
	}

	at, cancel := newTestActiveTunnel(30)

	done := make(chan struct{})
	go func() {
		m.reconnectWait(at)
		close(done)
	}()

	// Wait for sleep to be called, then cancel
	select {
	case d := <-sleepCalled:
		t.Logf("sleep called with duration %v", d)
	case <-time.After(2 * time.Second):
		t.Fatal("sleep was not called")
	}
	cancel()

	select {
	case <-done:
		// Good: returned quickly after cancel
	case <-time.After(2 * time.Second):
		t.Fatal("reconnectWait did not respect context cancellation")
	}
}

func TestIsSSHConnectionError(t *testing.T) {
	m := &TunnelManager{
		tunnels: make(map[string]*activeTunnel),
	}

	tests := []struct {
		err      error
		expected bool
	}{
		{nil, false},
		{fmt.Errorf("closed network connection"), true},
		{fmt.Errorf("session closed"), true},
		{fmt.Errorf("connection reset by peer"), true},
		{fmt.Errorf("broken pipe"), true},
		{fmt.Errorf("transport is closing"), true},
		{fmt.Errorf("i/o timeout"), true},
		{fmt.Errorf("connection refused"), false},
		{fmt.Errorf("no route to host"), false},
		{fmt.Errorf("target port unreachable"), false},
	}

	for _, tt := range tests {
		result := m.isSSHConnectionError(tt.err)
		if result != tt.expected {
			t.Errorf("isSSHConnectionError(%q) = %v, want %v", tt.err, result, tt.expected)
		}
	}
}

func TestTunnelStopReason_Values(t *testing.T) {
	reasons := []tunnelStopReason{stopUserCancelled, stopSSHDisconnected, stopListenerError}
	seen := make(map[tunnelStopReason]bool)
	for _, r := range reasons {
		if seen[r] {
			t.Errorf("Duplicate tunnelStopReason value: %d", r)
		}
		seen[r] = true
	}
	if len(seen) != 3 {
		t.Error("Expected 3 distinct tunnelStopReason values")
	}
}

// mockConnGetter is a mock ConnectionGetter for testing
type mockConnGetter struct {
	mu           sync.Mutex
	clients      map[string]*Client
	client       *Client
	shouldFail   bool
	failErr      error
	getCount     int
	releaseCount int
}

func (g *mockConnGetter) GetOrCreateClient(conn *models.SSHConnection) (*Client, error) {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.getCount++
	if g.shouldFail {
		return nil, g.failErr
	}
	if g.client != nil {
		return g.client, nil
	}
	if c, ok := g.clients[conn.ID]; ok {
		return c, nil
	}
	c := NewClient(conn)
	g.clients[conn.ID] = c
	return c, nil
}

func (g *mockConnGetter) ReleaseClient(connID string) {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.releaseCount++
}

// mockTunnelStore implements TunnelStore for testing
type mockTunnelStore struct {
	tunnels     map[string]*models.Tunnel
	connections map[string]*models.SSHConnection
}

func newMockTunnelStore() *mockTunnelStore {
	return &mockTunnelStore{
		tunnels:     make(map[string]*models.Tunnel),
		connections: make(map[string]*models.SSHConnection),
	}
}

func (s *mockTunnelStore) GetTunnel(id string) *models.Tunnel {
	return s.tunnels[id]
}

func (s *mockTunnelStore) SaveTunnel(tunnel *models.Tunnel) error {
	s.tunnels[tunnel.ID] = tunnel
	return nil
}

func (s *mockTunnelStore) GetConnection(id string) *models.SSHConnection {
	return s.connections[id]
}

// helper to create a TunnelManager with test mocks
func newTestTunnelManager() (*TunnelManager, *mockTunnelStore, *mockConnGetter) {
	store := newMockTunnelStore()
	connGetter := &mockConnGetter{clients: make(map[string]*Client)}
	m := NewTunnelManager(store, connGetter)
	return m, store, connGetter
}

// TestNewTunnelManager verifies initial state after construction
func TestNewTunnelManager(t *testing.T) {
	store := newMockTunnelStore()
	connGetter := &mockConnGetter{clients: make(map[string]*Client)}
	m := NewTunnelManager(store, connGetter)

	if m == nil {
		t.Fatal("NewTunnelManager returned nil")
	}
	if m.store != store {
		t.Error("store not set correctly")
	}
	if m.connGetter != connGetter {
		t.Error("connGetter not set correctly")
	}
	if m.tunnels == nil {
		t.Error("tunnels map is nil")
	}
	if len(m.tunnels) != 0 {
		t.Errorf("tunnels map should be empty, got %d entries", len(m.tunnels))
	}
	// callbacks is nil until AddStatusCallback is called, which is expected
	if len(m.callbacks) != 0 {
		t.Errorf("callbacks should be empty, got %d entries", len(m.callbacks))
	}
	if m.sleep == nil {
		t.Error("sleep function is nil")
	}
}

// TestAddStatusCallback verifies callbacks are called during status changes
func TestAddStatusCallback(t *testing.T) {
	m, store, _ := newTestTunnelManager()

	// Set up a tunnel and connection in the store
	connID := "conn-1"
	store.connections[connID] = &models.SSHConnection{ID: connID, Host: "example.com", Port: 22}

	tunnel := &models.Tunnel{
		ID:           "tunnel-1",
		ConnectionID: connID,
		LocalPort:    18080,
		TargetHost:   "localhost",
		TargetPort:   8080,
	}
	store.tunnels[tunnel.ID] = tunnel

	// Register a callback
	var called bool
	var cbTunnelID string
	var cbStatus models.TunnelStatus
	m.AddStatusCallback(func(tunnelID string, status models.TunnelStatus, err error) {
		called = true
		cbTunnelID = tunnelID
		cbStatus = status
	})

	// Manually set up an active tunnel and stop it to trigger notifyStatus
	ctx, cancel := context.WithCancel(context.Background())
	at := &activeTunnel{
		tunnel: tunnel,
		status: models.StatusConnected,
		ctx:    ctx,
		cancel: cancel,
	}
	m.mu.Lock()
	m.tunnels[tunnel.ID] = at
	m.mu.Unlock()

	// Stop the tunnel — this triggers notifyStatus(StatusDisconnected)
	err := m.StopTunnel(tunnel.ID)
	if err != nil {
		t.Fatalf("StopTunnel returned error: %v", err)
	}

	if !called {
		t.Fatal("callback was not called")
	}
	if cbTunnelID != tunnel.ID {
		t.Errorf("callback tunnelID = %q, want %q", cbTunnelID, tunnel.ID)
	}
	if cbStatus != models.StatusDisconnected {
		t.Errorf("callback status = %v, want %v", cbStatus, models.StatusDisconnected)
	}
}

// TestAddStatusCallback_Multiple verifies multiple callbacks are all called
func TestAddStatusCallback_Multiple(t *testing.T) {
	m, store, _ := newTestTunnelManager()

	connID := "conn-1"
	store.connections[connID] = &models.SSHConnection{ID: connID, Host: "example.com", Port: 22}

	tunnel := &models.Tunnel{
		ID:           "tunnel-1",
		ConnectionID: connID,
		LocalPort:    18081,
		TargetHost:   "localhost",
		TargetPort:   8080,
	}
	store.tunnels[tunnel.ID] = tunnel

	var callCount int
	m.AddStatusCallback(func(tunnelID string, status models.TunnelStatus, err error) {
		callCount++
	})
	m.AddStatusCallback(func(tunnelID string, status models.TunnelStatus, err error) {
		callCount++
	})

	ctx, cancel := context.WithCancel(context.Background())
	at := &activeTunnel{
		tunnel: tunnel,
		status: models.StatusConnected,
		ctx:    ctx,
		cancel: cancel,
	}
	m.mu.Lock()
	m.tunnels[tunnel.ID] = at
	m.mu.Unlock()

	m.StopTunnel(tunnel.ID)

	if callCount != 2 {
		t.Errorf("expected 2 callback invocations, got %d", callCount)
	}
}

// TestStartTunnel_NotFound verifies error when tunnel doesn't exist in store
func TestStartTunnel_NotFound(t *testing.T) {
	m, _, _ := newTestTunnelManager()

	err := m.StartTunnel("nonexistent-id")
	if err == nil {
		t.Fatal("expected error for non-existent tunnel, got nil")
	}
	if err.Error() != "tunnel not found" {
		t.Errorf("error message = %q, want %q", err.Error(), "tunnel not found")
	}
}

// TestStartTunnel_AlreadyRunning verifies error when tunnel is already running
func TestStartTunnel_AlreadyRunning(t *testing.T) {
	m, store, _ := newTestTunnelManager()

	tunnel := &models.Tunnel{
		ID:           "tunnel-1",
		ConnectionID: "conn-1",
		LocalPort:    18082,
		TargetHost:   "localhost",
		TargetPort:   8080,
	}
	store.tunnels[tunnel.ID] = tunnel

	// Manually insert an active tunnel
	ctx, cancel := context.WithCancel(context.Background())
	m.mu.Lock()
	m.tunnels[tunnel.ID] = &activeTunnel{
		tunnel: tunnel,
		status: models.StatusConnected,
		ctx:    ctx,
		cancel: cancel,
	}
	m.mu.Unlock()

	err := m.StartTunnel(tunnel.ID)
	if err == nil {
		t.Fatal("expected error for already running tunnel, got nil")
	}
	if err.Error() != "tunnel already running" {
		t.Errorf("error message = %q, want %q", err.Error(), "tunnel already running")
	}

	// Clean up
	cancel()
}

// TestStartTunnel_ConnectionNotFound verifies error when associated connection is missing
func TestStartTunnel_ConnectionNotFound(t *testing.T) {
	m, store, _ := newTestTunnelManager()

	tunnel := &models.Tunnel{
		ID:           "tunnel-1",
		ConnectionID: "missing-conn",
		LocalPort:    18083,
		TargetHost:   "localhost",
		TargetPort:   8080,
	}
	store.tunnels[tunnel.ID] = tunnel

	err := m.StartTunnel(tunnel.ID)
	if err == nil {
		t.Fatal("expected error for missing connection, got nil")
	}
}

// TestStopTunnel_NotRunning verifies stopping a non-existent tunnel returns nil
func TestStopTunnel_NotRunning(t *testing.T) {
	m, _, _ := newTestTunnelManager()

	err := m.StopTunnel("nonexistent-id")
	if err != nil {
		t.Errorf("expected nil for stopping non-existent tunnel, got %v", err)
	}
}

// TestGetStatus_NotRunning verifies status is Disconnected for unknown tunnel
func TestGetStatus_NotRunning(t *testing.T) {
	m, _, _ := newTestTunnelManager()

	status := m.GetStatus("nonexistent-id")
	if status != models.StatusDisconnected {
		t.Errorf("GetStatus for non-existent tunnel = %v, want %v", status, models.StatusDisconnected)
	}
}

// TestGetStatus_Running verifies status reflects active tunnel state
func TestGetStatus_Running(t *testing.T) {
	m, _, _ := newTestTunnelManager()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	tunnel := &models.Tunnel{ID: "tunnel-1"}
	m.mu.Lock()
	m.tunnels["tunnel-1"] = &activeTunnel{
		tunnel: tunnel,
		status: models.StatusConnected,
		ctx:    ctx,
		cancel: cancel,
	}
	m.mu.Unlock()

	status := m.GetStatus("tunnel-1")
	if status != models.StatusConnected {
		t.Errorf("GetStatus = %v, want %v", status, models.StatusConnected)
	}
}

// TestGetError_NotRunning verifies error is nil for unknown tunnel
func TestGetError_NotRunning(t *testing.T) {
	m, _, _ := newTestTunnelManager()

	err := m.GetError("nonexistent-id")
	if err != nil {
		t.Errorf("GetError for non-existent tunnel = %v, want nil", err)
	}
}

// TestGetError_Running verifies error reflects active tunnel state
func TestGetError_Running(t *testing.T) {
	m, _, _ := newTestTunnelManager()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	expectedErr := fmt.Errorf("something went wrong")
	tunnel := &models.Tunnel{ID: "tunnel-1"}
	m.mu.Lock()
	m.tunnels["tunnel-1"] = &activeTunnel{
		tunnel: tunnel,
		status: models.StatusError,
		err:    expectedErr,
		ctx:    ctx,
		cancel: cancel,
	}
	m.mu.Unlock()

	err := m.GetError("tunnel-1")
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if err.Error() != expectedErr.Error() {
		t.Errorf("GetError = %v, want %v", err, expectedErr)
	}
}

// TestIsRunning_True verifies IsRunning returns true for an active tunnel
func TestIsRunning_True(t *testing.T) {
	m, _, _ := newTestTunnelManager()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	m.mu.Lock()
	m.tunnels["tunnel-1"] = &activeTunnel{
		tunnel: &models.Tunnel{ID: "tunnel-1"},
		ctx:    ctx,
		cancel: cancel,
	}
	m.mu.Unlock()

	if !m.IsRunning("tunnel-1") {
		t.Error("IsRunning should return true for active tunnel")
	}
}

// TestIsRunning_False verifies IsRunning returns false for unknown tunnel
func TestIsRunning_False(t *testing.T) {
	m, _, _ := newTestTunnelManager()

	if m.IsRunning("nonexistent-id") {
		t.Error("IsRunning should return false for non-existent tunnel")
	}
}

// TestStopAll verifies all tunnels are stopped
func TestStopAll(t *testing.T) {
	m, store, _ := newTestTunnelManager()

	// Set up connections so StopTunnel can look them up
	store.connections["conn-1"] = &models.SSHConnection{ID: "conn-1", Host: "example.com", Port: 22}
	store.connections["conn-2"] = &models.SSHConnection{ID: "conn-2", Host: "example2.com", Port: 22}

	// Create two active tunnels
	for i := 1; i <= 2; i++ {
		id := fmt.Sprintf("tunnel-%d", i)
		connID := fmt.Sprintf("conn-%d", i)
		ctx, cancel := context.WithCancel(context.Background())
		m.mu.Lock()
		m.tunnels[id] = &activeTunnel{
			tunnel: &models.Tunnel{
				ID:           id,
				ConnectionID: connID,
			},
			status: models.StatusConnected,
			ctx:    ctx,
			cancel: cancel,
		}
		m.mu.Unlock()
	}

	if len(m.tunnels) != 2 {
		t.Fatalf("expected 2 tunnels, got %d", len(m.tunnels))
	}

	m.StopAll()

	// Use a short wait to ensure all goroutines complete
	time.Sleep(100 * time.Millisecond)

	m.mu.RLock()
	defer m.mu.RUnlock()
	if len(m.tunnels) != 0 {
		t.Errorf("expected 0 tunnels after StopAll, got %d", len(m.tunnels))
	}
	for id := range m.tunnels {
		if m.IsRunning(id) {
			t.Errorf("tunnel %s should not be running after StopAll", id)
		}
	}
}

// TestStopAll_Empty verifies StopAll on empty manager is a no-op
func TestStopAll_Empty(t *testing.T) {
	m, _, _ := newTestTunnelManager()

	// Should not panic or error
	m.StopAll()

	if len(m.tunnels) != 0 {
		t.Errorf("expected 0 tunnels, got %d", len(m.tunnels))
	}
}

// TestStartTunnel_PortInUse verifies error when local port is already bound
func TestStartTunnel_PortInUse(t *testing.T) {
	m, store, _ := newTestTunnelManager()

	connID := "conn-1"
	store.connections[connID] = &models.SSHConnection{ID: connID, Host: "example.com", Port: 22}

	// Bind a port first so it's in use
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("failed to bind test port: %v", err)
	}
	defer listener.Close()

	// Extract the actual port from the listener
	addr := listener.Addr().(*net.TCPAddr)
	port := addr.Port

	tunnel := &models.Tunnel{
		ID:           "tunnel-port-in-use",
		ConnectionID: connID,
		LocalPort:    port,
		TargetHost:   "localhost",
		TargetPort:   8080,
	}
	store.tunnels[tunnel.ID] = tunnel

	err = m.StartTunnel(tunnel.ID)
	if err == nil {
		t.Fatal("expected error when port is in use, got nil")
	}
	if !containsString(err.Error(), "already in use") {
		t.Errorf("error should mention port in use, got: %v", err)
	}
}

// containsString checks if substr is in s (case-sensitive)
func containsString(s, substr string) bool {
	return len(s) >= len(substr) && searchString(s, substr)
}

func searchString(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}

// TestTunnelState verifies TunnelState struct fields
func TestTunnelState(t *testing.T) {
	ts := TunnelState{
		Tunnel:    &models.Tunnel{ID: "t-1"},
		Status:    models.StatusConnected,
		Error:     nil,
		StartedAt: time.Now(),
	}
	if ts.Tunnel.ID != "t-1" {
		t.Errorf("TunnelState.Tunnel.ID = %q, want %q", ts.Tunnel.ID, "t-1")
	}
	if ts.Status != models.StatusConnected {
		t.Errorf("TunnelState.Status = %v, want %v", ts.Status, models.StatusConnected)
	}
}

// TestNotifyStatus verifies notifyStatus calls all registered callbacks
func TestNotifyStatus(t *testing.T) {
	m := &TunnelManager{
		tunnels:   make(map[string]*activeTunnel),
		callbacks: []StatusCallback{},
	}

	var calls []string
	m.AddStatusCallback(func(tunnelID string, status models.TunnelStatus, err error) {
		calls = append(calls, tunnelID+":"+status.String())
	})

	m.notifyStatus("t-1", models.StatusConnected, nil)
	if len(calls) != 1 {
		t.Fatalf("expected 1 callback, got %d", len(calls))
	}
	if calls[0] != "t-1:Connected" {
		t.Errorf("callback = %q, want %q", calls[0], "t-1:Connected")
	}
}

// TestNotifyStatus_WithError verifies error is passed to callback
func TestNotifyStatus_WithError(t *testing.T) {
	m := &TunnelManager{
		tunnels:   make(map[string]*activeTunnel),
		callbacks: []StatusCallback{},
	}

	var receivedErr error
	m.AddStatusCallback(func(tunnelID string, status models.TunnelStatus, err error) {
		receivedErr = err
	})

	testErr := fmt.Errorf("test error")
	m.notifyStatus("t-2", models.StatusError, testErr)
	if receivedErr == nil {
		t.Fatal("expected error in callback")
	}
	if receivedErr.Error() != "test error" {
		t.Errorf("error = %q, want %q", receivedErr.Error(), "test error")
	}
}

// TestStartTunnel_ConnectionNotFound verifies error when connection missing
func TestStartTunnel_ConnectionNotFound2(t *testing.T) {
	m, store, _ := newTestTunnelManager()

	tunnel := &models.Tunnel{
		ID:           "t-1",
		ConnectionID: "missing",
		LocalPort:    18084,
		TargetHost:   "localhost",
		TargetPort:   8080,
	}
	store.tunnels[tunnel.ID] = tunnel

	err := m.StartTunnel(tunnel.ID)
	if err == nil {
		t.Fatal("expected error for missing connection")
	}
}

// TestDefaultSleep tests the defaultSleep function with context cancellation
func TestDefaultSleep(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	go func() {
		time.Sleep(10 * time.Millisecond)
		cancel()
	}()

	start := time.Now()
	defaultSleep(ctx, 10*time.Second)
	elapsed := time.Since(start)

	if elapsed > 1*time.Second {
		t.Errorf("defaultSleep took too long: %v", elapsed)
	}
}

// TestDefaultSleep_Expired tests that defaultSleep returns when duration expires
func TestDefaultSleep_Expired(t *testing.T) {
	ctx := context.Background()

	start := time.Now()
	defaultSleep(ctx, 20*time.Millisecond)
	elapsed := time.Since(start)

	if elapsed < 15*time.Millisecond {
		t.Errorf("defaultSleep returned too quickly: %v", elapsed)
	}
}

// TestCalcReconnectDuration_ZeroInterval tests default interval when zero
func TestCalcReconnectDuration_ZeroInterval(t *testing.T) {
	m := &TunnelManager{
		tunnels: make(map[string]*activeTunnel),
	}

	at, cancel := newTestActiveTunnel(0) // zero interval, should default to 10
	defer cancel()

	d := m.calcReconnectDuration(at)
	if d < 5*time.Second || d > 15*time.Second {
		t.Errorf("calcReconnectDuration with zero interval = %v, want ~10s", d)
	}
}

// TestReconnectWait_IncrementsAttempts verifies reconnectWait increments attempts
func TestReconnectWait_IncrementsAttempts(t *testing.T) {
	m := &TunnelManager{
		tunnels: make(map[string]*activeTunnel),
	}

	sleepCalled := make(chan time.Duration, 1)
	m.sleep = func(ctx context.Context, d time.Duration) {
		sleepCalled <- d
		<-ctx.Done()
	}

	at, cancel := newTestActiveTunnel(10)
	defer cancel()

	at.reconnectAttempts = 0
	go m.reconnectWait(at)

	select {
	case d := <-sleepCalled:
		if d < 5*time.Second || d > 15*time.Second {
			t.Errorf("reconnectWait sleep duration = %v, want ~10s", d)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("sleep was not called")
	}

	cancel()
}

func TestAcceptConnectionsOrWaitDisconnect_SSHDisconnect(t *testing.T) {
	disconnectCh := make(chan struct{})
	listener := newFakeListener()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	m := &TunnelManager{
		tunnels:           make(map[string]*activeTunnel),
		isClientConnected: func(*Client) bool { return true },
		waitForDisconnect: func(_ *Client, _ context.Context) { <-disconnectCh },
	}

	at := &activeTunnel{
		tunnel:   &models.Tunnel{ID: "tunnel-ssh-disconnect"},
		client:   &Client{},
		listener: listener,
		ctx:      ctx,
		cancel:   cancel,
	}
	m.tunnels[at.tunnel.ID] = at

	reasonCh := make(chan tunnelStopReason, 1)
	go func() {
		reasonCh <- m.acceptConnectionsOrWaitDisconnect(at)
	}()

	close(disconnectCh)

	select {
	case reason := <-reasonCh:
		if reason != stopSSHDisconnected {
			t.Fatalf("reason = %v, want %v", reason, stopSSHDisconnected)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for disconnect to be detected")
	}

	// By design, acceptConnectionsOrWaitDisconnect does NOT remove the tunnel from
	// the manager map — removal is runTunnel's responsibility based on the stop reason.
	if !m.IsRunning(at.tunnel.ID) {
		t.Fatal("tunnel should still be in the manager map; removal is runTunnel's job")
	}

	_ = listener.Close()
	at.wg.Wait()
}

func TestHandleConnection_DialErrorStopsTunnel(t *testing.T) {
	m, _, _ := newTestTunnelManager()
	m.dialRemote = func(_ *Client, _, _ string) (net.Conn, error) {
		return nil, fmt.Errorf("broken pipe")
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	at := &activeTunnel{
		tunnel: &models.Tunnel{ID: "tunnel-handle-error"},
		client: &Client{},
		ctx:    ctx,
		cancel: cancel,
	}

	m.mu.Lock()
	m.tunnels[at.tunnel.ID] = at
	m.mu.Unlock()

	localConn := newFakeConn("local-error")
	at.wg.Add(1)
	go m.handleConnection(at, localConn)

	waitForCondition(t, 2*time.Second, func() bool {
		return !m.IsRunning(at.tunnel.ID)
	})

	if !localConn.isClosed() {
		t.Fatal("local connection should be closed when dialing the target fails")
	}
}

func TestHandleConnection_SuccessCopiesBothDirections(t *testing.T) {
	m, _, _ := newTestTunnelManager()

	remoteConn := newFakeConn("remote-success")
	m.dialRemote = func(_ *Client, _, _ string) (net.Conn, error) {
		return remoteConn, nil
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	at := &activeTunnel{
		tunnel: &models.Tunnel{
			ID:         "tunnel-handle-success",
			TargetHost: "target.internal",
			TargetPort: 8080,
		},
		client: &Client{},
		ctx:    ctx,
		cancel: cancel,
	}

	// fakeConn.Read returns (0, io.EOF) immediately, so both copy goroutines
	// exit right away and handleConnection should return without blocking.
	localConn := newFakeConn("local-success")
	at.wg.Add(1)
	m.handleConnection(at, localConn)

	if !localConn.isClosed() {
		t.Fatal("local connection should be closed after forwarding completes")
	}
	if !remoteConn.isClosed() {
		t.Fatal("remote connection should be closed after forwarding completes")
	}
}

func TestRunTunnel_ListenErrorAutoReconnectUsesInjectedDeps(t *testing.T) {
	m, _, connGetter := newTestTunnelManager()
	connGetter.client = &Client{}
	m.canListen = func(_ int, _ bool) error { return nil }
	m.listen = func(_, _ string) (net.Listener, error) {
		return nil, errors.New("listen failed")
	}

	sleepCalls := 0
	ctx, cancel := context.WithCancel(context.Background())
	m.sleep = func(_ context.Context, _ time.Duration) {
		sleepCalls++
		cancel()
	}

	conn := &models.SSHConnection{ID: "conn-listen-error"}
	at := &activeTunnel{
		tunnel: &models.Tunnel{
			ID:                "tunnel-listen-error",
			AutoReconnect:     true,
			ReconnectInterval: 1,
		},
		ctx:    ctx,
		cancel: cancel,
	}

	var statusesMu sync.Mutex
	var statuses []models.TunnelStatus
	m.AddStatusCallback(func(_ string, status models.TunnelStatus, _ error) {
		statusesMu.Lock()
		statuses = append(statuses, status)
		statusesMu.Unlock()
	})

	at.wg.Add(1)
	go m.runTunnel(at, conn)
	at.wg.Wait()

	if sleepCalls != 1 {
		t.Fatalf("sleep called %d times, want 1", sleepCalls)
	}
	if connGetter.releaseCount != 1 {
		t.Fatalf("ReleaseClient called %d times, want 1", connGetter.releaseCount)
	}
	statusesMu.Lock()
	defer statusesMu.Unlock()
	if len(statuses) < 3 {
		t.Fatalf("expected connecting/error/reconnecting statuses, got %v", statuses)
	}
	if statuses[0] != models.StatusConnecting || statuses[1] != models.StatusError || statuses[2] != models.StatusReconnecting {
		t.Fatalf("unexpected status sequence: %v", statuses)
	}
}

func TestRunTunnel_UserCancelReleasesClientOnce(t *testing.T) {
	m, _, connGetter := newTestTunnelManager()
	connGetter.client = &Client{}
	listener := newFakeListener()

	m.canListen = func(_ int, _ bool) error { return nil }
	m.listen = func(_, _ string) (net.Listener, error) {
		return listener, nil
	}
	m.isClientConnected = func(*Client) bool { return false }
	m.waitForDisconnect = func(*Client, context.Context) {}

	ctx, cancel := context.WithCancel(context.Background())
	at := &activeTunnel{
		tunnel: &models.Tunnel{
			ID:         "tunnel-user-cancel",
			LocalPort:  19091,
			TargetHost: "target.internal",
			TargetPort: 8080,
		},
		ctx:    ctx,
		cancel: cancel,
	}
	conn := &models.SSHConnection{ID: "conn-user-cancel"}

	var statusesMu sync.Mutex
	var statuses []models.TunnelStatus
	m.AddStatusCallback(func(_ string, status models.TunnelStatus, _ error) {
		statusesMu.Lock()
		statuses = append(statuses, status)
		statusesMu.Unlock()
	})

	at.wg.Add(1)
	go m.runTunnel(at, conn)

	waitForCondition(t, 2*time.Second, func() bool {
		statusesMu.Lock()
		defer statusesMu.Unlock()
		return len(statuses) >= 2
	})

	cancel()
	at.wg.Wait()

	if connGetter.releaseCount != 1 {
		t.Fatalf("ReleaseClient called %d times, want 1", connGetter.releaseCount)
	}
	statusesMu.Lock()
	defer statusesMu.Unlock()
	if statuses[0] != models.StatusConnecting || statuses[1] != models.StatusConnected {
		t.Fatalf("unexpected status sequence: %v", statuses)
	}
}

func TestStopTunnel_ReleasesClientExactlyOnce(t *testing.T) {
	m, _, connGetter := newTestTunnelManager()
	ctx, cancel := context.WithCancel(context.Background())
	at := &activeTunnel{
		tunnel:   &models.Tunnel{ID: "tunnel-release-once"},
		listener: newFakeListener(),
		client:   &Client{},
		ctx:      ctx,
		cancel:   cancel,
	}

	m.mu.Lock()
	m.tunnels[at.tunnel.ID] = at
	m.mu.Unlock()

	if err := m.StopTunnel(at.tunnel.ID); err != nil {
		t.Fatalf("StopTunnel: %v", err)
	}

	connGetter.mu.Lock()
	defer connGetter.mu.Unlock()
	if connGetter.releaseCount != 0 {
		t.Fatalf("StopTunnel should not release clients directly, got %d calls", connGetter.releaseCount)
	}
}
