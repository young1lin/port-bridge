package models

import (
	"fmt"

	"github.com/google/uuid"
)

// Tunnel represents a port forwarding rule
type Tunnel struct {
	ID                string `json:"id"`
	Name              string `json:"name"`
	LocalPort         int    `json:"local_port"`
	ConnectionID      string `json:"connection_id"`
	TargetHost        string `json:"target_host"`
	TargetPort        int    `json:"target_port"`
	Remark            string `json:"remark"`
	AutoReconnect     bool   `json:"auto_reconnect"`
	ReconnectInterval int    `json:"reconnect_interval"` // seconds
	AllowLAN          bool   `json:"allow_lan"`          // allow LAN access (0.0.0.0 vs 127.0.0.1)
}

// NewTunnel creates a new tunnel with defaults
func NewTunnel() *Tunnel {
	return &Tunnel{
		ID:                uuid.New().String(),
		AutoReconnect:     false,
		ReconnectInterval: 10,
	}
}

// TargetAddress returns the target host:port string
func (t *Tunnel) TargetAddress() string {
	return fmt.Sprintf("%s:%d", t.TargetHost, t.TargetPort)
}

// LocalAddress returns the local listen address
func (t *Tunnel) LocalAddress() string {
	if t.AllowLAN {
		return fmt.Sprintf("0.0.0.0:%d", t.LocalPort)
	}
	return fmt.Sprintf("127.0.0.1:%d", t.LocalPort)
}

// Clone creates a copy of the tunnel with a new ID
func (t *Tunnel) Clone() *Tunnel {
	clone := *t
	clone.ID = uuid.New().String()
	return &clone
}
