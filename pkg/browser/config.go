package browser

import "time"

// Config contains configuration options for initializing and running the browser instance.
type Config struct {
	// Headless determines whether Chrome/Chromium runs without a graphical user interface.
	Headless bool `toml:"headless"`

	// Kiosk enables full-screen kiosk mode for dedicated display environments.
	Kiosk bool `toml:"kiosk"`

	// WindowWidth specifies the initial viewport and window width in pixels.
	WindowWidth int `toml:"window_width"`

	// WindowHeight specifies the initial viewport and window height in pixels.
	WindowHeight int `toml:"window_height"`

	// UserDataDir specifies the profile/user data directory path.
	// If left empty, a temporary directory is created on startup and removed on close.
	UserDataDir string `toml:"user_data_dir"`

	// IgnoreCertErrors allows ignoring TLS certificate validation errors (useful for development/self-signed certs).
	IgnoreCertErrors bool `toml:"ignore_cert_errors"`

	// Timeout specifies the default timeout duration for browser operations.
	Timeout time.Duration `toml:"timeout"`

	// ExecPath allows providing a custom path to the Chrome/Chromium executable binary.
	ExecPath string `toml:"exec_path"`

	// CustomFlags allows passing additional custom command-line flags to the Chrome process.
	CustomFlags []string `toml:"custom_flags"`
}

// DefaultConfig returns a Config struct populated with recommended default settings.
func DefaultConfig() Config {
	return Config{
		Headless:         true,
		Kiosk:            false,
		WindowWidth:      1920,
		WindowHeight:     1080,
		IgnoreCertErrors: true,
		Timeout:          30 * time.Second,
	}
}
