package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadConfigDefault(t *testing.T) {
	// Change working directory to project root so relative certificate paths in display.toml resolve correctly
	cwd, err := os.Getwd()
	if err != nil {
		t.Fatalf("failed to get current working directory: %v", err)
	}
	if err := os.Chdir("../.."); err != nil {
		t.Fatalf("failed to change directory to project root: %v", err)
	}
	t.Cleanup(func() {
		_ = os.Chdir(cwd)
	})

	conf, err := LoadConfig("")
	if err != nil {
		t.Fatalf("failed to load default embedded config: %v", err)
	}

	if conf.Addr != "localhost:8080" && conf.Addr != "localhost:8760" {
		t.Errorf("expected Addr 'localhost:8760' or 'localhost:8080', got '%s'", conf.Addr)
	}

	if conf.ExtAddr != "http://localhost:8080" && conf.ExtAddr != "http://localhost:8760" {
		t.Errorf("expected ExtAddr 'http://localhost:8760' or 'http://localhost:8080', got '%s'", conf.ExtAddr)
	}

	if conf.Name != "display01" {
		t.Errorf("expected Name 'display01', got '%s'", conf.Name)
	}

	if conf.WSBroker != "https://localhost:8765" {
		t.Errorf("expected WSBroker 'https://localhost:8765', got '%s'", conf.WSBroker)
	}

	if !conf.Browser.Headless {
		t.Errorf("expected Browser.Headless to be true, got false")
	}

	if conf.Browser.WindowWidth != 1920 || conf.Browser.WindowHeight != 1080 {
		t.Errorf("expected Browser window 1920x1080, got %dx%d", conf.Browser.WindowWidth, conf.Browser.WindowHeight)
	}

	expectedChannels := []string{"all", "intern", "display"}
	if len(conf.Channels) != len(expectedChannels) {
		t.Fatalf("expected %d channels, got %d", len(expectedChannels), len(conf.Channels))
	}
	for i, ch := range expectedChannels {
		if conf.Channels[i] != ch {
			t.Errorf("expected channel[%d] '%s', got '%s'", i, ch, conf.Channels[i])
		}
	}

	if conf.ClientTLS.Type != "minivault" {
		t.Errorf("expected ClientTLS.Type 'minivault', got '%s'", conf.ClientTLS.Type)
	}

	if conf.ClientTLS.Vault == nil {
		t.Fatalf("expected ClientTLS.Vault to be non-nil")
	}

	if conf.ClientTLS.Vault.BaseURL != "https://localhost:8759/api/v1" {
		t.Errorf("expected BaseURL 'https://localhost:8759/api/v1', got '%s'", conf.ClientTLS.Vault.BaseURL)
	}

	if conf.ClientTLS.Vault.ParentToken != "pa.22f09dfd1cbfbe9f.c17b29cb2b730533" {
		t.Errorf("expected ParentToken 'pa.22f09dfd1cbfbe9f.c17b29cb2b730533', got '%s'", conf.ClientTLS.Vault.ParentToken)
	}

	if conf.WebTLS.Type != "dev" {
		t.Errorf("expected WebTLS.Type 'dev', got '%s'", conf.WebTLS.Type)
	}
}

func TestLoadConfigFile(t *testing.T) {
	tempDir := t.TempDir()
	configPath := filepath.Join(tempDir, "custom.toml")

	tomlContent := `
addr = "127.0.0.1:9090"
extaddr = "http://127.0.0.1:9090"
name = "custom_display"
wsbroker = "wss://broker.example.com:443"
channels = ["alerts", "status"]

[browser]
headless = false
kiosk = true
window_width = 1280
window_height = 720

[clienttls]
type = "dev"

[webtls]
type = "file"
`
	if err := os.WriteFile(configPath, []byte(tomlContent), 0644); err != nil {
		t.Fatalf("failed to write test config file: %v", err)
	}

	conf, err := LoadConfig(configPath)
	if err != nil {
		t.Fatalf("failed to load custom config: %v", err)
	}

	if conf.Addr != "127.0.0.1:9090" {
		t.Errorf("expected Addr '127.0.0.1:9090', got '%s'", conf.Addr)
	}

	if conf.ExtAddr != "http://127.0.0.1:9090" {
		t.Errorf("expected ExtAddr 'http://127.0.0.1:9090', got '%s'", conf.ExtAddr)
	}

	if conf.Name != "custom_display" {
		t.Errorf("expected Name 'custom_display', got '%s'", conf.Name)
	}

	if conf.WSBroker != "wss://broker.example.com:443" {
		t.Errorf("expected WSBroker 'wss://broker.example.com:443', got '%s'", conf.WSBroker)
	}

	if len(conf.Channels) != 2 || conf.Channels[0] != "alerts" || conf.Channels[1] != "status" {
		t.Errorf("unexpected channels: %v", conf.Channels)
	}

	if conf.Browser.Headless {
		t.Errorf("expected Browser.Headless false, got true")
	}

	if !conf.Browser.Kiosk {
		t.Errorf("expected Browser.Kiosk true, got false")
	}

	if conf.Browser.WindowWidth != 1280 || conf.Browser.WindowHeight != 720 {
		t.Errorf("expected Browser window 1280x720, got %dx%d", conf.Browser.WindowWidth, conf.Browser.WindowHeight)
	}

	if conf.ClientTLS.Type != "dev" {
		t.Errorf("expected ClientTLS.Type 'dev', got '%s'", conf.ClientTLS.Type)
	}

	if conf.WebTLS.Type != "file" {
		t.Errorf("expected WebTLS.Type 'file', got '%s'", conf.WebTLS.Type)
	}
}

func TestLoadConfigFileNotFound(t *testing.T) {
	_, err := LoadConfig("non_existent_config.toml")
	if err == nil {
		t.Errorf("expected error when loading non-existent config file, got nil")
	}
}
