package models

import "testing"

func TestNewTunnel(t *testing.T) {
	tun := NewTunnel()

	if tun.ID == "" {
		t.Error("expected non-empty ID")
	}
	if tun.AutoReconnect {
		t.Error("expected default AutoReconnect=false")
	}
	if tun.ReconnectInterval != 10 {
		t.Errorf("expected default ReconnectInterval=10, got %d", tun.ReconnectInterval)
	}
}

func TestTunnel_TargetAddress(t *testing.T) {
	tun := &Tunnel{TargetHost: "10.0.0.1", TargetPort: 3306}
	if got := tun.TargetAddress(); got != "10.0.0.1:3306" {
		t.Errorf("TargetAddress() = %q, want %q", got, "10.0.0.1:3306")
	}
}

func TestTunnel_LocalAddress_Default(t *testing.T) {
	tun := &Tunnel{LocalPort: 8080, AllowLAN: false}
	if got := tun.LocalAddress(); got != "127.0.0.1:8080" {
		t.Errorf("LocalAddress() = %q, want %q", got, "127.0.0.1:8080")
	}
}

func TestTunnel_LocalAddress_AllowLAN(t *testing.T) {
	tun := &Tunnel{LocalPort: 8080, AllowLAN: true}
	if got := tun.LocalAddress(); got != "0.0.0.0:8080" {
		t.Errorf("LocalAddress() = %q, want %q", got, "0.0.0.0:8080")
	}
}

func TestTunnel_Clone(t *testing.T) {
	original := NewTunnel()
	original.Name = "rule-1"
	original.LocalPort = 3306
	original.TargetHost = "10.0.0.1"
	original.TargetPort = 3306

	clone := original.Clone()

	if clone.ID == original.ID {
		t.Error("clone should have a different ID")
	}
	if clone.Name != original.Name {
		t.Errorf("clone.Name = %q, want %q", clone.Name, original.Name)
	}
	if clone.LocalPort != original.LocalPort {
		t.Errorf("clone.LocalPort = %d, want %d", clone.LocalPort, original.LocalPort)
	}

	clone.Name = "modified"
	if original.Name == "modified" {
		t.Error("modifying clone should not affect original")
	}
}
