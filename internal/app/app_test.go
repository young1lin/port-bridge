package app

import (
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/young1lin/port-bridge/internal/models"
	"github.com/young1lin/port-bridge/internal/ssh"
	"github.com/young1lin/port-bridge/internal/storage"
)

// newTestClientManager creates a ClientManager for testing
func newTestClientManager() *ClientManager {
	store := &storage.Store{}
	return &ClientManager{
		clients:  make(map[string]*ssh.Client),
		refCount: make(map[string]int),
		pending:  make(map[string]*pendingConnection),
		store:    store,
		connectFunc: func(conn *models.SSHConnection) (*ssh.Client, error) {
			return ssh.NewClient(conn), nil
		},
		isConnected: func(client *ssh.Client) bool {
			return client != nil && client.IsConnected()
		},
		disconnect: func(client *ssh.Client) error {
			if client == nil {
				return nil
			}
			return client.Disconnect()
		},
		wait: func(client *ssh.Client) {
			if client != nil {
				client.Wait()
			}
		},
	}
}

func isolateAppData(t *testing.T) {
	t.Helper()
	t.Setenv("APPDATA", t.TempDir())
}

func TestClientManager_ReleaseClient_NotFound(t *testing.T) {
	cm := newTestClientManager()
	cm.ReleaseClient("nonexistent-id")
}

func TestClientManager_DisconnectAll_Empty(t *testing.T) {
	cm := newTestClientManager()
	cm.DisconnectAll()
}

func TestClientManager_ConcurrentReleaseClient(t *testing.T) {
	cm := newTestClientManager()
	connID := "test-conn"

	conn := &models.SSHConnection{
		ID:       connID,
		Host:     "127.0.0.1",
		Port:     22,
		Username: "test",
		AuthType: models.AuthTypePassword,
	}
	client := ssh.NewClient(conn)
	cm.clients[connID] = client
	cm.refCount[connID] = 5

	var wg sync.WaitGroup
	for i := 0; i < 5; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			cm.ReleaseClient(connID)
		}()
	}
	wg.Wait()

	if _, exists := cm.clients[connID]; exists {
		t.Error("Client should be removed after all releases")
	}
}

func TestClientManager_IsConnected_Empty(t *testing.T) {
	cm := newTestClientManager()
	if cm.IsConnected("nonexistent") {
		t.Error("IsConnected should return false for non-existent client")
	}
}

// ---------------------------------------------------------------------------
// New tests for connectFunc injection, error paths, reference counting,
// single-flight, and initial state verification.
// ---------------------------------------------------------------------------

func TestNewClientManager(t *testing.T) {
	store := &storage.Store{}
	cm := NewClientManager(store)

	if cm == nil {
		t.Fatal("NewClientManager returned nil")
	}
	if cm.clients == nil {
		t.Error("clients map should be initialized")
	}
	if cm.refCount == nil {
		t.Error("refCount map should be initialized")
	}
	if cm.pending == nil {
		t.Error("pending map should be initialized")
	}
	if cm.connectFunc == nil {
		t.Error("connectFunc should be initialized")
	}
	if cm.isConnected == nil {
		t.Error("isConnected should be initialized")
	}
	if cm.disconnect == nil {
		t.Error("disconnect should be initialized")
	}
	if cm.wait == nil {
		t.Error("wait should be initialized")
	}
	if cm.store != store {
		t.Error("store should be the one passed to NewClientManager")
	}
	if len(cm.clients) != 0 {
		t.Errorf("clients map should be empty, got %d entries", len(cm.clients))
	}
	if len(cm.refCount) != 0 {
		t.Errorf("refCount map should be empty, got %d entries", len(cm.refCount))
	}
	if len(cm.pending) != 0 {
		t.Errorf("pending map should be empty, got %d entries", len(cm.pending))
	}
}

func TestNewClientManager_DefaultClosures(t *testing.T) {
	origNewSSHClient := newSSHClient
	origConnectSSHClient := connectSSHClient
	t.Cleanup(func() {
		newSSHClient = origNewSSHClient
		connectSSHClient = origConnectSSHClient
	})

	var createdWith *models.SSHConnection
	var connectedWith *ssh.Client

	newSSHClient = func(conn *models.SSHConnection) *ssh.Client {
		createdWith = conn
		return ssh.NewClient(conn)
	}
	connectSSHClient = func(client *ssh.Client) error {
		connectedWith = client
		return nil
	}

	cm := NewClientManager(nil)
	conn := &models.SSHConnection{ID: "defaults"}
	client, err := cm.connectFunc(conn)
	if err != nil {
		t.Fatalf("connectFunc: %v", err)
	}
	if createdWith != conn {
		t.Fatal("expected default connect closure to use newSSHClient")
	}
	if connectedWith != client {
		t.Fatal("expected default connect closure to call connectSSHClient")
	}
	if cm.isConnected(nil) {
		t.Fatal("isConnected(nil) should be false")
	}
	if err := cm.disconnect(nil); err != nil {
		t.Fatalf("disconnect(nil): %v", err)
	}
	cm.wait(nil)
}

func TestNewClientManager_DefaultClosuresWithClient(t *testing.T) {
	cm := NewClientManager(nil)
	client := ssh.NewClient(&models.SSHConnection{ID: "client", Host: "127.0.0.1", Port: 22})

	if cm.isConnected(client) {
		t.Fatal("expected fresh client to report disconnected")
	}
	if err := cm.disconnect(client); err != nil {
		t.Fatalf("disconnect(client): %v", err)
	}
	cm.wait(client)
}

func TestGetOrCreateClient_ConnectFailed(t *testing.T) {
	cm := NewClientManager(nil)
	cm.connectFunc = func(conn *models.SSHConnection) (*ssh.Client, error) {
		return nil, fmt.Errorf("connection refused")
	}

	_, err := cm.GetOrCreateClient(&models.SSHConnection{ID: "test-fail"})
	if err == nil {
		t.Fatal("expected error from connectFunc, got nil")
	}
	if err.Error() != "connection refused" {
		t.Errorf("expected 'connection refused', got '%s'", err.Error())
	}

	// Verify client was not stored
	cm.mu.Lock()
	if _, exists := cm.clients["test-fail"]; exists {
		t.Error("failed connection should not be stored in clients map")
	}
	if _, exists := cm.pending["test-fail"]; exists {
		t.Error("pending entry should be cleaned up after failure")
	}
	cm.mu.Unlock()
}

func TestGetOrCreateClient_ConnectSuccess(t *testing.T) {
	cm := NewClientManager(nil)
	var connectCalled bool
	cm.connectFunc = func(conn *models.SSHConnection) (*ssh.Client, error) {
		connectCalled = true
		if conn.ID != "test-success" {
			t.Errorf("connectFunc received wrong conn ID: %s", conn.ID)
		}
		// Return a client that has never been connected (IsConnected will be false)
		return ssh.NewClient(conn), nil
	}

	client, err := cm.GetOrCreateClient(&models.SSHConnection{ID: "test-success"})
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}
	if client == nil {
		t.Fatal("expected non-nil client")
	}
	if !connectCalled {
		t.Error("connectFunc should have been called")
	}

	// Verify refCount is 1
	cm.mu.Lock()
	if cm.refCount["test-success"] != 1 {
		t.Errorf("expected refCount 1, got %d", cm.refCount["test-success"])
	}
	if _, exists := cm.clients["test-success"]; !exists {
		t.Error("client should be stored in clients map")
	}
	cm.mu.Unlock()
}

func TestGetOrCreateClient_PendingError(t *testing.T) {
	cm := NewClientManager(nil)
	conn := &models.SSHConnection{ID: "pending-error"}
	p := &pendingConnection{
		err:  fmt.Errorf("pending failed"),
		done: make(chan struct{}),
	}
	close(p.done)

	cm.mu.Lock()
	cm.pending[conn.ID] = p
	cm.mu.Unlock()

	_, err := cm.GetOrCreateClient(conn)
	if err == nil || err.Error() != "pending failed" {
		t.Fatalf("expected pending error, got %v", err)
	}
}

func TestGetOrCreateClient_PendingClientMissingAfterSuccess(t *testing.T) {
	cm := NewClientManager(nil)
	conn := &models.SSHConnection{ID: "pending-missing"}
	p := &pendingConnection{done: make(chan struct{})}
	close(p.done)

	cm.mu.Lock()
	cm.pending[conn.ID] = p
	cm.mu.Unlock()

	_, err := cm.GetOrCreateClient(conn)
	if err == nil || err.Error() != "connection completed but client not available" {
		t.Fatalf("expected missing client error, got %v", err)
	}
}

func TestGetOrCreateClient_ReconnectsDisconnectedClient(t *testing.T) {
	cm := NewClientManager(nil)
	conn := &models.SSHConnection{ID: "reconnect"}
	staleClient := ssh.NewClient(conn)
	newClient := ssh.NewClient(conn)

	cm.mu.Lock()
	cm.clients[conn.ID] = staleClient
	cm.refCount[conn.ID] = 7
	cm.mu.Unlock()

	connectCalls := 0
	cm.connectFunc = func(got *models.SSHConnection) (*ssh.Client, error) {
		connectCalls++
		if got != conn {
			t.Fatal("expected reconnect to use original connection")
		}
		return newClient, nil
	}
	cm.isConnected = func(client *ssh.Client) bool {
		return client == newClient
	}

	client, err := cm.GetOrCreateClient(conn)
	if err != nil {
		t.Fatalf("GetOrCreateClient: %v", err)
	}
	if client != newClient {
		t.Fatal("expected disconnected client to be replaced")
	}
	if connectCalls != 1 {
		t.Fatalf("expected reconnect attempt once, got %d", connectCalls)
	}

	cm.mu.Lock()
	defer cm.mu.Unlock()
	if cm.refCount[conn.ID] != 1 {
		t.Fatalf("expected refCount reset to 1 after reconnect, got %d", cm.refCount[conn.ID])
	}
}

func TestGetOrCreateClient_AlreadyConnected(t *testing.T) {
	cm := NewClientManager(nil)
	var connectCount int
	connectedClients := map[*ssh.Client]bool{}
	cm.connectFunc = func(conn *models.SSHConnection) (*ssh.Client, error) {
		connectCount++
		client := ssh.NewClient(conn)
		connectedClients[client] = true
		return client, nil
	}
	cm.isConnected = func(client *ssh.Client) bool {
		return connectedClients[client]
	}

	conn := &models.SSHConnection{ID: "test-reuse"}

	first, err := cm.GetOrCreateClient(conn)
	if err != nil {
		t.Fatalf("first call failed: %v", err)
	}

	second, err := cm.GetOrCreateClient(conn)
	if err != nil {
		t.Fatalf("second call failed: %v", err)
	}
	if first != second {
		t.Fatal("expected second call to reuse the same client instance")
	}

	cm.mu.Lock()
	defer cm.mu.Unlock()

	if connectCount != 1 {
		t.Errorf("expected connectFunc called 1 time, got %d", connectCount)
	}
	if cm.refCount["test-reuse"] != 2 {
		t.Errorf("expected refCount 2 after reuse, got %d", cm.refCount["test-reuse"])
	}
}

func TestGetOrCreateClient_SingleFlight(t *testing.T) {
	cm := NewClientManager(nil)
	var connectCount int64
	var connectBlock chan struct{}
	connectedClients := map[*ssh.Client]bool{}
	var connectedMu sync.Mutex

	cm.connectFunc = func(conn *models.SSHConnection) (*ssh.Client, error) {
		atomic.AddInt64(&connectCount, 1)

		// Block until signaled, simulating a slow connection
		if connectBlock != nil {
			<-connectBlock
		}

		client := ssh.NewClient(conn)
		connectedMu.Lock()
		connectedClients[client] = true
		connectedMu.Unlock()
		return client, nil
	}
	cm.isConnected = func(client *ssh.Client) bool {
		connectedMu.Lock()
		defer connectedMu.Unlock()
		return connectedClients[client]
	}

	conn := &models.SSHConnection{ID: "test-singleflight"}
	connectBlock = make(chan struct{})

	var wg sync.WaitGroup
	errors := make(chan error, 10)
	clients := make(chan *ssh.Client, 10)

	// Launch 5 concurrent callers
	for i := 0; i < 5; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			client, err := cm.GetOrCreateClient(conn)
			if err != nil {
				errors <- err
				return
			}
			clients <- client
		}()
	}

	// Wait a moment to let all goroutines enter the pending wait
	time.Sleep(50 * time.Millisecond)

	// Unblock the connection
	close(connectBlock)
	wg.Wait()
	close(errors)
	close(clients)

	// The key assertion: only ONE actual connection attempt was made.
	// This proves the single-flight pattern is working correctly --
	// the other 4 callers waited on the pending connection instead
	// of starting their own.
	if n := atomic.LoadInt64(&connectCount); n != 1 {
		t.Errorf("expected connectFunc called exactly 1 time (single-flight), got %d", n)
	}
	for err := range errors {
		t.Errorf("unexpected single-flight error: %v", err)
	}
	var first *ssh.Client
	clientCount := 0
	for client := range clients {
		clientCount++
		if first == nil {
			first = client
			continue
		}
		if client != first {
			t.Fatal("expected all waiters to receive the same client instance")
		}
	}
	if clientCount != 5 {
		t.Fatalf("expected 5 callers to receive a client, got %d", clientCount)
	}
}

func TestReleaseClient_LastRef_Disconnects(t *testing.T) {
	cm := newTestClientManager()
	connID := "test-disconnect"
	var disconnectCount int
	var waitCount int

	conn := &models.SSHConnection{ID: connID}
	client := ssh.NewClient(conn)
	cm.clients[connID] = client
	cm.refCount[connID] = 1
	cm.disconnect = func(got *ssh.Client) error {
		if got != client {
			t.Fatal("disconnect called with unexpected client")
		}
		disconnectCount++
		return nil
	}
	cm.wait = func(got *ssh.Client) {
		if got != client {
			t.Fatal("wait called with unexpected client")
		}
		waitCount++
	}

	// Verify client exists before release
	if _, exists := cm.clients[connID]; !exists {
		t.Fatal("client should exist before release")
	}

	cm.ReleaseClient(connID)

	// After releasing the last reference, client should be removed from the map
	cm.mu.Lock()
	defer cm.mu.Unlock()
	if _, exists := cm.clients[connID]; exists {
		t.Error("client should be removed from map after last reference released")
	}
	if _, exists := cm.refCount[connID]; exists {
		t.Error("refCount entry should be removed after last reference released")
	}
	if disconnectCount != 1 {
		t.Errorf("expected disconnect called once, got %d", disconnectCount)
	}
	if waitCount != 1 {
		t.Errorf("expected wait called once, got %d", waitCount)
	}
}

func TestReleaseClient_MultipleRefs(t *testing.T) {
	cm := newTestClientManager()
	connID := "test-multiref"

	conn := &models.SSHConnection{ID: connID}
	client := ssh.NewClient(conn)
	cm.clients[connID] = client
	cm.refCount[connID] = 3

	// Release twice -- should still have one reference left
	cm.ReleaseClient(connID)
	cm.ReleaseClient(connID)

	cm.mu.Lock()
	defer cm.mu.Unlock()
	if cm.refCount[connID] != 1 {
		t.Errorf("expected refCount 1 after two releases, got %d", cm.refCount[connID])
	}
	if _, exists := cm.clients[connID]; !exists {
		t.Error("client should still exist with remaining references")
	}
}

func TestIsConnected_True(t *testing.T) {
	cm := newTestClientManager()
	connID := "test-conn-true"

	conn := &models.SSHConnection{ID: connID}
	client := ssh.NewClient(conn)
	cm.clients[connID] = client
	cm.isConnected = func(got *ssh.Client) bool {
		return got == client
	}

	if !cm.IsConnected(connID) {
		t.Error("IsConnected should return true for injected connected client")
	}
}

func TestIsConnected_False(t *testing.T) {
	cm := newTestClientManager()

	// Non-existent client
	if cm.IsConnected("nonexistent") {
		t.Error("IsConnected should return false for non-existent client")
	}

	// Client exists but is not connected
	connID := "test-conn-false"
	conn := &models.SSHConnection{ID: connID}
	client := ssh.NewClient(conn)
	cm.clients[connID] = client

	if cm.IsConnected(connID) {
		t.Error("IsConnected should return false for client that was never connected")
	}
}

func TestGetOrCreateClient_ConcurrentDifferentConnections(t *testing.T) {
	cm := NewClientManager(nil)
	var connectCount int64
	cm.connectFunc = func(conn *models.SSHConnection) (*ssh.Client, error) {
		atomic.AddInt64(&connectCount, 1)
		return ssh.NewClient(conn), nil
	}

	var wg sync.WaitGroup
	for i := 0; i < 5; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			conn := &models.SSHConnection{ID: fmt.Sprintf("conn-%d", id)}
			_, err := cm.GetOrCreateClient(conn)
			if err != nil {
				t.Errorf("GetOrCreateClient failed for conn-%d: %v", id, err)
			}
		}(i)
	}
	wg.Wait()

	if n := atomic.LoadInt64(&connectCount); n != 5 {
		t.Errorf("expected 5 connect calls for 5 different connections, got %d", n)
	}
}

func TestClientManager_DisconnectAll_DisconnectsEveryClient(t *testing.T) {
	cm := newTestClientManager()
	var disconnectCount int
	var waitCount int

	for i := 0; i < 3; i++ {
		id := fmt.Sprintf("conn-%d", i)
		cm.clients[id] = ssh.NewClient(&models.SSHConnection{ID: id})
		cm.refCount[id] = i + 1
	}

	cm.disconnect = func(client *ssh.Client) error {
		if client == nil {
			t.Fatal("disconnect received nil client")
		}
		disconnectCount++
		return nil
	}
	cm.wait = func(client *ssh.Client) {
		if client == nil {
			t.Fatal("wait received nil client")
		}
		waitCount++
	}

	cm.DisconnectAll()

	if disconnectCount != 3 {
		t.Fatalf("expected 3 disconnect calls, got %d", disconnectCount)
	}
	if waitCount != 3 {
		t.Fatalf("expected 3 wait calls, got %d", waitCount)
	}
	if len(cm.clients) != 0 {
		t.Fatalf("expected clients map cleared, got %d entries", len(cm.clients))
	}
	if len(cm.refCount) != 0 {
		t.Fatalf("expected refCount map cleared, got %d entries", len(cm.refCount))
	}
}

// ---------------------------------------------------------------------------
// App-level tests (NewApp, getters, Shutdown)
// ---------------------------------------------------------------------------

func TestNewApp(t *testing.T) {
	isolateAppData(t)
	app, err := NewApp()
	if err != nil {
		t.Fatalf("NewApp: %v", err)
	}
	defer app.Shutdown()

	if app == nil {
		t.Fatal("NewApp returned nil")
	}
	if app.GetStore() == nil {
		t.Error("GetStore should not be nil")
	}
	if app.GetTunnelManager() == nil {
		t.Error("GetTunnelManager should not be nil")
	}
	if app.GetClientManager() == nil {
		t.Error("GetClientManager should not be nil")
	}
}

func TestNewApp_StoreError(t *testing.T) {
	origNewStore := newStore
	t.Cleanup(func() { newStore = origNewStore })

	newStore = func() (*storage.Store, error) {
		return nil, fmt.Errorf("store failed")
	}

	app, err := NewApp()
	if err == nil {
		t.Fatal("expected NewApp error")
	}
	if app != nil {
		t.Fatal("expected nil app on store failure")
	}
}

func TestNewApp_UsesInjectedTunnelManager(t *testing.T) {
	origNewStore := newStore
	origNewTunnelManager := newTunnelManager
	t.Cleanup(func() {
		newStore = origNewStore
		newTunnelManager = origNewTunnelManager
	})

	store := &storage.Store{}
	newStore = func() (*storage.Store, error) { return store, nil }

	var gotStore *storage.Store
	var gotManager *ClientManager
	newTunnelManager = func(s ssh.TunnelStore, cm ssh.ConnectionGetter) *ssh.TunnelManager {
		gotStore = s.(*storage.Store)
		gotManager = cm.(*ClientManager)
		return &ssh.TunnelManager{}
	}

	app, err := NewApp()
	if err != nil {
		t.Fatalf("NewApp: %v", err)
	}
	if gotStore != store {
		t.Fatal("expected injected store to reach tunnel manager")
	}
	if gotManager != app.clientManager {
		t.Fatal("expected injected client manager to reach tunnel manager")
	}
}

func TestApp_GetStore(t *testing.T) {
	isolateAppData(t)
	app, err := NewApp()
	if err != nil {
		t.Fatalf("NewApp: %v", err)
	}
	defer app.Shutdown()

	store := app.GetStore()
	if store == nil {
		t.Fatal("GetStore returned nil")
	}
	// Verify it's functional
	conns := store.GetConnections()
	if conns == nil {
		t.Error("GetConnections should return non-nil")
	}
}

func TestApp_GetTunnelManager(t *testing.T) {
	isolateAppData(t)
	app, err := NewApp()
	if err != nil {
		t.Fatalf("NewApp: %v", err)
	}
	defer app.Shutdown()

	tm := app.GetTunnelManager()
	if tm == nil {
		t.Fatal("GetTunnelManager returned nil")
	}
}

func TestApp_GetClientManager(t *testing.T) {
	isolateAppData(t)
	app, err := NewApp()
	if err != nil {
		t.Fatalf("NewApp: %v", err)
	}
	defer app.Shutdown()

	cm := app.GetClientManager()
	if cm == nil {
		t.Fatal("GetClientManager returned nil")
	}
}

func TestApp_Shutdown(t *testing.T) {
	isolateAppData(t)
	app, err := NewApp()
	if err != nil {
		t.Fatalf("NewApp: %v", err)
	}

	// Shutdown should not panic
	app.Shutdown()
}
