// Package channels defines the Channel interface for chat platform integrations.
package channels

import (
	"context"
	"strings"

	"github.com/dayuer/nanobot-go/internal/bus"
)

// Channel is the interface that all chat platform integrations must implement.
// Mirrors Python's channels/base.py BaseChannel ABC.
type Channel interface {
	// Name returns the channel identifier (e.g., "telegram", "discord").
	Name() string

	// Start connects to the platform and begins listening. Blocks until ctx is cancelled.
	Start(ctx context.Context) error

	// Stop gracefully shuts down the channel.
	Stop() error

	// Send delivers an outbound message through this channel.
	Send(msg bus.OutboundMessage) error

	// IsRunning returns whether the channel is active.
	IsRunning() bool
}

// BaseChannel provides shared logic for all channel implementations.
type BaseChannel struct {
	ChannelName string
	Bus         *bus.MessageBus
	AllowFrom   []string
	Running     bool
}

// IsAllowed checks if a sender is permitted to interact with the bot.
func (b *BaseChannel) IsAllowed(senderID string) bool {
	if len(b.AllowFrom) == 0 {
		return true
	}
	for _, allowed := range b.AllowFrom {
		if allowed == senderID {
			return true
		}
	}
	// Support pipe-separated sender IDs
	if strings.Contains(senderID, "|") {
		for _, part := range strings.Split(senderID, "|") {
			if part == "" {
				continue
			}
			for _, allowed := range b.AllowFrom {
				if allowed == part {
					return true
				}
			}
		}
	}
	return false
}

// HandleMessage checks permissions and publishes to the bus.
func (b *BaseChannel) HandleMessage(senderID, chatID, content string, media []string, metadata map[string]any) {
	if !b.IsAllowed(senderID) {
		return
	}
	msg := bus.InboundMessage{
		Channel:  b.ChannelName,
		SenderID: senderID,
		ChatID:   chatID,
		Content:  content,
		Media:    media,
		Metadata: metadata,
	}
	b.Bus.PublishInbound(msg)
}
