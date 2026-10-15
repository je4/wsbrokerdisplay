package display

import (
	"context"
	"io"
	"sync"
	"testing"

	"emperror.dev/errors"
	"github.com/je4/utils/v2/pkg/zLogger"
	"github.com/je4/wsbrokerclient/pkg/client"
	"github.com/je4/wsbrokerclient/pkg/event"
	"github.com/je4/wsbrokerdisplay/pkg/browser"
	"github.com/rs/zerolog"
)

func newTestLogger() zLogger.ZLogger {
	loggerZero := zerolog.New(io.Discard).With().Timestamp().Logger()
	var logger zLogger.ZLogger = &loggerZero
	return logger
}

type mockClient struct {
	name       string
	mu         sync.Mutex
	handlers   []client.EventHandler
	sentEvents []*event.Event
}

func newMockClient(name string) *mockClient {
	return &mockClient{name: name}
}

func (m *mockClient) ClientName() string {
	return m.name
}

func (m *mockClient) OnMessage(handler client.EventHandler) client.UnsubscribeFunc {
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

func (m *mockClient) Send(ctx context.Context, evt *event.Event) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.sentEvents = append(m.sentEvents, evt)
	return nil
}

func (m *mockClient) EmitMessage(evt *event.Event) {
	m.mu.Lock()
	handlers := make([]client.EventHandler, len(m.handlers))
	copy(handlers, m.handlers)
	m.mu.Unlock()

	for _, h := range handlers {
		if h != nil {
			h(evt)
		}
	}
}

type mockBrowser struct {
	mu              sync.Mutex
	currentURL      string
	navigateErr     error
	navigateWaitErr error
	getURLErr       error
	screenshotErr   error
	lastNavigate    string
	lastWaitURL     string
	lastWaitSel     string
	lastOpts        browser.ScreenshotOptions
}

func (mb *mockBrowser) Navigate(ctx context.Context, targetURL string) error {
	mb.mu.Lock()
	defer mb.mu.Unlock()
	mb.lastNavigate = targetURL
	if mb.navigateErr != nil {
		return mb.navigateErr
	}
	mb.currentURL = targetURL
	return nil
}

func (mb *mockBrowser) NavigateWait(ctx context.Context, targetURL string, waitSelector string) error {
	mb.mu.Lock()
	defer mb.mu.Unlock()
	mb.lastWaitURL = targetURL
	mb.lastWaitSel = waitSelector
	if mb.navigateWaitErr != nil {
		return mb.navigateWaitErr
	}
	mb.currentURL = targetURL
	return nil
}

func (mb *mockBrowser) GetURL(ctx context.Context) (string, error) {
	mb.mu.Lock()
	defer mb.mu.Unlock()
	if mb.getURLErr != nil {
		return "", mb.getURLErr
	}
	return mb.currentURL, nil
}

func (mb *mockBrowser) Screenshot(ctx context.Context, opts browser.ScreenshotOptions) ([]byte, string, error) {
	mb.mu.Lock()
	defer mb.mu.Unlock()
	mb.lastOpts = opts
	if mb.screenshotErr != nil {
		return nil, "", mb.screenshotErr
	}
	mime := "image/jpeg"
	if opts.Format == "png" {
		mime = "image/png"
	}
	return []byte("fake_image_bytes"), mime, nil
}

func TestControllerNavigateSuccess(t *testing.T) {
	logger := newTestLogger()
	mockCl := newMockClient("display01")
	mockBr := &mockBrowser{}

	ctrl := NewController(mockCl, mockBr, logger)
	ctrl.Start()
	defer ctrl.Stop()

	cmd := CommandMessage{
		Action: ActionNavigate,
		URL:    "https://example.com/test",
	}
	evt, err := event.NewMessageEvent("remoteClient", "display01", cmd)
	if err != nil {
		t.Fatalf("failed to create event: %v", err)
	}
	evt.Token = "token-123"

	mockCl.EmitMessage(evt)

	if mockBr.lastNavigate != "https://example.com/test" {
		t.Errorf("expected browser navigate to 'https://example.com/test', got '%s'", mockBr.lastNavigate)
	}

	mockCl.mu.Lock()
	defer mockCl.mu.Unlock()
	if len(mockCl.sentEvents) != 1 {
		t.Fatalf("expected 1 event, got %d", len(mockCl.sentEvents))
	}
	sentEvt := mockCl.sentEvents[0]
	if sentEvt.Target != "remoteClient" || sentEvt.Token != "token-123" {
		t.Errorf("unexpected response routing: target=%s token=%s", sentEvt.Target, sentEvt.Token)
	}
	var navResp NavigateResponse
	if err := sentEvt.UnmarshalData(&navResp); err != nil {
		t.Fatalf("expected NavigateResponse unmarshal success, got err %v", err)
	}
	if navResp.Status != "ok" || navResp.URL != "https://example.com/test" {
		t.Errorf("unexpected NavigateResponse content: %+v", navResp)
	}
}

func TestControllerNavigateWait(t *testing.T) {
	logger := newTestLogger()
	mockCl := newMockClient("display01")
	mockBr := &mockBrowser{}

	ctrl := NewController(mockCl, mockBr, logger)
	ctrl.Start()
	defer ctrl.Stop()

	cmd := CommandMessage{
		Action:   ActionNavigate,
		URL:      "https://example.com/wait",
		Selector: "#app",
	}
	evt, err := event.NewMessageEvent("remoteClient", "display01", cmd)
	if err != nil {
		t.Fatalf("failed to create event: %v", err)
	}

	mockCl.EmitMessage(evt)

	if mockBr.lastWaitURL != "https://example.com/wait" || mockBr.lastWaitSel != "#app" {
		t.Errorf("expected NavigateWait with 'https://example.com/wait' and '#app', got '%s' '%s'", mockBr.lastWaitURL, mockBr.lastWaitSel)
	}
}

func TestControllerNavigateError(t *testing.T) {
	logger := newTestLogger()
	mockCl := newMockClient("display01")
	mockBr := &mockBrowser{navigateErr: errors.New("timeout loading page")}

	ctrl := NewController(mockCl, mockBr, logger)
	ctrl.Start()
	defer ctrl.Stop()

	cmd := CommandMessage{
		Action: ActionNavigate,
		URL:    "https://invalid.domain",
	}
	evt, err := event.NewMessageEvent("remoteClient", "display01", cmd)
	if err != nil {
		t.Fatalf("failed to create event: %v", err)
	}

	mockCl.EmitMessage(evt)

	mockCl.mu.Lock()
	defer mockCl.mu.Unlock()
	if len(mockCl.sentEvents) != 1 {
		t.Fatalf("expected 1 response event, got %d", len(mockCl.sentEvents))
	}
	var navResp NavigateResponse
	if err := mockCl.sentEvents[0].UnmarshalData(&navResp); err != nil {
		t.Fatalf("failed to unmarshal NavigateResponse: %v", err)
	}
	if navResp.Status != "error" || navResp.Error == "" {
		t.Errorf("expected error status and message, got %+v", navResp)
	}
}

func TestControllerNavigateMissingURL(t *testing.T) {
	logger := newTestLogger()
	mockCl := newMockClient("display01")
	mockBr := &mockBrowser{}

	ctrl := NewController(mockCl, mockBr, logger)
	ctrl.Start()
	defer ctrl.Stop()

	cmd := CommandMessage{
		Action: ActionNavigate,
		URL:    "",
	}
	evt, err := event.NewMessageEvent("remoteClient", "display01", cmd)
	if err != nil {
		t.Fatalf("failed to create event: %v", err)
	}

	mockCl.EmitMessage(evt)

	mockCl.mu.Lock()
	defer mockCl.mu.Unlock()
	if len(mockCl.sentEvents) != 1 {
		t.Fatalf("expected 1 response event, got %d", len(mockCl.sentEvents))
	}
	var navResp NavigateResponse
	if err := mockCl.sentEvents[0].UnmarshalData(&navResp); err != nil {
		t.Fatalf("failed to unmarshal NavigateResponse: %v", err)
	}
	if navResp.Status != "error" {
		t.Errorf("expected error for missing URL, got %+v", navResp)
	}
}

func TestControllerGetURLSuccess(t *testing.T) {
	logger := newTestLogger()
	mockCl := newMockClient("display01")
	mockBr := &mockBrowser{currentURL: "https://example.com/current-view"}

	ctrl := NewController(mockCl, mockBr, logger)
	ctrl.Start()
	defer ctrl.Stop()

	cmd := CommandMessage{
		Action: ActionGetURL,
	}
	evt, err := event.NewMessageEvent("remoteClient", "display01", cmd)
	if err != nil {
		t.Fatalf("failed to create event: %v", err)
	}

	mockCl.EmitMessage(evt)

	mockCl.mu.Lock()
	defer mockCl.mu.Unlock()
	if len(mockCl.sentEvents) != 1 {
		t.Fatalf("expected 1 response event, got %d", len(mockCl.sentEvents))
	}
	var urlResp URLResponse
	if err := mockCl.sentEvents[0].UnmarshalData(&urlResp); err != nil {
		t.Fatalf("failed to unmarshal URLResponse: %v", err)
	}
	if urlResp.Action != ActionGetURLResponse || urlResp.URL != "https://example.com/current-view" {
		t.Errorf("unexpected URLResponse: %+v", urlResp)
	}
}

func TestControllerGetURLError(t *testing.T) {
	logger := newTestLogger()
	mockCl := newMockClient("display01")
	mockBr := &mockBrowser{getURLErr: errors.New("browser context closed")}

	ctrl := NewController(mockCl, mockBr, logger)
	ctrl.Start()
	defer ctrl.Stop()

	cmd := CommandMessage{
		Action: ActionGetURL,
	}
	evt, err := event.NewMessageEvent("remoteClient", "display01", cmd)
	if err != nil {
		t.Fatalf("failed to create event: %v", err)
	}

	mockCl.EmitMessage(evt)

	mockCl.mu.Lock()
	defer mockCl.mu.Unlock()
	if len(mockCl.sentEvents) != 1 {
		t.Fatalf("expected 1 response event, got %d", len(mockCl.sentEvents))
	}
	var urlResp URLResponse
	if err := mockCl.sentEvents[0].UnmarshalData(&urlResp); err != nil {
		t.Fatalf("failed to unmarshal URLResponse: %v", err)
	}
	if urlResp.Error == "" {
		t.Errorf("expected error in URLResponse, got %+v", urlResp)
	}
}

func TestControllerGetScreenshotSuccess(t *testing.T) {
	logger := newTestLogger()
	mockCl := newMockClient("display01")
	mockBr := &mockBrowser{}

	ctrl := NewController(mockCl, mockBr, logger)
	ctrl.Start()
	defer ctrl.Stop()

	cmd := CommandMessage{
		Action:  ActionGetScreenshot,
		Format:  "jpeg",
		Quality: 80,
	}
	evt, err := event.NewMessageEvent("remoteClient", "display01", cmd)
	if err != nil {
		t.Fatalf("failed to create event: %v", err)
	}
	evt.Token = "shot-tok"

	mockCl.EmitMessage(evt)

	if mockBr.lastOpts.MaxWidth != 800 || mockBr.lastOpts.MaxHeight != 600 {
		t.Errorf("expected 800x600 max bounds, got %dx%d", mockBr.lastOpts.MaxWidth, mockBr.lastOpts.MaxHeight)
	}
	if mockBr.lastOpts.Format != "jpeg" || mockBr.lastOpts.Quality != 80 {
		t.Errorf("expected format jpeg quality 80, got %s %d", mockBr.lastOpts.Format, mockBr.lastOpts.Quality)
	}

	mockCl.mu.Lock()
	defer mockCl.mu.Unlock()
	if len(mockCl.sentEvents) != 1 {
		t.Fatalf("expected 1 sent event, got %d", len(mockCl.sentEvents))
	}
	sentEvt := mockCl.sentEvents[0]
	if sentEvt.Type != event.TypeFile {
		t.Fatalf("expected TypeFile event, got %s", sentEvt.Type)
	}
	if sentEvt.Target != "remoteClient" || sentEvt.Token != "shot-tok" {
		t.Errorf("unexpected sentFile target/token: %s %s", sentEvt.Target, sentEvt.Token)
	}
	var fileMsg event.FileMessage
	if err := sentEvt.UnmarshalData(&fileMsg); err != nil {
		t.Fatalf("failed to unmarshal FileMessage: %v", err)
	}
	if fileMsg.MimeType != "image/jpeg" || string(fileMsg.Data) != "fake_image_bytes" {
		t.Errorf("unexpected file content: mime=%s data=%s", fileMsg.MimeType, string(fileMsg.Data))
	}
}

func TestControllerGetScreenshotError(t *testing.T) {
	logger := newTestLogger()
	mockCl := newMockClient("display01")
	mockBr := &mockBrowser{screenshotErr: errors.New("render failure")}

	ctrl := NewController(mockCl, mockBr, logger)
	ctrl.Start()
	defer ctrl.Stop()

	cmd := CommandMessage{
		Action: ActionGetScreenshot,
	}
	evt, err := event.NewMessageEvent("remoteClient", "display01", cmd)
	if err != nil {
		t.Fatalf("failed to create event: %v", err)
	}

	mockCl.EmitMessage(evt)

	mockCl.mu.Lock()
	defer mockCl.mu.Unlock()
	if len(mockCl.sentEvents) != 1 {
		t.Fatalf("expected 1 response event, got %d", len(mockCl.sentEvents))
	}
	var shotResp ScreenshotResponse
	if err := mockCl.sentEvents[0].UnmarshalData(&shotResp); err != nil {
		t.Fatalf("failed to unmarshal ScreenshotResponse: %v", err)
	}
	if shotResp.Status != "error" || shotResp.Error == "" {
		t.Errorf("expected error in ScreenshotResponse, got %+v", shotResp)
	}
}

func TestControllerInvalidAction(t *testing.T) {
	logger := newTestLogger()
	mockCl := newMockClient("display01")
	mockBr := &mockBrowser{}

	ctrl := NewController(mockCl, mockBr, logger)
	ctrl.Start()
	defer ctrl.Stop()

	cmd := CommandMessage{
		Action: "unknown_command",
	}
	evt, err := event.NewMessageEvent("remoteClient", "display01", cmd)
	if err != nil {
		t.Fatalf("failed to create event: %v", err)
	}

	mockCl.EmitMessage(evt)

	mockCl.mu.Lock()
	defer mockCl.mu.Unlock()
	if len(mockCl.sentEvents) != 1 {
		t.Fatalf("expected 1 response event, got %d", len(mockCl.sentEvents))
	}
	var errResp NavigateResponse
	if err := mockCl.sentEvents[0].UnmarshalData(&errResp); err != nil {
		t.Fatalf("failed to unmarshal error response: %v", err)
	}
	if errResp.Status != "error" {
		t.Errorf("expected error response for unknown action, got %+v", errResp)
	}
}

func TestControllerIgnoreSelfAndResponseMessages(t *testing.T) {
	logger := newTestLogger()
	mockCl := newMockClient("display01")
	mockBr := &mockBrowser{}

	ctrl := NewController(mockCl, mockBr, logger)
	ctrl.Start()
	defer ctrl.Stop()

	// 1. Self message
	selfCmd := CommandMessage{
		Action: ActionNavigate,
		URL:    "https://example.com/self",
	}
	selfEvt, _ := event.NewMessageEvent("display01", "display01", selfCmd)
	mockCl.EmitMessage(selfEvt)

	// 2. Response message
	respCmd := CommandMessage{
		Action: ActionNavigateResponse,
		URL:    "https://example.com/resp",
	}
	respEvt, _ := event.NewMessageEvent("otherClient", "display01", respCmd)
	mockCl.EmitMessage(respEvt)

	mockCl.mu.Lock()
	defer mockCl.mu.Unlock()
	if len(mockCl.sentEvents) != 0 {
		t.Errorf("expected 0 events sent for self or response messages, got %d", len(mockCl.sentEvents))
	}
	if mockBr.lastNavigate != "" {
		t.Errorf("expected no browser navigate, got %s", mockBr.lastNavigate)
	}
}
