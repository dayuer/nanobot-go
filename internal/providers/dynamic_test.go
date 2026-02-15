package providers

import (
	"context"
	"sync"
	"testing"
)

// mockProvider is a simple LLMProvider for testing.
type mockDynamicProvider struct {
	model    string
	response *LLMResponse
}

func (m *mockDynamicProvider) Chat(_ context.Context, _ ChatRequest) (*LLMResponse, error) {
	return m.response, nil
}

func (m *mockDynamicProvider) DefaultModel() string {
	return m.model
}

func TestDynamicProvider_DelegatesToInner(t *testing.T) {
	content := "Hello from provider A"
	inner := &mockDynamicProvider{
		model:    "model-a",
		response: &LLMResponse{Content: &content, FinishReason: "stop"},
	}
	dp := NewDynamicProvider(inner)

	if dp.DefaultModel() != "model-a" {
		t.Errorf("DefaultModel() = %q, want %q", dp.DefaultModel(), "model-a")
	}

	resp, err := dp.Chat(context.Background(), ChatRequest{})
	if err != nil {
		t.Fatalf("Chat() error: %v", err)
	}
	if *resp.Content != content {
		t.Errorf("Chat() content = %q, want %q", *resp.Content, content)
	}
}

func TestDynamicProvider_Swap(t *testing.T) {
	contentA := "from A"
	contentB := "from B"
	providerA := &mockDynamicProvider{
		model:    "model-a",
		response: &LLMResponse{Content: &contentA, FinishReason: "stop"},
	}
	providerB := &mockDynamicProvider{
		model:    "model-b",
		response: &LLMResponse{Content: &contentB, FinishReason: "stop"},
	}

	dp := NewDynamicProvider(providerA)

	// Before swap
	if dp.DefaultModel() != "model-a" {
		t.Errorf("before swap: DefaultModel() = %q", dp.DefaultModel())
	}

	// Swap
	dp.Swap(providerB)

	if dp.DefaultModel() != "model-b" {
		t.Errorf("after swap: DefaultModel() = %q, want %q", dp.DefaultModel(), "model-b")
	}
	resp, _ := dp.Chat(context.Background(), ChatRequest{})
	if *resp.Content != contentB {
		t.Errorf("after swap: Chat() content = %q, want %q", *resp.Content, contentB)
	}
}

func TestDynamicProvider_ConcurrentAccess(t *testing.T) {
	content := "concurrent"
	inner := &mockDynamicProvider{
		model:    "model-c",
		response: &LLMResponse{Content: &content, FinishReason: "stop"},
	}
	dp := NewDynamicProvider(inner)

	var wg sync.WaitGroup
	// Concurrent reads
	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			dp.Chat(context.Background(), ChatRequest{})
			dp.DefaultModel()
		}()
	}

	// Concurrent swaps
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			newContent := "swapped"
			dp.Swap(&mockDynamicProvider{
				model:    "swapped-model",
				response: &LLMResponse{Content: &newContent, FinishReason: "stop"},
			})
		}()
	}

	wg.Wait()
	// No race condition panics = pass
}

func TestDynamicProvider_Inner(t *testing.T) {
	inner := &mockDynamicProvider{model: "original"}
	dp := NewDynamicProvider(inner)

	if dp.Inner() != inner {
		t.Error("Inner() should return the current provider")
	}

	newInner := &mockDynamicProvider{model: "replaced"}
	dp.Swap(newInner)
	if dp.Inner() != newInner {
		t.Error("Inner() should return the new provider after Swap")
	}
}
