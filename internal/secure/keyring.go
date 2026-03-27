// Package secure provides secure credential storage using system keyring.
package secure

// Keyring provides an interface for secure credential storage.
type Keyring interface {
	// Set stores a secret for the given service and user.
	Set(service, user, secret string) error

	// Get retrieves a secret for the given service and user.
	Get(service, user string) (string, error)

	// Delete removes a secret for the given service and user.
	Delete(service, user string) error
}

// ServiceName is the keyring service identifier.
const ServiceName = "port-bridge"
