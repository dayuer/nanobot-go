// Package cluster provides the Survival HTTP API server with
// WebSocket heartbeat, Backend pool registration, and multi-agent dispatch.
//
// This mirrors survival/nanobot/server.py's SurvivalNanobotServer class.
package cluster

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"sync"
	"sync/atomic"
	"time"

	"github.com/gorilla/websocket"

	"github.com/dayuer/nanobot-go/internal/confighub"
	"github.com/dayuer/nanobot-go/internal/lane"
	"github.com/dayuer/nanobot-go/internal/registry"
)

// Server is the Survival HTTP API server.
type Server struct {
	port       int
	apiKey     string
	instanceID string
	registry   *registry.Registry
	configHub  *confighub.ConfigHub
	laneManager *lane.Manager

	// WebSocket
	wsFingerprint string
	wsConns       map[*wsConn]bool
	wsMu          sync.Mutex
	ReRegisterFn  func() // called when all WS connections drop

	// Load stats
	activeRequests atomic.Int64
	totalRequests  atomic.Int64
	totalLatencyMs atomic.Int64
	startTime      time.Time

	mux *http.ServeMux
	srv *http.Server
}

// ServerConfig configures the cluster Server.
type ServerConfig struct {
	Port          int
	APIKey        string
	InstanceID    string
	WSFingerprint string
	Registry      *registry.Registry
	ConfigHub     *confighub.ConfigHub
}

// NewServer creates a new HTTP API server.
func NewServer(cfg ServerConfig) *Server {
	s := &Server{
		port:          cfg.Port,
		apiKey:        cfg.APIKey,
		instanceID:    cfg.InstanceID,
		wsFingerprint: cfg.WSFingerprint,
		registry:      cfg.Registry,
		configHub:     cfg.ConfigHub,
		wsConns:       make(map[*wsConn]bool),
		startTime:     time.Now(),
		mux:           http.NewServeMux(),
	}

	// Init lane manager
	s.laneManager = lane.NewManager(lane.ManagerConfig{
		Handler:       s.laneHandler,
		DefaultMode:   lane.ModeCollect,
		CollectWindow: 2 * time.Second,
	})

	// Register routes
	s.mux.HandleFunc("/health", s.handleHealth)
	s.mux.HandleFunc("/ws", s.handleWS)
	s.mux.HandleFunc("/api/status", s.withAuth(s.handleStatus))
	s.mux.HandleFunc("/api/load", s.withAuth(s.handleLoad))
	s.mux.HandleFunc("/api/chat", s.withAuth(s.handleChat))
	s.mux.HandleFunc("/api/agents", s.withAuth(s.handleAgents))
	s.mux.HandleFunc("/api/config", s.withAuth(s.handleConfig))

	return s
}

// Start starts the HTTP server and heartbeat loop.
func (s *Server) Start(ctx context.Context) error {
	s.srv = &http.Server{
		Addr:    fmt.Sprintf("0.0.0.0:%d", s.port),
		Handler: s.mux,
	}

	log.Printf("[Cluster] ✅ HTTP API → http://0.0.0.0:%d", s.port)
	log.Printf("[Cluster] ✅ WebSocket → ws://0.0.0.0:%d/ws", s.port)

	// Start heartbeat loop
	go s.heartbeatLoop(ctx)

	go func() {
		<-ctx.Done()
		s.closeAllWS()
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		s.srv.Shutdown(shutdownCtx)
	}()

	if err := s.srv.ListenAndServe(); err != http.ErrServerClosed {
		return err
	}
	return nil
}

// Stop stops the server gracefully.
func (s *Server) Stop() {
	if s.srv != nil {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		s.srv.Shutdown(ctx)
	}
	if s.laneManager != nil {
		s.laneManager.Stop()
	}
}

// --- Auth middleware ---

func (s *Server) withAuth(handler http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if s.apiKey != "" {
			auth := r.Header.Get("Authorization")
			if auth != "Bearer "+s.apiKey {
				http.Error(w, `{"error":"unauthorized"}`, http.StatusUnauthorized)
				return
			}
		}
		handler(w, r)
	}
}

// --- Handlers ---

func (s *Server) handleHealth(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, map[string]any{
		"status":     "ok",
		"instanceId": s.instanceID,
		"uptime":     int(time.Since(s.startTime).Seconds()),
	})
}

func (s *Server) handleStatus(w http.ResponseWriter, _ *http.Request) {
	status := map[string]any{
		"instanceId":     s.instanceID,
		"uptime":         int(time.Since(s.startTime).Seconds()),
		"activeRequests": s.activeRequests.Load(),
		"totalRequests":  s.totalRequests.Load(),
	}
	if s.registry != nil {
		status["agents"] = s.registry.ListAgents()
		status["agentCount"] = s.registry.Len()
	}
	if s.laneManager != nil {
		status["lanes"] = s.laneManager.Stats()
	}
	if s.configHub != nil {
		cfg := s.configHub.Current()
		status["config"] = map[string]any{
			"model":    cfg.Model,
			"provider": cfg.Provider,
		}
	}
	writeJSON(w, status)
}

func (s *Server) handleLoad(w http.ResponseWriter, _ *http.Request) {
	total := s.totalRequests.Load()
	var avgMs int64
	if total > 0 {
		avgMs = s.totalLatencyMs.Load() / total
	}
	writeJSON(w, map[string]any{
		"activeRequests": s.activeRequests.Load(),
		"totalRequests":  total,
		"avgLatencyMs":   avgMs,
	})
}

// chatRequest is the JSON body for /api/chat.
type chatRequest struct {
	Content    string         `json:"content"`
	SessionKey string         `json:"sessionKey"`
	Channel    string         `json:"channel"`
	ChatID     string         `json:"chatId"`
	PersonID   string         `json:"personId"`
	RoleID     string         `json:"roleId"`
	Mode       string         `json:"mode"`
	Metadata   map[string]any `json:"metadata"`
}

func (s *Server) handleChat(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, `{"error":"method not allowed"}`, http.StatusMethodNotAllowed)
		return
	}

	var req chatRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSONError(w, "invalid JSON", http.StatusBadRequest)
		return
	}
	if req.Content == "" {
		writeJSONError(w, "content is required", http.StatusBadRequest)
		return
	}
	if req.SessionKey == "" {
		req.SessionKey = fmt.Sprintf("%s:%s", req.Channel, req.ChatID)
	}

	s.activeRequests.Add(1)
	start := time.Now()

	defer func() {
		s.activeRequests.Add(-1)
		s.totalRequests.Add(1)
		s.totalLatencyMs.Add(time.Since(start).Milliseconds())
	}()

	// Submit to lane
	mode := lane.Mode(req.Mode)
	result, err := s.laneManager.Submit(r.Context(), lane.ChatRequest{
		Content:    req.Content,
		SessionKey: req.SessionKey,
		Channel:    req.Channel,
		ChatID:     req.ChatID,
		PersonID:   req.PersonID,
		RoleID:     req.RoleID,
		Metadata:   req.Metadata,
	}, mode)

	if err != nil {
		writeJSONError(w, err.Error(), http.StatusInternalServerError)
		return
	}

	writeJSON(w, map[string]any{
		"content":        result.Content,
		"agentId":        result.AgentID,
		"requestsMerged": result.RequestsMerged,
		"latencyMs":      time.Since(start).Milliseconds(),
	})
}

// laneHandler is the actual chat processing function called by the lane worker.
func (s *Server) laneHandler(ctx context.Context, req lane.ChatRequest) lane.ChatResult {
	if s.registry == nil {
		return lane.ChatResult{Error: "no agent registry configured"}
	}

	roleID := req.RoleID
	if roleID == "" {
		roleID = "general"
	}

	resp, err := s.registry.ProcessDirect(ctx, req.Content, req.SessionKey, req.Channel, req.ChatID, roleID)
	if err != nil {
		return lane.ChatResult{Error: err.Error()}
	}

	return lane.ChatResult{
		Content: resp,
		AgentID: roleID,
	}
}

func (s *Server) handleAgents(w http.ResponseWriter, _ *http.Request) {
	if s.registry == nil {
		writeJSON(w, map[string]any{"agents": []any{}, "total": 0})
		return
	}
	writeJSON(w, map[string]any{
		"agents": s.registry.ListAgents(),
		"total":  s.registry.Len(),
	})
}

func (s *Server) handleConfig(w http.ResponseWriter, r *http.Request) {
	if s.configHub == nil {
		writeJSONError(w, "config hub not configured", http.StatusNotImplemented)
		return
	}
	writeJSON(w, s.configHub.Current())
}

// --- WebSocket ---

var wsUpgrader = websocket.Upgrader{
	CheckOrigin: func(r *http.Request) bool { return true },
}

// wsConn wraps a websocket.Conn with a write mutex for thread safety.
// gorilla/websocket does NOT support concurrent writes.
type wsConn struct {
	*websocket.Conn
	mu sync.Mutex
}

func (c *wsConn) WriteJSONSafe(v any) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.Conn.WriteJSON(v)
}

func (c *wsConn) WritePing() error {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.Conn.WriteMessage(websocket.PingMessage, nil)
}

func (c *wsConn) WriteCloseSafe(code int, text string) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.Conn.WriteMessage(websocket.CloseMessage,
		websocket.FormatCloseMessage(code, text))
}

// handleWS is the WebSocket endpoint — Backend connects here for heartbeat and config push.
//
// Protocol:
//
//	nanobot → Backend:  {"type": "heartbeat",  "instanceId": "...", "load": {...}}
//	Backend → nanobot:  {"type": "ping"}       → nanobot replies with pong + load
//	Backend → nanobot:  {"type": "config_update", "data": {...}}
//	Backend → nanobot:  {"type": "task",       "data": {...}}
//
// Fingerprint auth: connect with ?fp=<fingerprint>, mismatch returns 403.
func (s *Server) handleWS(w http.ResponseWriter, r *http.Request) {
	// Fingerprint verification
	if s.wsFingerprint != "" {
		fp := r.URL.Query().Get("fp")
		if fp != s.wsFingerprint {
			peer := r.RemoteAddr
			log.Printf("[WS] 🚫 Fingerprint mismatch: %s (got=%q)", peer, fp)
			http.Error(w, "Invalid fingerprint", http.StatusForbidden)
			return
		}
	}

	raw, err := wsUpgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Printf("[WS] ⚠️ Upgrade failed: %v", err)
		return
	}

	conn := &wsConn{Conn: raw}
	peer := r.RemoteAddr
	log.Printf("[WS] 🔗 Connected: %s ✅", peer)

	s.wsMu.Lock()
	s.wsConns[conn] = true
	s.wsMu.Unlock()

	defer func() {
		raw.Close()
		s.wsMu.Lock()
		delete(s.wsConns, conn)
		remaining := len(s.wsConns)
		s.wsMu.Unlock()

		log.Printf("[WS] 🔌 Disconnected: %s", peer)

		// All connections dropped → trigger re-registration
		if remaining == 0 && s.ReRegisterFn != nil {
			log.Println("[WS] ⚠️ All connections lost, triggering re-registration...")
			go s.ReRegisterFn()
		}
	}()

	// Pong handler: Boss responds to our ping → extend read deadline
	raw.SetReadDeadline(time.Now().Add(60 * time.Second))
	raw.SetPongHandler(func(string) error {
		raw.SetReadDeadline(time.Now().Add(60 * time.Second))
		return nil
	})

	// Also handle any text message as activity → extend deadline
	for {
		_, message, err := raw.ReadMessage()
		if err != nil {
			if websocket.IsUnexpectedCloseError(err, websocket.CloseGoingAway, websocket.CloseNormalClosure) {
				log.Printf("[WS] ⚠️ Error: %v", err)
			}
			break
		}

		// Any message received → extend deadline
		raw.SetReadDeadline(time.Now().Add(60 * time.Second))

		var msg map[string]any
		if err := json.Unmarshal(message, &msg); err != nil {
			continue
		}

		msgType, _ := msg["type"].(string)

		switch msgType {
		case "ping":
			// Backend ping → respond with pong + load stats (via mutex)
			total := s.totalRequests.Load()
			var avgMs int64
			if total > 0 {
				avgMs = s.totalLatencyMs.Load() / total
			}
			pong := map[string]any{
				"type":       "pong",
				"instanceId": s.instanceID,
				"load": map[string]any{
					"activeRequests": s.activeRequests.Load(),
					"totalRequests":  total,
					"avgLatencyMs":   avgMs,
				},
			}
			conn.WriteJSONSafe(pong)

		case "config_update":
			data, _ := msg["data"].(map[string]any)
			log.Printf("[WS] 📡 Config update received: %v", data)
			if s.configHub != nil {
				raw, _ := json.Marshal(data)
				if err := s.configHub.HandleConfigUpdate(raw); err != nil {
					log.Printf("[WS] ⚠️ Config update failed: %v", err)
				}
			}

		case "task":
			data, _ := msg["data"].(map[string]any)
			log.Printf("[WS] 📡 Task push received: %v", data)
		}
	}
}

// heartbeatLoop sends WS-level pings + JSON heartbeat every 10 seconds.
// Mirrors Python aiohttp's heartbeat=10.0 + autoping=True.
func (s *Server) heartbeatLoop(ctx context.Context) {
	ticker := time.NewTicker(10 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			s.broadcastHeartbeat()
		}
	}
}

// broadcastHeartbeat sends WS ping frames + JSON heartbeat to all connections.
func (s *Server) broadcastHeartbeat() {
	s.wsMu.Lock()
	if len(s.wsConns) == 0 {
		s.wsMu.Unlock()
		return
	}
	conns := make([]*wsConn, 0, len(s.wsConns))
	for c := range s.wsConns {
		conns = append(conns, c)
	}
	s.wsMu.Unlock()

	total := s.totalRequests.Load()
	var avgMs int64
	if total > 0 {
		avgMs = s.totalLatencyMs.Load() / total
	}

	payload := map[string]any{
		"type":       "heartbeat",
		"instanceId": s.instanceID,
		"load": map[string]any{
			"activeRequests": s.activeRequests.Load(),
			"totalRequests":  total,
			"avgLatencyMs":   avgMs,
		},
	}

	var dead []*wsConn
	for _, c := range conns {
		// Send WS-level ping to keep connection alive
		if err := c.WritePing(); err != nil {
			dead = append(dead, c)
			continue
		}
		// Send JSON heartbeat for application-level monitoring
		if err := c.WriteJSONSafe(payload); err != nil {
			dead = append(dead, c)
		}
	}

	if len(dead) > 0 {
		s.wsMu.Lock()
		for _, c := range dead {
			delete(s.wsConns, c)
			c.Close()
		}
		s.wsMu.Unlock()
	}
}

// closeAllWS closes all WebSocket connections (called on shutdown).
func (s *Server) closeAllWS() {
	s.wsMu.Lock()
	defer s.wsMu.Unlock()
	for c := range s.wsConns {
		c.WriteCloseSafe(websocket.CloseGoingAway, "server shutdown")
		c.Close()
		delete(s.wsConns, c)
	}
}

// WSConnectionCount returns the number of active WebSocket connections.
func (s *Server) WSConnectionCount() int {
	s.wsMu.Lock()
	defer s.wsMu.Unlock()
	return len(s.wsConns)
}

var jsonOnce sync.Once

func writeJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(v)
}

func writeJSONError(w http.ResponseWriter, msg string, code int) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	json.NewEncoder(w).Encode(map[string]string{"error": msg})
}
