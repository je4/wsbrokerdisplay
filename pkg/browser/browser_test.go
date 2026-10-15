package browser

import (
	"bytes"
	"context"
	"image"
	"io"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/je4/utils/v2/pkg/zLogger"
	"github.com/rs/zerolog"
)

func newTestLogger() zLogger.ZLogger {
	loggerZero := zerolog.New(io.Discard).With().Timestamp().Logger()
	var logger zLogger.ZLogger = &loggerZero
	return logger
}

func TestDefaultConfig(t *testing.T) {
	conf := DefaultConfig()
	if !conf.Headless {
		t.Errorf("expected Headless to be true by default")
	}
	if conf.Kiosk {
		t.Errorf("expected Kiosk to be false by default")
	}
	if conf.WindowWidth != 1920 || conf.WindowHeight != 1080 {
		t.Errorf("expected 1920x1080 window size, got %dx%d", conf.WindowWidth, conf.WindowHeight)
	}
	if !conf.IgnoreCertErrors {
		t.Errorf("expected IgnoreCertErrors to be true by default")
	}
	if conf.Timeout != 30*time.Second {
		t.Errorf("expected Timeout 30s, got %v", conf.Timeout)
	}
}

func TestBrowserOperations(t *testing.T) {
	logger := newTestLogger()
	conf := DefaultConfig()
	conf.Headless = true

	b := New(conf, logger)
	if b.IsRunning() {
		t.Errorf("expected browser not to be running before Start")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	var consoleMessages []ConsoleMessage
	var consoleMu sync.Mutex
	b.SetConsoleHandler(func(msg ConsoleMessage) {
		consoleMu.Lock()
		defer consoleMu.Unlock()
		consoleMessages = append(consoleMessages, msg)
	})

	if err := b.Start(ctx); err != nil {
		t.Skipf("skipping test: chrome/chromium not available in environment: %v", err)
		return
	}
	defer func() {
		_ = b.Close()
	}()

	if !b.IsRunning() {
		t.Fatalf("expected browser to be running after Start")
	}

	htmlContent := `data:text/html,<!DOCTYPE html>
<html>
<head><title>Test Page</title></head>
<body>
	<h1 id="headline">Hello Browser</h1>
	<button id="btn" onclick="console.log('button clicked'); this.innerText = 'Clicked!';">Click Me</button>
</body>
</html>`

	if err := b.Navigate(ctx, htmlContent); err != nil {
		t.Fatalf("failed to navigate: %v", err)
	}

	// Test GetURL
	currentURL, err := b.GetURL(ctx)
	if err != nil {
		t.Fatalf("failed to get current URL: %v", err)
	}
	if !strings.HasPrefix(currentURL, "data:text/html") {
		t.Errorf("expected URL to start with data:text/html, got '%s'", currentURL)
	}

	// Test EvaluateGeneric
	title, err := EvaluateGeneric[string](ctx, b, `document.title`)
	if err != nil {
		t.Fatalf("failed to evaluate document.title: %v", err)
	}
	if title != "Test Page" {
		t.Errorf("expected title 'Test Page', got '%s'", title)
	}

	// Test JavaScript calculation
	sum, err := EvaluateGeneric[int](ctx, b, `40 + 2`)
	if err != nil {
		t.Fatalf("failed to evaluate calculation: %v", err)
	}
	if sum != 42 {
		t.Errorf("expected sum 42, got %d", sum)
	}

	btnHTML, err := EvaluateGeneric[string](ctx, b, `document.getElementById('btn').outerHTML`)
	if err != nil {
		t.Fatalf("failed to evaluate button HTML: %v", err)
	}
	t.Logf("Button HTML: %s", btnHTML)

	// Test Click on element
	if err := b.Click(ctx, ClickOptions{
		Selector: "#btn",
	}); err != nil {
		t.Fatalf("failed to click button: %v", err)
	}

	// Verify DOM update after click
	btnText, err := EvaluateGeneric[string](ctx, b, `document.getElementById('btn').innerText`)
	if err != nil {
		t.Fatalf("failed to get button text: %v", err)
	}
	if btnText != "Clicked!" {
		t.Errorf("expected button text 'Clicked!', got '%s'", btnText)
	}

	// Verify console message received
	consoleMu.Lock()
	foundConsoleMsg := false
	for _, m := range consoleMessages {
		if strings.Contains(m.Message, "button clicked") {
			foundConsoleMsg = true
			break
		}
	}
	consoleMu.Unlock()
	if !foundConsoleMsg {
		t.Errorf("expected to capture 'button clicked' console message")
	}

	// Test PNG Screenshot
	pngData, mimeType, err := b.Screenshot(ctx, ScreenshotOptions{
		Format: "png",
	})
	if err != nil {
		t.Fatalf("failed to take PNG screenshot: %v", err)
	}
	if mimeType != "image/png" || len(pngData) == 0 {
		t.Errorf("invalid PNG screenshot result: mime=%s len=%d", mimeType, len(pngData))
	}

	// Test JPEG Screenshot with Resizing
	jpegData, mimeTypeJPEG, err := b.Screenshot(ctx, ScreenshotOptions{
		Format:       "jpeg",
		Quality:      80,
		Width:        320,
		Height:       240,
		SharpenSigma: 0.5,
	})
	if err != nil {
		t.Fatalf("failed to take resized JPEG screenshot: %v", err)
	}
	if mimeTypeJPEG != "image/jpeg" || len(jpegData) == 0 {
		t.Errorf("invalid JPEG screenshot result: mime=%s len=%d", mimeTypeJPEG, len(jpegData))
	}

	// Test Screenshot with MaxWidth and MaxHeight bounding (max 800x600)
	boundedData, mimeTypeBounded, err := b.Screenshot(ctx, ScreenshotOptions{
		Format:    "jpeg",
		Quality:   85,
		MaxWidth:  800,
		MaxHeight: 600,
	})
	if err != nil {
		t.Fatalf("failed to take bounded screenshot: %v", err)
	}
	if mimeTypeBounded != "image/jpeg" || len(boundedData) == 0 {
		t.Errorf("invalid bounded screenshot result: mime=%s len=%d", mimeTypeBounded, len(boundedData))
	}
	boundedImg, _, err := image.Decode(bytes.NewReader(boundedData))
	if err != nil {
		t.Fatalf("failed to decode bounded screenshot: %v", err)
	}
	bounds := boundedImg.Bounds()
	if bounds.Dx() > 800 || bounds.Dy() > 600 {
		t.Errorf("expected screenshot dimensions <= 800x600, got %dx%d", bounds.Dx(), bounds.Dy())
	}

	// Test Close
	if err := b.Close(); err != nil {
		t.Fatalf("failed to close browser: %v", err)
	}

	if b.IsRunning() {
		t.Errorf("expected browser not to be running after Close")
	}
}

func TestCustomUserDataDir(t *testing.T) {
	tempDir := t.TempDir()
	conf := DefaultConfig()
	conf.UserDataDir = tempDir

	logger := newTestLogger()
	b := New(conf, logger)

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	if err := b.Start(ctx); err != nil {
		t.Skipf("skipping test: chrome/chromium not available: %v", err)
		return
	}
	_ = b.Close()

	// Ensure custom UserDataDir was not deleted
	if _, err := os.Stat(tempDir); os.IsNotExist(err) {
		t.Errorf("custom UserDataDir should not be removed on close")
	}
}

func TestEvaluateFunction(t *testing.T) {
	logger := newTestLogger()
	conf := DefaultConfig()
	b := New(conf, logger)

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	if err := b.Start(ctx); err != nil {
		t.Skipf("skipping test: chrome not available: %v", err)
		return
	}
	defer func() {
		_ = b.Close()
	}()

	html := `data:text/html,<html><head><script>
		function greet(salutation, name) {
			return salutation + " " + name + "!";
		}
	</script></head><body></body></html>`

	if err := b.Navigate(ctx, html); err != nil {
		t.Fatalf("failed to navigate: %v", err)
	}

	greeting, err := EvaluateFunction[string](ctx, b, "greet", "Hello", "World")
	if err != nil {
		t.Fatalf("failed to evaluate function: %v", err)
	}
	if greeting != "Hello World!" {
		t.Errorf("expected 'Hello World!', got '%s'", greeting)
	}
}
