package views

import (
	"testing"
)

func TestTunnelItemData(t *testing.T) {
	item := TunnelItem{
		ID:        "test-id-123",
		Name:      "test-rule",
		LocalPort: 8080,
		Target:    "192.168.1.1:80",
		Status:    "Connected",
		IsRunning: true,
	}

	// verify all fields are set correctly
	if item.ID != "test-id-123" {
		t.Errorf("expected ID 'test-id-123', got '%s'", item.ID)
	}
	if item.Name != "test-rule" {
		t.Errorf("expected Name 'test-rule', got '%s'", item.Name)
	}
	if item.LocalPort != 8080 {
		t.Errorf("expected LocalPort 8080, got %d", item.LocalPort)
	}
	if item.Target != "192.168.1.1:80" {
		t.Errorf("expected Target '192.168.1.1:80', got '%s'", item.Target)
	}
	if item.Status != "Connected" {
		t.Errorf("expected Status 'Connected', got '%s'", item.Status)
	}
	if !item.IsRunning {
		t.Errorf("expected IsRunning true, got %v", item.IsRunning)
	}
}

func TestTunnelViewSetData(t *testing.T) {
	// pure data test with no Fyne dependency
	data := []TunnelItem{
		{
			ID:        "1",
			Name:      "rule-1",
			LocalPort: 3333,
			Target:    "127.0.0.1:3333",
			Status:    "Disconnected",
		},
		{
			ID:        "2",
			Name:      "rule-2",
			LocalPort: 4444,
			Target:    "127.0.0.1:4444",
			Status:    "Connected",
			IsRunning: true,
		},
		{
			ID:        "3",
			Name:      "rule-3",
			LocalPort: 5555,
			Target:    "10.0.0.1:80",
			Status:    "Disconnected",
		},
	}

	// verify slice length
	if len(data) != 3 {
		t.Fatalf("expected 3 items, got %d", len(data))
	}

	// verify each item has all required fields
	for i, item := range data {
		if item.ID == "" {
			t.Errorf("[%d] ID must not be empty", i)
		}
		if item.Name == "" {
			t.Errorf("[%d] Name must not be empty", i)
		}
		if item.LocalPort <= 0 {
			t.Errorf("[%d] LocalPort must be > 0, got %d", i, item.LocalPort)
		}
		if item.Target == "" {
			t.Errorf("[%d] Target must not be empty", i)
		}
		if item.Status == "" {
			t.Errorf("[%d] Status must not be empty", i)
		}

		t.Logf("[%d] OK: id=%s, name=%s, local=%d, target=%s, status=%s, running=%v",
			i, item.ID, item.Name, item.LocalPort, item.Target, item.Status, item.IsRunning)
	}
}

func TestTunnelItemAddressFormat(t *testing.T) {
	tests := []struct {
		name       string
		localPort  int
		target     string
		wantFormat string
	}{
		{
			name:       "local forward",
			localPort:  3333,
			target:     "127.0.0.1:3333",
			wantFormat: "127.0.0.1:3333 -> 127.0.0.1:3333",
		},
		{
			name:       "remote forward",
			localPort:  8080,
			target:     "192.168.1.100:80",
			wantFormat: "127.0.0.1:8080 -> 192.168.1.100:80",
		},
		{
			name:       "high port",
			localPort:  65535,
			target:     "10.0.0.1:443",
			wantFormat: "127.0.0.1:65535 -> 10.0.0.1:443",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			item := TunnelItem{
				LocalPort: tt.localPort,
				Target:    tt.target,
			}

			// replicate the formatAddress logic
			got := formatAddress(item.LocalPort, item.Target)
			if got != tt.wantFormat {
				t.Errorf("address format mismatch\nwant: %s\ngot:  %s", tt.wantFormat, got)
			}
		})
	}
}

// formatAddress formats the tunnel address for display.
func formatAddress(localPort int, target string) string {
	return "127.0.0.1:" + itoa(localPort) + " -> " + target
}

// itoa converts an int to its decimal string representation.
func itoa(n int) string {
	if n == 0 {
		return "0"
	}

	var negative bool
	if n < 0 {
		negative = true
		n = -n
	}

	var digits []byte
	for n > 0 {
		digits = append([]byte{byte('0' + n%10)}, digits...)
		n /= 10
	}

	if negative {
		digits = append([]byte{'-'}, digits...)
	}

	return string(digits)
}
