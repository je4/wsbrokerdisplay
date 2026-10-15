package display

// Action constants for display commands and responses.
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
)

// CommandMessage represents an incoming command request payload received via WSBroker.
// It supports navigation, page URL retrieval, and bounded screenshot capture.
type CommandMessage struct {
	// Action specifies the operation to execute ("navigate", "getscreenshot", or "geturl").
	Action string `json:"action"`
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
	// Action is always "geturl_response".
	Action string `json:"action"`
	// URL contains the current primary browser location URL.
	URL string `json:"url"`
	// Error contains an error message if the URL inspection failed.
	Error string `json:"error,omitempty"`
}

// NavigateResponse represents the outgoing response to a "navigate" command.
type NavigateResponse struct {
	// Action is always "navigate_response".
	Action string `json:"action"`
	// Status indicates operation outcome ("ok" or "error").
	Status string `json:"status"`
	// URL contains the target URL that was requested.
	URL string `json:"url"`
	// Error contains an error description if navigation failed.
	Error string `json:"error,omitempty"`
}

// ScreenshotResponse represents an error or status response when screenshot capture fails.
type ScreenshotResponse struct {
	// Action is always "getscreenshot_response".
	Action string `json:"action"`
	// Status indicates operation outcome ("ok" or "error").
	Status string `json:"status"`
	// Error contains an error description if screenshot capture failed.
	Error string `json:"error,omitempty"`
}
