package browser

import "time"

// ConsoleMessage represents a browser console log event captured from JavaScript execution.
type ConsoleMessage struct {
	// Level indicates the severity level of the console entry (e.g. "log", "warn", "error", "info").
	Level string `json:"level"`
	// Message contains the concatenated text arguments of the console call.
	Message string `json:"message"`
	// Timestamp records the time when the console event occurred.
	Timestamp time.Time `json:"timestamp"`
}

// ConsoleHandler is a callback function invoked when a console message is received from the browser.
type ConsoleHandler func(msg ConsoleMessage)

// SetConsoleHandler registers a callback function to receive browser console events in real-time.
func (b *Browser) SetConsoleHandler(handler ConsoleHandler) {
	b.handlerMu.Lock()
	defer b.handlerMu.Unlock()
	b.onConsole = handler
}
