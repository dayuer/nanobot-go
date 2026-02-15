package channels

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"strings"
	"time"

	"github.com/dayuer/nanobot-go/internal/bus"
)

// FeishuChannel implements the Feishu/Lark bot channel.
// Uses webhook-style event receiving (HTTP endpoint).
type FeishuChannel struct {
	BaseChannel
	AppID       string
	AppSecret   string
	WebhookPort int // Port for receiving events
	cancelFn    context.CancelFunc
	accessToken string
	tokenExpiry time.Time
}

// NewFeishuChannel creates a FeishuChannel.
func NewFeishuChannel(appID, appSecret string, port int, allowFrom []string, msgBus *bus.MessageBus) *FeishuChannel {
	if port == 0 {
		port = 9000
	}
	return &FeishuChannel{
		BaseChannel: BaseChannel{
			ChannelName: "feishu",
			Bus:         msgBus,
			AllowFrom:   allowFrom,
		},
		AppID:       appID,
		AppSecret:   appSecret,
		WebhookPort: port,
	}
}

func (f *FeishuChannel) Name() string     { return "feishu" }
func (f *FeishuChannel) IsRunning() bool   { return f.Running }

// Start begins listening for Feishu events.
func (f *FeishuChannel) Start(ctx context.Context) error {
	if f.AppID == "" || f.AppSecret == "" {
		return fmt.Errorf("feishu app_id and app_secret not configured")
	}
	f.Running = true
	ctx, f.cancelFn = context.WithCancel(ctx)

	// Get initial access token
	if err := f.refreshToken(); err != nil {
		log.Printf("Feishu initial token error: %v", err)
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/webhook/event", f.handleEvent)
	server := &http.Server{
		Addr:    fmt.Sprintf(":%d", f.WebhookPort),
		Handler: mux,
	}

	go func() {
		<-ctx.Done()
		server.Close()
	}()

	log.Printf("Feishu bot listening on :%d", f.WebhookPort)
	err := server.ListenAndServe()
	if err != nil && err != http.ErrServerClosed {
		return err
	}
	f.Running = false
	return nil
}

// Stop stops the Feishu bot.
func (f *FeishuChannel) Stop() error {
	f.Running = false
	if f.cancelFn != nil {
		f.cancelFn()
	}
	return nil
}

// Send sends a message via Feishu API.
func (f *FeishuChannel) Send(msg bus.OutboundMessage) error {
	if err := f.ensureToken(); err != nil {
		return err
	}

	receiveIDType := "open_id"
	if strings.HasPrefix(msg.ChatID, "oc_") {
		receiveIDType = "chat_id"
	}

	card := map[string]any{
		"config":   map[string]any{"wide_screen_mode": true},
		"elements": []map[string]any{{"tag": "markdown", "content": msg.Content}},
	}
	cardJSON, _ := json.Marshal(card)

	body := map[string]any{
		"receive_id": msg.ChatID,
		"msg_type":   "interactive",
		"content":    string(cardJSON),
	}
	bodyJSON, _ := json.Marshal(body)

	url := fmt.Sprintf("https://open.feishu.cn/open-apis/im/v1/messages?receive_id_type=%s", receiveIDType)
	req, _ := http.NewRequest("POST", url, strings.NewReader(string(bodyJSON)))
	req.Header.Set("Content-Type", "application/json; charset=utf-8")
	req.Header.Set("Authorization", "Bearer "+f.accessToken)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	resp.Body.Close()
	return nil
}

func (f *FeishuChannel) handleEvent(w http.ResponseWriter, r *http.Request) {
	var payload map[string]any
	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		http.Error(w, "bad request", 400)
		return
	}

	// URL verification challenge
	if challenge, ok := payload["challenge"].(string); ok {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]string{"challenge": challenge})
		return
	}

	w.WriteHeader(200)

	// Process event
	header, _ := payload["header"].(map[string]any)
	event, _ := payload["event"].(map[string]any)
	if header == nil || event == nil {
		return
	}

	eventType, _ := header["event_type"].(string)
	if eventType != "im.message.receive_v1" {
		return
	}

	message, _ := event["message"].(map[string]any)
	sender, _ := event["sender"].(map[string]any)
	if message == nil || sender == nil {
		return
	}

	// Skip bot messages
	senderType, _ := sender["sender_type"].(string)
	if senderType == "bot" {
		return
	}

	senderID := "unknown"
	if sid, ok := sender["sender_id"].(map[string]any); ok {
		if oid, ok := sid["open_id"].(string); ok {
			senderID = oid
		}
	}

	chatID, _ := message["chat_id"].(string)
	msgType, _ := message["message_type"].(string)
	content, _ := message["content"].(string)

	// Parse content
	var text string
	if msgType == "text" {
		var parsed map[string]string
		if json.Unmarshal([]byte(content), &parsed) == nil {
			text = parsed["text"]
		}
	} else {
		text = fmt.Sprintf("[%s]", msgType)
	}

	if text == "" {
		return
	}

	f.HandleMessage(senderID, chatID, text, nil, map[string]any{
		"msg_type": msgType,
	})
}

func (f *FeishuChannel) ensureToken() error {
	if time.Now().Before(f.tokenExpiry) {
		return nil
	}
	return f.refreshToken()
}

func (f *FeishuChannel) refreshToken() error {
	body, _ := json.Marshal(map[string]string{
		"app_id":     f.AppID,
		"app_secret": f.AppSecret,
	})
	resp, err := http.Post(
		"https://open.feishu.cn/open-apis/auth/v3/tenant_access_token/internal",
		"application/json",
		strings.NewReader(string(body)),
	)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	var result map[string]any
	json.NewDecoder(resp.Body).Decode(&result)

	token, _ := result["tenant_access_token"].(string)
	expire, _ := result["expire"].(float64)
	if token != "" {
		f.accessToken = token
		f.tokenExpiry = time.Now().Add(time.Duration(expire-60) * time.Second)
	}
	return nil
}
