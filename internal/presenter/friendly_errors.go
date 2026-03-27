package presenter

import (
	"errors"
	"fmt"
	"net"
	"os"
	"strings"

	"github.com/young1lin/port-bridge/internal/i18n"
)

// FriendlyError represents a user-friendly error message with optional suggestions
type FriendlyError struct {
	Message    string
	Suggestion string
	Cause      error
}

func (e *FriendlyError) Error() string {
	if e.Suggestion != "" {
		return fmt.Sprintf("%s\n\n%s", e.Message, e.Suggestion)
	}
	return e.Message
}

func (e *FriendlyError) Unwrap() error {
	return e.Cause
}

// MakeFriendlyError converts technical errors to user-friendly messages
func MakeFriendlyError(err error) error {
	if err == nil {
		return nil
	}

	errStr := strings.ToLower(err.Error())

	// Network connection errors
	if isNetworkError(errStr) {
		return &FriendlyError{
			Message:    i18n.L("Cannot connect to server"),
			Suggestion: i18n.L("Please check:\n• Server address and port are correct\n• SSH service is running on the server\n• Firewall allows the connection\n• Network connectivity is available"),
			Cause:      err,
		}
	}

	// Authentication errors
	if isAuthError(errStr) {
		return &FriendlyError{
			Message:    i18n.L("Authentication failed"),
			Suggestion: i18n.L("Please check:\n• Username is correct\n• Password is correct\n• SSH key is valid and has proper permissions\n• Key passphrase is correct"),
			Cause:      err,
		}
	}

	// Port binding errors
	if isPortError(errStr) {
		return &FriendlyError{
			Message:    i18n.L("Port is unavailable"),
			Suggestion: i18n.L("The local port is already in use. Please:\n• Choose a different port\n• Close the application using this port\n• Check if another tunnel is using this port"),
			Cause:      err,
		}
	}

	// Timeout errors
	if isTimeoutError(errStr) {
		return &FriendlyError{
			Message:    i18n.L("Connection timeout"),
			Suggestion: i18n.L("The server is not responding. Please:\n• Check if the server address is correct\n• Verify network connectivity\n• Check if the server is under heavy load"),
			Cause:      err,
		}
	}

	// Host key errors
	if isHostKeyError(errStr) {
		return &FriendlyError{
			Message:    i18n.L("Host key verification failed"),
			Suggestion: i18n.L("The server's identity has changed. This could indicate:\n• Server was reinstalled\n• Possible security attack (MITM)\n\nPlease verify the server's fingerprint before proceeding."),
			Cause:      err,
		}
	}

	// File/permission errors
	if isPermissionError(errStr) {
		return &FriendlyError{
			Message:    i18n.L("Permission denied"),
			Suggestion: i18n.L("Please check:\n• SSH key file permissions (should be readable only by you)\n• You have access to the required resources"),
			Cause:      err,
		}
	}

	// Target unreachable
	if isTargetUnreachableError(errStr) {
		return &FriendlyError{
			Message:    i18n.L("Target service unreachable"),
			Suggestion: i18n.L("Cannot connect to the target service. Please check:\n• Target host and port are correct\n• Target service is running\n• Network routing allows the connection"),
			Cause:      err,
		}
	}

	// Proxy errors
	if isProxyError(errStr) {
		return &FriendlyError{
			Message:    i18n.L("Proxy connection failed"),
			Suggestion: i18n.L("Cannot connect through SOCKS5 proxy. Please check:\n• Proxy address and port are correct\n• Proxy credentials are valid\n• Proxy server is running"),
			Cause:      err,
		}
	}

	// Default: wrap with generic message
	return &FriendlyError{
		Message: i18n.L("Operation failed"),
		Cause:   err,
	}
}

func isNetworkError(errStr string) bool {
	keywords := []string{
		"connection refused",
		"no route to host",
		"network is unreachable",
		"connection reset",
		"broken pipe",
		"socket",
		"address not available",
	}
	for _, k := range keywords {
		if strings.Contains(errStr, k) {
			return true
		}
	}

	// Check for net.OpError
	var netErr net.Error
	if errors.As(errors.New(errStr), &netErr) {
		return true
	}

	return false
}

func isAuthError(errStr string) bool {
	keywords := []string{
		"unable to authenticate",
		"authentication failed",
		"permission denied",
		"access denied",
		"invalid credentials",
		"ssh: handshake failed",
		"no supported methods",
	}
	for _, k := range keywords {
		if strings.Contains(errStr, k) {
			return true
		}
	}
	return false
}

func isPortError(errStr string) bool {
	keywords := []string{
		"address already in use",
		"port is already in use",
		"bind: address already in use",
		"listen tcp",
	}
	for _, k := range keywords {
		if strings.Contains(errStr, k) {
			return true
		}
	}
	return false
}

func isTimeoutError(errStr string) bool {
	keywords := []string{
		"timeout",
		"timed out",
		"deadline exceeded",
		"i/o timeout",
	}
	for _, k := range keywords {
		if strings.Contains(errStr, k) {
			return true
		}
	}
	return false
}

func isHostKeyError(errStr string) bool {
	keywords := []string{
		"host key",
		"known_hosts",
		"key mismatch",
		"mitm",
		"man-in-the-middle",
	}
	for _, k := range keywords {
		if strings.Contains(errStr, k) {
			return true
		}
	}
	return false
}

func isPermissionError(errStr string) bool {
	keywords := []string{
		"permission denied",
		"access is denied",
		"unauthorized",
		"forbidden",
	}
	for _, k := range keywords {
		if strings.Contains(errStr, k) {
			return true
		}
	}

	// Check for os.PathError with permission issues
	var pathErr *os.PathError
	if errors.As(errors.New(errStr), &pathErr) {
		if strings.Contains(strings.ToLower(pathErr.Err.Error()), "permission") {
			return true
		}
	}

	return false
}

func isTargetUnreachableError(errStr string) bool {
	keywords := []string{
		"target unreachable",
		"connection refused",
		"no route",
		"host unreachable",
		"dial tcp",
	}
	// Exclude SSH-level connection refused (handled by network error)
	if strings.Contains(errStr, "ssh:") {
		return false
	}
	for _, k := range keywords {
		if strings.Contains(errStr, k) {
			return true
		}
	}
	return false
}

func isProxyError(errStr string) bool {
	keywords := []string{
		"socks",
		"proxy",
		"proxy connection",
	}
	for _, k := range keywords {
		if strings.Contains(errStr, k) {
			return true
		}
	}
	return false
}
