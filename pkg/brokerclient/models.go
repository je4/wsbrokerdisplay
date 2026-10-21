package brokerclient

import (
	"github.com/je4/wsbrokerdisplay/pkg/rest"
)

// Action constants for remote browser control commands and responses.
const (
	// ActionNavigate identifies a command requesting browser navigation to a target URL.
	ActionNavigate = "navigate"
	// ActionNavigateResponse identifies a response acknowledgment for a navigation command.
	ActionNavigateResponse = "navigate_response"
	// ActionGetURL identifies a command requesting the active browser page URL.
	ActionGetURL = "geturl"
	// ActionGetURLResponse identifies a response containing the active browser page URL.
	ActionGetURLResponse = "geturl_response"
	// ActionGetScreenshot identifies a command requesting a bounded viewport screenshot.
	ActionGetScreenshot = "getscreenshot"
	// ActionScreenshotResponse identifies an error or acknowledgment response for a screenshot request.
	ActionScreenshotResponse = "getscreenshot_response"
	// ActionGetStatus identifies a command requesting display telemetry.
	ActionGetStatus = "getstatus"
	// ActionGetStatusResponse identifies a response containing display telemetry.
	ActionGetStatusResponse = "getstatus_response"
)

// CommandMessage represents a command request payload sent to the display service via WSBroker.
type CommandMessage struct {
	// Event specifies the event operation name (e.g. "display:getstatus", "display:navigate").
	Event string `json:"event,omitempty"`
	// Action specifies the operation to execute ("navigate", "getscreenshot", "geturl", or "getstatus").
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

// URLResponse represents the response to a "geturl" command.
type URLResponse struct {
	// Action is always "geturl_response".
	Action string `json:"action,omitempty"`
	// Event specifies the event name if present.
	Event string `json:"event,omitempty"`
	// URL contains the current primary browser location URL.
	URL string `json:"url"`
	// Error contains an error message if the URL inspection failed.
	Error string `json:"error,omitempty"`
}

// NavigateResponse represents the response to a "navigate" command.
type NavigateResponse struct {
	// Action is always "navigate_response".
	Action string `json:"action,omitempty"`
	// Event specifies the event name if present.
	Event string `json:"event,omitempty"`
	// Status indicates operation outcome ("ok" or "error").
	Status string `json:"status"`
	// URL contains the target URL that was requested.
	URL string `json:"url"`
	// Error contains an error description if navigation failed.
	Error string `json:"error,omitempty"`
}

// ScreenshotResponse represents an error response when screenshot capture fails.
type ScreenshotResponse struct {
	// Action is always "getscreenshot_response".
	Action string `json:"action,omitempty"`
	// Event specifies the event name if present.
	Event string `json:"event,omitempty"`
	// Status indicates operation outcome ("ok" or "error").
	Status string `json:"status"`
	// Error contains an error description if screenshot capture failed.
	Error string `json:"error,omitempty"`
}

// StatusResponse represents the response containing display telemetry or error details.
type StatusResponse struct {
	// Action is "getstatus_response" for action-based responses.
	Action string `json:"action,omitempty"`
	// Event is "display:getstatus_response" for event-based responses.
	Event string `json:"event,omitempty"`
	// Status indicates operation outcome ("ok" or "error").
	Status string `json:"status"`
	// Data contains the telemetry dashboard data when status is "ok".
	Data *rest.DashboardData `json:"data,omitempty"`
	// Error contains an error description if status retrieval failed.
	Error string `json:"error,omitempty"`
}

// ScreenshotOptions configures optional parameters for screenshot capture requests.
type ScreenshotOptions struct {
	// Format is the image format ("jpeg" or "png", default: "jpeg").
	Format string
	// Quality is the JPEG compression quality (1-100, default: 85).
	Quality int
}

// ScreenshotResult contains the captured screenshot image binary data and metadata.
type ScreenshotResult struct {
	// Filename is the generated or received filename of the screenshot.
	Filename string
	// MimeType is the MIME type (e.g. "image/jpeg" or "image/png").
	MimeType string
	// Data contains the binary image bytes.
	Data []byte
}
