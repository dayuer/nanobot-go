package providers

import (
	"context"
	"sync"
)

// DynamicProvider wraps a Provider with atomic hot-swap support.
//
// All Chat() calls are proxied to the current inner provider.
// Swap() atomically replaces the inner provider with zero downtime:
// in-flight requests finish on the old provider; new requests use the new one.
type DynamicProvider struct {
	mu    sync.RWMutex
	inner LLMProvider
}

// NewDynamicProvider creates a DynamicProvider wrapping the given provider.
func NewDynamicProvider(initial LLMProvider) *DynamicProvider {
	return &DynamicProvider{inner: initial}
}

// Chat delegates to the current inner provider (read-lock).
func (d *DynamicProvider) Chat(ctx context.Context, req ChatRequest) (*LLMResponse, error) {
	d.mu.RLock()
	p := d.inner
	d.mu.RUnlock()
	return p.Chat(ctx, req)
}

// DefaultModel returns the current inner provider's default model.
func (d *DynamicProvider) DefaultModel() string {
	d.mu.RLock()
	p := d.inner
	d.mu.RUnlock()
	return p.DefaultModel()
}

// Swap atomically replaces the inner provider.
// In-flight Chat() calls continue on the old provider; subsequent calls
// use the new one.
func (d *DynamicProvider) Swap(newProvider LLMProvider) {
	d.mu.Lock()
	d.inner = newProvider
	d.mu.Unlock()
}

// Inner returns the current inner provider (for inspection/debugging).
func (d *DynamicProvider) Inner() LLMProvider {
	d.mu.RLock()
	defer d.mu.RUnlock()
	return d.inner
}
