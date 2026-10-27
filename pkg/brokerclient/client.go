// Package brokerclient provides a high-level client library for remotely controlling
// browser displays connected via the WSBroker WebSocket infrastructure.
// Callers can issue navigation commands, retrieve active URLs, and capture
// resolution-bounded screenshots synchronously or asynchronously.
package brokerclient

import (
	"context"
	"sync"

	"emperror.dev/errors"
	"github.com/je4/utils/v2/pkg/zLogger"
	rawclient "github.com/je4/wsbrokerclient/pkg/client"
	"github.com/je4/wsbrokerclient/pkg/event"
	"github.com/je4/wsbrokerdisplay/pkg/rest"
)

// BrokerClient abstracts the underlying WSBroker client transport operations.
// Both *rawclient.Client and test mock implementations satisfy this interface.
type BrokerClient interface {
	// ClientName returns the configured unique identifier of the WSBroker client.
	ClientName() string
	// Send transmits an event envelope over the WebSocket connection.
	Send(ctx context.Context, evt *event.Event) error
	// Request sends an event and waits synchronously for a correlated response matching evt.ID / evt.Token.
	Request(ctx context.Context, evt *event.Event) (*event.Event, error)
	// Request0 sends an event with a newly generated ID without waiting for any response.
	Request0(ctx context.Context, evt *event.Event) error
	// Request1 sends an event and waits synchronously for a single correlated response using RequestN.
	Request1(ctx context.Context, evt *event.Event) (*event.Event, error)
	// RequestN sends an event and streams up to numResults correlated responses into ch.
	RequestN(ctx context.Context, evt *event.Event, numResults int, ch chan *event.Event) error
	// Reply sends a correlated response message to originalEvt with the provided data payload.
	Reply(ctx context.Context, originalEvt *event.Event, data any) error
	// ReplyType sends a correlated response of a specific replyType to originalEvt with data payload.
	ReplyType(ctx context.Context, originalEvt *event.Event, replyType event.EventType, data any) error
	// SendMessage sends a payload to a target (channel or direct client).
	SendMessage(ctx context.Context, target string, data any) error
	// Broadcast sends a broadcast payload to a channel.
	Broadcast(ctx context.Context, channel string, data any) error
	// SendDirect sends a direct message to a named client.
	SendDirect(ctx context.Context, targetClient string, data any) error
	// OnEvent registers a handler callback for all incoming events.
	OnEvent(handler rawclient.EventHandler) rawclient.UnsubscribeFunc
	// OnMessage registers a handler callback for incoming TypeMessage events.
	OnMessage(handler rawclient.EventHandler) rawclient.UnsubscribeFunc
	// OnDirect registers a handler callback for events addressed directly to this client.
	OnDirect(handler rawclient.EventHandler) rawclient.UnsubscribeFunc
	// OnChannel registers a handler callback for events broadcast to a channel.
	OnChannel(channel string, handler rawclient.EventHandler) rawclient.UnsubscribeFunc
	// Connect connects to the WSBroker server asynchronously.
	Connect(ctx context.Context) error
	// ConnectSync connects to the WSBroker server synchronously.
	ConnectSync(ctx context.Context) error
	// Close gracefully closes the client connection.
	Close() error
}

// Client is a high-level remote browser control client wrapping BrokerClient.
// It leverages wsbrokerclient's native Request/Reply capabilities to provide
// synchronous and asynchronous APIs for controlling display browsers.
type Client struct {
	raw    BrokerClient
	logger zLogger.ZLogger
	mu     sync.RWMutex
	closed bool
}

// New creates a new remote browser control Client instance wrapping an existing WSBroker client.
// Returns an error if the raw client is nil.
func New(raw BrokerClient, logger zLogger.ZLogger) (*Client, error) {
	if raw == nil {
		return nil, errors.New("raw wsbroker client cannot be nil")
	}

	return &Client{
		raw:    raw,
		logger: logger,
	}, nil
}

// RawClient returns the underlying raw WSBroker client instance.
func (c *Client) RawClient() BrokerClient {
	return c.raw
}

// ClientName returns the configured unique identifier of the underlying WSBroker client.
func (c *Client) ClientName() string {
	return c.raw.ClientName()
}

// Connect establishes the WebSocket connection to the broker asynchronously.
func (c *Client) Connect(ctx context.Context) error {
	return c.raw.Connect(ctx)
}

// ConnectSync establishes the WebSocket connection to the broker synchronously.
func (c *Client) ConnectSync(ctx context.Context) error {
	return c.raw.ConnectSync(ctx)
}

// Close gracefully closes the client and the underlying broker connection.
func (c *Client) Close() error {
	c.mu.Lock()
	if c.closed {
		c.mu.Unlock()
		return nil
	}
	c.closed = true
	c.mu.Unlock()

	return c.raw.Close()
}

// Send transmits an event envelope over the WebSocket connection.
func (c *Client) Send(ctx context.Context, evt *event.Event) error {
	return c.raw.Send(ctx, evt)
}

// Request sends an event envelope and waits synchronously for a correlated response matching evt.ID / evt.Token.
func (c *Client) Request(ctx context.Context, evt *event.Event) (*event.Event, error) {
	return c.raw.Request(ctx, evt)
}

// Request0 sends an event envelope without waiting for any response.
func (c *Client) Request0(ctx context.Context, evt *event.Event) error {
	return c.raw.Request0(ctx, evt)
}

// Request1 sends an event envelope and waits synchronously for a single correlated response.
func (c *Client) Request1(ctx context.Context, evt *event.Event) (*event.Event, error) {
	return c.raw.Request1(ctx, evt)
}

// RequestN sends an event envelope and streams up to numResults correlated responses into ch.
func (c *Client) RequestN(ctx context.Context, evt *event.Event, numResults int, ch chan *event.Event) error {
	return c.raw.RequestN(ctx, evt, numResults, ch)
}

// Reply sends a correlated TypeMessage response event to originalEvt carrying data payload.
func (c *Client) Reply(ctx context.Context, originalEvt *event.Event, data any) error {
	return c.raw.Reply(ctx, originalEvt, data)
}

// ReplyType sends a correlated response event of the specified replyType to originalEvt.
func (c *Client) ReplyType(ctx context.Context, originalEvt *event.Event, replyType event.EventType, data any) error {
	return c.raw.ReplyType(ctx, originalEvt, replyType, data)
}

// SendMessage sends a payload to a target (channel or direct client).
func (c *Client) SendMessage(ctx context.Context, target string, data any) error {
	return c.raw.SendMessage(ctx, target, data)
}

// Broadcast sends a broadcast payload to a channel.
func (c *Client) Broadcast(ctx context.Context, channel string, data any) error {
	return c.raw.Broadcast(ctx, channel, data)
}

// SendDirect sends a direct message to a named client.
func (c *Client) SendDirect(ctx context.Context, targetClient string, data any) error {
	return c.raw.SendDirect(ctx, targetClient, data)
}

// OnEvent registers a listener callback for all incoming events.
func (c *Client) OnEvent(handler func(evt *event.Event)) rawclient.UnsubscribeFunc {
	return c.raw.OnEvent(handler)
}

// OnMessage registers a listener callback for incoming TypeMessage events.
func (c *Client) OnMessage(handler func(evt *event.Event)) rawclient.UnsubscribeFunc {
	return c.raw.OnMessage(handler)
}

// OnDirect registers a listener callback for events addressed directly to this client.
func (c *Client) OnDirect(handler func(evt *event.Event)) rawclient.UnsubscribeFunc {
	return c.raw.OnDirect(handler)
}

// OnChannel registers a listener callback for events broadcast to a channel.
func (c *Client) OnChannel(channel string, handler func(evt *event.Event)) rawclient.UnsubscribeFunc {
	return c.raw.OnChannel(channel, handler)
}

// Navigate sends a synchronous navigation command to the target display client and waits for acknowledgment.
// An optional waitSelector can be specified to wait until the DOM element is rendered.
func (c *Client) Navigate(ctx context.Context, target string, targetURL string, waitSelector ...string) (*NavigateResponse, error) {
	selector := ""
	if len(waitSelector) > 0 {
		selector = waitSelector[0]
	}

	cmd := CommandMessage{
		Action:   ActionNavigate,
		Event:    "display:navigate",
		URL:      targetURL,
		Selector: selector,
	}

	evt, err := event.NewMessageEvent(c.raw.ClientName(), target, cmd)
	if err != nil {
		return nil, errors.Wrap(err, "failed to create navigate command event")
	}

	respEvt, err := c.raw.Request(ctx, evt)
	if err != nil {
		return nil, errors.Wrap(err, "failed to send navigate command")
	}
	if respEvt == nil {
		return nil, errors.New("nil response received for navigate request")
	}

	var resp NavigateResponse
	if err := respEvt.UnmarshalData(&resp); err != nil {
		return nil, errors.Wrap(err, "failed to unmarshal navigate response")
	}
	if resp.Status != "ok" && resp.Error != "" {
		return &resp, errors.Errorf("navigate error: %s", resp.Error)
	}
	return &resp, nil
}

// GetURL queries the current primary page location URL of the target display client synchronously.
func (c *Client) GetURL(ctx context.Context, target string) (string, error) {
	cmd := CommandMessage{
		Action: ActionGetURL,
		Event:  "display:geturl",
	}

	evt, err := event.NewMessageEvent(c.raw.ClientName(), target, cmd)
	if err != nil {
		return "", errors.Wrap(err, "failed to create geturl command event")
	}

	respEvt, err := c.raw.Request(ctx, evt)
	if err != nil {
		return "", errors.Wrap(err, "failed to send geturl command")
	}
	if respEvt == nil {
		return "", errors.New("nil response received for geturl request")
	}

	var resp URLResponse
	if err := respEvt.UnmarshalData(&resp); err != nil {
		return "", errors.Wrap(err, "failed to unmarshal geturl response")
	}
	if resp.Error != "" {
		return "", errors.Errorf("geturl error: %s", resp.Error)
	}
	return resp.URL, nil
}

// GetScreenshot requests a bounded viewport screenshot from the target display client synchronously.
// The resulting image is constrained to a maximum resolution of 800x600 pixels.
func (c *Client) GetScreenshot(ctx context.Context, target string, opts ...ScreenshotOptions) (*ScreenshotResult, error) {
	opt := ScreenshotOptions{Format: "jpeg", Quality: 85}
	if len(opts) > 0 {
		if opts[0].Format != "" {
			opt.Format = opts[0].Format
		}
		if opts[0].Quality > 0 {
			opt.Quality = opts[0].Quality
		}
	}

	cmd := CommandMessage{
		Action:  ActionGetScreenshot,
		Event:   "display:getscreenshot",
		Format:  opt.Format,
		Quality: opt.Quality,
	}

	evt, err := event.NewMessageEvent(c.raw.ClientName(), target, cmd)
	if err != nil {
		return nil, errors.Wrap(err, "failed to create getscreenshot command event")
	}

	respEvt, err := c.raw.Request(ctx, evt)
	if err != nil {
		return nil, errors.Wrap(err, "failed to send getscreenshot command")
	}
	if respEvt == nil {
		return nil, errors.New("nil response received for getscreenshot request")
	}

	if respEvt.Type == event.TypeFile {
		var fileMsg event.FileMessage
		if err := respEvt.UnmarshalData(&fileMsg); err != nil {
			return nil, errors.Wrap(err, "failed to unmarshal screenshot file message")
		}
		return &ScreenshotResult{
			Filename: fileMsg.Filename,
			MimeType: fileMsg.MimeType,
			Data:     fileMsg.Data,
		}, nil
	}

	// Check if an error message response was returned
	var resp ScreenshotResponse
	if err := respEvt.UnmarshalData(&resp); err == nil && resp.Error != "" {
		return nil, errors.Errorf("screenshot error: %s", resp.Error)
	}
	return nil, errors.New("unexpected response payload received for screenshot request")
}

// GetStatus queries the telemetry dashboard data of the target display client synchronously.
func (c *Client) GetStatus(ctx context.Context, target string) (*rest.DashboardData, error) {
	cmd := CommandMessage{
		Action: ActionGetStatus,
		Event:  "display:getstatus",
	}

	evt, err := event.NewMessageEvent(c.raw.ClientName(), target, cmd)
	if err != nil {
		return nil, errors.Wrap(err, "failed to create getstatus command event")
	}

	respEvt, err := c.raw.Request(ctx, evt)
	if err != nil {
		return nil, errors.Wrap(err, "failed to send getstatus command")
	}
	if respEvt == nil {
		return nil, errors.New("nil response received for getstatus request")
	}

	var resp StatusResponse
	if err := respEvt.UnmarshalData(&resp); err != nil {
		return nil, errors.Wrap(err, "failed to unmarshal getstatus response")
	}
	if resp.Status != "ok" && resp.Error != "" {
		return nil, errors.Errorf("getstatus error: %s", resp.Error)
	}
	if resp.Error != "" {
		return nil, errors.Errorf("getstatus error: %s", resp.Error)
	}
	if resp.Data == nil {
		return nil, errors.New("getstatus response missing data payload")
	}
	return resp.Data, nil
}

// SendNavigate transmits an asynchronous (fire-and-forget) navigation command.
func (c *Client) SendNavigate(ctx context.Context, target string, targetURL string, waitSelector string, token ...string) error {
	cmd := CommandMessage{
		Action:   ActionNavigate,
		Event:    "display:navigate",
		URL:      targetURL,
		Selector: waitSelector,
	}
	evt, err := event.NewMessageEvent(c.raw.ClientName(), target, cmd)
	if err != nil {
		return errors.Wrap(err, "failed to create navigate command event")
	}
	if len(token) > 0 && token[0] != "" {
		evt.Token = token[0]
	}
	return c.raw.Request0(ctx, evt)
}

// SendGetURL transmits an asynchronous (fire-and-forget) geturl command.
func (c *Client) SendGetURL(ctx context.Context, target string, token ...string) error {
	cmd := CommandMessage{
		Action: ActionGetURL,
		Event:  "display:geturl",
	}
	evt, err := event.NewMessageEvent(c.raw.ClientName(), target, cmd)
	if err != nil {
		return errors.Wrap(err, "failed to create geturl command event")
	}
	if len(token) > 0 && token[0] != "" {
		evt.Token = token[0]
	}
	return c.raw.Request0(ctx, evt)
}

// SendGetScreenshot transmits an asynchronous (fire-and-forget) getscreenshot command.
func (c *Client) SendGetScreenshot(ctx context.Context, target string, format string, quality int, token ...string) error {
	cmd := CommandMessage{
		Action:  ActionGetScreenshot,
		Event:   "display:getscreenshot",
		Format:  format,
		Quality: quality,
	}
	evt, err := event.NewMessageEvent(c.raw.ClientName(), target, cmd)
	if err != nil {
		return errors.Wrap(err, "failed to create getscreenshot command event")
	}
	if len(token) > 0 && token[0] != "" {
		evt.Token = token[0]
	}
	return c.raw.Request0(ctx, evt)
}

// SendGetStatus transmits an asynchronous (fire-and-forget) getstatus command.
func (c *Client) SendGetStatus(ctx context.Context, target string, token ...string) error {
	cmd := CommandMessage{
		Action: ActionGetStatus,
		Event:  "display:getstatus",
	}
	evt, err := event.NewMessageEvent(c.raw.ClientName(), target, cmd)
	if err != nil {
		return errors.Wrap(err, "failed to create getstatus command event")
	}
	if len(token) > 0 && token[0] != "" {
		evt.Token = token[0]
	}
	return c.raw.Request0(ctx, evt)
}
