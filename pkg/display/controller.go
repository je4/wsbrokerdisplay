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
	"github.com/je4/wsbrokerdisplay/pkg/rest"
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
	client         ClientInterface
	browser        BrowserInterface
	logger         zLogger.ZLogger
	timeout        time.Duration
	unsubscribes   []client.UnsubscribeFunc
	onSend         func(evt *event.Event)
	statusProvider func() rest.DashboardData
	mu             sync.Mutex
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

// WithOnSend registers a callback invoked whenever the controller sends an outgoing event.
func WithOnSend(fn func(evt *event.Event)) Option {
	return func(c *Controller) {
		c.onSend = fn
	}
}

// WithStatusProvider sets a callback for providing live display telemetry.
func WithStatusProvider(fn func() rest.DashboardData) Option {
	return func(c *Controller) {
		c.statusProvider = fn
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

	eventName := cmd.Event
	if eventName == "" {
		eventName = cmd.Action
	}

	// Ignore empty actions or response messages sent over channels
	if eventName == "" || strings.HasSuffix(eventName, "_response") {
		return
	}

	target := evt.Source
	if target == "" {
		target = evt.Target
	}

	ctx, cancel := context.WithTimeout(context.Background(), c.timeout)
	defer cancel()

	switch eventName {
	case EventNavigate, "navigate":
		c.handleNavigate(ctx, evt, cmd)
	case EventGetURL, "geturl":
		c.handleGetURL(ctx, evt)
	case EventGetScreenshot, "getscreenshot":
		c.handleGetScreenshot(ctx, evt, cmd)
	case EventGetStatus, "getstatus":
		c.handleGetStatus(ctx, evt)
	default:
		c.logger.Warn().Str("Event", eventName).Msg("unknown command Event received")
		_ = c.sendResponse(ctx, evt, NavigateResponse{
			Event:  "error",
			Status: "error",
			Error:  fmt.Sprintf("unknown Event '%s'", eventName),
		})
	}
}

// handleGetStatus retrieves the live display telemetry from the status provider,
// strips the message log, and transmits the response back to the requester.
func (c *Controller) handleGetStatus(ctx context.Context, origEvt *event.Event) {
	if c.statusProvider == nil {
		c.logger.Warn().Msg("status provider not configured")
		_ = c.sendResponse(ctx, origEvt, StatusResponse{
			Event:  EventGetStatusResponse,
			Status: "error",
			Error:  "status provider not configured",
		})
		return
	}

	data := c.statusProvider()
	data.Messages = nil // explicitly strip message log to keep payload lightweight

	c.logger.Info().Str("client", data.ClientName).Msg("status retrieved successfully")
	_ = c.sendResponse(ctx, origEvt, StatusResponse{
		Event:  EventGetStatusResponse,
		Status: "ok",
		Data:   &data,
	})
}

// handleNavigate executes a browser navigation command and returns the navigation status.
func (c *Controller) handleNavigate(ctx context.Context, origEvt *event.Event, cmd CommandMessage) {
	if cmd.URL == "" {
		_ = c.sendResponse(ctx, origEvt, NavigateResponse{
			Event:  EventNavigateResponse,
			Status: "error",
			Error:  "missing URL parameter",
		})
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
		_ = c.sendResponse(ctx, origEvt, NavigateResponse{
			Event:  EventNavigateResponse,
			Status: "error",
			URL:    cmd.URL,
			Error:  err.Error(),
		})
		return
	}

	c.logger.Info().Str("url", cmd.URL).Msg("navigated successfully")
	_ = c.sendResponse(ctx, origEvt, NavigateResponse{
		Event:  EventNavigateResponse,
		Status: "ok",
		URL:    cmd.URL,
	})
}

// handleGetURL retrieves the current browser URL and transmits the response back to requester.
func (c *Controller) handleGetURL(ctx context.Context, origEvt *event.Event) {
	currentURL, err := c.browser.GetURL(ctx)
	if err != nil {
		c.logger.Error().Err(err).Msg("failed to get current URL")
		_ = c.sendResponse(ctx, origEvt, URLResponse{
			Event: EventGetURLResponse,
			Error: err.Error(),
		})
		return
	}

	c.logger.Info().Str("url", currentURL).Msg("current URL retrieved")
	_ = c.sendResponse(ctx, origEvt, URLResponse{
		Event: EventGetURLResponse,
		URL:   currentURL,
	})
}

// handleGetScreenshot captures a screenshot downscaled to fit within max 800x600 pixels
// and delivers the resulting image as a binary file event.
func (c *Controller) handleGetScreenshot(ctx context.Context, origEvt *event.Event, cmd CommandMessage) {
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
		_ = c.sendResponse(ctx, origEvt, ScreenshotResponse{
			Event:  EventScreenshotResponse,
			Status: "error",
			Error:  err.Error(),
		})
		return
	}

	ext := "jpg"
	if strings.Contains(mimeType, "png") {
		ext = "png"
	}
	filename := fmt.Sprintf("screenshot_%d.%s", time.Now().Unix(), ext)

	if err := c.sendFile(ctx, origEvt, filename, mimeType, data); err != nil {
		c.logger.Error().Err(err).Msg("failed to send screenshot file event")
	} else {
		c.logger.Info().Str("filename", filename).Int("bytes", len(data)).Msg("screenshot sent successfully")
	}
}

// sendResponse transmits a structured response message to the target client or channel.
func (c *Controller) sendResponse(ctx context.Context, origEvt *event.Event, response any) error {
	var replyEvt *event.Event
	var err error
	if origEvt != nil {
		replyEvt, err = origEvt.CreateReply(event.TypeMessage, response)
		if err != nil {
			return errors.Wrap(err, "failed to create message reply event")
		}
	} else {
		replyEvt, err = event.NewMessageEvent(c.client.ClientName(), "", response)
		if err != nil {
			return errors.Wrap(err, "failed to create message event")
		}
	}
	replyEvt.Source = c.client.ClientName()
	if c.onSend != nil {
		c.onSend(replyEvt)
	}
	return c.client.Send(ctx, replyEvt)
}

// sendFile transmits a binary file payload to the target.
func (c *Controller) sendFile(ctx context.Context, origEvt *event.Event, filename, mimeType string, data []byte) error {
	fileMsg := event.NewFileMessage(filename, mimeType, data)
	var replyEvt *event.Event
	var err error
	if origEvt != nil {
		replyEvt, err = origEvt.CreateReply(event.TypeFile, fileMsg)
		if err != nil {
			return errors.Wrap(err, "failed to create file reply event")
		}
	} else {
		replyEvt, err = event.NewFileEvent(c.client.ClientName(), "", fileMsg)
		if err != nil {
			return errors.Wrap(err, "failed to create file event")
		}
	}
	replyEvt.Source = c.client.ClientName()
	if c.onSend != nil {
		c.onSend(replyEvt)
	}
	return c.client.Send(ctx, replyEvt)
}
