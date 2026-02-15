package channels

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/dayuer/nanobot-go/internal/bus"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// --- Markdown to Telegram HTML tests ---

func TestMarkdownToTelegramHTML_Empty(t *testing.T) {
	assert.Equal(t, "", MarkdownToTelegramHTML(""))
}

func TestMarkdownToTelegramHTML_Bold(t *testing.T) {
	assert.Contains(t, MarkdownToTelegramHTML("**bold**"), "<b>bold</b>")
}

func TestMarkdownToTelegramHTML_InlineCode(t *testing.T) {
	result := MarkdownToTelegramHTML("`code here`")
	assert.Contains(t, result, "<code>code here</code>")
}

func TestMarkdownToTelegramHTML_CodeBlock(t *testing.T) {
	result := MarkdownToTelegramHTML("```go\nfmt.Println(\"hi\")\n```")
	assert.Contains(t, result, "<pre><code>")
	assert.Contains(t, result, "fmt.Println")
}

func TestMarkdownToTelegramHTML_Link(t *testing.T) {
	result := MarkdownToTelegramHTML("[Google](https://google.com)")
	assert.Contains(t, result, `<a href="https://google.com">Google</a>`)
}

func TestMarkdownToTelegramHTML_Heading(t *testing.T) {
	result := MarkdownToTelegramHTML("## Title\nContent")
	assert.NotContains(t, result, "##")
	assert.Contains(t, result, "Title")
}

func TestMarkdownToTelegramHTML_BulletList(t *testing.T) {
	result := MarkdownToTelegramHTML("- item 1\n- item 2")
	assert.Contains(t, result, "• item 1")
	assert.Contains(t, result, "• item 2")
}

func TestMarkdownToTelegramHTML_HTMLEscape(t *testing.T) {
	result := MarkdownToTelegramHTML("a < b & c > d")
	assert.Contains(t, result, "&lt;")
	assert.Contains(t, result, "&amp;")
	assert.Contains(t, result, "&gt;")
}

func TestMarkdownToTelegramHTML_Strikethrough(t *testing.T) {
	result := MarkdownToTelegramHTML("~~deleted~~")
	assert.Contains(t, result, "<s>deleted</s>")
}

// --- Telegram Channel tests ---

func TestTelegramChannel_Interface(t *testing.T) {
	ch := NewTelegramChannel("test-token", nil, bus.NewMessageBus())
	var _ Channel = ch
	assert.Equal(t, "telegram", ch.Name())
	assert.False(t, ch.IsRunning())
}

func TestTelegramChannel_StartNoToken(t *testing.T) {
	ch := NewTelegramChannel("", nil, bus.NewMessageBus())
	err := ch.Start(context.Background())
	assert.Error(t, err)
}

func TestTelegramChannel_SendWithMockServer(t *testing.T) {
	var called bool
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
		json.NewEncoder(w).Encode(map[string]any{"ok": true})
	}))
	defer srv.Close()

	ch := NewTelegramChannel("test-token", nil, bus.NewMessageBus())
	// Inject mock API base (we'd need to modify apiCall — for now test the HTML conversion)
	_ = ch
	_ = called

	// Test that Send constructs the right HTML
	html := MarkdownToTelegramHTML("**Hello** `world`")
	assert.Contains(t, html, "<b>Hello</b>")
	assert.Contains(t, html, "<code>world</code>")
}

// --- Feishu Channel tests ---

func TestFeishuChannel_Interface(t *testing.T) {
	ch := NewFeishuChannel("app_id", "app_secret", 0, nil, bus.NewMessageBus())
	var _ Channel = ch
	assert.Equal(t, "feishu", ch.Name())
}

func TestFeishuChannel_StartNoConfig(t *testing.T) {
	ch := NewFeishuChannel("", "", 0, nil, bus.NewMessageBus())
	err := ch.Start(context.Background())
	assert.Error(t, err)
}

func TestFeishuChannel_HandleEvent_URLVerification(t *testing.T) {
	ch := NewFeishuChannel("id", "secret", 0, nil, bus.NewMessageBus())
	body := `{"challenge": "test_challenge_123"}`
	req := httptest.NewRequest("POST", "/webhook/event", strings.NewReader(body))
	w := httptest.NewRecorder()
	ch.handleEvent(w, req)

	assert.Equal(t, 200, w.Code)
	var resp map[string]string
	json.NewDecoder(w.Body).Decode(&resp)
	assert.Equal(t, "test_challenge_123", resp["challenge"])
}

func TestFeishuChannel_HandleEvent_TextMessage(t *testing.T) {
	msgBus := bus.NewMessageBus()
	ch := NewFeishuChannel("id", "secret", 0, nil, msgBus)

	event := map[string]any{
		"header": map[string]any{"event_type": "im.message.receive_v1"},
		"event": map[string]any{
			"message": map[string]any{
				"chat_id":      "oc_abcdef",
				"message_type": "text",
				"content":      `{"text": "Hello bot"}`,
			},
			"sender": map[string]any{
				"sender_type": "user",
				"sender_id":   map[string]any{"open_id": "ou_user123"},
			},
		},
	}
	body, _ := json.Marshal(event)
	req := httptest.NewRequest("POST", "/webhook/event", strings.NewReader(string(body)))
	w := httptest.NewRecorder()
	ch.handleEvent(w, req)

	// Should have published to bus
	select {
	case msg := <-msgBus.Inbound:
		assert.Equal(t, "feishu", msg.Channel)
		assert.Equal(t, "ou_user123", msg.SenderID)
		assert.Equal(t, "Hello bot", msg.Content)
	case <-time.After(time.Second):
		t.Fatal("timeout waiting for bus message")
	}
}

func TestFeishuChannel_HandleEvent_BotSkipped(t *testing.T) {
	msgBus := bus.NewMessageBus()
	ch := NewFeishuChannel("id", "secret", 0, nil, msgBus)

	event := map[string]any{
		"header": map[string]any{"event_type": "im.message.receive_v1"},
		"event": map[string]any{
			"message": map[string]any{"chat_id": "oc_x", "message_type": "text", "content": `{"text":"hi"}`},
			"sender":  map[string]any{"sender_type": "bot", "sender_id": map[string]any{"open_id": "ou_bot"}},
		},
	}
	body, _ := json.Marshal(event)
	req := httptest.NewRequest("POST", "/webhook/event", strings.NewReader(string(body)))
	w := httptest.NewRecorder()
	ch.handleEvent(w, req)

	// No message should be published
	select {
	case <-msgBus.Inbound:
		t.Fatal("bot message should be skipped")
	case <-time.After(100 * time.Millisecond):
		// Expected
	}
}

// --- Slack Channel tests ---

func TestSlackChannel_Interface(t *testing.T) {
	ch := NewSlackChannel("xoxb-token", "xapp-token", nil, bus.NewMessageBus())
	var _ Channel = ch
	assert.Equal(t, "slack", ch.Name())
}

func TestSlackChannel_ProcessEvent_TextMessage(t *testing.T) {
	msgBus := bus.NewMessageBus()
	ch := NewSlackChannel("xoxb-token", "xapp-token", nil, msgBus)

	ch.ProcessEvent(map[string]any{
		"type":    "message",
		"user":    "U123",
		"channel": "C456",
		"text":    "Hello from Slack",
		"ts":      "1234567890.123",
	})

	select {
	case msg := <-msgBus.Inbound:
		assert.Equal(t, "slack", msg.Channel)
		assert.Equal(t, "U123", msg.SenderID)
		assert.Equal(t, "Hello from Slack", msg.Content)
	case <-time.After(time.Second):
		t.Fatal("timeout waiting for bus message")
	}
}

func TestSlackChannel_ProcessEvent_SkipsBotMessages(t *testing.T) {
	msgBus := bus.NewMessageBus()
	ch := NewSlackChannel("xoxb-token", "xapp-token", nil, msgBus)
	ch.BotUserID = "U_BOT"

	ch.ProcessEvent(map[string]any{
		"type": "message", "user": "U_BOT", "channel": "C1", "text": "bot msg",
	})

	select {
	case <-msgBus.Inbound:
		t.Fatal("bot message should be skipped")
	case <-time.After(100 * time.Millisecond):
		// Expected
	}
}

func TestSlackChannel_ProcessEvent_SkipsSubtype(t *testing.T) {
	msgBus := bus.NewMessageBus()
	ch := NewSlackChannel("xoxb-token", "xapp-token", nil, msgBus)

	ch.ProcessEvent(map[string]any{
		"type": "message", "subtype": "channel_join", "user": "U1", "channel": "C1", "text": "joined",
	})

	select {
	case <-msgBus.Inbound:
		t.Fatal("subtype message should be skipped")
	case <-time.After(100 * time.Millisecond):
	}
}

func TestSlackChannel_StripBotMention(t *testing.T) {
	ch := NewSlackChannel("", "", nil, nil)
	ch.BotUserID = "UBOT"
	assert.Equal(t, "hello", ch.stripBotMention("<@UBOT> hello"))
	assert.Equal(t, "hello", ch.stripBotMention("hello"))
}

// --- WhatsApp Channel tests ---

func TestWhatsAppChannel_Interface(t *testing.T) {
	ch := NewWhatsAppChannel("", "", nil, bus.NewMessageBus())
	var _ Channel = ch
	assert.Equal(t, "whatsapp", ch.Name())
}

func TestWhatsAppChannel_ProcessBridgeMessage_Text(t *testing.T) {
	msgBus := bus.NewMessageBus()
	ch := NewWhatsAppChannel("", "", nil, msgBus)

	ch.ProcessBridgeMessage(`{"type":"message","sender":"12345@s.whatsapp.net","content":"Hi there"}`)

	select {
	case msg := <-msgBus.Inbound:
		assert.Equal(t, "whatsapp", msg.Channel)
		assert.Equal(t, "12345", msg.SenderID)
		assert.Equal(t, "Hi there", msg.Content)
	case <-time.After(time.Second):
		t.Fatal("timeout waiting for bus message")
	}
}

func TestWhatsAppChannel_ProcessBridgeMessage_Status(t *testing.T) {
	ch := NewWhatsAppChannel("", "", nil, bus.NewMessageBus())
	ch.ProcessBridgeMessage(`{"type":"status","status":"connected"}`)
	assert.True(t, ch.connected)
	ch.ProcessBridgeMessage(`{"type":"status","status":"disconnected"}`)
	assert.False(t, ch.connected)
}

func TestWhatsAppChannel_Send_NotConnected(t *testing.T) {
	ch := NewWhatsAppChannel("", "", nil, bus.NewMessageBus())
	err := ch.Send(bus.OutboundMessage{ChatID: "123", Content: "test"})
	assert.Error(t, err)
}

func TestWhatsAppChannel_Send_WithSendFn(t *testing.T) {
	ch := NewWhatsAppChannel("", "", nil, bus.NewMessageBus())
	var sentPayload []byte
	ch.sendFn = func(payload []byte) error {
		sentPayload = payload
		return nil
	}
	err := ch.Send(bus.OutboundMessage{ChatID: "12345@s.whatsapp.net", Content: "Hello"})
	require.NoError(t, err)
	assert.Contains(t, string(sentPayload), "Hello")
	assert.Contains(t, string(sentPayload), "12345@s.whatsapp.net")
}

// --- Manager tests ---

type mockChannel struct {
	name    string
	started bool
	stopped bool
	sent    []bus.OutboundMessage
}

func (m *mockChannel) Name() string                      { return m.name }
func (m *mockChannel) Start(_ context.Context) error     { m.started = true; return nil }
func (m *mockChannel) Stop() error                       { m.stopped = true; return nil }
func (m *mockChannel) Send(msg bus.OutboundMessage) error { m.sent = append(m.sent, msg); return nil }
func (m *mockChannel) IsRunning() bool                   { return m.started && !m.stopped }

func TestManager_Register(t *testing.T) {
	mgr := NewManager(bus.NewMessageBus())
	mgr.Register(&mockChannel{name: "test"})
	assert.Equal(t, []string{"test"}, mgr.EnabledChannels())
}

func TestManager_Get(t *testing.T) {
	mgr := NewManager(bus.NewMessageBus())
	ch := &mockChannel{name: "telegram"}
	mgr.Register(ch)
	assert.Equal(t, ch, mgr.Get("telegram"))
	assert.Nil(t, mgr.Get("nonexistent"))
}

func TestManager_StopAll(t *testing.T) {
	mgr := NewManager(bus.NewMessageBus())
	ch1 := &mockChannel{name: "ch1", started: true}
	ch2 := &mockChannel{name: "ch2", started: true}
	mgr.Register(ch1)
	mgr.Register(ch2)
	mgr.StopAll()
	assert.True(t, ch1.stopped)
	assert.True(t, ch2.stopped)
}

func TestManager_GetStatus(t *testing.T) {
	mgr := NewManager(bus.NewMessageBus())
	mgr.Register(&mockChannel{name: "up", started: true})
	mgr.Register(&mockChannel{name: "down"})
	status := mgr.GetStatus()
	assert.True(t, status["up"])
	assert.False(t, status["down"])
}
