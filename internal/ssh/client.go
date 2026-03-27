package ssh

import (
	"context"
	"fmt"
	"log"
	"net"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/young1lin/port-bridge/internal/models"
	"golang.org/x/crypto/ssh"
	"golang.org/x/net/proxy"
)

// Client wraps SSH connection functionality
type Client struct {
	conn            *models.SSHConnection
	client          *ssh.Client
	mu              sync.Mutex
	keepAliveStop   chan struct{} // channel to stop keep-alive goroutine
	keepAliveStopMu sync.Once     // ensures keepAliveStop is closed only once
	wg              sync.WaitGroup
	dialFunc        func(client *ssh.Client, network, addr string) (net.Conn, error)
	sendRequest     func(client *ssh.Client) error
	closeConn       func(client *ssh.Client) error
	newTicker       func(d time.Duration) keepAliveTicker
}

type keepAliveTicker interface {
	Chan() <-chan time.Time
	Stop()
}

type realKeepAliveTicker struct {
	ticker *time.Ticker
}

func (t realKeepAliveTicker) Chan() <-chan time.Time {
	return t.ticker.C
}

func (t realKeepAliveTicker) Stop() {
	t.ticker.Stop()
}

// NewClient creates a new SSH client
func NewClient(conn *models.SSHConnection) *Client {
	log.Printf("[DEBUG] Creating SSH client for %s (%s)", conn.Name, conn.Address())
	return &Client{
		conn: conn,
		dialFunc: func(client *ssh.Client, network, addr string) (net.Conn, error) {
			return client.Dial(network, addr)
		},
		sendRequest: func(client *ssh.Client) error {
			_, _, err := client.SendRequest("keepalive@golang.org", true, nil)
			return err
		},
		closeConn: func(client *ssh.Client) error {
			return client.Close()
		},
		newTicker: func(d time.Duration) keepAliveTicker {
			return realKeepAliveTicker{ticker: time.NewTicker(d)}
		},
	}
}

// Connect establishes the SSH connection
func (c *Client) Connect() error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.client != nil {
		log.Printf("[DEBUG] Already connected to %s", c.conn.Address())
		return nil // Already connected
	}

	log.Printf("[DEBUG] Connecting to %s as %s (auth: %s)", c.conn.Address(), c.conn.Username, c.conn.AuthType)

	hostKeyCallback, hkErr := GetHostKeyCallback()
	if hkErr != nil {
		log.Printf("[WARN] Host key callback init error, using insecure fallback: %v", hkErr)
		hostKeyCallback = ssh.InsecureIgnoreHostKey()
	}

	config := &ssh.ClientConfig{
		User:            c.conn.Username,
		HostKeyCallback: hostKeyCallback,
		Timeout:         10 * time.Second,
	}

	switch c.conn.AuthType {
	case models.AuthTypePassword:
		log.Printf("[DEBUG] Using password authentication")
		config.Auth = []ssh.AuthMethod{
			ssh.Password(c.conn.Password),
		}
	case models.AuthTypeKey:
		log.Printf("[DEBUG] Using key authentication")
		key, err := c.loadPrivateKey()
		if err != nil {
			log.Printf("[ERROR] Failed to load private key: %v", err)
			return fmt.Errorf("failed to load private key: %w", err)
		}
		config.Auth = []ssh.AuthMethod{
			ssh.PublicKeys(key),
		}
	default:
		return fmt.Errorf("unsupported auth type: %s", c.conn.AuthType)
	}

	log.Printf("[DEBUG] Dialing %s...", c.conn.Address())

	var client *ssh.Client
	var err error
	if c.conn.UseProxy {
		// Connect through SOCKS5 proxy
		client, err = c.dialThroughSOCKS5(config)
	} else {
		// Direct connection
		client, err = ssh.Dial("tcp", c.conn.Address(), config)
	}

	if err != nil {
		log.Printf("[ERROR] Failed to connect to %s: %v", c.conn.Address(), err)
		return fmt.Errorf("failed to connect: %w", err)
	}

	c.client = client
	c.keepAliveStop = make(chan struct{})

	// Start keep-alive goroutine with WaitGroup tracking
	c.wg.Add(1)
	go func() {
		defer c.wg.Done()
		c.keepAlive()
	}()

	log.Printf("[DEBUG] Successfully connected to %s", c.conn.Address())
	return nil
}

// dialThroughSOCKS5 establishes an SSH connection through a SOCKS5 proxy
func (c *Client) dialThroughSOCKS5(config *ssh.ClientConfig) (*ssh.Client, error) {
	proxyAddr := fmt.Sprintf("%s:%d", c.conn.ProxyHost, c.conn.ProxyPort)
	log.Printf("[DEBUG] Connecting through SOCKS5 proxy: %s", proxyAddr)

	var auth proxy.Auth
	if c.conn.ProxyUsername != "" {
		auth = proxy.Auth{
			User:     c.conn.ProxyUsername,
			Password: c.conn.ProxyPassword,
		}
	}

	dialer, err := proxy.SOCKS5("tcp", proxyAddr, &auth, proxy.Direct)
	if err != nil {
		return nil, fmt.Errorf("failed to create SOCKS5 dialer: %w", err)
	}

	conn, err := dialer.Dial("tcp", c.conn.Address())
	if err != nil {
		return nil, fmt.Errorf("failed to connect through proxy: %w", err)
	}

	sshConn, chans, reqs, err := ssh.NewClientConn(conn, c.conn.Address(), config)
	if err != nil {
		conn.Close()
		return nil, fmt.Errorf("failed to create SSH connection: %w", err)
	}

	return ssh.NewClient(sshConn, chans, reqs), nil
}

// loadPrivateKey loads and parses the SSH private key
func (c *Client) loadPrivateKey() (ssh.Signer, error) {
	keyPath := c.conn.KeyPath
	if keyPath == "" {
		// Default to ~/.ssh/id_rsa
		home, err := os.UserHomeDir()
		if err != nil {
			return nil, err
		}
		keyPath = filepath.Join(home, ".ssh", "id_rsa")
	}

	log.Printf("[DEBUG] Loading private key from: %s", keyPath)
	keyData, err := os.ReadFile(keyPath)
	if err != nil {
		log.Printf("[ERROR] Failed to read key file: %v", err)
		return nil, err
	}

	var signer ssh.Signer
	if c.conn.KeyPassphrase != "" {
		log.Printf("[DEBUG] Parsing key with passphrase")
		signer, err = ssh.ParsePrivateKeyWithPassphrase(keyData, []byte(c.conn.KeyPassphrase))
	} else {
		log.Printf("[DEBUG] Parsing key without passphrase")
		signer, err = ssh.ParsePrivateKey(keyData)
	}

	if err != nil {
		log.Printf("[ERROR] Failed to parse key: %v", err)
		return nil, err
	}

	log.Printf("[DEBUG] Private key loaded successfully")
	return signer, err
}

// Disconnect closes the SSH connection and waits for background goroutines to finish
func (c *Client) Disconnect() error {
	c.mu.Lock()

	// Stop keep-alive goroutine first (use sync.Once to prevent double close)
	if c.keepAliveStop != nil {
		c.keepAliveStopMu.Do(func() {
			close(c.keepAliveStop)
		})
		c.keepAliveStop = nil
	}

	var err error
	if c.client != nil {
		log.Printf("[DEBUG] Disconnecting from %s", c.conn.Address())
		err = c.client.Close()
		c.client = nil
		if err != nil {
			log.Printf("[ERROR] Error during disconnect: %v", err)
		}
	}
	c.mu.Unlock()

	// Wait for keepAlive goroutine to exit (outside lock)
	c.wg.Wait()
	return err
}

// Wait blocks until all background goroutines (keepAlive) have exited
func (c *Client) Wait() {
	c.wg.Wait()
}

// IsConnected returns whether the client is connected
func (c *Client) IsConnected() bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.client != nil
}

// GetClient returns the underlying SSH client
func (c *Client) GetClient() *ssh.Client {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.client
}

// Dial creates a connection through SSH to the target
func (c *Client) Dial(network, addr string) (net.Conn, error) {
	c.mu.Lock()
	client := c.client
	c.mu.Unlock()

	if client == nil {
		return nil, fmt.Errorf("not connected")
	}

	log.Printf("[DEBUG] Dialing through SSH: %s://%s", network, addr)
	return c.dialFunc(client, network, addr)
}

// TestConnection tests if the connection can be established
func (c *Client) TestConnection() error {
	log.Printf("[DEBUG] Testing connection to %s", c.conn.Address())
	if err := c.Connect(); err != nil {
		return err
	}
	defer c.Disconnect()
	log.Printf("[DEBUG] Connection test to %s successful", c.conn.Address())
	return nil
}

// TestTargetPort tests if a target port is reachable through the SSH connection
// This requires the client to already be connected
func (c *Client) TestTargetPort(host string, port int) error {
	c.mu.Lock()
	client := c.client
	c.mu.Unlock()

	if client == nil {
		return fmt.Errorf("not connected")
	}

	targetAddr := fmt.Sprintf("%s:%d", host, port)
	log.Printf("[DEBUG] Testing target port through SSH: %s", targetAddr)

	// Try to dial the target through SSH with a short timeout
	conn, err := c.dialFunc(client, "tcp", targetAddr)
	if err != nil {
		log.Printf("[DEBUG] Target port test failed: %v", err)
		return fmt.Errorf("target port %s unreachable: %w", targetAddr, err)
	}
	conn.Close()
	log.Printf("[DEBUG] Target port test successful: %s", targetAddr)
	return nil
}

// WaitForDisconnect blocks until the SSH connection is closed
func (c *Client) WaitForDisconnect() {
	c.mu.Lock()
	client := c.client
	c.mu.Unlock()

	if client != nil {
		client.Wait()
		log.Printf("[DEBUG] SSH connection closed for client")
	}
}

// WaitForDisconnectContext blocks until the SSH connection is closed or context is cancelled
func (c *Client) WaitForDisconnectContext(ctx context.Context) {
	c.mu.Lock()
	client := c.client
	c.mu.Unlock()

	if client == nil {
		return
	}

	done := make(chan struct{})
	go func() {
		client.Wait()
		close(done)
	}()

	select {
	case <-done:
		log.Printf("[DEBUG] SSH connection closed for client")
	case <-ctx.Done():
		log.Printf("[DEBUG] WaitForDisconnectContext cancelled for client")
	}
}

// keepAlive sends periodic keep-alive requests to prevent idle disconnect
func (c *Client) keepAlive() {
	ticker := c.newTicker(55 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-c.keepAliveStop:
			log.Printf("[DEBUG] Keep-alive stopped for %s", c.conn.Address())
			return
		case <-ticker.Chan():
			c.mu.Lock()
			client := c.client
			c.mu.Unlock()

			if client == nil {
				return
			}

			// Send a keep-alive request (global request with wantReply=true)
			err := c.sendRequest(client)
			if err != nil {
				log.Printf("[DEBUG] Keep-alive failed for %s: %v — closing connection", c.conn.Address(), err)
				// Actively close the underlying connection so client.Wait() unblocks
				// immediately and the tunnel manager can detect the disconnect.
				c.closeConn(client)
				return
			}
		}
	}
}
