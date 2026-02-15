package agent

import (
	"context"
	"testing"

	"github.com/dayuer/nanobot-go/internal/bus"
	"github.com/dayuer/nanobot-go/internal/providers"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// mockProvider implements providers.LLMProvider for testing.
type mockProvider struct {
	responses []*providers.LLMResponse
	callCount int
}

func (m *mockProvider) Chat(_ context.Context, _ providers.ChatRequest) (*providers.LLMResponse, error) {
	if m.callCount >= len(m.responses) {
		s := "No more responses"
		return &providers.LLMResponse{Content: &s, FinishReason: "stop"}, nil
	}
	resp := m.responses[m.callCount]
	m.callCount++
	return resp, nil
}

func (m *mockProvider) DefaultModel() string { return "mock-model" }

func strP(s string) *string { return &s }

func TestAgentLoop_RunAgentLoop_TextOnly(t *testing.T) {
	mp := &mockProvider{
		responses: []*providers.LLMResponse{
			{Content: strP("Hello human!"), FinishReason: "stop"},
		},
	}
	msgBus := bus.NewMessageBus()
	loop := NewAgentLoop(msgBus, mp, AgentConfig{
		Workspace: t.TempDir(),
	})

	ctx := context.Background()
	msgs := []map[string]any{
		{"role": "system", "content": "You are helpful"},
		{"role": "user", "content": "Hi"},
	}
	content, toolsUsed, err := loop.RunAgentLoop(ctx, msgs)
	require.NoError(t, err)
	assert.Equal(t, "Hello human!", content)
	assert.Empty(t, toolsUsed)
}

func TestAgentLoop_RunAgentLoop_WithToolCalls(t *testing.T) {
	mp := &mockProvider{
		responses: []*providers.LLMResponse{
			{
				Content:      strP(""),
				FinishReason: "tool_calls",
				ToolCalls: []providers.ToolCallRequest{
					{ID: "call_1", Name: "list_dir", Arguments: map[string]any{"path": "/tmp"}},
				},
			},
			{Content: strP("Directory listed"), FinishReason: "stop"},
		},
	}
	msgBus := bus.NewMessageBus()
	loop := NewAgentLoop(msgBus, mp, AgentConfig{
		Workspace: t.TempDir(),
	})
	// Register a list_dir tool so it can be executed
	loop.Tools.Register(&mockToolForLoop{name: "list_dir"})

	ctx := context.Background()
	msgs := []map[string]any{
		{"role": "user", "content": "List /tmp"},
	}
	content, toolsUsed, err := loop.RunAgentLoop(ctx, msgs)
	require.NoError(t, err)
	assert.Equal(t, "Directory listed", content)
	assert.Contains(t, toolsUsed, "list_dir")
}

func TestAgentLoop_MaxIterations(t *testing.T) {
	// Provider always returns tool calls — should hit max iterations
	mp := &mockProvider{
		responses: make([]*providers.LLMResponse, 100),
	}
	for i := range mp.responses {
		mp.responses[i] = &providers.LLMResponse{
			Content:      strP(""),
			FinishReason: "tool_calls",
			ToolCalls:    []providers.ToolCallRequest{{ID: "c", Name: "noop", Arguments: nil}},
		}
	}
	msgBus := bus.NewMessageBus()
	loop := NewAgentLoop(msgBus, mp, AgentConfig{
		Workspace:     t.TempDir(),
		MaxIterations: 3,
	})
	loop.Tools.Register(&mockToolForLoop{name: "noop"})

	content, _, err := loop.RunAgentLoop(context.Background(), nil)
	require.NoError(t, err)
	assert.Equal(t, "Max iterations reached", content)
	assert.Equal(t, 3, mp.callCount)
}

func TestAgentLoop_ProcessDirect(t *testing.T) {
	mp := &mockProvider{
		responses: []*providers.LLMResponse{
			{Content: strP("CLI response"), FinishReason: "stop"},
		},
	}
	msgBus := bus.NewMessageBus()
	loop := NewAgentLoop(msgBus, mp, AgentConfig{
		Workspace: t.TempDir(),
	})

	content, err := loop.ProcessDirect(context.Background(), "Hello CLI", "", "", "")
	require.NoError(t, err)
	assert.Equal(t, "CLI response", content)
}

func TestAgentLoop_DefaultConfig(t *testing.T) {
	mp := &mockProvider{}
	loop := NewAgentLoop(bus.NewMessageBus(), mp, AgentConfig{Workspace: t.TempDir()})
	assert.Equal(t, "mock-model", loop.Model)
	assert.Equal(t, 20, loop.MaxIterations)
	assert.Equal(t, 4096, loop.MaxTokens)
	assert.Equal(t, 50, loop.MemoryWindow)
}

// mockToolForLoop is a minimal tool implementation for testing the agent loop.
type mockToolForLoop struct {
	name string
}

func (m *mockToolForLoop) Name() string        { return m.name }
func (m *mockToolForLoop) Description() string  { return "mock" }
func (m *mockToolForLoop) Parameters() map[string]any { return map[string]any{} }
func (m *mockToolForLoop) Execute(_ context.Context, _ map[string]any) (string, error) {
	return "mock result", nil
}
