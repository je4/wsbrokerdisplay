package display

import (
	"github.com/je4/wsbrokerdisplay/pkg/rest"
)

// Action constants for display commands and responses.
const (
	// EventNavigate identifies a command requesting browser navigation to a target URL.
	EventNavigate = "display:navigate"
	// EventNavigateResponse identifies a response acknowledgment for a navigation command.
	EventNavigateResponse = "display:navigate_response"
	// EventGetURL identifies a command requesting the active browser page URL.
	EventGetURL = "display:geturl"
	// EventGetURLResponse identifies a response containing the active browser page URL.
	EventGetURLResponse = "display:geturl_response"
	// EventGetScreenshot identifies a command requesting a bounded viewport screenshot.
	EventGetScreenshot = "display:getscreenshot"
	// EventScreenshotResponse identifies an error or acknowledgment response for a screenshot request.
	EventScreenshotResponse = "display:getscreenshot_response"
	// EventGetStatus identifies a command requesting live display telemetry.
	EventGetStatus = "display:getstatus"
	// EventGetStatusResponse identifies a response containing display telemetry.
	EventGetStatusResponse = "display:getstatus_response"
)

// CommandMessage represents an incoming command request payload received via WSBroker.
// It supports navigation, page URL retrieval, bounded screenshot capture, and status queries.
type CommandMessage struct {
	// Event specifies the operation to execute ("display:navigate", "display:getscreenshot", "display:geturl", "display:getstatus", etc.).
	Event string `json:"event,omitempty"`
	// Action is supported for backwards compatibility.
	Action string `json:"action,omitempty"`
	// URL specifies the target web address for "navigate" commands.
	URL string `json:"url,omitempty"`
	// Selector specifies an optional CSS query selector to wait for after navigation.
	Selector string `json:"selector,omitempty"`
	// Format specifies the desired screenshot format ("png" or "jpeg", default: "jpeg").
	Format string `json:"format,omitempty"`
	// Quality specifies the JPEG compression quality (1-100, default: 85).
	Quality int `json:"quality,omitempty"`
}

// URLResponse represents the outgoing response to a "geturl" command.
type URLResponse struct {
	// Event is always "display:geturl_response".
	Event string `json:"event"`
	// URL contains the current primary browser location URL.
	URL string `json:"url"`
	// Error contains an error message if the URL inspection failed.
	Error string `json:"error,omitempty"`
}

// NavigateResponse represents the outgoing response to a "navigate" command.
type NavigateResponse struct {
	// Event is always "display:navigate_response".
	Event string `json:"event"`
	// Status indicates operation outcome ("ok" or "error").
	Status string `json:"status"`
	// URL contains the target URL that was requested.
	URL string `json:"url"`
	// Error contains an error description if navigation failed.
	Error string `json:"error,omitempty"`
}

// ScreenshotResponse represents an error or status response when screenshot capture fails.
type ScreenshotResponse struct {
	// Event is always "display:getscreenshot_response".
	Event string `json:"event"`
	// Status indicates operation outcome ("ok" or "error").
	Status string `json:"status"`
	// Error contains an error description if screenshot capture failed.
	Error string `json:"error,omitempty"`
}

// StatusResponse represents the response containing display telemetry or error details.
type StatusResponse struct {
	// Event is always "display:getstatus_response".
	Event string `json:"event"`
	// Status indicates operation outcome ("ok" or "error").
	Status string `json:"status"`
	// Data contains the telemetry dashboard data when status is "ok".
	Data *rest.DashboardData `json:"data,omitempty"`
	// Error contains an error description if status retrieval failed.
	Error string `json:"error,omitempty"`
}
