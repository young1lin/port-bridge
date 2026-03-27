package models

import (
	"fmt"

	"github.com/google/uuid"
)

// AuthType represents the authentication method
type AuthType string

const (
	AuthTypePassword AuthType = "password"
	AuthTypeKey      AuthType = "key"
)

// SSHConnection represents a saved SSH connection configuration
type SSHConnection struct {
	ID       string   `json:"id"`
	Name     string   `json:"name"`
	Host     string   `json:"host"`
	Port     int      `json:"port"`
	Username string   `json:"username"`
	AuthType AuthType `json:"auth_type"`

	// Password - not serialized, loaded from keyring using PasswordRef
	Password      string `json:"-"`
	PasswordRef   string `json:"password_ref,omitempty"`
	KeyPath       string `json:"key_path,omitempty"`
	KeyPassphrase string `json:"-"`
	KeyPassRef    string `json:"key_pass_ref,omitempty"`

	// SOCKS5 proxy configuration
	UseProxy      bool   `json:"use_proxy"`
	ProxyHost     string `json:"proxy_host,omitempty"`
	ProxyPort     int    `json:"proxy_port,omitempty"`
	ProxyUsername string `json:"proxy_username,omitempty"`
	ProxyPassword string `json:"-"`
	ProxyPassRef  string `json:"proxy_pass_ref,omitempty"`
}

// NewSSHConnection creates a new SSH connection with defaults
func NewSSHConnection() *SSHConnection {
	return &SSHConnection{
		ID:       uuid.New().String(),
		Port:     22,
		AuthType: AuthTypePassword,
	}
}

// Address returns the host:port string
func (c *SSHConnection) Address() string {
	return fmt.Sprintf("%s:%d", c.Host, c.Port)
}

// Clone creates a copy of the connection with a new ID
func (c *SSHConnection) Clone() *SSHConnection {
	clone := *c
	clone.ID = uuid.New().String()
	return &clone
}
