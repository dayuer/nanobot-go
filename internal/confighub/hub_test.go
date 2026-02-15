package confighub

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
)

func TestNew_LocalFallback(t *testing.T) {
	fallback := LLMConfig{
		Model:       "deepseek/deepseek-chat",
		APIKey:      "sk-local",
		Temperature: 0.7,
		MaxTokens:   4096,
		Provider:    "deepseek",
	}
	hub := New(fallback)
	got := hub.Current()

	if got.Model != "deepseek/deepseek-chat" {
		t.Errorf("Model = %q, want %q", got.Model, "deepseek/deepseek-chat")
	}
	if got.APIKey != "sk-local" {
		t.Errorf("APIKey = %q, want %q", got.APIKey, "sk-local")
	}
}

func TestFetch_OverridesLocal(t *testing.T) {
	// Registry returns different config
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("instanceId") != "test-1" {
			t.Errorf("instanceId = %q, want %q", r.URL.Query().Get("instanceId"), "test-1")
		}
		json.NewEncoder(w).Encode(LLMConfig{
			Model:       "gpt-4o",
			APIKey:      "sk-registry",
			Temperature: 0.3,
			MaxTokens:   8192,
			Provider:    "openai",
		})
	}))
	defer srv.Close()

	hub := New(
		LLMConfig{Model: "local-model", APIKey: "sk-local"},
		WithRegistryURL(srv.URL),
		WithInstanceID("test-1"),
	)

	if err := hub.Fetch(context.Background()); err != nil {
		t.Fatalf("Fetch() error: %v", err)
	}

	got := hub.Current()
	if got.Model != "gpt-4o" {
		t.Errorf("after Fetch, Model = %q, want %q", got.Model, "gpt-4o")
	}
	if got.APIKey != "sk-registry" {
		t.Errorf("after Fetch, APIKey = %q, want %q", got.APIKey, "sk-registry")
	}
	if got.Provider != "openai" {
		t.Errorf("after Fetch, Provider = %q, want %q", got.Provider, "openai")
	}
}

func TestFetch_RegistryDown_KeepsLocal(t *testing.T) {
	hub := New(
		LLMConfig{Model: "local-model"},
		WithRegistryURL("http://localhost:1"), // unreachable
	)

	err := hub.Fetch(context.Background())
	if err == nil {
		t.Fatal("expected error from unreachable registry")
	}

	got := hub.Current()
	if got.Model != "local-model" {
		t.Errorf("after failed Fetch, Model = %q, want %q", got.Model, "local-model")
	}
}

func TestFetch_NoRegistryURL(t *testing.T) {
	hub := New(LLMConfig{Model: "local-model"})
	if err := hub.Fetch(context.Background()); err != nil {
		t.Errorf("Fetch with no registry URL should not error, got: %v", err)
	}
}

func TestApply_FiresCallbacks(t *testing.T) {
	hub := New(LLMConfig{Model: "old-model"})

	var called atomic.Int32
	hub.OnChange(func(cfg *LLMConfig) {
		called.Add(1)
		if cfg.Model != "new-model" {
			t.Errorf("callback got Model = %q, want %q", cfg.Model, "new-model")
		}
	})

	if err := hub.Apply(&LLMConfig{Model: "new-model"}); err != nil {
		t.Fatalf("Apply() error: %v", err)
	}
	if called.Load() != 1 {
		t.Errorf("callback called %d times, want 1", called.Load())
	}
}

func TestHandleConfigUpdate_MergesPartial(t *testing.T) {
	hub := New(LLMConfig{
		Model:       "deepseek-chat",
		APIKey:      "sk-keep",
		Temperature: 0.7,
		MaxTokens:   4096,
	})

	// Partial update: only change model
	data := json.RawMessage(`{"model": "gpt-4o-mini"}`)
	if err := hub.HandleConfigUpdate(data); err != nil {
		t.Fatalf("HandleConfigUpdate() error: %v", err)
	}

	got := hub.Current()
	if got.Model != "gpt-4o-mini" {
		t.Errorf("Model = %q, want %q", got.Model, "gpt-4o-mini")
	}
	if got.APIKey != "sk-keep" {
		t.Errorf("APIKey should be preserved, got %q", got.APIKey)
	}
	if got.Temperature != 0.7 {
		t.Errorf("Temperature should be preserved, got %f", got.Temperature)
	}
}

func TestResolve_PerAgentOverride(t *testing.T) {
	cfg := LLMConfig{
		Model:       "deepseek-chat",
		APIKey:      "sk-default",
		Temperature: 0.7,
		MaxTokens:   4096,
		AgentOverrides: map[string]AgentLLMConfig{
			"stockgod": {Model: "gpt-4o", Temperature: 0.3},
		},
	}

	// Agent with override
	got := cfg.Resolve("stockgod")
	if got.Model != "gpt-4o" {
		t.Errorf("stockgod Model = %q, want %q", got.Model, "gpt-4o")
	}
	if got.Temperature != 0.3 {
		t.Errorf("stockgod Temperature = %f, want 0.3", got.Temperature)
	}
	if got.APIKey != "sk-default" {
		t.Errorf("stockgod APIKey should inherit, got %q", got.APIKey)
	}

	// Agent without override
	got2 := cfg.Resolve("general")
	if got2.Model != "deepseek-chat" {
		t.Errorf("general Model = %q, want %q", got2.Model, "deepseek-chat")
	}
}
