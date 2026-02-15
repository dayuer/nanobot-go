package providers

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// --- Provider Interface Compliance ---

func TestProvider_ImplementsLLMProvider(t *testing.T) {
	var _ LLMProvider = &Provider{}
}

// --- Model Resolution ---

func TestResolveModel_NoGateway_DeepSeek(t *testing.T) {
	p := NewProvider("key", "", "deepseek-chat", "")
	model := p.resolveModel("deepseek-chat")
	assert.Equal(t, "deepseek/deepseek-chat", model)
}

func TestResolveModel_NoGateway_AlreadyPrefixed(t *testing.T) {
	p := NewProvider("key", "", "deepseek/deepseek-chat", "")
	model := p.resolveModel("deepseek/deepseek-chat")
	assert.Equal(t, "deepseek/deepseek-chat", model) // no double-prefix
}

func TestResolveModel_NoGateway_Anthropic(t *testing.T) {
	p := NewProvider("key", "", "claude-3-haiku", "")
	model := p.resolveModel("claude-3-haiku")
	// Anthropic has no litellm_prefix, so model is unchanged
	assert.Equal(t, "claude-3-haiku", model)
}

func TestResolveModel_Gateway_StripPrefix(t *testing.T) {
	// AiHubMix strips provider prefix and adds "openai/"
	p := NewProvider("key", "https://aihubmix.com/v1", "anthropic/claude-3", "aihubmix")
	model := p.resolveModel("anthropic/claude-3")
	assert.Equal(t, "openai/claude-3", model)
}

func TestResolveModel_Gateway_OpenRouter(t *testing.T) {
	p := NewProvider("sk-or-abc", "", "claude-3", "openrouter")
	model := p.resolveModel("claude-3")
	assert.Equal(t, "openrouter/claude-3", model)
}

// --- Response Parsing ---

func TestParseResponse_TextOnly(t *testing.T) {
	p := &Provider{}
	body := `{"choices":[{"message":{"content":"Hello!"},"finish_reason":"stop"}],"usage":{"prompt_tokens":10,"completion_tokens":5,"total_tokens":15}}`
	resp, err := p.parseResponse([]byte(body))
	require.NoError(t, err)
	require.NotNil(t, resp.Content)
	assert.Equal(t, "Hello!", *resp.Content)
	assert.Equal(t, "stop", resp.FinishReason)
	assert.False(t, resp.HasToolCalls())
	assert.Equal(t, 15, resp.Usage["total_tokens"])
}

func TestParseResponse_ToolCalls(t *testing.T) {
	p := &Provider{}
	body := `{"choices":[{"message":{"content":null,"tool_calls":[{"id":"call_1","function":{"name":"read_file","arguments":"{\"path\":\"/tmp/test\"}"}}]},"finish_reason":"tool_calls"}]}`
	resp, err := p.parseResponse([]byte(body))
	require.NoError(t, err)
	assert.True(t, resp.HasToolCalls())
	require.Len(t, resp.ToolCalls, 1)
	assert.Equal(t, "read_file", resp.ToolCalls[0].Name)
	assert.Equal(t, "/tmp/test", resp.ToolCalls[0].Arguments["path"])
}

func TestParseResponse_ReasoningContent(t *testing.T) {
	p := &Provider{}
	body := `{"choices":[{"message":{"content":"Answer","reasoning_content":"Let me think..."},"finish_reason":"stop"}]}`
	resp, err := p.parseResponse([]byte(body))
	require.NoError(t, err)
	require.NotNil(t, resp.ReasoningContent)
	assert.Equal(t, "Let me think...", *resp.ReasoningContent)
}

func TestParseResponse_EmptyChoices(t *testing.T) {
	p := &Provider{}
	body := `{"choices":[]}`
	resp, err := p.parseResponse([]byte(body))
	require.NoError(t, err)
	assert.Contains(t, *resp.Content, "no choices")
}

func TestParseResponse_InvalidJSON(t *testing.T) {
	p := &Provider{}
	resp, err := p.parseResponse([]byte("not json"))
	require.NoError(t, err) // graceful error
	assert.Contains(t, *resp.Content, "Error parsing")
}

// --- Integration with Mock Server ---

func TestProvider_Chat_MockServer(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "POST", r.Method)
		assert.Equal(t, "application/json", r.Header.Get("Content-Type"))
		assert.Equal(t, "Bearer test-key", r.Header.Get("Authorization"))

		var body map[string]any
		json.NewDecoder(r.Body).Decode(&body)
		assert.Equal(t, "gpt-4", body["model"])

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{
			"choices": []map[string]any{
				{
					"message":       map[string]any{"content": "I am GPT-4"},
					"finish_reason": "stop",
				},
			},
			"usage": map[string]any{"prompt_tokens": 5, "completion_tokens": 3, "total_tokens": 8},
		})
	}))
	defer server.Close()

	p := NewProvider("test-key", server.URL, "gpt-4", "")
	resp, err := p.Chat(context.Background(), ChatRequest{
		Messages: []Message{{Role: "user", Content: "Hello"}},
		Model:    "gpt-4",
	})
	require.NoError(t, err)
	require.NotNil(t, resp.Content)
	assert.Equal(t, "I am GPT-4", *resp.Content)
	assert.Equal(t, 8, resp.Usage["total_tokens"])
}

func TestProvider_Chat_ToolCalls_MockServer(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body map[string]any
		json.NewDecoder(r.Body).Decode(&body)
		// Verify tools were sent
		assert.NotNil(t, body["tools"])
		assert.Equal(t, "auto", body["tool_choice"])

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{
			"choices": []map[string]any{
				{
					"message": map[string]any{
						"content": nil,
						"tool_calls": []map[string]any{
							{
								"id": "call_abc",
								"function": map[string]any{
									"name":      "exec",
									"arguments": `{"command":"ls"}`,
								},
							},
						},
					},
					"finish_reason": "tool_calls",
				},
			},
		})
	}))
	defer server.Close()

	p := NewProvider("key", server.URL, "gpt-4", "")
	resp, err := p.Chat(context.Background(), ChatRequest{
		Messages: []Message{{Role: "user", Content: "List files"}},
		Tools: []map[string]any{
			{"type": "function", "function": map[string]any{"name": "exec"}},
		},
	})
	require.NoError(t, err)
	assert.True(t, resp.HasToolCalls())
	assert.Equal(t, "exec", resp.ToolCalls[0].Name)
	assert.Equal(t, "ls", resp.ToolCalls[0].Arguments["command"])
}

func TestProvider_Chat_HTTPError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(500)
		w.Write([]byte("internal error"))
	}))
	defer server.Close()

	p := NewProvider("key", server.URL, "gpt-4", "")
	resp, err := p.Chat(context.Background(), ChatRequest{
		Messages: []Message{{Role: "user", Content: "Hello"}},
	})
	require.NoError(t, err)
	assert.Contains(t, *resp.Content, "Error calling LLM")
	assert.Equal(t, "error", resp.FinishReason)
}

func TestProvider_Chat_ExtraHeaders(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "my-code", r.Header.Get("APP-Code"))
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{
			"choices": []map[string]any{
				{"message": map[string]any{"content": "ok"}, "finish_reason": "stop"},
			},
		})
	}))
	defer server.Close()

	p := NewProvider("key", server.URL, "gpt-4", "")
	p.ExtraHeaders = map[string]string{"APP-Code": "my-code"}
	resp, err := p.Chat(context.Background(), ChatRequest{
		Messages: []Message{{Role: "user", Content: "test"}},
	})
	require.NoError(t, err)
	assert.Equal(t, "ok", *resp.Content)
}

// --- DefaultModel ---

func TestProvider_DefaultModel(t *testing.T) {
	p := NewProvider("", "", "gpt-4", "")
	assert.Equal(t, "gpt-4", p.DefaultModel())
}

func TestProvider_DefaultModelFallback(t *testing.T) {
	p := NewProvider("", "", "", "")
	assert.Equal(t, "anthropic/claude-sonnet-4-5", p.DefaultModel())
}
