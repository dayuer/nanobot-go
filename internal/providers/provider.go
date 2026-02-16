// Package providers — provider.go
// OpenAI-compatible LLM provider using standard HTTP.
// Mirrors upstream litellm_provider.py but uses Go's net/http directly,
// calling OpenAI-compatible endpoints (works with Anthropic via OpenRouter,
// direct OpenAI, DeepSeek, etc.).
package providers

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"
)

// Provider is an OpenAI-compatible LLM provider.
type Provider struct {
	APIKey       string
	APIBase      string
	Model        string // default model
	ExtraHeaders map[string]string
	HTTPClient   *http.Client

	gateway *ProviderSpec // detected gateway, if any
}

// NewProvider creates a Provider with given config.
func NewProvider(apiKey, apiBase, defaultModel, providerName string) *Provider {
	if defaultModel == "" {
		defaultModel = "anthropic/claude-sonnet-4-5"
	}

	p := &Provider{
		APIKey:     apiKey,
		APIBase:    apiBase,
		Model:      defaultModel,
		HTTPClient: &http.Client{Timeout: 120 * time.Second},
	}

	p.gateway = FindGateway(providerName, apiKey, apiBase)
	return p
}

// DefaultModel satisfies the LLMProvider interface.
func (p *Provider) DefaultModel() string { return p.Model }

// Chat sends a chat completion request.
func (p *Provider) Chat(ctx context.Context, req ChatRequest) (*LLMResponse, error) {
	model := req.Model
	if model == "" {
		model = p.Model
	}
	model = p.resolveModel(model)

	maxTokens := req.MaxTokens
	if maxTokens < 1 {
		maxTokens = 4096
	}

	temp := req.Temperature
	// Apply model overrides
	p.applyModelOverrides(model, &temp)

	// Build OpenAI-compatible request body
	body := map[string]any{
		"model":       model,
		"messages":    req.Messages,
		"max_tokens":  maxTokens,
		"temperature": temp,
	}
	if len(req.Tools) > 0 {
		body["tools"] = req.Tools
		body["tool_choice"] = "auto"
	}

	jsonBody, err := json.Marshal(body)
	if err != nil {
		return nil, fmt.Errorf("marshal request: %w", err)
	}

	apiBase := p.APIBase
	apiKey := p.APIKey

	// Auto-resolve: if no explicit API base, look up the provider spec by model name
	// and use its DefaultAPIBase + EnvKey. This enables config-driven multi-provider.
	if apiBase == "" {
		spec := FindByModel(model)
		if spec != nil {
			if spec.DefaultAPIBase != "" {
				apiBase = spec.DefaultAPIBase
			}
			// Also resolve API key from env if not already set
			if apiKey == "" && spec.EnvKey != "" {
				apiKey = os.Getenv(spec.EnvKey)
			}
			// Handle extra env vars (e.g. zhipu sets ZHIPUAI_API_KEY from ZAI_API_KEY)
			for _, extra := range spec.EnvExtras {
				envName := extra[0]
				envVal := extra[1]
				if envVal == "{api_key}" && apiKey != "" {
					os.Setenv(envName, apiKey)
				}
			}
		}
	}

	if apiBase == "" {
		apiBase = "https://api.openai.com/v1"
	}
	endpoint := strings.TrimRight(apiBase, "/") + "/chat/completions"

	httpReq, err := http.NewRequestWithContext(ctx, "POST", endpoint, bytes.NewReader(jsonBody))
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")
	if apiKey != "" {
		httpReq.Header.Set("Authorization", "Bearer "+apiKey)
	}
	for k, v := range p.ExtraHeaders {
		httpReq.Header.Set(k, v)
	}

	resp, err := p.HTTPClient.Do(httpReq)
	if err != nil {
		return &LLMResponse{
			Content:      strPtr(fmt.Sprintf("Error calling LLM: %v", err)),
			FinishReason: "error",
		}, nil
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return &LLMResponse{
			Content:      strPtr(fmt.Sprintf("Error reading response: %v", err)),
			FinishReason: "error",
		}, nil
	}

	if resp.StatusCode != 200 {
		return &LLMResponse{
			Content:      strPtr(fmt.Sprintf("Error calling LLM (HTTP %d): %s", resp.StatusCode, string(respBody))),
			FinishReason: "error",
		}, nil
	}

	return p.parseResponse(respBody)
}

func (p *Provider) resolveModel(model string) string {
	if p.gateway != nil {
		prefix := p.gateway.LiteLLMPrefix
		if p.gateway.StripModelPrefix {
			parts := strings.SplitN(model, "/", 2)
			model = parts[len(parts)-1]
		}
		if prefix != "" && !strings.HasPrefix(model, prefix+"/") {
			model = prefix + "/" + model
		}
		return model
	}

	// When calling a provider's own API directly (not gateway),
	// strip the "provider/" prefix from model names like "deepseek/deepseek-chat"
	spec := FindByModel(model)
	if spec != nil && spec.DefaultAPIBase != "" {
		// Direct API call — strip prefix
		if idx := strings.Index(model, "/"); idx >= 0 {
			model = model[idx+1:]
		}
	}
	return model
}

func (p *Provider) applyModelOverrides(model string, temperature *float64) {
	lower := strings.ToLower(model)
	spec := FindByModel(model)
	if spec == nil {
		return
	}
	for _, ov := range spec.ModelOverrides {
		if strings.Contains(lower, ov.Pattern) {
			if t, ok := ov.Overrides["temperature"].(float64); ok {
				*temperature = t
			}
			return
		}
	}
}

// openAIResponse mirrors the OpenAI chat completion response structure.
type openAIResponse struct {
	Choices []struct {
		Message struct {
			Content          *string `json:"content"`
			ReasoningContent *string `json:"reasoning_content"`
			ToolCalls        []struct {
				ID       string `json:"id"`
				Function struct {
					Name      string `json:"name"`
					Arguments string `json:"arguments"`
				} `json:"function"`
			} `json:"tool_calls"`
		} `json:"message"`
		FinishReason string `json:"finish_reason"`
	} `json:"choices"`
	Usage *struct {
		PromptTokens     int `json:"prompt_tokens"`
		CompletionTokens int `json:"completion_tokens"`
		TotalTokens      int `json:"total_tokens"`
	} `json:"usage"`
}

func (p *Provider) parseResponse(body []byte) (*LLMResponse, error) {
	var resp openAIResponse
	if err := json.Unmarshal(body, &resp); err != nil {
		return &LLMResponse{
			Content:      strPtr(fmt.Sprintf("Error parsing response: %v", err)),
			FinishReason: "error",
		}, nil
	}

	if len(resp.Choices) == 0 {
		return &LLMResponse{
			Content:      strPtr("Error: no choices in response"),
			FinishReason: "error",
		}, nil
	}

	choice := resp.Choices[0]
	msg := choice.Message

	var toolCalls []ToolCallRequest
	for _, tc := range msg.ToolCalls {
		var args map[string]any
		if tc.Function.Arguments != "" {
			json.Unmarshal([]byte(tc.Function.Arguments), &args)
		}
		toolCalls = append(toolCalls, ToolCallRequest{
			ID:        tc.ID,
			Name:      tc.Function.Name,
			Arguments: args,
		})
	}

	usage := map[string]int{}
	if resp.Usage != nil {
		usage["prompt_tokens"] = resp.Usage.PromptTokens
		usage["completion_tokens"] = resp.Usage.CompletionTokens
		usage["total_tokens"] = resp.Usage.TotalTokens
	}

	finishReason := choice.FinishReason
	if finishReason == "" {
		finishReason = "stop"
	}

	return &LLMResponse{
		Content:          msg.Content,
		ToolCalls:        toolCalls,
		FinishReason:     finishReason,
		Usage:            usage,
		ReasoningContent: msg.ReasoningContent,
	}, nil
}

func strPtr(s string) *string { return &s }
