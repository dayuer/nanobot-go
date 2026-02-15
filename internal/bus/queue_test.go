package bus

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestNewMessageBus(t *testing.T) {
	bus := NewMessageBus()
	assert.NotNil(t, bus)
	assert.Equal(t, 0, bus.InboundSize())
	assert.Equal(t, 0, bus.OutboundSize())
}

func TestMessageBus_PublishConsumeInbound(t *testing.T) {
	bus := NewMessageBus()
	msg := InboundMessage{Channel: "telegram", Content: "hello"}

	bus.PublishInbound(msg)
	assert.Equal(t, 1, bus.InboundSize())

	received := <-bus.Inbound
	assert.Equal(t, "hello", received.Content)
	assert.Equal(t, "telegram", received.Channel)
}

func TestMessageBus_SubscribeAndDispatch(t *testing.T) {
	bus := NewMessageBus()

	var received []OutboundMessage
	var mu sync.Mutex

	bus.Subscribe("telegram", func(msg OutboundMessage) {
		mu.Lock()
		received = append(received, msg)
		mu.Unlock()
	})

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	go bus.DispatchOutbound(ctx)

	bus.PublishOutbound(OutboundMessage{Channel: "telegram", Content: "reply"})

	// Wait for dispatch
	time.Sleep(50 * time.Millisecond)

	mu.Lock()
	defer mu.Unlock()
	assert.Len(t, received, 1)
	assert.Equal(t, "reply", received[0].Content)
}

func TestMessageBus_SubscribeDoesNotReceiveOtherChannels(t *testing.T) {
	bus := NewMessageBus()

	var received []OutboundMessage
	var mu sync.Mutex

	bus.Subscribe("telegram", func(msg OutboundMessage) {
		mu.Lock()
		received = append(received, msg)
		mu.Unlock()
	})

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	go bus.DispatchOutbound(ctx)

	bus.PublishOutbound(OutboundMessage{Channel: "discord", Content: "wrong"})
	time.Sleep(50 * time.Millisecond)

	mu.Lock()
	defer mu.Unlock()
	assert.Len(t, received, 0)
}

func TestMessageBus_ConcurrentPublish(t *testing.T) {
	bus := NewMessageBus()
	var wg sync.WaitGroup

	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			bus.PublishInbound(InboundMessage{Channel: "test", Content: "msg"})
		}(i)
	}
	wg.Wait()
	assert.Equal(t, 100, bus.InboundSize())
}
