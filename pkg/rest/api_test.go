package rest

import (
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/rs/zerolog"
)

func TestSwaggerAndPing(t *testing.T) {
	logger := zerolog.Nop()

	ctrl, err := NewController(":0", "http://localhost:8080/display", &logger)
	if err != nil {
		t.Fatalf("failed to create controller: %v", err)
	}

	// Test 1: GET /api/v1/ping (should return 200 pong)
	{
		w := httptest.NewRecorder()
		req, _ := http.NewRequest(http.MethodGet, "/api/v1/ping", nil)
		req.RemoteAddr = "127.0.0.1:12345"
		ctrl.router.ServeHTTP(w, req)
		if w.Code != http.StatusOK {
			t.Errorf("expected 200 for ping, got %d: %s", w.Code, w.Body.String())
		}
		if !strings.Contains(w.Body.String(), "pong") {
			t.Errorf("expected ping response to contain 'pong', got '%s'", w.Body.String())
		}
	}

	// Test 2: GET /swagger/index.html
	{
		w := httptest.NewRecorder()
		req, _ := http.NewRequest(http.MethodGet, "/swagger/index.html", nil)
		req.RemoteAddr = "127.0.0.1:12345"
		ctrl.router.ServeHTTP(w, req)
		if w.Code != http.StatusOK {
			t.Errorf("expected 200 for swagger index.html, got %d: %s", w.Code, w.Body.String())
		}
		if !strings.Contains(w.Body.String(), "WSBroker Display API") {
			t.Errorf("expected swagger UI to contain title 'WSBroker Display API', got '%s'", w.Body.String())
		}
	}

	// Test 3: GET /swagger/doc.json (OpenAPI 3.1 spec)
	{
		w := httptest.NewRecorder()
		req, _ := http.NewRequest(http.MethodGet, "/swagger/doc.json", nil)
		req.RemoteAddr = "127.0.0.1:12345"
		ctrl.router.ServeHTTP(w, req)
		if w.Code != http.StatusOK {
			t.Errorf("expected 200 for swagger doc.json, got %d: %s", w.Code, w.Body.String())
		}

		var spec map[string]any
		if err := json.Unmarshal(w.Body.Bytes(), &spec); err != nil {
			t.Fatalf("failed to unmarshal swagger doc JSON: %v", err)
		}
		if openapi, ok := spec["openapi"].(string); !ok || !strings.HasPrefix(openapi, "3.1") {
			t.Errorf("expected openapi 3.1.x version, got %v", spec["openapi"])
		}

		servers, ok := spec["servers"].([]any)
		if !ok || len(servers) == 0 {
			t.Fatalf("expected servers array in swagger doc, got %v", spec["servers"])
		}
		serverObj, ok := servers[0].(map[string]any)
		if !ok {
			t.Fatalf("expected server object in servers array, got %v", servers[0])
		}
		if serverObj["url"] != "/display/api/v1" {
			t.Errorf("expected server url '/display/api/v1', got %v", serverObj["url"])
		}
	}

	// Test 4: Default base path without subpath (/api/v1)
	{
		ctrlDefault, err := NewController(":0", "http://localhost:8080", &logger)
		if err != nil {
			t.Fatalf("failed to create default controller: %v", err)
		}

		w := httptest.NewRecorder()
		req, _ := http.NewRequest(http.MethodGet, "/swagger/doc.json", nil)
		req.RemoteAddr = "127.0.0.1:12345"
		ctrlDefault.router.ServeHTTP(w, req)
		if w.Code != http.StatusOK {
			t.Errorf("expected 200 for swagger doc.json, got %d: %s", w.Code, w.Body.String())
		}

		var spec map[string]any
		if err := json.Unmarshal(w.Body.Bytes(), &spec); err != nil {
			t.Fatalf("failed to unmarshal swagger doc JSON: %v", err)
		}
		servers, ok := spec["servers"].([]any)
		if !ok || len(servers) == 0 {
			t.Fatalf("expected servers array in swagger doc, got %v", spec["servers"])
		}
		serverObj, ok := servers[0].(map[string]any)
		if !ok {
			t.Fatalf("expected server object in servers array, got %v", servers[0])
		}
		if serverObj["url"] != "/api/v1" {
			t.Errorf("expected server url '/api/v1', got %v", serverObj["url"])
		}
	}
}

func TestLocalhostRestriction(t *testing.T) {
	logger := zerolog.Nop()

	ctrl, err := NewController(":0", "http://localhost:8080", &logger)
	if err != nil {
		t.Fatalf("failed to create controller: %v", err)
	}

	// Request from loopback (127.0.0.1) should succeed
	{
		w := httptest.NewRecorder()
		req, _ := http.NewRequest(http.MethodGet, "/api/v1/ping", nil)
		req.RemoteAddr = "127.0.0.1:12345"
		ctrl.router.ServeHTTP(w, req)
		if w.Code != http.StatusOK {
			t.Errorf("expected 200 for loopback ping, got %d", w.Code)
		}
	}

	// Request from loopback (IPv6 ::1) should succeed
	{
		w := httptest.NewRecorder()
		req, _ := http.NewRequest(http.MethodGet, "/api/v1/ping", nil)
		req.RemoteAddr = "[::1]:12345"
		ctrl.router.ServeHTTP(w, req)
		if w.Code != http.StatusOK {
			t.Errorf("expected 200 for IPv6 loopback ping, got %d", w.Code)
		}
	}

	// Request from non-loopback IP (e.g. 192.168.1.100) should be forbidden
	{
		w := httptest.NewRecorder()
		req, _ := http.NewRequest(http.MethodGet, "/api/v1/ping", nil)
		req.RemoteAddr = "192.168.1.100:12345"
		ctrl.router.ServeHTTP(w, req)
		if w.Code != http.StatusForbidden {
			t.Errorf("expected 403 Forbidden for external IP, got %d", w.Code)
		}
	}
}

func TestStartAndGracefulStop(t *testing.T) {
	logger := zerolog.Nop()

	// Find an available port on 127.0.0.1
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("failed to listen on ephemeral port: %v", err)
	}
	addr := listener.Addr().String()
	_ = listener.Close()

	ctrl, err := NewController(addr, fmt.Sprintf("http://%s", addr), &logger)
	if err != nil {
		t.Fatalf("failed to create controller: %v", err)
	}

	wg := &sync.WaitGroup{}
	ctrl.Start(wg)

	// Wait briefly for server to start listening
	time.Sleep(100 * time.Millisecond)

	resp, err := http.Get(fmt.Sprintf("http://%s/api/v1/ping", addr))
	if err != nil {
		t.Fatalf("failed to execute GET /api/v1/ping: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("expected status 200, got %d", resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("failed to read response body: %v", err)
	}
	if string(body) != "pong" {
		t.Errorf("expected 'pong', got '%s'", string(body))
	}

	ctrl.GracefulStop()
	wg.Wait()
}

func TestDashboardAndStaticRoutes(t *testing.T) {
	logger := zerolog.Nop()

	data := DashboardData{
		ClientName: "test-display-01",
		WSBroker:   "https://broker.example.com",
		Channels:   []string{"general", "alerts"},
		Addr:       "localhost:8760",
		ExtAddr:    "http://localhost:8760",
		TLSType:    "dev",
		Hostname:   "kiosk-host",
		OS:         "linux",
		Arch:       "amd64",
		GoVersion:  "go1.22",
		Uptime:     "1h23m45s",
		StartTime:  time.Now().UTC().Add(-time.Hour),
		BrowserConfig: BrowserInfo{
			Kiosk:    false,
			Headless: false,
			Width:    1920,
			Height:   1080,
		},
	}
	data.NTP.Server = "pool.ntp.org"
	data.NTP.Synced = true
	data.NTP.OffsetString = "+1.234ms"
	data.NTP.RTTString = "12.345ms"
	data.NTP.Stratum = 2
	data.NTP.ReferenceID = "192.168.1.1"

	ctrl, err := NewController(":0", "http://localhost:8760", &logger, WithDataProvider(func() DashboardData {
		return data
	}))
	if err != nil {
		t.Fatalf("failed to create controller: %v", err)
	}

	// 1. GET /
	{
		w := httptest.NewRecorder()
		req, _ := http.NewRequest(http.MethodGet, "/", nil)
		req.RemoteAddr = "127.0.0.1:12345"
		ctrl.router.ServeHTTP(w, req)
		if w.Code != http.StatusOK {
			t.Errorf("expected 200 for GET /, got %d: %s", w.Code, w.Body.String())
		}
		body := w.Body.String()
		if !strings.Contains(body, "WSBroker Display") {
			t.Errorf("expected GET / to contain 'WSBroker Display'")
		}
		if !strings.Contains(body, "tailwind") {
			t.Errorf("expected GET / to contain Tailwind CSS script")
		}
		if !strings.Contains(body, "htmx") {
			t.Errorf("expected GET / to contain HTMX script")
		}
		if !strings.Contains(body, "test-display-01") {
			t.Errorf("expected GET / to contain client name 'test-display-01'")
		}
	}

	// 2. GET /dashboard
	{
		w := httptest.NewRecorder()
		req, _ := http.NewRequest(http.MethodGet, "/dashboard", nil)
		req.RemoteAddr = "127.0.0.1:12345"
		ctrl.router.ServeHTTP(w, req)
		if w.Code != http.StatusOK {
			t.Errorf("expected 200 for GET /dashboard, got %d", w.Code)
		}
	}

	// 3. GET /dashboard/cards
	{
		w := httptest.NewRecorder()
		req, _ := http.NewRequest(http.MethodGet, "/dashboard/cards", nil)
		req.RemoteAddr = "127.0.0.1:12345"
		ctrl.router.ServeHTTP(w, req)
		if w.Code != http.StatusOK {
			t.Errorf("expected 200 for GET /dashboard/cards, got %d", w.Code)
		}
		body := w.Body.String()
		t.Logf("cards body: %s", body)
		if !strings.Contains(body, "Network Time (NTP)") {
			t.Errorf("expected cards partial to contain 'Network Time (NTP)'")
		}
		if !strings.Contains(body, "1.234ms") {
			t.Errorf("expected cards partial to contain offset '1.234ms'")
		}
	}

	// 4. GET /static/css/dashboard.css
	{
		w := httptest.NewRecorder()
		req, _ := http.NewRequest(http.MethodGet, "/static/css/dashboard.css", nil)
		req.RemoteAddr = "127.0.0.1:12345"
		ctrl.router.ServeHTTP(w, req)
		if w.Code != http.StatusOK {
			t.Errorf("expected 200 for GET /static/css/dashboard.css, got %d", w.Code)
		}
		if !strings.Contains(w.Body.String(), "--bg-kiosk") {
			t.Errorf("expected CSS content in /static/css/dashboard.css")
		}
	}

	// 5. GET /static/js/dashboard.js
	{
		w := httptest.NewRecorder()
		req, _ := http.NewRequest(http.MethodGet, "/static/js/dashboard.js", nil)
		req.RemoteAddr = "127.0.0.1:12345"
		ctrl.router.ServeHTTP(w, req)
		if w.Code != http.StatusOK {
			t.Errorf("expected 200 for GET /static/js/dashboard.js, got %d", w.Code)
		}
		if !strings.Contains(w.Body.String(), "updateClock") {
			t.Errorf("expected JS content in /static/js/dashboard.js")
		}
	}
}

func TestStatusAndNTPEndpoints(t *testing.T) {
	logger := zerolog.Nop()

	data := DashboardData{
		ClientName: "status-test-client",
		WSBroker:   "wss://broker.test:8765",
		Channels:   []string{"screen1"},
		Addr:       "localhost:8760",
		ExtAddr:    "http://localhost:8760",
		TLSType:    "minivault",
		Hostname:   "node-42",
		OS:         "windows",
		Arch:       "amd64",
		GoVersion:  "go1.22.1",
		Uptime:     "42m",
		StartTime:  time.Now().UTC(),
	}
	data.NTP.Server = "time.cloudflare.com"
	data.NTP.Synced = true
	data.NTP.OffsetString = "-0.500ms"
	data.NTP.RTTString = "8.200ms"
	data.NTP.Stratum = 1
	data.NTP.ReferenceID = "GPS"

	ctrl, err := NewController(":0", "http://localhost:8760", &logger, WithDataProvider(func() DashboardData {
		return data
	}))
	if err != nil {
		t.Fatalf("failed to create controller: %v", err)
	}

	// 1. GET /api/v1/status
	{
		w := httptest.NewRecorder()
		req, _ := http.NewRequest(http.MethodGet, "/api/v1/status", nil)
		req.RemoteAddr = "127.0.0.1:12345"
		ctrl.router.ServeHTTP(w, req)
		if w.Code != http.StatusOK {
			t.Errorf("expected 200 for GET /api/v1/status, got %d", w.Code)
		}

		var received DashboardData
		if err := json.Unmarshal(w.Body.Bytes(), &received); err != nil {
			t.Fatalf("failed to unmarshal JSON response: %v", err)
		}
		if received.ClientName != "status-test-client" {
			t.Errorf("expected ClientName 'status-test-client', got '%s'", received.ClientName)
		}
		if received.NTP.Server != "time.cloudflare.com" {
			t.Errorf("expected NTP server 'time.cloudflare.com', got '%s'", received.NTP.Server)
		}
		if !received.NTP.Synced {
			t.Errorf("expected NTP Synced=true")
		}
	}

	// 2. GET /api/v1/ntp
	{
		w := httptest.NewRecorder()
		req, _ := http.NewRequest(http.MethodGet, "/api/v1/ntp", nil)
		req.RemoteAddr = "127.0.0.1:12345"
		ctrl.router.ServeHTTP(w, req)
		if w.Code != http.StatusOK {
			t.Errorf("expected 200 for GET /api/v1/ntp, got %d", w.Code)
		}

		var receivedNTP struct {
			Server  string `json:"server"`
			Synced  bool   `json:"synced"`
			Stratum int    `json:"stratum"`
		}
		if err := json.Unmarshal(w.Body.Bytes(), &receivedNTP); err != nil {
			t.Fatalf("failed to unmarshal JSON response: %v", err)
		}
		if receivedNTP.Server != "time.cloudflare.com" {
			t.Errorf("expected Server 'time.cloudflare.com', got '%s'", receivedNTP.Server)
		}
		if receivedNTP.Stratum != 1 {
			t.Errorf("expected Stratum 1, got %d", receivedNTP.Stratum)
		}
	}
}

func TestMessageLog(t *testing.T) {
	log := NewMessageLog(3)
	if log.Len() != 0 {
		t.Fatalf("expected initial len 0, got %d", log.Len())
	}
	if log.Capacity() != 3 {
		t.Fatalf("expected capacity 3, got %d", log.Capacity())
	}

	log.Add(WSMessage{
		Type:   "message",
		Source: "c1",
		Data:   "first",
	})
	log.Add(WSMessage{
		Type:   "message",
		Source: "c2",
		Data:   "second",
	})
	log.Add(WSMessage{
		Type:   "message",
		Source: "c3",
		Data:   "third",
	})

	if log.Len() != 3 {
		t.Fatalf("expected len 3, got %d", log.Len())
	}

	// Adding fourth should drop first (capacity=3)
	log.Add(WSMessage{
		Type:   "message",
		Source: "c4",
		Data:   "fourth",
	})

	msgs := log.GetAll()
	if len(msgs) != 3 {
		t.Fatalf("expected 3 entries after rolling buffer drop, got %d", len(msgs))
	}
	// GetAll should return newest first ("fourth", "third", "second")
	if msgs[0].Data != "fourth" || msgs[1].Data != "third" || msgs[2].Data != "second" {
		t.Errorf("unexpected message order for GetAll (newest first): %+v", msgs)
	}

	// GetAllChronological should return oldest first ("second", "third", "fourth")
	chronoMsgs := log.GetAllChronological()
	if chronoMsgs[0].Data != "second" || chronoMsgs[1].Data != "third" || chronoMsgs[2].Data != "fourth" {
		t.Errorf("unexpected message order for GetAllChronological: %+v", chronoMsgs)
	}

	// Resize capacity down to 2
	log.SetCapacity(2)
	if log.Capacity() != 2 {
		t.Fatalf("expected capacity 2, got %d", log.Capacity())
	}
	if log.Len() != 2 {
		t.Fatalf("expected len 2 after SetCapacity(2), got %d", log.Len())
	}
	msgs = log.GetAll()
	if msgs[0].Data != "fourth" || msgs[1].Data != "third" {
		t.Errorf("unexpected message order after SetCapacity: %+v", msgs)
	}

	log.Clear()
	if log.Len() != 0 {
		t.Fatalf("expected len 0 after Clear, got %d", log.Len())
	}
}

func TestMessagesEndpointAndDashboardCards(t *testing.T) {
	logger := zerolog.Nop()

	msgLog := NewMessageLog(10)
	msgLog.Add(WSMessage{
		Timestamp: time.Now().UTC(),
		Direction: "in",
		Type:      "message",
		Source:    "remote01",
		Target:    "display01",
		Token:     "tok-123",
		Data:      `{"action":"navigate","url":"https://example.com"}`,
	})
	msgLog.Add(WSMessage{
		Timestamp: time.Now().UTC(),
		Direction: "out",
		Type:      "message",
		Source:    "display01",
		Target:    "remote01",
		Token:     "tok-123",
		Data:      `{"action":"navigate_response","status":"ok"}`,
	})

	ctrl, err := NewController(":0", "http://localhost:8760", &logger, WithDataProvider(func() DashboardData {
		return DashboardData{
			ClientName: "display01",
			Messages:   msgLog.GetAll(),
		}
	}))
	if err != nil {
		t.Fatalf("failed to create controller: %v", err)
	}

	// 1. GET /api/v1/messages
	{
		w := httptest.NewRecorder()
		req, _ := http.NewRequest(http.MethodGet, "/api/v1/messages", nil)
		req.RemoteAddr = "127.0.0.1:12345"
		ctrl.router.ServeHTTP(w, req)
		if w.Code != http.StatusOK {
			t.Errorf("expected 200 for GET /api/v1/messages, got %d: %s", w.Code, w.Body.String())
		}

		var msgs []WSMessage
		if err := json.Unmarshal(w.Body.Bytes(), &msgs); err != nil {
			t.Fatalf("failed to unmarshal JSON messages: %v", err)
		}
		if len(msgs) != 2 {
			t.Fatalf("expected 2 messages, got %d", len(msgs))
		}
		if msgs[0].Direction != "out" || msgs[0].Source != "display01" || msgs[0].Token != "tok-123" {
			t.Errorf("unexpected message 0 (expected newest 'out'): %+v", msgs[0])
		}
		if msgs[1].Direction != "in" || msgs[1].Source != "remote01" {
			t.Errorf("unexpected message 1 (expected older 'in'): %+v", msgs[1])
		}
	}

	// 2. GET /dashboard/cards should render WebSocket Message Log table
	{
		w := httptest.NewRecorder()
		req, _ := http.NewRequest(http.MethodGet, "/dashboard/cards", nil)
		req.RemoteAddr = "127.0.0.1:12345"
		ctrl.router.ServeHTTP(w, req)
		if w.Code != http.StatusOK {
			t.Errorf("expected 200 for GET /dashboard/cards, got %d", w.Code)
		}

		body := w.Body.String()
		if !strings.Contains(body, "WebSocket Message Log") {
			t.Errorf("expected cards template to contain 'WebSocket Message Log'")
		}
		if !strings.Contains(body, "2 entries") {
			t.Errorf("expected cards template to contain '2 entries'")
		}
		if !strings.Contains(body, "remote01") || !strings.Contains(body, "tok-123") {
			t.Errorf("expected cards template to contain message details")
		}
		if !strings.Contains(body, "IN") || !strings.Contains(body, "OUT") {
			t.Errorf("expected cards template to contain direction badges IN and OUT")
		}
	}
}
