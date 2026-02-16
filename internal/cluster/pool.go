// pool.go — Backend pool registration lifecycle.
//
// Mirrors survival/nanobot/server.py's bootstrap / register / unregister flow.
//
// Lifecycle:
//   1. Bootstrap  → POST /api/nanobot/pool {"action":"bootstrap"} → get instanceId
//   2. Register   → POST /api/nanobot/pool {"action":"register"}  → join pool
//   3. Heartbeat  → WS broadcast every 5s (handled by server)
//   4. Unregister → POST /api/nanobot/pool {"action":"unregister"} → leave pool on shutdown
package cluster

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"os"
	"strings"
	"time"
)

// PoolClient manages the nanobot instance's lifecycle with the backend pool.
type PoolClient struct {
	backendURL  string
	apiKey      string
	instanceID  string
	selfURL     string
	model       string
	toolCount   int
	fingerprint string
	httpClient  *http.Client
}

// PoolConfig holds pool registration settings.
type PoolConfig struct {
	BackendURL  string // SURVIVAL_API_URL
	APIKey      string // NANOBOT_API_KEY (for Authorization: Bearer)
	Port        int
	Model       string
	ToolCount   int
	Fingerprint string // WS fingerprint (uuid hex[:16])
	SelfURL     string // explicit override (optional)
}

// NewPoolClient creates a pool client.
func NewPoolClient(cfg PoolConfig) *PoolClient {
	selfURL := cfg.SelfURL
	if selfURL == "" {
		// Auto-detect: connect to backend IP to find correct LAN interface
		localIP := detectLocalIPVia(cfg.BackendURL)
		selfURL = fmt.Sprintf("http://%s:%d", localIP, cfg.Port)
	}

	return &PoolClient{
		backendURL:  cfg.BackendURL,
		apiKey:      cfg.APIKey,
		selfURL:     selfURL,
		model:       cfg.Model,
		toolCount:   cfg.ToolCount,
		fingerprint: cfg.Fingerprint,
		httpClient:  &http.Client{Timeout: 10 * time.Second},
	}
}

// Bootstrap requests an instanceId from the backend.
// POST /api/nanobot/pool {"action":"bootstrap","ip","hostname","port"}
func (p *PoolClient) Bootstrap(port int) (string, error) {
	if p.backendURL == "" {
		hostname, _ := os.Hostname()
		return fmt.Sprintf("nanobot-%s-%d", hostname, port), nil
	}

	localIP := detectLocalIPVia(p.backendURL)
	hostname, _ := os.Hostname()
	if h := os.Getenv("HOSTNAME"); h != "" {
		hostname = h
	}

	payload := map[string]any{
		"action":   "bootstrap",
		"ip":       localIP,
		"hostname": hostname,
		"port":     port,
	}

	data, err := p.postPool(payload)
	if err != nil {
		log.Printf("[Pool] ⚠️ Bootstrap failed (%s): %v", p.backendURL, err)
		return fmt.Sprintf("nanobot-%s-%d", hostname, port), nil
	}

	if id, ok := data["instanceId"].(string); ok && id != "" {
		p.instanceID = id
		return id, nil
	}

	return fmt.Sprintf("nanobot-%s-%d", hostname, port), nil
}

// Register registers this instance with the backend pool (single attempt).
// POST /api/nanobot/pool {"action":"register","instanceId","url","model","toolCount","wsFingerprint"}
func (p *PoolClient) Register() error {
	if p.backendURL == "" {
		log.Println("[Pool] ℹ️ No backend URL, skipping pool registration")
		return nil
	}

	payload := map[string]any{
		"action":        "register",
		"instanceId":    p.instanceID,
		"url":           p.selfURL,
		"model":         p.model,
		"toolCount":     p.toolCount,
		"wsFingerprint": p.fingerprint,
	}

	data, err := p.postPool(payload)
	if err != nil {
		log.Printf("[Pool] ⚠️ Registration failed (%s): %v", p.backendURL, err)
		return err
	}

	if success, ok := data["success"].(bool); ok && success {
		log.Printf("[Pool] 🟢 Registered to backend pool: %s", p.instanceID)
	} else {
		log.Printf("[Pool] ⚠️ Registration response: %v", data)
	}

	return nil
}

// RegisterWithRetry retries registration every 5s until success or context cancellation.
// Use this for initial startup and WS disconnect re-registration.
func (p *PoolClient) RegisterWithRetry(ctx context.Context) {
	if p.backendURL == "" {
		return
	}

	// First attempt
	if err := p.Register(); err == nil {
		return // success on first try
	}
	log.Println("[Pool] → Will retry registration every 5s...")

	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			log.Println("[Pool] ⛔ Registration retry cancelled")
			return
		case <-ticker.C:
			if err := p.Register(); err == nil {
				return // success
			}
		}
	}
}

// Unregister removes this instance from the backend pool (called on shutdown).
// POST /api/nanobot/pool {"action":"unregister","instanceId"}
func (p *PoolClient) Unregister() {
	if p.backendURL == "" || p.instanceID == "" {
		return
	}

	payload := map[string]any{
		"action":     "unregister",
		"instanceId": p.instanceID,
	}

	// Use a short timeout for shutdown
	client := &http.Client{Timeout: 5 * time.Second}
	body, _ := json.Marshal(payload)
	req, err := http.NewRequest("POST", p.backendURL+"/api/nanobot/pool", bytes.NewReader(body))
	if err != nil {
		return
	}
	req.Header.Set("Content-Type", "application/json")
	if p.apiKey != "" {
		req.Header.Set("Authorization", "Bearer "+p.apiKey)
	}

	resp, err := client.Do(req)
	if err != nil {
		return // Don't report errors on shutdown
	}
	resp.Body.Close()
	log.Printf("[Pool] 👋 Unregistered from backend pool: %s", p.instanceID)
}

// SetInstanceID sets the instance ID (used when bootstrapped externally).
func (p *PoolClient) SetInstanceID(id string) {
	p.instanceID = id
}

// InstanceID returns the current instance ID.
func (p *PoolClient) InstanceID() string {
	return p.instanceID
}

// SelfURL returns the self URL.
func (p *PoolClient) SelfURL() string {
	return p.selfURL
}

// --- internal helpers ---

func (p *PoolClient) postPool(payload map[string]any) (map[string]any, error) {
	body, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("marshal: %w", err)
	}

	req, err := http.NewRequest("POST", p.backendURL+"/api/nanobot/pool", bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	if p.apiKey != "" {
		req.Header.Set("Authorization", "Bearer "+p.apiKey)
	}

	resp, err := p.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("request: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read response: %w", err)
	}

	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("HTTP %d: %s", resp.StatusCode, string(respBody))
	}

	var data map[string]any
	if err := json.Unmarshal(respBody, &data); err != nil {
		return nil, fmt.Errorf("decode response: %w", err)
	}
	return data, nil
}

// detectLocalIPVia auto-detects the local IP that can reach the given URL.
// It connects to the target host to determine which local interface is used,
// ensuring the registry gets a reachable address (not a VPN/tunnel IP).
func detectLocalIPVia(targetURL string) string {
	// Parse host from URL
	target := "8.8.8.8:80" // fallback
	if targetURL != "" {
		// Extract host:port from URL like "http://192.168.3.129:3000"
		u := targetURL
		// Strip scheme
		if idx := strings.Index(u, "://"); idx >= 0 {
			u = u[idx+3:]
		}
		// Strip path
		if idx := strings.Index(u, "/"); idx >= 0 {
			u = u[:idx]
		}
		// Ensure port
		if !strings.Contains(u, ":") {
			u = u + ":80"
		}
		target = u
	}

	conn, err := net.DialTimeout("udp", target, 2*time.Second)
	if err != nil {
		return "127.0.0.1"
	}
	defer conn.Close()
	addr := conn.LocalAddr().(*net.UDPAddr)
	return addr.IP.String()
}
