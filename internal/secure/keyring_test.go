//go:build port_bridge
// +build port_bridge

package secure

import (
	"testing"
)

func TestMockKeyring_SetGet(t *testing.T) {
	k := NewMockKeyring()

	err := k.Set(ServiceName, "user1", "secret123")
	if err != nil {
		t.Fatalf("Set: %v", err)
	}

	got, err := k.Get(ServiceName, "user1")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got != "secret123" {
		t.Errorf("Get = %q, want %q", got, "secret123")
	}
}

func TestMockKeyring_Get_NotFound(t *testing.T) {
	k := NewMockKeyring()

	_, err := k.Get(ServiceName, "nonexistent")
	if err != ErrNotFound {
		t.Errorf("expected ErrNotFound, got %v", err)
	}
}

func TestMockKeyring_Delete(t *testing.T) {
	k := NewMockKeyring()

	k.Set(ServiceName, "del-me", "value")
	k.Delete(ServiceName, "del-me")

	_, err := k.Get(ServiceName, "del-me")
	if err != ErrNotFound {
		t.Errorf("expected ErrNotFound after delete, got %v", err)
	}
}

func TestMockKeyring_Delete_Nonexistent(t *testing.T) {
	k := NewMockKeyring()
	// Should not panic
	k.Delete(ServiceName, "does-not-exist")
}

func TestMockKeyring_KeyFormat(t *testing.T) {
	k := NewMockKeyring()
	k.Set("my-service", "my-user", "pw")

	got, err := k.Get("my-service", "my-user")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got != "pw" {
		t.Errorf("Get = %q, want %q", got, "pw")
	}

	// Same service+user key but different value should overwrite
	k.Set("my-service", "my-user", "pw2")
	got, _ = k.Get("my-service", "my-user")
	if got != "pw2" {
		t.Errorf("Get after overwrite = %q, want %q", got, "pw2")
	}
}

func TestMockKeyring_MultipleKeys(t *testing.T) {
	k := NewMockKeyring()

	k.Set("svc", "a", "1")
	k.Set("svc", "b", "2")
	k.Set("other", "a", "3")

	if got, _ := k.Get("svc", "a"); got != "1" {
		t.Errorf("got %q, want %q", got, "1")
	}
	if got, _ := k.Get("svc", "b"); got != "2" {
		t.Errorf("got %q, want %q", got, "2")
	}
	if got, _ := k.Get("other", "a"); got != "3" {
		t.Errorf("got %q, want %q", got, "3")
	}
}

func TestNewKeyring(t *testing.T) {
	k := NewKeyring()
	if k == nil {
		t.Fatal("NewKeyring returned nil")
	}
	// In port_bridge build, it should be a MockKeyring
	_, ok := k.(*MockKeyring)
	if !ok {
		t.Error("NewKeyring should return *MockKeyring in port_bridge build")
	}
}

func TestNotFoundError_Error(t *testing.T) {
	err := &NotFoundError{}
	if err.Error() != "secret not found" {
		t.Errorf("NotFoundError.Error() = %q, want %q", err.Error(), "secret not found")
	}
}
