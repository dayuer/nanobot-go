// Package providers defines the LLM provider interface and response types.
package providers

import "context"

// ToolCallRequest represents a tool call from the LLM.
type ToolCallRequest struct {
	ID        string         `json:"id"`
	Name      string         `json:"name"`
	Arguments map[string]any `json:"arguments"`
}

// LLMResponse is the standardized response from any LLM provider.
type LLMResponse struct {
	Content          *string           `json:"content"`
	ToolCalls        []ToolCallRequest `json:"tool_calls,omitempty"`
	FinishReason     string            `json:"finish_reason"`
	Usage            map[string]int    `json:"usage,omitempty"`
	ReasoningContent *string           `json:"reasoning_content,omitempty"`
}

// HasToolCalls returns true if the response contains tool calls.
func (r *LLMResponse) HasToolCalls() bool {
	return len(r.ToolCalls) > 0
}

// Message represents a chat message.
type Message struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

// ChatRequest holds all parameters for a chat completion call.
type ChatRequest struct {
	Messages    []Message      `json:"messages"`
	Tools       []map[string]any `json:"tools,omitempty"`
	Model       string         `json:"model,omitempty"`
	MaxTokens   int            `json:"max_tokens"`
	Temperature float64        `json:"temperature"`
}

// LLMProvider is the interface for all LLM backends.
// Mirrors Python's providers/base.py LLMProvider ABC.
type LLMProvider interface {
	// Chat sends a chat completion request.
	Chat(ctx context.Context, req ChatRequest) (*LLMResponse, error)

	// DefaultModel returns the default model identifier.
	DefaultModel() string
}
