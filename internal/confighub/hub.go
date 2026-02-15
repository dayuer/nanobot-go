// Package confighub manages dynamic LLM configuration.
//
// It fetches config from a registry center on startup and supports
// runtime hot-switching via WebSocket config_update push events.
//
// Config priority (highest wins):
//
//	Layer 3: Runtime push (WS config_update)
//	Layer 2: Registry fetch (startup GET /api/config)
//	Layer 1: Local fallback (config.json / env vars)
package confighub

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"sync"
	"time"
)

// LLMConfig is the dynamic LLM configuration fetched from the registry center.
type LLMConfig struct {
	Model       string  `json:"model"`
	APIKey      string  `json:"apiKey"`
	APIBase     string  `json:"apiBase"`
	Temperature float64 `json:"temperature"`
	MaxTokens   int     `json:"maxTokens"`
	Provider    string  `json:"provider"` // e.g. "deepseek", "openai", "anthropic"

	// Per-agent overrides (optional).
	// Key: agentID, Value: agent-specific LLM settings.
	AgentOverrides map[string]AgentLLMConfig `json:"agentOverrides,omitempty"`
}

// AgentLLMConfig holds per-agent LLM overrides.
// Zero-value fields inherit from the parent LLMConfig.
type AgentLLMConfig struct {
	Model       string  `json:"model,omitempty"`
	APIKey      string  `json:"apiKey,omitempty"`
	APIBase     string  `json:"apiBase,omitempty"`
	Temperature float64 `json:"temperature,omitempty"`
	MaxTokens   int     `json:"maxTokens,omitempty"`
	Provider    string  `json:"provider,omitempty"`
}

// Resolve returns the effective LLM settings for a given agent,
// falling back to the parent config for unset fields.
func (c *LLMConfig) Resolve(agentID string) AgentLLMConfig {
	base := AgentLLMConfig{
		Model:       c.Model,
		APIKey:      c.APIKey,
		APIBase:     c.APIBase,
		Temperature: c.Temperature,
		MaxTokens:   c.MaxTokens,
		Provider:    c.Provider,
	}
	ov, ok := c.AgentOverrides[agentID]
	if !ok {
		return base
	}
	if ov.Model != "" {
		base.Model = ov.Model
	}
	if ov.APIKey != "" {
		base.APIKey = ov.APIKey
	}
	if ov.APIBase != "" {
		base.APIBase = ov.APIBase
	}
	if ov.Temperature != 0 {
		base.Temperature = ov.Temperature
	}
	if ov.MaxTokens != 0 {
		base.MaxTokens = ov.MaxTokens
	}
	if ov.Provider != "" {
		base.Provider = ov.Provider
	}
	return base
}

// ConfigHub manages dynamic configuration with registry fetch + runtime push.
type ConfigHub struct {
	mu          sync.RWMutex
	registryURL string // registry center base URL (empty = disabled)
	instanceID  string
	apiKey      string // auth token for registry API
	current     *LLMConfig
	onChange    []func(*LLMConfig)
	httpClient  *http.Client
}

// Option configures a ConfigHub.
type Option func(*ConfigHub)

// WithRegistryURL sets the registry center URL.
func WithRegistryURL(url string) Option {
	return func(h *ConfigHub) { h.registryURL = url }
}

// WithInstanceID sets the nanobot instance ID.
func WithInstanceID(id string) Option {
	return func(h *ConfigHub) { h.instanceID = id }
}

// WithAPIKey sets the auth token for registry calls.
func WithAPIKey(key string) Option {
	return func(h *ConfigHub) { h.apiKey = key }
}

// New creates a ConfigHub with a local fallback config applied.
func New(fallback LLMConfig, opts ...Option) *ConfigHub {
	h := &ConfigHub{
		current:    &fallback,
		httpClient: &http.Client{Timeout: 10 * time.Second},
	}
	for _, o := range opts {
		o(h)
	}
	return h
}

// Current returns the currently-active LLM config (thread-safe).
func (h *ConfigHub) Current() *LLMConfig {
	h.mu.RLock()
	defer h.mu.RUnlock()
	cfg := *h.current // shallow copy
	return &cfg
}

// OnChange registers a callback invoked when config changes.
// Callbacks are called synchronously in the order registered;
// long-running work should be spawned in a goroutine.
func (h *ConfigHub) OnChange(fn func(*LLMConfig)) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.onChange = append(h.onChange, fn)
}

// Fetch pulls config from the registry center.
// If the registry URL is empty or the request fails, it keeps the current
// (local fallback) config and returns the error without crashing.
func (h *ConfigHub) Fetch(ctx context.Context) error {
	if h.registryURL == "" {
		log.Println("[ConfigHub] No registry URL configured, using local config")
		return nil
	}

	url := fmt.Sprintf("%s/api/nanobot/config?instanceId=%s", h.registryURL, h.instanceID)
	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return fmt.Errorf("build request: %w", err)
	}
	if h.apiKey != "" {
		req.Header.Set("Authorization", "Bearer "+h.apiKey)
	}

	resp, err := h.httpClient.Do(req)
	if err != nil {
		log.Printf("[ConfigHub] ⚠️ Registry fetch failed: %v (keeping local config)", err)
		return fmt.Errorf("registry fetch: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNotFound {
		log.Println("[ConfigHub] ℹ️ Config endpoint not available on backend (404), using local config")
		return nil
	}

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		log.Printf("[ConfigHub] ⚠️ Registry returned %d: %s", resp.StatusCode, string(body))
		return fmt.Errorf("registry returned HTTP %d", resp.StatusCode)
	}

	var cfg LLMConfig
	if err := json.NewDecoder(resp.Body).Decode(&cfg); err != nil {
		return fmt.Errorf("decode response: %w", err)
	}

	return h.Apply(&cfg)
}

// Apply sets a new config and fires all onChange callbacks.
func (h *ConfigHub) Apply(cfg *LLMConfig) error {
	h.mu.Lock()
	old := h.current
	h.current = cfg
	callbacks := make([]func(*LLMConfig), len(h.onChange))
	copy(callbacks, h.onChange)
	h.mu.Unlock()

	log.Printf("[ConfigHub] ✅ Config updated: model=%s provider=%s", cfg.Model, cfg.Provider)
	if old.Model != cfg.Model {
		log.Printf("[ConfigHub]   model: %s → %s", old.Model, cfg.Model)
	}

	for _, fn := range callbacks {
		fn(cfg)
	}
	return nil
}

// HandleConfigUpdate processes a WebSocket config_update message.
// Expected format: {"model": "...", "apiKey": "...", ...}
func (h *ConfigHub) HandleConfigUpdate(data json.RawMessage) error {
	// Start from current config so unset fields are preserved.
	h.mu.RLock()
	merged := *h.current
	h.mu.RUnlock()

	if err := json.Unmarshal(data, &merged); err != nil {
		return fmt.Errorf("unmarshal config update: %w", err)
	}

	return h.Apply(&merged)
}
