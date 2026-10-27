package brokerclient

import (
	"context"
	"encoding/json"
	"sync"
	"testing"
	"time"

	"emperror.dev/errors"
	"github.com/je4/utils/v2/pkg/zLogger"
	rawclient "github.com/je4/wsbrokerclient/pkg/client"
	"github.com/je4/wsbrokerclient/pkg/event"
	"github.com/je4/wsbrokerdisplay/pkg/rest"
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
	onRequest     func(ctx context.Context, evt *event.Event) (*event.Event, error)
	waiters       map[string]chan *event.Event
	connectCalled bool
	closed        bool
}

func newMockBrokerClient(name string) *mockBrokerClient {
	return &mockBrokerClient{
		name:    name,
		waiters: make(map[string]chan *event.Event),
	}
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

func (m *mockBrokerClient) Request(ctx context.Context, evt *event.Event) (*event.Event, error) {
	m.mu.Lock()
	onRequest := m.onRequest
	m.mu.Unlock()
	if onRequest != nil {
		return onRequest(ctx, evt)
	}

	ch := make(chan *event.Event, 1)
	if err := m.RequestN(ctx, evt, 1, ch); err != nil {
		return nil, err
	}
	return <-ch, nil
}

func (m *mockBrokerClient) Request0(ctx context.Context, evt *event.Event) error {
	return m.Send(ctx, evt)
}

func (m *mockBrokerClient) Request1(ctx context.Context, evt *event.Event) (*event.Event, error) {
	return m.Request(ctx, evt)
}

func (m *mockBrokerClient) RequestN(ctx context.Context, evt *event.Event, numResults int, ch chan *event.Event) error {
	if evt.ID == "" {
		evt.ID = event.GenerateEventID()
	}
	if evt.Source == "" {
		evt.Source = m.name
	}
	if numResults <= 0 {
		return m.Send(ctx, evt)
	}

	m.mu.Lock()
	if m.waiters == nil {
		m.waiters = make(map[string]chan *event.Event)
	}
	waiterCh := make(chan *event.Event, numResults)
	m.waiters[evt.ID] = waiterCh
	if evt.Token != "" {
		m.waiters[evt.Token] = waiterCh
	}
	m.mu.Unlock()

	defer func() {
		m.mu.Lock()
		delete(m.waiters, evt.ID)
		if evt.Token != "" {
			delete(m.waiters, evt.Token)
		}
		m.mu.Unlock()
	}()

	if err := m.Send(ctx, evt); err != nil {
		return err
	}

	for range numResults {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case resp, ok := <-waiterCh:
			if !ok || resp == nil {
				return errors.New("closed")
			}
			ch <- resp
		}
	}
	return nil
}

func (m *mockBrokerClient) Reply(ctx context.Context, originalEvt *event.Event, data any) error {
	return m.ReplyType(ctx, originalEvt, event.TypeMessage, data)
}

func (m *mockBrokerClient) ReplyType(ctx context.Context, originalEvt *event.Event, replyType event.EventType, data any) error {
	if originalEvt == nil {
		return errors.New("nil event")
	}
	replyEvt, err := originalEvt.CreateReply(replyType, data)
	if err != nil {
		return err
	}
	replyEvt.Source = m.name
	return m.Send(ctx, replyEvt)
}

func (m *mockBrokerClient) SendMessage(ctx context.Context, target string, data any) error {
	evt, err := event.NewMessageEvent(m.name, target, data)
	if err != nil {
		return err
	}
	return m.Send(ctx, evt)
}

func (m *mockBrokerClient) Broadcast(ctx context.Context, channel string, data any) error {
	return m.SendMessage(ctx, channel, data)
}

func (m *mockBrokerClient) SendDirect(ctx context.Context, targetClient string, data any) error {
	return m.SendMessage(ctx, targetClient, data)
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
	if evt.RefID != "" && m.waiters != nil {
		if ch, found := m.waiters[evt.RefID]; found {
			select {
			case ch <- evt:
			default:
			}
		}
	}
	if evt.Token != "" && m.waiters != nil {
		if ch, found := m.waiters[evt.Token]; found {
			select {
			case ch <- evt:
			default:
			}
		}
	}
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

	// StatusResponse
	statResp := StatusResponse{
		Action: ActionGetStatusResponse,
		Status: "ok",
		Data: &rest.DashboardData{
			ClientName: "disp-101",
			Hostname:   "node-alpha",
			OS:         "linux",
		},
	}
	statData, err := json.Marshal(statResp)
	if err != nil {
		t.Fatalf("failed to marshal StatusResponse: %v", err)
	}
	var parsedStat StatusResponse
	if err := json.Unmarshal(statData, &parsedStat); err != nil {
		t.Fatalf("failed to unmarshal StatusResponse: %v", err)
	}
	if parsedStat.Status != "ok" || parsedStat.Data == nil || parsedStat.Data.ClientName != "disp-101" {
		t.Errorf("unexpected StatusResponse: %+v", parsedStat)
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
		if evt != nil {
			var cmd CommandMessage
			if err := evt.UnmarshalData(&cmd); err == nil && cmd.Action == ActionNavigate {
				resp := NavigateResponse{
					Action: ActionNavigateResponse,
					Status: "ok",
					URL:    cmd.URL,
				}
				respEvt, _ := evt.CreateReply(event.TypeMessage, resp)
				respEvt.Source = "display01"
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
		if evt != nil {
			var cmd CommandMessage
			if err := evt.UnmarshalData(&cmd); err == nil && cmd.Action == ActionGetURL {
				resp := URLResponse{
					Action: ActionGetURLResponse,
					URL:    "https://example.com/current",
				}
				respEvt, _ := evt.CreateReply(event.TypeMessage, resp)
				respEvt.Source = "display01"
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
		if evt != nil {
			var cmd CommandMessage
			if err := evt.UnmarshalData(&cmd); err == nil && cmd.Action == ActionGetScreenshot {
				fileMsg := event.NewFileMessage("shot.jpg", "image/jpeg", []byte("sample_screenshot_bytes"))
				respEvt, _ := evt.CreateReply(event.TypeFile, fileMsg)
				respEvt.Source = "display01"
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
		if evt != nil {
			resp := ScreenshotResponse{
				Action: ActionScreenshotResponse,
				Status: "error",
				Error:  "browser renderer crashed",
			}
			respEvt, _ := evt.CreateReply(event.TypeMessage, resp)
			respEvt.Source = "display01"
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

func TestGetStatusMethod(t *testing.T) {
	logger := newTestLogger()
	mock := newMockBrokerClient("controller01")

	c, err := New(mock, logger)
	if err != nil {
		t.Fatalf("failed to create client: %v", err)
	}

	mock.onSend = func(ctx context.Context, evt *event.Event) error {
		if evt != nil {
			var cmd CommandMessage
			if err := evt.UnmarshalData(&cmd); err == nil && (cmd.Action == ActionGetStatus || cmd.Event == "display:getstatus") {
				resp := StatusResponse{
					Event:  "display:getstatus_response",
					Action: ActionGetStatusResponse,
					Status: "ok",
					Data: &rest.DashboardData{
						ClientName: "display01",
						Hostname:   "node-display",
						OS:         "linux",
						Arch:       "amd64",
						Uptime:     "45m",
					},
				}
				respEvt, err := evt.CreateReply(event.TypeMessage, resp)
				if err != nil {
					t.Fatalf("CreateReply failed: %v", err)
				}
				respEvt.Source = "display01"
				go mock.EmitEvent(respEvt)
			}
		}
		return nil
	}

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	data, err := c.GetStatus(ctx, "display01")
	if err != nil {
		t.Fatalf("GetStatus failed: %v", err)
	}
	if data == nil || data.ClientName != "display01" || data.Hostname != "node-display" {
		t.Errorf("unexpected telemetry dashboard data: %+v", data)
	}
}

func TestGetStatusErrorResponse(t *testing.T) {
	logger := newTestLogger()
	mock := newMockBrokerClient("controller01")

	c, err := New(mock, logger)
	if err != nil {
		t.Fatalf("failed to create client: %v", err)
	}

	mock.onSend = func(ctx context.Context, evt *event.Event) error {
		if evt != nil {
			resp := StatusResponse{
				Action: ActionGetStatusResponse,
				Status: "error",
				Error:  "status provider not configured",
			}
			respEvt, _ := evt.CreateReply(event.TypeMessage, resp)
			respEvt.Source = "display01"
			go mock.EmitEvent(respEvt)
		}
		return nil
	}

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	_, err = c.GetStatus(ctx, "display01")
	if err == nil {
		t.Fatalf("expected error from failed getstatus, got nil")
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

	// SendGetStatus
	if err := c.SendGetStatus(ctx, "display01", "tok4"); err != nil {
		t.Errorf("SendGetStatus failed: %v", err)
	}

	mock.mu.Lock()
	defer mock.mu.Unlock()
	if len(mock.sentEvents) != 4 {
		t.Fatalf("expected 4 captured events, got %d", len(mock.sentEvents))
	}

	if mock.sentEvents[0].Token != "tok1" || mock.sentEvents[1].Token != "tok2" || mock.sentEvents[2].Token != "tok3" || mock.sentEvents[3].Token != "tok4" {
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

	if err := c.Close(); err != nil {
		t.Errorf("Close failed: %v", err)
	}

	// Second Close should be idempotent
	if err := c.Close(); err != nil {
		t.Errorf("idempotent Close failed: %v", err)
	}
	if !mock.closed {
		t.Errorf("expected underlying mock client to be closed")
	}
}

func TestClientRequestAndReplyMethods(t *testing.T) {
	logger := newTestLogger()
	mock := newMockBrokerClient("controller01")

	c, err := New(mock, logger)
	if err != nil {
		t.Fatalf("failed to create client: %v", err)
	}

	mock.onSend = func(ctx context.Context, evt *event.Event) error {
		if evt != nil {
			var msg string
			_ = evt.UnmarshalData(&msg)
			if msg == "ping-request" {
				replyEvt, _ := evt.CreateReply(event.TypeMessage, "pong-response")
				replyEvt.Source = "server01"
				go mock.EmitEvent(replyEvt)
			}
		}
		return nil
	}

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	// 1. Test Request / Request1
	reqEvt, err := event.NewMessageEvent("controller01", "server01", "ping-request")
	if err != nil {
		t.Fatalf("failed to create request event: %v", err)
	}
	respEvt, err := c.Request(ctx, reqEvt)
	if err != nil {
		t.Fatalf("Request failed: %v", err)
	}
	var respMsg string
	if err := respEvt.UnmarshalData(&respMsg); err != nil {
		t.Fatalf("failed to unmarshal response message: %v", err)
	}
	if respMsg != "pong-response" || respEvt.RefID != reqEvt.ID {
		t.Errorf("unexpected response event: %+v", respEvt)
	}

	// 2. Test Request1 alias
	reqEvt2, _ := event.NewMessageEvent("controller01", "server01", "ping-request")
	respEvt2, err := c.Request1(ctx, reqEvt2)
	if err != nil {
		t.Fatalf("Request1 failed: %v", err)
	}
	var respMsg2 string
	_ = respEvt2.UnmarshalData(&respMsg2)
	if respMsg2 != "pong-response" {
		t.Errorf("unexpected Request1 response: %s", respMsg2)
	}

	// 3. Test Request0 (fire and forget)
	reqEvt0, _ := event.NewMessageEvent("controller01", "server01", "fire-and-forget")
	if err := c.Request0(ctx, reqEvt0); err != nil {
		t.Fatalf("Request0 failed: %v", err)
	}

	// 4. Test RequestN
	reqEvtN, _ := event.NewMessageEvent("controller01", "server01", "ping-request")
	ch := make(chan *event.Event, 1)
	if err := c.RequestN(ctx, reqEvtN, 1, ch); err != nil {
		t.Fatalf("RequestN failed: %v", err)
	}
	select {
	case received := <-ch:
		var nMsg string
		_ = received.UnmarshalData(&nMsg)
		if nMsg != "pong-response" {
			t.Errorf("unexpected RequestN streamed event: %+v", received)
		}
	case <-time.After(1 * time.Second):
		t.Fatalf("timeout waiting for RequestN response")
	}

	// 5. Test Reply and ReplyType
	origEvt, _ := event.NewMessageEvent("client01", "controller01", "query")
	if err := c.Reply(ctx, origEvt, "reply-data"); err != nil {
		t.Fatalf("Reply failed: %v", err)
	}
	if err := c.ReplyType(ctx, origEvt, event.TypeFile, event.NewFileMessage("test.txt", "text/plain", []byte("hello"))); err != nil {
		t.Fatalf("ReplyType failed: %v", err)
	}

	// 6. Test SendMessage, Broadcast, SendDirect
	if err := c.SendMessage(ctx, "target01", "msg"); err != nil {
		t.Fatalf("SendMessage failed: %v", err)
	}
	if err := c.Broadcast(ctx, "channel01", "broadcast"); err != nil {
		t.Fatalf("Broadcast failed: %v", err)
	}
	if err := c.SendDirect(ctx, "client02", "direct"); err != nil {
		t.Fatalf("SendDirect failed: %v", err)
	}
}
