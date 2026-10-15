package client

import (
	"context"
	"encoding/json"
	"sync"
	"testing"
	"time"

	"github.com/je4/utils/v2/pkg/zLogger"
	rawclient "github.com/je4/wsbrokerclient/pkg/client"
	"github.com/je4/wsbrokerclient/pkg/event"
	"github.com/rs/zerolog"
)

func newTestLogger() zLogger.ZLogger {
	nop := zerolog.Nop()
	var logger zLogger.ZLogger = &nop
	return logger
}

type mockBrokerClient struct {
	name          string
	mu            sync.Mutex
	handlers      []rawclient.EventHandler
	sentEvents    []*event.Event
	onSend        func(ctx context.Context, evt *event.Event) error
	connectCalled bool
	closed        bool
}

func newMockBrokerClient(name string) *mockBrokerClient {
	return &mockBrokerClient{name: name}
}

func (m *mockBrokerClient) ClientName() string {
	return m.name
}

func (m *mockBrokerClient) Send(ctx context.Context, evt *event.Event) error {
	m.mu.Lock()
	m.sentEvents = append(m.sentEvents, evt)
	onSend := m.onSend
	m.mu.Unlock()

	if onSend != nil {
		return onSend(ctx, evt)
	}
	return nil
}

func (m *mockBrokerClient) OnEvent(handler rawclient.EventHandler) rawclient.UnsubscribeFunc {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.handlers = append(m.handlers, handler)
	idx := len(m.handlers) - 1
	return func() {
		m.mu.Lock()
		defer m.mu.Unlock()
		if idx < len(m.handlers) {
			m.handlers[idx] = nil
		}
	}
}

func (m *mockBrokerClient) OnMessage(handler rawclient.EventHandler) rawclient.UnsubscribeFunc {
	return m.OnEvent(handler)
}

func (m *mockBrokerClient) OnDirect(handler rawclient.EventHandler) rawclient.UnsubscribeFunc {
	return m.OnEvent(handler)
}

func (m *mockBrokerClient) OnChannel(channel string, handler rawclient.EventHandler) rawclient.UnsubscribeFunc {
	return m.OnEvent(handler)
}

func (m *mockBrokerClient) Connect(ctx context.Context) error {
	m.mu.Lock()
	m.connectCalled = true
	m.mu.Unlock()
	return nil
}

func (m *mockBrokerClient) ConnectSync(ctx context.Context) error {
	m.mu.Lock()
	m.connectCalled = true
	m.mu.Unlock()
	return nil
}

func (m *mockBrokerClient) Close() error {
	m.mu.Lock()
	m.closed = true
	m.mu.Unlock()
	return nil
}

func (m *mockBrokerClient) EmitEvent(evt *event.Event) {
	m.mu.Lock()
	handlers := make([]rawclient.EventHandler, len(m.handlers))
	copy(handlers, m.handlers)
	m.mu.Unlock()

	for _, h := range handlers {
		if h != nil {
			h(evt)
		}
	}
}

func TestClientNew(t *testing.T) {
	logger := newTestLogger()

	// 1. Verify New with nil raw client returns error
	c, err := New(nil, logger)
	if err == nil {
		t.Fatalf("expected error when raw client is nil, got nil")
	}
	if c != nil {
		t.Fatalf("expected nil client instance on error, got %+v", c)
	}

	// 2. Verify New with real raw client from wsbrokerclient
	rawCfg := rawclient.DefaultConfig()
	raw, err := rawclient.New(rawCfg, rawclient.WithClientName("controller01"), rawclient.WithLogger(logger))
	if err != nil {
		t.Fatalf("failed to create raw client: %v", err)
	}

	c, err = New(raw, logger)
	if err != nil {
		t.Fatalf("failed to create client: %v", err)
	}

	if c.ClientName() != "controller01" {
		t.Errorf("expected client name controller01, got %s", c.ClientName())
	}
	if c.RawClient() != raw {
		t.Errorf("expected RawClient() to match injected raw client")
	}
}

func TestCommandMessageSerialization(t *testing.T) {
	cmd := CommandMessage{
		Action:   ActionNavigate,
		URL:      "https://example.com/test",
		Selector: "#main-content",
		Format:   "png",
		Quality:  90,
	}

	data, err := json.Marshal(cmd)
	if err != nil {
		t.Fatalf("failed to marshal CommandMessage: %v", err)
	}

	var parsed CommandMessage
	if err := json.Unmarshal(data, &parsed); err != nil {
		t.Fatalf("failed to unmarshal CommandMessage: %v", err)
	}

	if parsed.Action != ActionNavigate || parsed.URL != "https://example.com/test" || parsed.Selector != "#main-content" {
		t.Errorf("unexpected parsed CommandMessage: %+v", parsed)
	}
	if parsed.Format != "png" || parsed.Quality != 90 {
		t.Errorf("unexpected format/quality: %s %d", parsed.Format, parsed.Quality)
	}
}

func TestResponseSerialization(t *testing.T) {
	// NavigateResponse
	navResp := NavigateResponse{
		Action: ActionNavigateResponse,
		Status: "ok",
		URL:    "https://example.com/page",
	}
	navData, err := json.Marshal(navResp)
	if err != nil {
		t.Fatalf("failed to marshal NavigateResponse: %v", err)
	}
	var parsedNav NavigateResponse
	if err := json.Unmarshal(navData, &parsedNav); err != nil {
		t.Fatalf("failed to unmarshal NavigateResponse: %v", err)
	}
	if parsedNav.Action != ActionNavigateResponse || parsedNav.Status != "ok" || parsedNav.URL != "https://example.com/page" {
		t.Errorf("unexpected NavigateResponse: %+v", parsedNav)
	}

	// URLResponse
	urlResp := URLResponse{
		Action: ActionGetURLResponse,
		URL:    "https://example.com/active",
	}
	urlData, err := json.Marshal(urlResp)
	if err != nil {
		t.Fatalf("failed to marshal URLResponse: %v", err)
	}
	var parsedURL URLResponse
	if err := json.Unmarshal(urlData, &parsedURL); err != nil {
		t.Fatalf("failed to unmarshal URLResponse: %v", err)
	}
	if parsedURL.Action != ActionGetURLResponse || parsedURL.URL != "https://example.com/active" {
		t.Errorf("unexpected URLResponse: %+v", parsedURL)
	}

	// ScreenshotResponse
	shotResp := ScreenshotResponse{
		Action: ActionScreenshotResponse,
		Status: "error",
		Error:  "context deadline exceeded",
	}
	shotData, err := json.Marshal(shotResp)
	if err != nil {
		t.Fatalf("failed to marshal ScreenshotResponse: %v", err)
	}
	var parsedShot ScreenshotResponse
	if err := json.Unmarshal(shotData, &parsedShot); err != nil {
		t.Fatalf("failed to unmarshal ScreenshotResponse: %v", err)
	}
	if parsedShot.Status != "error" || parsedShot.Error != "context deadline exceeded" {
		t.Errorf("unexpected ScreenshotResponse: %+v", parsedShot)
	}
}

func TestNavigateMethod(t *testing.T) {
	logger := newTestLogger()
	mock := newMockBrokerClient("controller01")

	c, err := New(mock, logger)
	if err != nil {
		t.Fatalf("failed to create client: %v", err)
	}

	mock.onSend = func(ctx context.Context, evt *event.Event) error {
		if evt != nil && evt.Token != "" {
			var cmd CommandMessage
			if err := evt.UnmarshalData(&cmd); err == nil && cmd.Action == ActionNavigate {
				resp := NavigateResponse{
					Action: ActionNavigateResponse,
					Status: "ok",
					URL:    cmd.URL,
				}
				respEvt, _ := event.NewMessageEvent("display01", evt.Source, resp)
				respEvt.Token = evt.Token
				go mock.EmitEvent(respEvt)
			}
		}
		return nil
	}

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	resp, err := c.Navigate(ctx, "display01", "https://example.com/welcome", "#app")
	if err != nil {
		t.Fatalf("Navigate failed: %v", err)
	}
	if resp.Status != "ok" || resp.URL != "https://example.com/welcome" {
		t.Errorf("unexpected NavigateResponse: %+v", resp)
	}

	mock.mu.Lock()
	defer mock.mu.Unlock()
	if len(mock.sentEvents) != 1 {
		t.Fatalf("expected 1 sent event, got %d", len(mock.sentEvents))
	}
	var sentCmd CommandMessage
	if err := mock.sentEvents[0].UnmarshalData(&sentCmd); err != nil {
		t.Fatalf("failed to unmarshal sent command: %v", err)
	}
	if sentCmd.URL != "https://example.com/welcome" || sentCmd.Selector != "#app" {
		t.Errorf("unexpected sent command content: %+v", sentCmd)
	}
}

func TestGetURLMethod(t *testing.T) {
	logger := newTestLogger()
	mock := newMockBrokerClient("controller01")

	c, err := New(mock, logger)
	if err != nil {
		t.Fatalf("failed to create client: %v", err)
	}

	mock.onSend = func(ctx context.Context, evt *event.Event) error {
		if evt != nil && evt.Token != "" {
			var cmd CommandMessage
			if err := evt.UnmarshalData(&cmd); err == nil && cmd.Action == ActionGetURL {
				resp := URLResponse{
					Action: ActionGetURLResponse,
					URL:    "https://example.com/current",
				}
				respEvt, _ := event.NewMessageEvent("display01", evt.Source, resp)
				respEvt.Token = evt.Token
				go mock.EmitEvent(respEvt)
			}
		}
		return nil
	}

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	url, err := c.GetURL(ctx, "display01")
	if err != nil {
		t.Fatalf("GetURL failed: %v", err)
	}
	if url != "https://example.com/current" {
		t.Errorf("expected URL https://example.com/current, got %s", url)
	}
}

func TestGetScreenshotMethod(t *testing.T) {
	logger := newTestLogger()
	mock := newMockBrokerClient("controller01")

	c, err := New(mock, logger)
	if err != nil {
		t.Fatalf("failed to create client: %v", err)
	}

	mock.onSend = func(ctx context.Context, evt *event.Event) error {
		if evt != nil && evt.Token != "" {
			var cmd CommandMessage
			if err := evt.UnmarshalData(&cmd); err == nil && cmd.Action == ActionGetScreenshot {
				fileMsg := event.NewFileMessage("shot.jpg", "image/jpeg", []byte("sample_screenshot_bytes"))
				respEvt, _ := event.NewFileEvent("display01", evt.Source, fileMsg, evt.Token)
				go mock.EmitEvent(respEvt)
			}
		}
		return nil
	}

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	res, err := c.GetScreenshot(ctx, "display01", ScreenshotOptions{Format: "jpeg", Quality: 85})
	if err != nil {
		t.Fatalf("GetScreenshot failed: %v", err)
	}
	if res.Filename != "shot.jpg" || res.MimeType != "image/jpeg" || string(res.Data) != "sample_screenshot_bytes" {
		t.Errorf("unexpected ScreenshotResult: %+v", res)
	}
}

func TestGetScreenshotErrorResponse(t *testing.T) {
	logger := newTestLogger()
	mock := newMockBrokerClient("controller01")

	c, err := New(mock, logger)
	if err != nil {
		t.Fatalf("failed to create client: %v", err)
	}

	mock.onSend = func(ctx context.Context, evt *event.Event) error {
		if evt != nil && evt.Token != "" {
			resp := ScreenshotResponse{
				Action: ActionScreenshotResponse,
				Status: "error",
				Error:  "browser renderer crashed",
			}
			respEvt, _ := event.NewMessageEvent("display01", evt.Source, resp)
			respEvt.Token = evt.Token
			go mock.EmitEvent(respEvt)
		}
		return nil
	}

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	_, err = c.GetScreenshot(ctx, "display01")
	if err == nil {
		t.Fatalf("expected error from failed screenshot, got nil")
	}
}

func TestAsyncSendMethods(t *testing.T) {
	logger := newTestLogger()
	mock := newMockBrokerClient("controller01")

	c, err := New(mock, logger)
	if err != nil {
		t.Fatalf("failed to create client: %v", err)
	}

	ctx := context.Background()

	// SendNavigate
	if err := c.SendNavigate(ctx, "display01", "https://example.com/async", "#content", "tok1"); err != nil {
		t.Errorf("SendNavigate failed: %v", err)
	}

	// SendGetURL
	if err := c.SendGetURL(ctx, "display01", "tok2"); err != nil {
		t.Errorf("SendGetURL failed: %v", err)
	}

	// SendGetScreenshot
	if err := c.SendGetScreenshot(ctx, "display01", "png", 90, "tok3"); err != nil {
		t.Errorf("SendGetScreenshot failed: %v", err)
	}

	mock.mu.Lock()
	defer mock.mu.Unlock()
	if len(mock.sentEvents) != 3 {
		t.Fatalf("expected 3 captured events, got %d", len(mock.sentEvents))
	}

	if mock.sentEvents[0].Token != "tok1" || mock.sentEvents[1].Token != "tok2" || mock.sentEvents[2].Token != "tok3" {
		t.Errorf("unexpected tokens in captured events")
	}
}

func TestClientClose(t *testing.T) {
	logger := newTestLogger()
	mock := newMockBrokerClient("controller01")

	c, err := New(mock, logger)
	if err != nil {
		t.Fatalf("failed to create client: %v", err)
	}

	token := "pending_tok"
	ch := c.registerWaiter(token)

	go func() {
		time.Sleep(50 * time.Millisecond)
		_ = c.Close()
	}()

	select {
	case evt, ok := <-ch:
		if ok && evt != nil {
			t.Errorf("expected channel close or nil on Client.Close()")
		}
	case <-time.After(1 * time.Second):
		t.Errorf("timeout waiting for waiter channel closure on Client.Close()")
	}

	// Second Close should be idempotent
	if err := c.Close(); err != nil {
		t.Errorf("idempotent Close failed: %v", err)
	}
	if !mock.closed {
		t.Errorf("expected underlying mock client to be closed")
	}
}
