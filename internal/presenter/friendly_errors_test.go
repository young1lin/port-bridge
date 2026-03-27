package presenter

import (
	"errors"
	"strings"
	"testing"
)

func TestMakeFriendlyError_Nil(t *testing.T) {
	if got := MakeFriendlyError(nil); got != nil {
		t.Fatalf("expected nil, got %v", got)
	}
}

func TestFriendlyError_ErrorWithSuggestion(t *testing.T) {
	fe := &FriendlyError{Message: "msg", Suggestion: "hint", Cause: errors.New("raw")}
	if !strings.Contains(fe.Error(), "msg") || !strings.Contains(fe.Error(), "hint") {
		t.Fatalf("Error() should contain message and suggestion, got %q", fe.Error())
	}
}

func TestFriendlyError_ErrorWithoutSuggestion(t *testing.T) {
	fe := &FriendlyError{Message: "msg only", Cause: errors.New("raw")}
	if fe.Error() != "msg only" {
		t.Fatalf("expected %q, got %q", "msg only", fe.Error())
	}
}

func TestFriendlyError_Unwrap(t *testing.T) {
	cause := errors.New("root cause")
	fe := &FriendlyError{Message: "msg", Cause: cause}
	if fe.Unwrap() != cause {
		t.Fatalf("Unwrap() should return cause")
	}
}

func TestFriendlyError_UnwrapNil(t *testing.T) {
	fe := &FriendlyError{Message: "msg"}
	if fe.Unwrap() != nil {
		t.Fatalf("Unwrap() on nil Cause should return nil")
	}
}

func TestMakeFriendlyError_ReturnsFriendlyError(t *testing.T) {
	err := errors.New("some unknown error xyz")
	got := MakeFriendlyError(err)
	var fe *FriendlyError
	if !errors.As(got, &fe) {
		t.Fatal("MakeFriendlyError should return *FriendlyError")
	}
	if !errors.Is(got, err) {
		t.Fatal("FriendlyError should wrap the original error")
	}
}

var makeFriendlyErrorCases = []struct {
	name    string
	input   string
	wantMsg string
}{
	// Network errors
	{"connection refused", "connection refused", "Cannot connect to server"},
	{"no route to host", "no route to host", "Cannot connect to server"},
	{"network is unreachable", "network is unreachable", "Cannot connect to server"},
	{"connection reset", "connection reset by peer", "Cannot connect to server"},
	{"broken pipe", "broken pipe", "Cannot connect to server"},
	{"socket error", "socket: operation not supported", "Cannot connect to server"},
	{"address not available", "address not available", "Cannot connect to server"},

	// Auth errors
	{"unable to authenticate", "unable to authenticate", "Authentication failed"},
	{"authentication failed", "authentication failed", "Authentication failed"},
	{"invalid credentials", "invalid credentials", "Authentication failed"},
	{"ssh handshake", "ssh: handshake failed", "Authentication failed"},
	{"no supported methods", "no supported methods remain", "Authentication failed"},

	// Port errors
	{"address already in use", "bind: address already in use", "Port is unavailable"},
	{"port already in use", "port is already in use", "Port is unavailable"},
	{"listen tcp", "listen tcp :8080: bind: address already in use", "Port is unavailable"},

	// Timeout errors
	{"timeout keyword", "i/o timeout", "Connection timeout"},
	{"timed out", "operation timed out", "Connection timeout"},
	{"deadline exceeded", "context deadline exceeded", "Connection timeout"},

	// Host key errors
	{"host key", "host key mismatch", "Host key verification failed"},
	{"known_hosts", "error in known_hosts file", "Host key verification failed"},
	{"key mismatch", "key mismatch detected", "Host key verification failed"},
	{"mitm", "possible mitm attack", "Host key verification failed"},
	{"man-in-the-middle", "man-in-the-middle detected", "Host key verification failed"},

	// Permission errors (not caught by auth first)
	{"access is denied", "access is denied", "Permission denied"},
	{"unauthorized", "unauthorized access", "Permission denied"},
	{"forbidden", "forbidden resource", "Permission denied"},

	// Target unreachable (no "ssh:" prefix)
	{"target unreachable", "target unreachable", "Target service unreachable"},
	{"host unreachable", "host unreachable", "Target service unreachable"},
	{"dial tcp", "dial tcp 127.0.0.1:3306: refused by firewall", "Target service unreachable"},

	// Proxy errors
	{"socks proxy", "socks5 connect error", "Proxy connection failed"},
	{"proxy keyword", "proxy server error", "Proxy connection failed"},

	// Default fallback
	{"unknown error", "some completely unknown error message xyz", "Operation failed"},
}

func TestMakeFriendlyError_Categories(t *testing.T) {
	for _, tc := range makeFriendlyErrorCases {
		t.Run(tc.name, func(t *testing.T) {
			err := errors.New(tc.input)
			got := MakeFriendlyError(err)
			var fe *FriendlyError
			if !errors.As(got, &fe) {
				t.Fatalf("expected *FriendlyError, got %T", got)
			}
			if fe.Message != tc.wantMsg {
				t.Fatalf("input %q: expected message %q, got %q", tc.input, tc.wantMsg, fe.Message)
			}
			if fe.Cause != err {
				t.Fatal("Cause should be the original error")
			}
		})
	}
}

func TestMakeFriendlyError_TargetUnreachable_SSHPrefixExcluded(t *testing.T) {
	// "connection refused" with "ssh:" prefix should be network error, not target unreachable
	err := errors.New("ssh: connection refused")
	got := MakeFriendlyError(err)
	var fe *FriendlyError
	if !errors.As(got, &fe) {
		t.Fatalf("expected *FriendlyError")
	}
	if fe.Message != "Cannot connect to server" {
		t.Fatalf("ssh: prefix should route to network error, got %q", fe.Message)
	}
}

func TestMakeFriendlyError_PermissionDenied_IsAuthFirst(t *testing.T) {
	// "permission denied" appears in isAuthError keywords, so it should map to auth
	err := errors.New("permission denied (publickey)")
	got := MakeFriendlyError(err)
	var fe *FriendlyError
	if !errors.As(got, &fe) {
		t.Fatalf("expected *FriendlyError")
	}
	if fe.Message != "Authentication failed" {
		t.Fatalf("'permission denied' should match auth first, got %q", fe.Message)
	}
}

func TestMakeFriendlyError_SuggestionPresent(t *testing.T) {
	err := errors.New("connection refused")
	got := MakeFriendlyError(err)
	var fe *FriendlyError
	errors.As(got, &fe)
	if fe.Suggestion == "" {
		t.Fatal("network error should have a suggestion")
	}
}

func TestMakeFriendlyError_DefaultHasNoSuggestion(t *testing.T) {
	err := errors.New("some completely unknown error xyz abc 12345")
	got := MakeFriendlyError(err)
	var fe *FriendlyError
	errors.As(got, &fe)
	if fe.Suggestion != "" {
		t.Fatalf("default fallback should have no suggestion, got %q", fe.Suggestion)
	}
}
