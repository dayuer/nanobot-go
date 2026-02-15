package channels

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"sync"

	"github.com/dayuer/nanobot-go/internal/bus"
)

// WhatsAppChannel implements the WhatsApp bot channel via a Node.js bridge WebSocket.
type WhatsAppChannel struct {
	BaseChannel
	BridgeURL   string
	BridgeToken string
	connected   bool
	cancelFn    context.CancelFunc
	mu          sync.Mutex

	// sendFn is an injectable message sender function (for testing).
	sendFn func(payload []byte) error
}

// NewWhatsAppChannel creates a WhatsAppChannel.
func NewWhatsAppChannel(bridgeURL, bridgeToken string, allowFrom []string, msgBus *bus.MessageBus) *WhatsAppChannel {
	if bridgeURL == "" {
		bridgeURL = "ws://localhost:3001"
	}
	return &WhatsAppChannel{
		BaseChannel: BaseChannel{
			ChannelName: "whatsapp",
			Bus:         msgBus,
			AllowFrom:   allowFrom,
		},
		BridgeURL:   bridgeURL,
		BridgeToken: bridgeToken,
	}
}

func (w *WhatsAppChannel) Name() string     { return "whatsapp" }
func (w *WhatsAppChannel) IsRunning() bool   { return w.Running }

// Start connects to the WhatsApp bridge WebSocket.
func (w *WhatsAppChannel) Start(ctx context.Context) error {
	w.Running = true
	ctx, w.cancelFn = context.WithCancel(ctx)

	log.Printf("WhatsApp bridge URL: %s (stub — real WebSocket impl needed)", w.BridgeURL)

	// Stub: wait for context cancellation
	// Real implementation would connect via gorilla/websocket
	<-ctx.Done()
	w.Running = false
	return nil
}

// Stop stops the WhatsApp channel.
func (w *WhatsAppChannel) Stop() error {
	w.Running = false
	w.connected = false
	if w.cancelFn != nil {
		w.cancelFn()
	}
	return nil
}

// Send sends a message through the WhatsApp bridge.
func (w *WhatsAppChannel) Send(msg bus.OutboundMessage) error {
	if !w.connected && w.sendFn == nil {
		return fmt.Errorf("whatsapp bridge not connected")
	}
	payload, _ := json.Marshal(map[string]string{
		"type": "send",
		"to":   msg.ChatID,
		"text": msg.Content,
	})
	if w.sendFn != nil {
		return w.sendFn(payload)
	}
	// Real implementation would write to WebSocket
	return nil
}

// ProcessBridgeMessage handles an incoming message from the bridge (exported for testing).
func (w *WhatsAppChannel) ProcessBridgeMessage(raw string) {
	var data map[string]any
	if json.Unmarshal([]byte(raw), &data) != nil {
		return
	}

	msgType, _ := data["type"].(string)

	switch msgType {
	case "message":
		sender, _ := data["sender"].(string)
		pn, _ := data["pn"].(string)
		content, _ := data["content"].(string)

		userID := pn
		if userID == "" {
			userID = sender
		}
		senderID := userID
		if idx := len(senderID); idx > 0 {
			if parts := splitAt(senderID, "@"); len(parts) > 1 {
				senderID = parts[0]
			}
		}

		w.HandleMessage(senderID, sender, content, nil, map[string]any{
			"message_id": data["id"],
			"is_group":   data["isGroup"],
		})

	case "status":
		status, _ := data["status"].(string)
		log.Printf("WhatsApp status: %s", status)
		w.mu.Lock()
		w.connected = status == "connected"
		w.mu.Unlock()

	case "qr":
		log.Println("Scan QR code in bridge terminal to connect WhatsApp")

	case "error":
		errMsg, _ := data["error"].(string)
		log.Printf("WhatsApp bridge error: %s", errMsg)
	}
}

func splitAt(s, sep string) []string {
	for i := 0; i < len(s); i++ {
		if s[i:i+len(sep)] == sep {
			return []string{s[:i], s[i+len(sep):]}
		}
	}
	return []string{s}
}
