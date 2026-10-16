package rest

import (
	"time"

	"github.com/je4/wsbrokerdisplay/pkg/ntp"
)

// WSMessage represents a logged WebSocket message event.
type WSMessage struct {
	Timestamp time.Time `json:"timestamp"`
	Direction string    `json:"direction"` // "in" (received) or "out" (sent)
	Type      string    `json:"type"`      // "message", "file", "ping", "pong", "attach", etc.
	Source    string    `json:"source"`
	Target    string    `json:"target,omitempty"`
	Token     string    `json:"token,omitempty"`
	Data      string    `json:"data,omitempty"`
}

// BrowserInfo contains details about the browser display configuration.
type BrowserInfo struct {
	Kiosk    bool   `json:"kiosk"`
	Headless bool   `json:"headless"`
	Width    int    `json:"width"`
	Height   int    `json:"height"`
	Address  string `json:"address,omitempty"`
}

// DashboardData encapsulates all telemetry, system metadata, NTP status, and recent WebSocket messages for the dashboard.
type DashboardData struct {
	ClientName    string        `json:"client_name"`
	WSBroker      string        `json:"ws_broker"`
	Channels      []string      `json:"channels"`
	Addr          string        `json:"addr"`
	ExtAddr       string        `json:"ext_addr"`
	TLSType       string        `json:"tls_type"`
	Hostname      string        `json:"hostname"`
	OS            string        `json:"os"`
	Arch          string        `json:"arch"`
	GoVersion     string        `json:"go_version"`
	Uptime        string        `json:"uptime"`
	StartTime     time.Time     `json:"start_time"`
	BrowserConfig BrowserInfo   `json:"browser"`
	NTP           ntp.NTPStatus `json:"ntp"`
	Messages      []WSMessage   `json:"messages,omitempty"`
}
