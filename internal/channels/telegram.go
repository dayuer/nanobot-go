package channels

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"regexp"
	"strings"
	"time"

	"github.com/dayuer/nanobot-go/internal/bus"
)

// TelegramChannel implements the Telegram bot channel using long polling.
type TelegramChannel struct {
	BaseChannel
	Token    string
	Proxy    string
	botUser  string
	client   *http.Client
	cancelFn context.CancelFunc
}

// NewTelegramChannel creates a TelegramChannel.
func NewTelegramChannel(token string, allowFrom []string, msgBus *bus.MessageBus) *TelegramChannel {
	return &TelegramChannel{
		BaseChannel: BaseChannel{
			ChannelName: "telegram",
			Bus:         msgBus,
			AllowFrom:   allowFrom,
		},
		Token:  token,
		client: &http.Client{Timeout: 60 * time.Second},
	}
}

func (t *TelegramChannel) Name() string     { return "telegram" }
func (t *TelegramChannel) IsRunning() bool   { return t.Running }

// Start begins long polling for Telegram updates.
func (t *TelegramChannel) Start(ctx context.Context) error {
	if t.Token == "" {
		return fmt.Errorf("telegram bot token not configured")
	}
	t.Running = true
	ctx, t.cancelFn = context.WithCancel(ctx)

	// Get bot info
	info, err := t.apiCall("getMe", nil)
	if err != nil {
		return fmt.Errorf("telegram getMe: %w", err)
	}
	if result, ok := info["result"].(map[string]any); ok {
		if username, ok := result["username"].(string); ok {
			t.botUser = username
			log.Printf("Telegram bot @%s connected", username)
		}
	}

	// Long polling loop
	offset := 0
	for {
		select {
		case <-ctx.Done():
			t.Running = false
			return nil
		default:
		}

		updates, err := t.apiCall("getUpdates", map[string]any{
			"offset":  offset,
			"timeout": 30,
			"allowed_updates": []string{"message"},
		})
		if err != nil {
			log.Printf("Telegram getUpdates error: %v", err)
			time.Sleep(5 * time.Second)
			continue
		}

		results, _ := updates["result"].([]any)
		for _, u := range results {
			update, ok := u.(map[string]any)
			if !ok {
				continue
			}
			if uid, ok := update["update_id"].(float64); ok {
				offset = int(uid) + 1
			}
			t.processUpdate(update)
		}
	}
}

// Stop stops the Telegram bot.
func (t *TelegramChannel) Stop() error {
	t.Running = false
	if t.cancelFn != nil {
		t.cancelFn()
	}
	return nil
}

// Send sends a message via Telegram.
func (t *TelegramChannel) Send(msg bus.OutboundMessage) error {
	html := MarkdownToTelegramHTML(msg.Content)
	_, err := t.apiCall("sendMessage", map[string]any{
		"chat_id":    msg.ChatID,
		"text":       html,
		"parse_mode": "HTML",
	})
	if err != nil {
		// Fallback to plain text
		_, err = t.apiCall("sendMessage", map[string]any{
			"chat_id": msg.ChatID,
			"text":    msg.Content,
		})
	}
	return err
}

func (t *TelegramChannel) processUpdate(update map[string]any) {
	msg, ok := update["message"].(map[string]any)
	if !ok {
		return
	}
	from, _ := msg["from"].(map[string]any)
	chat, _ := msg["chat"].(map[string]any)
	if from == nil || chat == nil {
		return
	}

	userID := fmt.Sprintf("%.0f", from["id"])
	if username, ok := from["username"].(string); ok && username != "" {
		userID = fmt.Sprintf("%s|%s", userID, username)
	}
	chatID := fmt.Sprintf("%.0f", chat["id"])
	text, _ := msg["text"].(string)
	caption, _ := msg["caption"].(string)
	if text == "" && caption != "" {
		text = caption
	}
	if text == "" {
		text = "[empty message]"
	}

	t.HandleMessage(userID, chatID, text, nil, map[string]any{
		"message_id": msg["message_id"],
	})
}

func (t *TelegramChannel) apiCall(method string, params map[string]any) (map[string]any, error) {
	url := fmt.Sprintf("https://api.telegram.org/bot%s/%s", t.Token, method)
	body, _ := json.Marshal(params)
	req, _ := http.NewRequest("POST", url, strings.NewReader(string(body)))
	req.Header.Set("Content-Type", "application/json")
	resp, err := t.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	var result map[string]any
	json.NewDecoder(resp.Body).Decode(&result)
	return result, nil
}

// MarkdownToTelegramHTML converts markdown to Telegram-safe HTML.
// Exported for testing.
func MarkdownToTelegramHTML(text string) string {
	if text == "" {
		return ""
	}

	// 1. Extract and protect code blocks
	var codeBlocks []string
	codeBlockRe := regexp.MustCompile("(?s)```[\\w]*\\n?([\\s\\S]*?)```")
	text = codeBlockRe.ReplaceAllStringFunc(text, func(m string) string {
		matches := codeBlockRe.FindStringSubmatch(m)
		if len(matches) > 1 {
			codeBlocks = append(codeBlocks, matches[1])
			return fmt.Sprintf("\x00CB%d\x00", len(codeBlocks)-1)
		}
		return m
	})

	// 2. Extract and protect inline code
	var inlineCodes []string
	inlineCodeRe := regexp.MustCompile("`([^`]+)`")
	text = inlineCodeRe.ReplaceAllStringFunc(text, func(m string) string {
		matches := inlineCodeRe.FindStringSubmatch(m)
		if len(matches) > 1 {
			inlineCodes = append(inlineCodes, matches[1])
			return fmt.Sprintf("\x00IC%d\x00", len(inlineCodes)-1)
		}
		return m
	})

	// 3. Headers
	headingRe := regexp.MustCompile(`(?m)^#{1,6}\s+(.+)$`)
	text = headingRe.ReplaceAllString(text, "$1")

	// 4. Blockquotes
	bqRe := regexp.MustCompile(`(?m)^>\s*(.*)$`)
	text = bqRe.ReplaceAllString(text, "$1")

	// 5. Escape HTML
	text = strings.ReplaceAll(text, "&", "&amp;")
	text = strings.ReplaceAll(text, "<", "&lt;")
	text = strings.ReplaceAll(text, ">", "&gt;")

	// 6. Links [text](url)
	linkRe := regexp.MustCompile(`\[([^\]]+)\]\(([^)]+)\)`)
	text = linkRe.ReplaceAllString(text, `<a href="$2">$1</a>`)

	// 7. Bold **text** or __text__
	boldRe := regexp.MustCompile(`\*\*(.+?)\*\*`)
	text = boldRe.ReplaceAllString(text, "<b>$1</b>")
	boldRe2 := regexp.MustCompile(`__(.+?)__`)
	text = boldRe2.ReplaceAllString(text, "<b>$1</b>")

	// 8. Italic _text_
	italicRe := regexp.MustCompile(`(?:^|[^a-zA-Z0-9])_([^_]+)_(?:[^a-zA-Z0-9]|$)`)
	text = italicRe.ReplaceAllStringFunc(text, func(m string) string {
		sub := italicRe.FindStringSubmatch(m)
		if len(sub) > 1 {
			prefix := ""
			suffix := ""
			if len(m) > 0 && m[0] != '_' {
				prefix = string(m[0])
			}
			if len(m) > 0 && m[len(m)-1] != '_' {
				suffix = string(m[len(m)-1])
			}
			return prefix + "<i>" + sub[1] + "</i>" + suffix
		}
		return m
	})

	// 9. Strikethrough ~~text~~
	strikeRe := regexp.MustCompile(`~~(.+?)~~`)
	text = strikeRe.ReplaceAllString(text, "<s>$1</s>")

	// 10. Bullet lists
	bulletRe := regexp.MustCompile(`(?m)^[-*]\s+`)
	text = bulletRe.ReplaceAllString(text, "• ")

	// 11. Restore inline code
	for i, code := range inlineCodes {
		escaped := strings.ReplaceAll(code, "&", "&amp;")
		escaped = strings.ReplaceAll(escaped, "<", "&lt;")
		escaped = strings.ReplaceAll(escaped, ">", "&gt;")
		text = strings.ReplaceAll(text, fmt.Sprintf("\x00IC%d\x00", i), "<code>"+escaped+"</code>")
	}

	// 12. Restore code blocks
	for i, code := range codeBlocks {
		escaped := strings.ReplaceAll(code, "&", "&amp;")
		escaped = strings.ReplaceAll(escaped, "<", "&lt;")
		escaped = strings.ReplaceAll(escaped, ">", "&gt;")
		text = strings.ReplaceAll(text, fmt.Sprintf("\x00CB%d\x00", i), "<pre><code>"+escaped+"</code></pre>")
	}

	return text
}
