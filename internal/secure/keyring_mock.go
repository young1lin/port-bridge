//go:build port_bridge
// +build port_bridge

package secure

import (
	"sync"
)

// MockKeyring is an in-memory keyring for testing.
type MockKeyring struct {
	mu      sync.RWMutex
	secrets map[string]string
}

// NewMockKeyring creates a new mock keyring for testing.
func NewMockKeyring() *MockKeyring {
	return &MockKeyring{
		secrets: make(map[string]string),
	}
}

// NewKeyring returns a mock keyring in test builds.
func NewKeyring() Keyring {
	return NewMockKeyring()
}

// key generates a storage key.
func (k *MockKeyring) key(service, user string) string {
	return service + ":" + user
}

// Set stores a secret in memory.
func (k *MockKeyring) Set(service, user, secret string) error {
	k.mu.Lock()
	defer k.mu.Unlock()
	k.secrets[k.key(service, user)] = secret
	return nil
}

// Get retrieves a secret from memory.
func (k *MockKeyring) Get(service, user string) (string, error) {
	k.mu.RLock()
	defer k.mu.RUnlock()
	secret, ok := k.secrets[k.key(service, user)]
	if !ok {
		return "", ErrNotFound
	}
	return secret, nil
}

// Delete removes a secret from memory.
func (k *MockKeyring) Delete(service, user string) error {
	k.mu.Lock()
	defer k.mu.Unlock()
	delete(k.secrets, k.key(service, user))
	return nil
}

// ErrNotFound is returned when a secret is not found.
var ErrNotFound = &NotFoundError{}

// NotFoundError indicates a secret was not found.
type NotFoundError struct{}

func (e *NotFoundError) Error() string {
	return "secret not found"
}
