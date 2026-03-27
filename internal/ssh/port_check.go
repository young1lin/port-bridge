package ssh

import (
	"fmt"
	"net"
	"time"
)

// IsPortInUse checks if a local port is already in use
func IsPortInUse(port int) bool {
	addr := fmt.Sprintf("127.0.0.1:%d", port)
	conn, err := net.DialTimeout("tcp", addr, time.Second)
	if err != nil {
		return false
	}
	conn.Close()
	return true
}

// CanListen checks if we can listen on the specified port
func CanListen(port int, allowLAN bool) error {
	addr := "127.0.0.1"
	if allowLAN {
		addr = "0.0.0.0"
	}
	listenAddr := fmt.Sprintf("%s:%d", addr, port)
	listener, err := net.Listen("tcp", listenAddr)
	if err != nil {
		return fmt.Errorf("port %d is not available: %w", port, err)
	}
	listener.Close()
	return nil
}
