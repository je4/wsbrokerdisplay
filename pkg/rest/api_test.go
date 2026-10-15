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
