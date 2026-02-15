package registry

import (
	"context"

	"github.com/dayuer/nanobot-go/internal/bus"
	"github.com/dayuer/nanobot-go/internal/providers"
)

// mockProvider satisfies providers.LLMProvider for testing.
type mockProvider struct {
	model string
}

func (m *mockProvider) Chat(_ context.Context, _ providers.ChatRequest) (*providers.LLMResponse, error) {
	content := "mock response"
	return &providers.LLMResponse{Content: &content, FinishReason: "stop"}, nil
}

func (m *mockProvider) DefaultModel() string { return m.model }

// bus_stub creates a MessageBus for testing.
func bus_stub() *bus.MessageBus {
	return bus.NewMessageBus()
}
