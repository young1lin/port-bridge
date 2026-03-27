package app

import (
	"fmt"
	"log"
	"sync"

	"github.com/young1lin/port-bridge/internal/models"
	"github.com/young1lin/port-bridge/internal/ssh"
	"github.com/young1lin/port-bridge/internal/storage"
)

var (
	newSSHClient     = ssh.NewClient
	newStore         = storage.NewStore
	newTunnelManager = func(store ssh.TunnelStore, connGetter ssh.ConnectionGetter) *ssh.TunnelManager {
		return ssh.NewTunnelManager(store, connGetter)
	}
	connectSSHClient = func(client *ssh.Client) error { return client.Connect() }
)

// App is the main application manager
type App struct {
	store         *storage.Store
	tunnelManager *ssh.TunnelManager
	clientManager *ClientManager
	mu            sync.RWMutex
}

// pendingConnection represents an in-flight SSH connection attempt (single-flight pattern)
type pendingConnection struct {
	client *ssh.Client
	err    error
	done   chan struct{} // closed when connection attempt finishes
}

// ClientManager manages SSH client connections with reference counting
type ClientManager struct {
	mu          sync.Mutex
	clients     map[string]*ssh.Client
	refCount    map[string]int // Reference count for each client
	pending     map[string]*pendingConnection
	store       *storage.Store
	connectFunc func(conn *models.SSHConnection) (*ssh.Client, error)
	isConnected func(client *ssh.Client) bool
	disconnect  func(client *ssh.Client) error
	wait        func(client *ssh.Client)
}

// NewClientManager creates a new client manager
func NewClientManager(store *storage.Store) *ClientManager {
	log.Println("[DEBUG] Creating client manager")
	cm := &ClientManager{
		clients:  make(map[string]*ssh.Client),
		refCount: make(map[string]int),
		pending:  make(map[string]*pendingConnection),
		store:    store,
		connectFunc: func(conn *models.SSHConnection) (*ssh.Client, error) {
			client := newSSHClient(conn)
			return client, connectSSHClient(client)
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
	return cm
}

// GetOrCreateClient gets an existing client or creates a new one.
// Uses single-flight pattern: concurrent callers for the same connection share one Connect() attempt.
func (cm *ClientManager) GetOrCreateClient(conn *models.SSHConnection) (*ssh.Client, error) {
	log.Printf("[DEBUG] GetOrCreateClient: connID=%s", conn.ID)

	// Fast path: check for existing connected client
	cm.mu.Lock()
	if client, exists := cm.clients[conn.ID]; exists {
		if cm.isConnected(client) {
			cm.refCount[conn.ID]++
			log.Printf("[DEBUG] Reusing existing connected client: connID=%s (refCount=%d)", conn.ID, cm.refCount[conn.ID])
			cm.mu.Unlock()
			return client, nil
		}
	}

	// Check if a connection is already in progress
	if p, exists := cm.pending[conn.ID]; exists {
		cm.mu.Unlock()
		log.Printf("[DEBUG] Waiting for in-progress connection: connID=%s", conn.ID)
		<-p.done
		if p.err != nil {
			return nil, p.err
		}
		// Connection succeeded, grab it
		cm.mu.Lock()
		if client, exists := cm.clients[conn.ID]; exists && cm.isConnected(client) {
			cm.refCount[conn.ID]++
			log.Printf("[DEBUG] Acquired pending client: connID=%s (refCount=%d)", conn.ID, cm.refCount[conn.ID])
			cm.mu.Unlock()
			return client, nil
		}
		cm.mu.Unlock()
		return nil, fmt.Errorf("connection completed but client not available")
	}

	// No pending connection, start one
	p := &pendingConnection{done: make(chan struct{})}
	cm.pending[conn.ID] = p
	cm.mu.Unlock()

	// Connect outside the lock (network I/O must not block other callers)
	log.Printf("[DEBUG] Creating new SSH client: connID=%s", conn.ID)
	client, err := cm.connectFunc(conn)

	// Record result and notify waiters
	cm.mu.Lock()
	delete(cm.pending, conn.ID)
	if err != nil {
		log.Printf("[ERROR] Failed to connect SSH client: connID=%s, err=%v", conn.ID, err)
		cm.mu.Unlock()
		p.err = err
		close(p.done)
		return nil, err
	}

	cm.clients[conn.ID] = client
	cm.refCount[conn.ID] = 1
	cm.mu.Unlock()
	close(p.done)

	log.Printf("[DEBUG] SSH client created and connected: connID=%s", conn.ID)
	return client, nil
}

// ReleaseClient releases a client connection (decrements ref count, disconnects if last user)
func (cm *ClientManager) ReleaseClient(connID string) {
	cm.mu.Lock()
	client, exists := cm.clients[connID]
	if !exists {
		cm.mu.Unlock()
		log.Printf("[DEBUG] Client not found for release: connID=%s", connID)
		return
	}

	cm.refCount[connID]--
	log.Printf("[DEBUG] Client refCount decremented: connID=%s, refCount=%d", connID, cm.refCount[connID])

	if cm.refCount[connID] <= 0 {
		// Last user, remove from map first
		delete(cm.clients, connID)
		delete(cm.refCount, connID)
		cm.mu.Unlock()
		// Disconnect outside the lock
		log.Printf("[DEBUG] Last user, disconnecting client: connID=%s", connID)
		_ = cm.disconnect(client)
		cm.wait(client)
		log.Printf("[DEBUG] Client released and disconnected: connID=%s", connID)
		return
	}

	cm.mu.Unlock()
	log.Printf("[DEBUG] Client still in use by %d tunnels: connID=%s", cm.refCount[connID], connID)
}

// IsConnected checks if a connection is currently active
func (cm *ClientManager) IsConnected(connID string) bool {
	cm.mu.Lock()
	defer cm.mu.Unlock()

	if client, exists := cm.clients[connID]; exists {
		return cm.isConnected(client)
	}
	return false
}

// DisconnectAll disconnects all clients
func (cm *ClientManager) DisconnectAll() {
	cm.mu.Lock()
	clients := make(map[string]*ssh.Client, len(cm.clients))
	for id, client := range cm.clients {
		clients[id] = client
	}
	cm.clients = make(map[string]*ssh.Client)
	cm.refCount = make(map[string]int)
	cm.mu.Unlock()

	log.Printf("[DEBUG] Disconnecting all clients, count=%d", len(clients))
	for id, client := range clients {
		log.Printf("[DEBUG] Disconnecting client: %s", id)
		_ = cm.disconnect(client)
		cm.wait(client)
	}
	log.Println("[DEBUG] All clients disconnected")
}

// NewApp creates a new application instance using the platform config directory.
func NewApp() (*App, error) {
	log.Println("[DEBUG] Creating application instance")

	log.Println("[DEBUG] Initializing storage...")
	store, err := newStore()
	if err != nil {
		log.Printf("[ERROR] Failed to initialize store: %v", err)
		return nil, fmt.Errorf("failed to initialize store: %w", err)
	}

	return newAppFromStore(store), nil
}

// NewAppAt creates a new application instance with config stored in the given
// directory. Tests pass t.TempDir() here to avoid touching real config or env vars.
func NewAppAt(configDir string) (*App, error) {
	store, err := storage.NewStoreAt(configDir)
	if err != nil {
		return nil, fmt.Errorf("failed to initialize store: %w", err)
	}
	return newAppFromStore(store), nil
}

func newAppFromStore(store *storage.Store) *App {
	clientManager := NewClientManager(store)
	app := &App{
		store:         store,
		clientManager: clientManager,
	}
	log.Println("[DEBUG] Creating tunnel manager...")
	app.tunnelManager = newTunnelManager(store, clientManager)
	log.Println("[DEBUG] Application instance created successfully")
	return app
}

// GetStore returns the data store
func (a *App) GetStore() *storage.Store {
	return a.store
}

// GetTunnelManager returns the tunnel manager
func (a *App) GetTunnelManager() *ssh.TunnelManager {
	return a.tunnelManager
}

// GetClientManager returns the client manager
func (a *App) GetClientManager() *ClientManager {
	return a.clientManager
}

// Shutdown gracefully shuts down the application
func (a *App) Shutdown() {
	log.Println("[DEBUG] Shutting down application...")
	a.tunnelManager.StopAll()
	a.clientManager.DisconnectAll()
	log.Println("[DEBUG] Application shutdown complete")
}
