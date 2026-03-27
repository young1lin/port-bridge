package ssh

import (
	"context"
	"fmt"
	"io"
	"log"
	"math/rand/v2"
	"net"
	"strings"
	"sync"
	"time"

	"github.com/young1lin/port-bridge/internal/models"
)

// TunnelState represents the runtime state of a tunnel
type TunnelState struct {
	Tunnel    *models.Tunnel
	Status    models.TunnelStatus
	Error     error
	StartedAt time.Time
}

// tunnelStopReason indicates why a tunnel's accept loop exited
type tunnelStopReason int

const (
	stopUserCancelled   tunnelStopReason = iota // user called StopTunnel
	stopSSHDisconnected                         // SSH connection lost
	stopListenerError                           // listener accept error
)

// TunnelManager manages all active tunnels
type TunnelManager struct {
	mu                sync.RWMutex
	tunnels           map[string]*activeTunnel
	store             TunnelStore
	connGetter        ConnectionGetter
	callbacks         []StatusCallback
	sleep             func(ctx context.Context, d time.Duration)       // injectable for testing
	canListen         func(port int, allowLAN bool) error              // injectable for testing
	listen            func(network, addr string) (net.Listener, error) // injectable for testing
	dialRemote        func(client *Client, network, addr string) (net.Conn, error)
	copyStream        func(dst io.Writer, src io.Reader) (int64, error)
	isClientConnected func(client *Client) bool
	waitForDisconnect func(client *Client, ctx context.Context)
}

// TunnelStore interface for tunnel persistence
type TunnelStore interface {
	GetTunnel(id string) *models.Tunnel
	SaveTunnel(tunnel *models.Tunnel) error
	GetConnection(id string) *models.SSHConnection
}

// ConnectionGetter provides SSH connections
type ConnectionGetter interface {
	GetOrCreateClient(conn *models.SSHConnection) (*Client, error)
	ReleaseClient(connID string)
}

// StatusCallback is called when tunnel status changes
type StatusCallback func(tunnelID string, status models.TunnelStatus, err error)

// activeTunnel holds the runtime state of an active tunnel
type activeTunnel struct {
	tunnel            *models.Tunnel
	client            *Client
	listener          net.Listener
	status            models.TunnelStatus
	err               error
	ctx               context.Context
	cancel            context.CancelFunc
	wg                sync.WaitGroup
	reconnectAttempts int
}

// NewTunnelManager creates a new tunnel manager
func NewTunnelManager(store TunnelStore, connGetter ConnectionGetter) *TunnelManager {
	log.Println("[DEBUG] Creating tunnel manager")
	return &TunnelManager{
		tunnels:           make(map[string]*activeTunnel),
		store:             store,
		connGetter:        connGetter,
		sleep:             defaultSleep,
		canListen:         CanListen,
		listen:            net.Listen,
		dialRemote:        defaultDialRemote,
		copyStream:        io.Copy,
		isClientConnected: defaultIsClientConnected,
		waitForDisconnect: defaultWaitForDisconnect,
	}
}

// defaultSleep waits for the given duration or until ctx is cancelled.
func defaultSleep(ctx context.Context, d time.Duration) {
	select {
	case <-time.After(d):
	case <-ctx.Done():
	}
}

func defaultDialRemote(client *Client, network, addr string) (net.Conn, error) {
	return client.Dial(network, addr)
}

func defaultIsClientConnected(client *Client) bool {
	return client != nil && client.IsConnected()
}

func defaultWaitForDisconnect(client *Client, ctx context.Context) {
	if client != nil {
		client.WaitForDisconnectContext(ctx)
	}
}

// AddStatusCallback registers a status change callback
func (m *TunnelManager) AddStatusCallback(cb StatusCallback) {
	log.Println("[DEBUG] Adding status callback to tunnel manager")
	m.callbacks = append(m.callbacks, cb)
}

// notifyStatus notifies all callbacks of a status change
func (m *TunnelManager) notifyStatus(tunnelID string, status models.TunnelStatus, err error) {
	log.Printf("[DEBUG] Notifying status change: tunnel=%s, status=%s, err=%v", tunnelID, status.String(), err)
	for _, cb := range m.callbacks {
		cb(tunnelID, status, err)
	}
}

// StartTunnel starts a tunnel by ID
func (m *TunnelManager) StartTunnel(tunnelID string) error {
	log.Printf("[DEBUG] StartTunnel requested: id=%s", tunnelID)
	m.mu.Lock()
	defer m.mu.Unlock()

	// Check if already running
	if _, exists := m.tunnels[tunnelID]; exists {
		log.Printf("[ERROR] Tunnel already running: id=%s", tunnelID)
		return fmt.Errorf("tunnel already running")
	}

	tunnel := m.store.GetTunnel(tunnelID)
	if tunnel == nil {
		log.Printf("[ERROR] Tunnel not found: id=%s", tunnelID)
		return fmt.Errorf("tunnel not found")
	}

	log.Printf("[DEBUG] Found tunnel: name=%s, local=%d, target=%s:%d",
		tunnel.Name, tunnel.LocalPort, tunnel.TargetHost, tunnel.TargetPort)

	// Check local port availability (before starting)
	log.Printf("[DEBUG] Checking local port availability: %d", tunnel.LocalPort)
	if err := m.canListen(tunnel.LocalPort, tunnel.AllowLAN); err != nil {
		errMsg := fmt.Errorf("local port %d is already in use, please choose another port", tunnel.LocalPort)
		log.Printf("[ERROR] %s", errMsg)
		return errMsg
	}
	log.Printf("[DEBUG] Local port %d is available", tunnel.LocalPort)

	conn := m.store.GetConnection(tunnel.ConnectionID)
	if conn == nil {
		err := fmt.Errorf("SSH connection not found (id=%s)", tunnel.ConnectionID)
		log.Printf("[ERROR] %s", err)
		return err
	}
	log.Printf("[DEBUG] Found associated connection: name=%s, host=%s", conn.Name, conn.Host)

	// Create tunnel context
	ctx, cancel := context.WithCancel(context.Background())

	at := &activeTunnel{
		tunnel: tunnel,
		status: models.StatusConnecting,
		ctx:    ctx,
		cancel: cancel,
	}

	m.tunnels[tunnelID] = at

	// Start tunnel in background
	at.wg.Add(1)
	log.Printf("[DEBUG] Starting tunnel goroutine: id=%s", tunnelID)
	go m.runTunnel(at, conn)

	return nil
}

// runTunnel manages the tunnel lifecycle
func (m *TunnelManager) runTunnel(at *activeTunnel, conn *models.SSHConnection) {
	defer at.wg.Done()
	log.Printf("[DEBUG] Tunnel goroutine started: id=%s", at.tunnel.ID)

	for {
		select {
		case <-at.ctx.Done():
			log.Printf("[DEBUG] Tunnel context cancelled: id=%s", at.tunnel.ID)
			return
		default:
		}

		// Update status to connecting
		at.status = models.StatusConnecting
		m.notifyStatus(at.tunnel.ID, models.StatusConnecting, nil)

		// Get or create SSH client
		log.Printf("[DEBUG] Getting SSH client for connection: id=%s", conn.ID)
		client, err := m.connGetter.GetOrCreateClient(conn)
		if err != nil {
			at.err = fmt.Errorf("SSH connection failed: %w", err)
			at.status = models.StatusError
			log.Printf("[ERROR] Failed to get SSH client: %v", err)
			m.notifyStatus(at.tunnel.ID, models.StatusError, at.err)

			if at.tunnel.AutoReconnect {
				log.Printf("[DEBUG] Auto-reconnect enabled, will retry")
				at.status = models.StatusReconnecting
				m.notifyStatus(at.tunnel.ID, models.StatusReconnecting, at.err)
				m.reconnectWait(at)
				continue
			}
			return
		}
		log.Printf("[DEBUG] SSH client obtained successfully")
		at.client = client

		// Start local listener
		listenAddr := at.tunnel.LocalAddress()
		log.Printf("[DEBUG] Starting local listener on: %s", listenAddr)
		listener, err := m.listen("tcp", listenAddr)
		if err != nil {
			at.err = fmt.Errorf("local listen failed: %w", err)
			at.status = models.StatusError
			log.Printf("[ERROR] Failed to start listener: %v", err)
			m.notifyStatus(at.tunnel.ID, models.StatusError, at.err)
			m.connGetter.ReleaseClient(conn.ID)

			if at.tunnel.AutoReconnect {
				log.Printf("[DEBUG] Auto-reconnect enabled, will retry")
				at.status = models.StatusReconnecting
				m.notifyStatus(at.tunnel.ID, models.StatusReconnecting, at.err)
				m.reconnectWait(at)
				continue
			}
			return
		}

		at.listener = listener
		at.status = models.StatusConnected
		at.err = nil
		at.reconnectAttempts = 0
		log.Printf("[DEBUG] Tunnel connected: id=%s, listening on %s", at.tunnel.ID, listenAddr)
		m.notifyStatus(at.tunnel.ID, models.StatusConnected, nil)

		// Accept connections or wait for SSH disconnect
		log.Printf("[DEBUG] Accepting connections on tunnel: id=%s", at.tunnel.ID)
		reason := m.acceptConnectionsOrWaitDisconnect(at)

		// Close listener if still open
		if at.listener != nil {
			at.listener.Close()
			at.listener = nil
		}

		// Release client at this single point (no double-release)
		m.connGetter.ReleaseClient(conn.ID)
		at.client = nil

		// Determine next action based on stop reason
		switch reason {
		case stopUserCancelled:
			log.Printf("[DEBUG] Tunnel stopped by user: id=%s", at.tunnel.ID)
			return
		case stopSSHDisconnected:
			if at.tunnel.AutoReconnect && at.ctx.Err() == nil {
				log.Printf("[DEBUG] SSH disconnected, auto-reconnecting: id=%s", at.tunnel.ID)
				at.status = models.StatusReconnecting
				m.notifyStatus(at.tunnel.ID, models.StatusReconnecting, nil)

				if err := m.canListen(at.tunnel.LocalPort, at.tunnel.AllowLAN); err != nil {
					at.err = fmt.Errorf("local port %d is already in use", at.tunnel.LocalPort)
					at.status = models.StatusError
					// Release lock before notifying to avoid deadlock
					m.mu.Lock()
					tunnelID := at.tunnel.ID
					delete(m.tunnels, tunnelID)
					m.mu.Unlock()
					m.notifyStatus(tunnelID, models.StatusError, at.err)
					return
				}
				continue
			}
			// Release lock before notifying to avoid deadlock
			m.mu.Lock()
			tunnelID := at.tunnel.ID
			delete(m.tunnels, tunnelID)
			m.mu.Unlock()
			at.status = models.StatusDisconnected
			m.notifyStatus(tunnelID, models.StatusDisconnected, nil)
			return
		case stopListenerError:
			if at.tunnel.AutoReconnect && at.ctx.Err() == nil {
				log.Printf("[DEBUG] Listener error, auto-reconnecting: id=%s", at.tunnel.ID)
				at.status = models.StatusReconnecting
				m.notifyStatus(at.tunnel.ID, models.StatusReconnecting, nil)
				continue
			}
			// Release lock before notifying to avoid deadlock
			m.mu.Lock()
			tunnelID := at.tunnel.ID
			delete(m.tunnels, tunnelID)
			m.mu.Unlock()
			if at.err == nil {
				at.err = fmt.Errorf("listener stopped unexpectedly")
			}
			at.status = models.StatusError
			m.notifyStatus(tunnelID, models.StatusError, at.err)
			return
		}
	}
}

// acceptConnectionsOrWaitDisconnect accepts connections or stops when SSH disconnects.
// Returns the reason why the accept loop exited.
func (m *TunnelManager) acceptConnectionsOrWaitDisconnect(at *activeTunnel) tunnelStopReason {
	// Use a local context so the accept and SSH-monitor goroutines exit as soon
	// as this function returns, regardless of whether at.ctx is still live
	// (e.g. during an auto-reconnect cycle).
	ctx, cancel := context.WithCancel(at.ctx)
	defer cancel()

	// Start accepting connections in a goroutine (tracked by wg)
	acceptChan := make(chan net.Conn)
	acceptErrChan := make(chan error)

	at.wg.Add(1)
	go func() {
		defer at.wg.Done()
		for {
			select {
			case <-ctx.Done():
				return
			default:
			}
			localConn, err := at.listener.Accept()
			if err != nil {
				select {
				case acceptErrChan <- err:
				case <-ctx.Done():
				}
				return
			}
			select {
			case acceptChan <- localConn:
			case <-ctx.Done():
				localConn.Close()
				return
			}
		}
	}()

	// Start a goroutine to monitor SSH connection (tracked by wg)
	sshDisconnected := make(chan struct{}, 1)
	at.wg.Add(1)
	go func() {
		defer at.wg.Done()
		if m.isClientConnected(at.client) {
			m.waitForDisconnect(at.client, ctx)
			select {
			case <-ctx.Done():
				return
			case sshDisconnected <- struct{}{}:
			}
		}
	}()

	for {
		select {
		case <-at.ctx.Done():
			return stopUserCancelled
		case <-sshDisconnected:
			log.Printf("[DEBUG] SSH connection lost, stopping tunnel: %s", at.tunnel.ID)
			// Do NOT delete from m.tunnels here — runTunnel decides whether to reconnect or stop.
			// Deleting here while runTunnel continues the loop caused a state desync where
			// the goroutine kept running (port still bound) but the manager had no record of it.
			return stopSSHDisconnected
		case localConn := <-acceptChan:
			log.Printf("[DEBUG] New connection accepted on tunnel: id=%s, remote=%s", at.tunnel.ID, localConn.RemoteAddr())
			at.wg.Add(1)
			go m.handleConnection(at, localConn)
		case err := <-acceptErrChan:
			if at.ctx.Err() != nil {
				return stopUserCancelled
			}
			at.err = err
			at.status = models.StatusError
			log.Printf("[DEBUG] Listener accept error: %v", err)
			return stopListenerError
		}
	}
}

// handleConnection handles a single forwarded connection with timeout protection
func (m *TunnelManager) handleConnection(at *activeTunnel, localConn net.Conn) {
	defer at.wg.Done()
	defer localConn.Close()

	targetAddr := at.tunnel.TargetAddress()
	log.Printf("[DEBUG] Forwarding connection to: %s", targetAddr)

	// Dial through SSH with timeout protection
	dialCtx, dialCancel := context.WithTimeout(at.ctx, 30*time.Second)
	defer dialCancel()

	// Use a channel to get the dial result
	type dialResult struct {
		conn net.Conn
		err  error
	}
	dialChan := make(chan dialResult, 1)

	go func() {
		conn, err := m.dialRemote(at.client, "tcp", targetAddr)
		dialChan <- dialResult{conn: conn, err: err}
	}()

	var remoteConn net.Conn
	select {
	case <-dialCtx.Done():
		log.Printf("[WARN] Dial timeout for %s", targetAddr)
		// Drain the buffered channel to close any connection that arrives after the timeout.
		go func() {
			if result := <-dialChan; result.conn != nil {
				result.conn.Close()
			}
		}()
		return
	case result := <-dialChan:
		if result.err != nil {
			log.Printf("[ERROR] Failed to dial remote: %v", result.err)
			// Check if this is an SSH connection error (not just target unreachable)
			if m.isSSHConnectionError(result.err) {
				log.Printf("[DEBUG] SSH connection appears to be lost, stopping tunnel: %s", at.tunnel.ID)
				go m.StopTunnel(at.tunnel.ID)
			}
			return
		}
		remoteConn = result.conn
	}
	defer remoteConn.Close()

	log.Printf("[DEBUG] Connection established: %s <-> %s", localConn.RemoteAddr(), targetAddr)

	// Set initial deadline for connections (will be extended by activity)
	// This prevents goroutines from blocking forever if connections become stale
	deadline := time.Now().Add(5 * time.Minute)
	localConn.SetDeadline(deadline)
	remoteConn.SetDeadline(deadline)

	// Bidirectional copy
	done := make(chan struct{}, 2)

	// Copy with deadline extension on activity
	go func() {
		buf := make([]byte, 32*1024)
		for {
			n, err := localConn.Read(buf)
			if n > 0 {
				localConn.SetDeadline(time.Now().Add(5 * time.Minute))
				remoteConn.SetDeadline(time.Now().Add(5 * time.Minute))
				if _, writeErr := remoteConn.Write(buf[:n]); writeErr != nil {
					break
				}
			}
			if err != nil {
				break
			}
		}
		done <- struct{}{}
	}()

	go func() {
		buf := make([]byte, 32*1024)
		for {
			n, err := remoteConn.Read(buf)
			if n > 0 {
				localConn.SetDeadline(time.Now().Add(5 * time.Minute))
				remoteConn.SetDeadline(time.Now().Add(5 * time.Minute))
				if _, writeErr := localConn.Write(buf[:n]); writeErr != nil {
					break
				}
			}
			if err != nil {
				break
			}
		}
		done <- struct{}{}
	}()

	// Wait for either direction to finish or context cancellation, then close
	// both connections so the other goroutine unblocks.
	select {
	case <-done:
		log.Printf("[DEBUG] Connection closed: %s", localConn.RemoteAddr())
		localConn.Close()
		remoteConn.Close()
		<-done // wait for the other copy goroutine
	case <-at.ctx.Done():
		log.Printf("[DEBUG] Connection cancelled: %s", localConn.RemoteAddr())
		localConn.Close()
		remoteConn.Close()
		<-done // wait for both copy goroutines
		<-done
	}
}

// isSSHConnectionError checks if an error indicates SSH connection is lost
func (m *TunnelManager) isSSHConnectionError(err error) bool {
	if err == nil {
		return false
	}
	errStr := err.Error()
	// These errors indicate SSH connection is broken, not just target unreachable
	sshErrors := []string{
		"closed network connection",
		"session closed",
		"connection reset",
		"broken pipe",
		"transport is closing",
		"i/o timeout",
	}
	for _, e := range sshErrors {
		if strings.Contains(strings.ToLower(errStr), e) {
			return true
		}
	}
	return false
}

// reconnectWait waits before attempting reconnection using exponential backoff with jitter.
// Formula: min(baseInterval * 2^attempt, 5min) with ±25% jitter
func (m *TunnelManager) reconnectWait(at *activeTunnel) {
	duration := m.calcReconnectDuration(at)
	at.reconnectAttempts++

	log.Printf("[DEBUG] Waiting %.1f seconds before reconnect attempt (attempt #%d)",
		float64(duration)/float64(time.Second), at.reconnectAttempts)
	m.sleep(at.ctx, duration)
}

// calcReconnectDuration computes the wait duration using exponential backoff with jitter.
func (m *TunnelManager) calcReconnectDuration(at *activeTunnel) time.Duration {
	baseInterval := at.tunnel.ReconnectInterval
	if baseInterval <= 0 {
		baseInterval = 10
	}

	// Exponential backoff capped at 5 minutes
	backoff := float64(baseInterval)
	for i := 0; i < at.reconnectAttempts; i++ {
		backoff *= 2
		if backoff > 300 {
			backoff = 300
			break
		}
	}

	// Add ±25% jitter
	jitter := backoff * (0.75 + rand.Float64()*0.5)
	return time.Duration(jitter * float64(time.Second))
}

// StopTunnel stops a tunnel by ID
func (m *TunnelManager) StopTunnel(tunnelID string) error {
	log.Printf("[DEBUG] StopTunnel requested: id=%s", tunnelID)
	m.mu.Lock()

	at, exists := m.tunnels[tunnelID]
	if !exists {
		m.mu.Unlock()
		log.Printf("[DEBUG] Tunnel not running: id=%s", tunnelID)
		return nil // Not running
	}

	log.Printf("[DEBUG] Stopping tunnel: id=%s", tunnelID)

	// Cancel context to stop everything
	at.cancel()

	// Close listener
	if at.listener != nil {
		log.Printf("[DEBUG] Closing listener for tunnel: id=%s", tunnelID)
		at.listener.Close()
	}

	// Wait for goroutines to finish
	log.Printf("[DEBUG] Waiting for tunnel goroutines to finish: id=%s", tunnelID)
	at.wg.Wait()

	delete(m.tunnels, tunnelID)
	m.mu.Unlock() // Release lock BEFORE notifying to avoid deadlock

	m.notifyStatus(tunnelID, models.StatusDisconnected, nil)
	log.Printf("[DEBUG] Tunnel stopped: id=%s", tunnelID)

	return nil
}

// GetStatus returns the current status of a tunnel
func (m *TunnelManager) GetStatus(tunnelID string) models.TunnelStatus {
	m.mu.RLock()
	defer m.mu.RUnlock()

	if at, exists := m.tunnels[tunnelID]; exists {
		return at.status
	}
	return models.StatusDisconnected
}

// GetError returns the last error of a tunnel
func (m *TunnelManager) GetError(tunnelID string) error {
	m.mu.RLock()
	defer m.mu.RUnlock()

	if at, exists := m.tunnels[tunnelID]; exists {
		return at.err
	}
	return nil
}

// IsRunning returns whether a tunnel is currently running
func (m *TunnelManager) IsRunning(tunnelID string) bool {
	m.mu.RLock()
	defer m.mu.RUnlock()
	_, exists := m.tunnels[tunnelID]
	return exists
}

// StopAll stops all running tunnels
func (m *TunnelManager) StopAll() {
	log.Println("[DEBUG] StopAll tunnels requested")
	m.mu.Lock()
	tunnelIDs := make([]string, 0, len(m.tunnels))
	for id := range m.tunnels {
		tunnelIDs = append(tunnelIDs, id)
	}
	m.mu.Unlock()

	log.Printf("[DEBUG] Stopping %d tunnels", len(tunnelIDs))
	for _, id := range tunnelIDs {
		m.StopTunnel(id)
	}
}

// GetRunningCount returns the number of currently running tunnels
func (m *TunnelManager) GetRunningCount() int {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return len(m.tunnels)
}

// GetRunningTunnelIDs returns the IDs of all currently running tunnels
func (m *TunnelManager) GetRunningTunnelIDs() []string {
	m.mu.RLock()
	defer m.mu.RUnlock()
	ids := make([]string, 0, len(m.tunnels))
	for id := range m.tunnels {
		ids = append(ids, id)
	}
	return ids
}
