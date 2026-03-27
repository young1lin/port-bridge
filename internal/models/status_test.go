package models

import (
	"testing"
)

func TestTunnelStatus_String(t *testing.T) {
	tests := []struct {
		status TunnelStatus
		want   string
	}{
		{StatusDisconnected, "Disconnected"},
		{StatusConnecting, "Connecting"},
		{StatusConnected, "Connected"},
		{StatusReconnecting, "Reconnecting"},
		{StatusError, "Error"},
		{TunnelStatus(99), "Unknown"},
	}
	for _, tt := range tests {
		if got := tt.status.String(); got != tt.want {
			t.Errorf("TunnelStatus(%d).String() = %q, want %q", tt.status, got, tt.want)
		}
	}
}

func TestTunnelStatus_Color(t *testing.T) {
	tests := []struct {
		status  TunnelStatus
		r, g, b uint8
	}{
		{StatusDisconnected, 158, 158, 158},
		{StatusConnecting, 255, 193, 7},
		{StatusConnected, 76, 175, 80},
		{StatusReconnecting, 255, 152, 0},
		{StatusError, 244, 67, 54},
		{TunnelStatus(99), 158, 158, 158},
	}
	for _, tt := range tests {
		c := tt.status.Color()
		r, g, b, _ := c.RGBA()
		if uint8(r>>8) != tt.r || uint8(g>>8) != tt.g || uint8(b>>8) != tt.b {
			t.Errorf("TunnelStatus(%d).Color() = RGBA(%d,%d,%d), want RGBA(%d,%d,%d)",
				tt.status, r>>8, g>>8, b>>8, tt.r, tt.g, tt.b)
		}
	}
}

func TestTunnelStatus_Color_NonNil(t *testing.T) {
	for _, s := range []TunnelStatus{StatusDisconnected, StatusConnecting, StatusConnected, StatusReconnecting, StatusError} {
		if s.Color() == nil {
			t.Errorf("TunnelStatus(%d).Color() should not return nil", s)
		}
	}
}
