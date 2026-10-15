// Package display coordinates incoming WSBroker events and browser execution.
// It subscribes to remote control commands (navigate, geturl, getscreenshot),
// invokes the corresponding browser operations via Chrome DevTools Protocol,
// and transmits structured responses or binary file events back to the caller.
package display

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"

	"emperror.dev/errors"
	"github.com/je4/utils/v2/pkg/zLogger"
	"github.com/je4/wsbrokerclient/pkg/client"
	"github.com/je4/wsbrokerclient/pkg/event"
	"github.com/je4/wsbrokerdisplay/pkg/browser"
)

// BrowserInterface represents browser actions required by the display coordinator.
// It abstracts browser navigation, page inspection, and screenshot operations.
type BrowserInterface interface {
	// Navigate loads the target URL and waits for DOM readiness.
	Navigate(ctx context.Context, targetURL string) error
	// NavigateWait loads the target URL and waits until waitSelector is ready.
	NavigateWait(ctx context.Context, targetURL string, waitSelector string) error
	// GetURL returns the active primary page location URL.
	GetURL(ctx context.Context) (string, error)
	// Screenshot captures the browser page based on the given ScreenshotOptions.
	Screenshot(ctx context.Context, opts browser.ScreenshotOptions) ([]byte, string, error)
}

// ClientInterface represents WSBroker client actions required by the display coordinator.
// It abstracts event subscriptions and event dispatching.
type ClientInterface interface {
	// ClientName returns the configured unique identifier of the WSBroker client.
	ClientName() string
	// OnMessage registers an event handler for incoming TypeMessage events.
	OnMessage(handler client.EventHandler) client.UnsubscribeFunc
	// Send transmits an event envelope over the WebSocket connection.
	Send(ctx context.Context, evt *event.Event) error
}

// Controller coordinates incoming WSBroker events and browser execution.
// It manages event subscriptions, deserializes command envelopes, dispatches
// operations to the browser controller, and returns structured responses.
type Controller struct {
	client       ClientInterface
	browser      BrowserInterface
	logger       zLogger.ZLogger
	timeout      time.Duration
	unsubscribes []client.UnsubscribeFunc
	mu           sync.Mutex
}

// Option configures the Controller.
type Option func(*Controller)

// WithTimeout sets a custom command execution timeout for browser actions.
func WithTimeout(d time.Duration) Option {
	return func(c *Controller) {
		if d > 0 {
			c.timeout = d
		}
	}
}

// NewController creates a new display Controller instance with dependencies and optional configuration options.
func NewController(client ClientInterface, browser BrowserInterface, logger zLogger.ZLogger, opts ...Option) *Controller {
	c := &Controller{
		client:  client,
		browser: browser,
		logger:  logger,
		timeout: 30 * time.Second,
	}
	for _, opt := range opts {
		opt(c)
	}
	return c
}

// Start registers event listeners on the WSBroker client to begin processing commands.
func (c *Controller) Start() {
	c.mu.Lock()
	defer c.mu.Unlock()

	unsub := c.client.OnMessage(c.HandleEvent)
	c.unsubscribes = append(c.unsubscribes, unsub)
	c.logger.Info().Msg("display controller event listeners registered")
}

// Stop unregisters all active event listeners and halts command processing.
func (c *Controller) Stop() {
	c.mu.Lock()
	defer c.mu.Unlock()

	for _, unsub := range c.unsubscribes {
		if unsub != nil {
			unsub()
		}
	}
	c.unsubscribes = nil
	c.logger.Info().Msg("display controller stopped")
}

// HandleEvent processes an incoming WSBroker event, parses command payloads, and routes actions.
func (c *Controller) HandleEvent(evt *event.Event) {
	if evt == nil {
		return
	}

	// Ignore events emitted by ourselves to prevent loops
	if evt.Source == c.client.ClientName() {
		return
	}

	var cmd CommandMessage
	if err := evt.UnmarshalData(&cmd); err != nil {
		c.logger.Warn().Err(err).Str("source", evt.Source).Msg("cannot unmarshal command payload")
		return
	}

	// Ignore empty actions or response messages sent over channels
	if cmd.Action == "" || strings.HasSuffix(cmd.Action, "_response") {
		return
	}

	target := evt.Source
	if target == "" {
		target = evt.Target
	}

	ctx, cancel := context.WithTimeout(context.Background(), c.timeout)
	defer cancel()

	switch cmd.Action {
	case ActionNavigate:
		c.handleNavigate(ctx, target, evt.Token, cmd)
	case ActionGetURL:
		c.handleGetURL(ctx, target, evt.Token)
	case ActionGetScreenshot:
		c.handleGetScreenshot(ctx, target, evt.Token, cmd)
	default:
		c.logger.Warn().Str("action", cmd.Action).Msg("unknown command action received")
		_ = c.sendResponse(ctx, target, NavigateResponse{
			Action: "error",
			Status: "error",
			Error:  fmt.Sprintf("unknown action '%s'", cmd.Action),
		}, evt.Token)
	}
}

// handleNavigate executes a browser navigation command and returns the navigation status.
func (c *Controller) handleNavigate(ctx context.Context, target, token string, cmd CommandMessage) {
	if cmd.URL == "" {
		_ = c.sendResponse(ctx, target, NavigateResponse{
			Action: ActionNavigateResponse,
			Status: "error",
			Error:  "missing URL parameter",
		}, token)
		return
	}

	var err error
	if cmd.Selector != "" {
		err = c.browser.NavigateWait(ctx, cmd.URL, cmd.Selector)
	} else {
		err = c.browser.Navigate(ctx, cmd.URL)
	}

	if err != nil {
		c.logger.Error().Err(err).Str("url", cmd.URL).Msg("failed to navigate")
		_ = c.sendResponse(ctx, target, NavigateResponse{
			Action: ActionNavigateResponse,
			Status: "error",
			URL:    cmd.URL,
			Error:  err.Error(),
		}, token)
		return
	}

	c.logger.Info().Str("url", cmd.URL).Msg("navigated successfully")
	_ = c.sendResponse(ctx, target, NavigateResponse{
		Action: ActionNavigateResponse,
		Status: "ok",
		URL:    cmd.URL,
	}, token)
}

// handleGetURL retrieves the current browser URL and transmits the response back to requester.
func (c *Controller) handleGetURL(ctx context.Context, target, token string) {
	currentURL, err := c.browser.GetURL(ctx)
	if err != nil {
		c.logger.Error().Err(err).Msg("failed to get current URL")
		_ = c.sendResponse(ctx, target, URLResponse{
			Action: ActionGetURLResponse,
			Error:  err.Error(),
		}, token)
		return
	}

	c.logger.Info().Str("url", currentURL).Msg("current URL retrieved")
	_ = c.sendResponse(ctx, target, URLResponse{
		Action: ActionGetURLResponse,
		URL:    currentURL,
	}, token)
}

// handleGetScreenshot captures a screenshot downscaled to fit within max 800x600 pixels
// and delivers the resulting image as a binary file event.
func (c *Controller) handleGetScreenshot(ctx context.Context, target, token string, cmd CommandMessage) {
	format := cmd.Format
	if format == "" {
		format = "jpeg"
	}
	quality := cmd.Quality
	if quality <= 0 {
		quality = 85
	}

	opts := browser.ScreenshotOptions{
		Format:    format,
		Quality:   quality,
		MaxWidth:  800,
		MaxHeight: 600,
	}

	data, mimeType, err := c.browser.Screenshot(ctx, opts)
	if err != nil {
		c.logger.Error().Err(err).Msg("failed to capture screenshot")
		_ = c.sendResponse(ctx, target, ScreenshotResponse{
			Action: ActionScreenshotResponse,
			Status: "error",
			Error:  err.Error(),
		}, token)
		return
	}

	ext := "jpg"
	if strings.Contains(mimeType, "png") {
		ext = "png"
	}
	filename := fmt.Sprintf("screenshot_%d.%s", time.Now().Unix(), ext)

	if err := c.sendFile(ctx, target, filename, mimeType, data, token); err != nil {
		c.logger.Error().Err(err).Msg("failed to send screenshot file event")
	} else {
		c.logger.Info().Str("filename", filename).Int("bytes", len(data)).Msg("screenshot sent successfully")
	}
}

// sendResponse transmits a structured response message to the target client or channel.
func (c *Controller) sendResponse(ctx context.Context, target string, response any, token ...string) error {
	evt, err := event.NewMessageEvent(c.client.ClientName(), target, response)
	if err != nil {
		return errors.Wrap(err, "failed to create message event")
	}
	if len(token) > 0 && token[0] != "" {
		evt.Token = token[0]
	}
	return c.client.Send(ctx, evt)
}

// sendFile transmits a binary file payload to the target.
func (c *Controller) sendFile(ctx context.Context, target string, filename, mimeType string, data []byte, token ...string) error {
	fileMsg := event.NewFileMessage(filename, mimeType, data)
	evt, err := event.NewFileEvent(c.client.ClientName(), target, fileMsg, token...)
	if err != nil {
		return errors.Wrap(err, "failed to create file event")
	}
	return c.client.Send(ctx, evt)
}
