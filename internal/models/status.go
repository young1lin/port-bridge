package models

import "image/color"

// TunnelStatus represents the current status of a tunnel
type TunnelStatus int

const (
	StatusDisconnected TunnelStatus = iota
	StatusConnecting
	StatusConnected
	StatusReconnecting
	StatusError
)

// String returns the human-readable status (English, used as i18n key)
func (s TunnelStatus) String() string {
	switch s {
	case StatusDisconnected:
		return "Disconnected"
	case StatusConnecting:
		return "Connecting"
	case StatusConnected:
		return "Connected"
	case StatusReconnecting:
		return "Reconnecting"
	case StatusError:
		return "Error"
	default:
		return "Unknown"
	}
}

// Color returns the status color
func (s TunnelStatus) Color() color.Color {
	switch s {
	case StatusDisconnected:
		return color.RGBA{R: 158, G: 158, B: 158, A: 255} // Gray
	case StatusConnecting:
		return color.RGBA{R: 255, G: 193, B: 7, A: 255} // Yellow/Orange
	case StatusConnected:
		return color.RGBA{R: 76, G: 175, B: 80, A: 255} // Green
	case StatusReconnecting:
		return color.RGBA{R: 255, G: 152, B: 0, A: 255} // Orange
	case StatusError:
		return color.RGBA{R: 244, G: 67, B: 54, A: 255} // Red
	default:
		return color.RGBA{R: 158, G: 158, B: 158, A: 255}
	}
}
