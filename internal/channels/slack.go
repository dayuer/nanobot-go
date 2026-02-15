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

// SlackChannel implements the Slack bot channel using Socket Mode / Events API.
type SlackChannel struct {
	BaseChannel
	BotToken  string
	AppToken  string
	BotUserID string
	cancelFn  context.CancelFunc
	client    *http.Client
}

// NewSlackChannel creates a SlackChannel.
func NewSlackChannel(botToken, appToken string, allowFrom []string, msgBus *bus.MessageBus) *SlackChannel {
	return &SlackChannel{
		BaseChannel: BaseChannel{
			ChannelName: "slack",
			Bus:         msgBus,
			AllowFrom:   allowFrom,
		},
		BotToken: botToken,
		AppToken: appToken,
		client:   &http.Client{Timeout: 30 * time.Second},
	}
}

func (s *SlackChannel) Name() string     { return "slack" }
func (s *SlackChannel) IsRunning() bool   { return s.Running }

// Start begins listening for Slack events.
func (s *SlackChannel) Start(ctx context.Context) error {
	if s.BotToken == "" || s.AppToken == "" {
		return fmt.Errorf("slack bot/app token not configured")
	}
	s.Running = true
	ctx, s.cancelFn = context.WithCancel(ctx)

	// Get bot user ID
	if result, err := s.slackAPI("auth.test", nil); err == nil {
		if uid, ok := result["user_id"].(string); ok {
			s.BotUserID = uid
			log.Printf("Slack bot connected as %s", uid)
		}
	}

	// Wait for context cancellation (real implementation would use WebSocket)
	<-ctx.Done()
	s.Running = false
	return nil
}

// Stop stops the Slack bot.
func (s *SlackChannel) Stop() error {
	s.Running = false
	if s.cancelFn != nil {
		s.cancelFn()
	}
	return nil
}

// Send sends a message via Slack API.
func (s *SlackChannel) Send(msg bus.OutboundMessage) error {
	params := map[string]any{
		"channel": msg.ChatID,
		"text":    msg.Content,
	}

	// Thread support
	if msg.Metadata != nil {
		if slackMeta, ok := msg.Metadata["slack"].(map[string]any); ok {
			threadTS, _ := slackMeta["thread_ts"].(string)
			channelType, _ := slackMeta["channel_type"].(string)
			if threadTS != "" && channelType != "im" {
				params["thread_ts"] = threadTS
			}
		}
	}

	_, err := s.slackAPI("chat.postMessage", params)
	return err
}

// ProcessEvent handles an incoming Slack event (for testing and HTTP endpoint integration).
func (s *SlackChannel) ProcessEvent(event map[string]any) {
	eventType, _ := event["type"].(string)
	if eventType != "message" && eventType != "app_mention" {
		return
	}

	senderID, _ := event["user"].(string)
	chatID, _ := event["channel"].(string)
	text, _ := event["text"].(string)

	// Skip bot messages
	if event["subtype"] != nil {
		return
	}
	if s.BotUserID != "" && senderID == s.BotUserID {
		return
	}

	// Avoid double-processing mentions
	if eventType == "message" && s.BotUserID != "" && strings.Contains(text, "<@"+s.BotUserID+">") {
		return
	}

	if senderID == "" || chatID == "" {
		return
	}

	// Strip bot mention
	text = s.stripBotMention(text)

	threadTS := ""
	if ts, ok := event["thread_ts"].(string); ok {
		threadTS = ts
	} else if ts, ok := event["ts"].(string); ok {
		threadTS = ts
	}

	s.HandleMessage(senderID, chatID, text, nil, map[string]any{
		"slack": map[string]any{
			"thread_ts":    threadTS,
			"channel_type": event["channel_type"],
		},
	})
}

func (s *SlackChannel) stripBotMention(text string) string {
	if text == "" || s.BotUserID == "" {
		return text
	}
	mention := "<@" + s.BotUserID + ">"
	text = strings.ReplaceAll(text, mention, "")
	return strings.TrimSpace(text)
}

func (s *SlackChannel) slackAPI(method string, params map[string]any) (map[string]any, error) {
	url := "https://slack.com/api/" + method
	body, _ := json.Marshal(params)
	req, _ := http.NewRequest("POST", url, strings.NewReader(string(body)))
	req.Header.Set("Content-Type", "application/json; charset=utf-8")
	req.Header.Set("Authorization", "Bearer "+s.BotToken)

	resp, err := s.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	var result map[string]any
	json.NewDecoder(resp.Body).Decode(&result)
	return result, nil
}
