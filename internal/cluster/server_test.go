package cluster

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func newTestServer() *Server {
	return NewServer(ServerConfig{
		Port:       0,
		InstanceID: "test-instance",
	})
}

func TestHandleHealth(t *testing.T) {
	s := newTestServer()
	req := httptest.NewRequest("GET", "/health", nil)
	w := httptest.NewRecorder()

	s.mux.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("status = %d, want 200", w.Code)
	}

	var body map[string]any
	json.NewDecoder(w.Body).Decode(&body)
	if body["status"] != "ok" {
		t.Errorf("status = %v, want ok", body["status"])
	}
	if body["instanceId"] != "test-instance" {
		t.Errorf("instanceId = %v", body["instanceId"])
	}
}

func TestHandleStatus_NoAuth(t *testing.T) {
	s := NewServer(ServerConfig{
		Port:       0,
		APIKey:     "secret-key",
		InstanceID: "test",
	})

	req := httptest.NewRequest("GET", "/api/status", nil)
	w := httptest.NewRecorder()
	s.mux.ServeHTTP(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Errorf("status = %d, want 401", w.Code)
	}
}

func TestHandleStatus_WithAuth(t *testing.T) {
	s := NewServer(ServerConfig{
		Port:       0,
		APIKey:     "secret-key",
		InstanceID: "test",
	})

	req := httptest.NewRequest("GET", "/api/status", nil)
	req.Header.Set("Authorization", "Bearer secret-key")
	w := httptest.NewRecorder()
	s.mux.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("status = %d, want 200", w.Code)
	}
}

func TestHandleChat_EmptyContent(t *testing.T) {
	s := newTestServer()
	body := `{"content":"","sessionKey":"s1"}`
	req := httptest.NewRequest("POST", "/api/chat", strings.NewReader(body))
	w := httptest.NewRecorder()

	s.mux.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", w.Code)
	}
}

func TestHandleChat_MethodNotAllowed(t *testing.T) {
	s := newTestServer()
	req := httptest.NewRequest("GET", "/api/chat", nil)
	w := httptest.NewRecorder()
	s.mux.ServeHTTP(w, req)

	if w.Code != http.StatusMethodNotAllowed {
		t.Errorf("status = %d, want 405", w.Code)
	}
}

func TestHandleLoad(t *testing.T) {
	s := newTestServer()
	req := httptest.NewRequest("GET", "/api/load", nil)
	w := httptest.NewRecorder()
	s.mux.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("status = %d, want 200", w.Code)
	}

	var body map[string]any
	json.NewDecoder(w.Body).Decode(&body)
	if _, ok := body["activeRequests"]; !ok {
		t.Error("missing activeRequests field")
	}
}

func TestHandleAgents_NoRegistry(t *testing.T) {
	s := newTestServer()
	req := httptest.NewRequest("GET", "/api/agents", nil)
	w := httptest.NewRecorder()
	s.mux.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("status = %d, want 200", w.Code)
	}

	var body map[string]any
	json.NewDecoder(w.Body).Decode(&body)
	total, _ := body["total"].(float64)
	if total != 0 {
		t.Errorf("total = %v, want 0", total)
	}
}
