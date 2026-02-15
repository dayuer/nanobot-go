package bus

import (
	"context"
	"sync"
)

// MessageBus provides async message routing between channels and the agent core.
// Uses Go channels instead of Python's asyncio.Queue for natural concurrency.
type MessageBus struct {
	Inbound  chan InboundMessage
	Outbound chan OutboundMessage

	mu          sync.RWMutex
	subscribers map[string][]func(OutboundMessage)
	cancel      context.CancelFunc
}

// NewMessageBus creates a new message bus with buffered channels.
func NewMessageBus() *MessageBus {
	return &MessageBus{
		Inbound:     make(chan InboundMessage, 100),
		Outbound:    make(chan OutboundMessage, 100),
		subscribers: make(map[string][]func(OutboundMessage)),
	}
}

// PublishInbound sends a message from a channel to the agent.
func (b *MessageBus) PublishInbound(msg InboundMessage) {
	b.Inbound <- msg
}

// PublishOutbound sends a response from the agent to channels.
func (b *MessageBus) PublishOutbound(msg OutboundMessage) {
	b.Outbound <- msg
}

// Subscribe registers a callback for outbound messages on a specific channel.
func (b *MessageBus) Subscribe(channel string, callback func(OutboundMessage)) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.subscribers[channel] = append(b.subscribers[channel], callback)
}

// DispatchOutbound runs the outbound dispatch loop. Blocks until ctx is cancelled.
func (b *MessageBus) DispatchOutbound(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			return
		case msg := <-b.Outbound:
			b.mu.RLock()
			subs := b.subscribers[msg.Channel]
			b.mu.RUnlock()
			for _, cb := range subs {
				cb(msg)
			}
		}
	}
}

// InboundSize returns the number of pending inbound messages.
func (b *MessageBus) InboundSize() int {
	return len(b.Inbound)
}

// OutboundSize returns the number of pending outbound messages.
func (b *MessageBus) OutboundSize() int {
	return len(b.Outbound)
}
