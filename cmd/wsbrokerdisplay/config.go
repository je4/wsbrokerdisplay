package main

import (
	"emperror.dev/errors"
	"github.com/BurntSushi/toml"
	"github.com/je4/wsbrokerdisplay/config"
	"github.com/je4/wsbrokerdisplay/pkg/browser"
	"go.ub.unibas.ch/cloud/certloader/v2/pkg/loader"
)

// DisplayConfig defines configuration parameters for the wsbrokerdisplay application.
//
// Fields:
//   - Addr: Local listen address for the HTTP REST service (e.g. "localhost:8080").
//   - ExtAddr: External address for Swagger/documentation and links (e.g. "http://localhost:8080").
//   - Name: Unique client identifier (e.g. "display01") used for wsbrokerclient identity and authorization.
//   - WSBroker: Target broker endpoint URL (e.g. "https://localhost:8765").
//   - Channels: Initial list of channels to subscribe to on startup (e.g. ["all", "intern", "display"]).
//   - Browser: Chromium browser engine settings (headless, kiosk, viewport dimensions, etc.).
//   - ClientTLS: mTLS configuration via certloader (MiniVault, File, Env, Dev, or ACME).
//   - WebTLS: TLS configuration for the local web display interface.
type DisplayConfig struct {
	Addr      string         `toml:"addr"`
	ExtAddr   string         `toml:"extaddr"`
	Name      string         `toml:"name"`
	WSBroker  string         `toml:"wsbroker"`
	Channels  []string       `toml:"channels"`
	Browser   browser.Config `toml:"browser"`
	ClientTLS loader.Config  `toml:"clienttls"`
	WebTLS    loader.Config  `toml:"webtls"`
}

// Config is an alias for DisplayConfig.
type Config = DisplayConfig

// LoadConfig loads the DisplayConfig from the specified TOML file path, or falls back
// to the embedded default configuration (config/display.toml) if path is empty.
func LoadConfig(path string) (*DisplayConfig, error) {
	conf := &DisplayConfig{}
	if path == "" {
		if _, err := toml.Decode(config.DisplayConfigTOML, conf); err != nil {
			return nil, errors.Wrap(err, "cannot decode embedded default config")
		}
		return conf, nil
	}
	if _, err := toml.DecodeFile(path, conf); err != nil {
		return nil, errors.Wrapf(err, "cannot decode toml file '%s'", path)
	}
	return conf, nil
}
