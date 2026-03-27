//go:build integration
// +build integration

package ssh

import (
	"net"
	"testing"
)

func TestCanListen_Localhost(t *testing.T) {
	// Use a high port that's unlikely to be in use
	port := 59301
	err := CanListen(port, false)
	if err != nil {
		t.Fatalf("CanListen(%d, false) should succeed on a free port: %v", port, err)
	}
}

func TestCanListen_LAN(t *testing.T) {
	port := 59302
	err := CanListen(port, true)
	if err != nil {
		t.Fatalf("CanListen(%d, true) should succeed on a free port: %v", port, err)
	}
}

func TestCanListen_Twice(t *testing.T) {
	port := 59303
	// First listen should succeed
	err := CanListen(port, false)
	if err != nil {
		t.Fatalf("First CanListen should succeed: %v", err)
	}

	// Actually bind the port
	listener, err := net.Listen("tcp", "127.0.0.1:59303")
	if err != nil {
		t.Fatalf("Failed to bind port for test: %v", err)
	}
	defer listener.Close()

	// Second CanListen should fail since port is now in use
	err = CanListen(port, false)
	if err == nil {
		t.Fatal("CanListen should fail when port is already bound")
	}
}

func TestIsPortInUse_NotInUse(t *testing.T) {
	port := 59304
	if IsPortInUse(port) {
		t.Fatalf("IsPortInUse(%d) should return false for unused port", port)
	}
}

func TestIsPortInUse_InUse(t *testing.T) {
	port := 59305
	// Bind the port
	listener, err := net.Listen("tcp", "127.0.0.1:59305")
	if err != nil {
		t.Fatalf("Failed to bind port for test: %v", err)
	}
	defer listener.Close()

	if !IsPortInUse(port) {
		t.Fatalf("IsPortInUse(%d) should return true for bound port", port)
	}
}
