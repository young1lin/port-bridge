package models

import "testing"

func TestNewSSHConnection(t *testing.T) {
	c := NewSSHConnection()

	if c.ID == "" {
		t.Error("expected non-empty ID")
	}
	if c.Port != 22 {
		t.Errorf("expected default Port=22, got %d", c.Port)
	}
	if c.AuthType != AuthTypePassword {
		t.Errorf("expected default AuthType=AuthTypePassword, got %s", c.AuthType)
	}
}

func TestSSHConnection_Address(t *testing.T) {
	tests := []struct {
		host string
		port int
		want string
	}{
		{"192.168.1.1", 22, "192.168.1.1:22"},
		{"example.com", 2222, "example.com:2222"},
		{"10.0.0.1", 0, "10.0.0.1:0"},
	}
	for _, tt := range tests {
		c := &SSHConnection{Host: tt.host, Port: tt.port}
		if got := c.Address(); got != tt.want {
			t.Errorf("Address() = %q, want %q", got, tt.want)
		}
	}
}

func TestSSHConnection_Clone(t *testing.T) {
	original := NewSSHConnection()
	original.Name = "test-conn"
	original.Host = "example.com"
	original.Port = 2222
	original.Password = "secret"

	clone := original.Clone()

	if clone.ID == original.ID {
		t.Error("clone should have a different ID")
	}
	if clone.Name != original.Name {
		t.Errorf("clone.Name = %q, want %q", clone.Name, original.Name)
	}
	if clone.Host != original.Host {
		t.Errorf("clone.Host = %q, want %q", clone.Host, original.Host)
	}
	if clone.Port != original.Port {
		t.Errorf("clone.Port = %d, want %d", clone.Port, original.Port)
	}
	if clone.Password != original.Password {
		t.Error("clone should copy Password")
	}

	// Verify independence
	clone.Name = "modified"
	if original.Name == "modified" {
		t.Error("modifying clone should not affect original")
	}
}
