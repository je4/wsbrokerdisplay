# Remote Browser Control Client Library (`pkg/client`)

The `pkg/client` package provides a high-level Go client library / SDK for remotely controlling browser displays connected to the **WSBroker** WebSocket broker infrastructure.

External applications, dashboards, and automated control services can use this package to instruct display instances to navigate to web pages, inspect the current active URL, and capture resolution-bounded screenshots synchronously or asynchronously.

---

## Key Features

- **Dependency Inversion**: Wraps `BrokerClient` (implemented by `github.com/je4/wsbrokerclient/pkg/client.Client`), allowing full caller control over transport configuration, mTLS certloader setup, and connection lifecycle.
- **Synchronous Remote Control APIs**:
  - `Navigate(ctx, target, targetURL, waitSelector...)`: Navigates the target browser and blocks until acknowledgment is received.
  - `GetURL(ctx, target)`: Queries the currently rendered page URL on the target display.
  - `GetScreenshot(ctx, target, opts...)`: Requests and receives screenshot image data (JPEG or PNG) bounded to maximum 800x600 pixels.
- **Asynchronous / Fire-and-Forget Helpers**:
  - `SendNavigate(ctx, target, targetURL, waitSelector, token...)`
  - `SendGetURL(ctx, target, token...)`
  - `SendGetScreenshot(ctx, target, format, quality, token...)`
- **Automatic Correlation Tracking**: Generates unique request correlation tokens and routes responses back to caller wait channels.

---

## API Summary

### Constructor & Lifecycle
- `New(raw BrokerClient, logger zLogger.ZLogger) (*Client, error)`: Instantiates a client wrapping an existing WSBroker client.
- `Connect(ctx context.Context) error`: Connects to the broker asynchronously.
- `ConnectSync(ctx context.Context) error`: Connects to the broker synchronously.
- `Close() error`: Gracefully closes waiters, listeners, and the underlying connection.
- `RawClient() BrokerClient`: Returns the underlying WSBroker client.
- `ClientName() string`: Returns the client identifier.

### Remote Control Methods
- `Navigate(ctx context.Context, target string, targetURL string, waitSelector ...string) (*NavigateResponse, error)`
- `GetURL(ctx context.Context, target string) (string, error)`
- `GetScreenshot(ctx context.Context, target string, opts ...ScreenshotOptions) (*ScreenshotResult, error)`

### Asynchronous Senders
- `SendNavigate(ctx context.Context, target string, targetURL string, waitSelector string, token ...string) error`
- `SendGetURL(ctx context.Context, target string, token ...string) error`
- `SendGetScreenshot(ctx context.Context, target string, format string, quality int, token ...string) error`

---

## Data Models

```go
type CommandMessage struct {
    Action   string `json:"action"`             // "navigate", "getscreenshot", "geturl"
    URL      string `json:"url,omitempty"`      // Target URL for navigate
    Selector string `json:"selector,omitempty"` // CSS wait selector for navigate
    Format   string `json:"format,omitempty"`   // "jpeg" or "png"
    Quality  int    `json:"quality,omitempty"`  // JPEG quality (1-100)
}

type NavigateResponse struct {
    Action string `json:"action"` // "navigate_response"
    Status string `json:"status"` // "ok" or "error"
    URL    string `json:"url"`
    Error  string `json:"error,omitempty"`
}

type URLResponse struct {
    Action string `json:"action"` // "geturl_response"
    URL    string `json:"url"`
    Error  string `json:"error,omitempty"`
}

type ScreenshotResult struct {
    Filename string
    MimeType string
    Data     []byte
}
```

---

## Usage Example

```go
package main

import (
    "context"
    "fmt"
    "log"
    "time"

    rawclient "github.com/je4/wsbrokerclient/pkg/client"
    "github.com/je4/wsbrokerdisplay/pkg/client"
)

func main() {
    // 1. Initialize underlying WSBroker client
    rawCfg := rawclient.DefaultConfig()
    raw, err := rawclient.New(rawCfg,
        rawclient.WithEndpoint("https://localhost:8765"),
        rawclient.WithClientName("adminController"),
        rawclient.WithAutoReconnect(true),
        rawclient.WithLogger(logger),
    )
    if err != nil {
        log.Fatalf("failed to create raw client: %v", err)
    }

    // 2. Wrap with remote browser control client
    c, err := client.New(raw, logger)
    if err != nil {
        log.Fatalf("failed to create remote control client: %v", err)
    }

    if err := c.ConnectSync(context.Background()); err != nil {
        log.Fatalf("failed to connect to broker: %v", err)
    }
    defer c.Close()

    targetDisplay := "display01"
    ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
    defer cancel()

    // 3. Navigate display to a URL
    navResp, err := c.Navigate(ctx, targetDisplay, "https://example.com", "#content")
    if err != nil {
        log.Fatalf("navigation failed: %v", err)
    }
    fmt.Printf("Navigated display to %s (status: %s)\n", navResp.URL, navResp.Status)

    // 4. Query active URL
    activeURL, err := c.GetURL(ctx, targetDisplay)
    if err != nil {
        log.Fatalf("geturl failed: %v", err)
    }
    fmt.Printf("Active display URL: %s\n", activeURL)

    // 5. Capture bounded screenshot (max 800x600)
    shot, err := c.GetScreenshot(ctx, targetDisplay, client.ScreenshotOptions{
        Format:  "jpeg",
        Quality: 85,
    })
    if err != nil {
        log.Fatalf("screenshot failed: %v", err)
    }
    fmt.Printf("Received screenshot %s (%s, %d bytes)\n", shot.Filename, shot.MimeType, len(shot.Data))
}
```
