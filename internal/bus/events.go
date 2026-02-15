// Package bus provides the async message bus for decoupled channel-agent communication.
package bus

import "time"

// InboundMessage is received from a chat channel.
type InboundMessage struct {
	Channel   string            `json:"channel"`
	SenderID  string            `json:"sender_id"`
	ChatID    string            `json:"chat_id"`
	Content   string            `json:"content"`
	Timestamp time.Time         `json:"timestamp"`
	Media     []string          `json:"media,omitempty"`
	Metadata  map[string]any    `json:"metadata,omitempty"`
}

// SessionKey returns the unique key for session identification.
func (m *InboundMessage) SessionKey() string {
	return m.Channel + ":" + m.ChatID
}

// OutboundMessage is sent to a chat channel.
type OutboundMessage struct {
	Channel  string         `json:"channel"`
	ChatID   string         `json:"chat_id"`
	Content  string         `json:"content"`
	ReplyTo  string         `json:"reply_to,omitempty"`
	Media    []string       `json:"media,omitempty"`
	Metadata map[string]any `json:"metadata,omitempty"`
}
