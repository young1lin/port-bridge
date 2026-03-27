//go:build !port_bridge
// +build !port_bridge

package secure

import (
	"github.com/zalando/go-keyring"
)

// DefaultKeyring uses the system keyring for credential storage.
type DefaultKeyring struct{}

// NewKeyring creates a new keyring instance.
func NewKeyring() Keyring {
	return &DefaultKeyring{}
}

// Set stores a secret in the system keyring.
func (k *DefaultKeyring) Set(service, user, secret string) error {
	return keyring.Set(service, user, secret)
}

// Get retrieves a secret from the system keyring.
func (k *DefaultKeyring) Get(service, user string) (string, error) {
	return keyring.Get(service, user)
}

// Delete removes a secret from the system keyring.
func (k *DefaultKeyring) Delete(service, user string) error {
	return keyring.Delete(service, user)
}
