package tools

import (
	"context"
	"fmt"

	"github.com/dayuer/nanobot-go/internal/bus"
)

// SendFunc is the callback type for sending outbound messages.
type SendFunc func(msg bus.OutboundMessage) error

// MessageTool sends messages to users on chat channels.
type MessageTool struct {
	SendCallback   SendFunc
	DefaultChannel string
	DefaultChatID  string
}

func (t *MessageTool) Name() string        { return "message" }
func (t *MessageTool) Description() string  { return "Send a message to the user." }
func (t *MessageTool) Parameters() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"content": map[string]any{"type": "string", "description": "The message content to send"},
			"channel": map[string]any{"type": "string", "description": "Optional: target channel"},
			"chat_id": map[string]any{"type": "string", "description": "Optional: target chat/user ID"},
		},
		"required": []string{"content"},
	}
}

// SetContext sets the current message context.
func (t *MessageTool) SetContext(channel, chatID string) {
	t.DefaultChannel = channel
	t.DefaultChatID = chatID
}

func (t *MessageTool) Execute(_ context.Context, args map[string]any) (string, error) {
	content, _ := args["content"].(string)
	channel, _ := args["channel"].(string)
	chatID, _ := args["chat_id"].(string)

	if channel == "" {
		channel = t.DefaultChannel
	}
	if chatID == "" {
		chatID = t.DefaultChatID
	}
	if channel == "" || chatID == "" {
		return "Error: No target channel/chat specified", nil
	}
	if t.SendCallback == nil {
		return "Error: Message sending not configured", nil
	}

	msg := bus.OutboundMessage{
		Channel: channel,
		ChatID:  chatID,
		Content: content,
	}
	if err := t.SendCallback(msg); err != nil {
		return fmt.Sprintf("Error sending message: %v", err), nil
	}
	return fmt.Sprintf("Message sent to %s:%s", channel, chatID), nil
}

// SpawnFunc is the callback for spawning subagents.
type SpawnFunc func(task, label, channel, chatID string) (string, error)

// SpawnTool spawns a subagent for background task execution.
type SpawnTool struct {
	SpawnCallback  SpawnFunc
	OriginChannel  string
	OriginChatID   string
}

func (t *SpawnTool) Name() string        { return "spawn" }
func (t *SpawnTool) Description() string  { return "Spawn a subagent to handle a task in the background." }
func (t *SpawnTool) Parameters() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"task":  map[string]any{"type": "string", "description": "The task for the subagent"},
			"label": map[string]any{"type": "string", "description": "Optional short label"},
		},
		"required": []string{"task"},
	}
}

// SetContext sets the origin context for subagent announcements.
func (t *SpawnTool) SetContext(channel, chatID string) {
	t.OriginChannel = channel
	t.OriginChatID = chatID
}

func (t *SpawnTool) Execute(_ context.Context, args map[string]any) (string, error) {
	task, _ := args["task"].(string)
	label, _ := args["label"].(string)

	if t.SpawnCallback == nil {
		return "Error: Subagent spawning not configured", nil
	}
	return t.SpawnCallback(task, label, t.OriginChannel, t.OriginChatID)
}

// CronAction describes a scheduled task action.
type CronAction string

const (
	CronAdd    CronAction = "add"
	CronList   CronAction = "list"
	CronRemove CronAction = "remove"
)

// CronCallback is the interface for scheduling operations.
type CronCallback interface {
	AddJob(name, message, channel, chatID string, everySeconds int, cronExpr string, at string) (string, error)
	ListJobs() (string, error)
	RemoveJob(jobID string) (string, error)
}

// CronTool manages scheduled reminders and recurring tasks.
type CronTool struct {
	Cron    CronCallback
	Channel string
	ChatID  string
}

func (t *CronTool) Name() string        { return "cron" }
func (t *CronTool) Description() string  { return "Schedule reminders and recurring tasks. Actions: add, list, remove." }
func (t *CronTool) Parameters() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"action":        map[string]any{"type": "string", "enum": []string{"add", "list", "remove"}},
			"message":       map[string]any{"type": "string", "description": "Reminder message (for add)"},
			"every_seconds": map[string]any{"type": "integer", "description": "Interval in seconds"},
			"cron_expr":     map[string]any{"type": "string", "description": "Cron expression"},
			"at":            map[string]any{"type": "string", "description": "ISO datetime for one-time"},
			"job_id":        map[string]any{"type": "string", "description": "Job ID (for remove)"},
		},
		"required": []string{"action"},
	}
}

// SetContext sets the delivery target for scheduled messages.
func (t *CronTool) SetContext(channel, chatID string) {
	t.Channel = channel
	t.ChatID = chatID
}

func (t *CronTool) Execute(_ context.Context, args map[string]any) (string, error) {
	action, _ := args["action"].(string)

	if t.Cron == nil {
		return "Error: Cron service not configured", nil
	}

	switch CronAction(action) {
	case CronAdd:
		message, _ := args["message"].(string)
		if message == "" {
			return "Error: message is required for add", nil
		}
		if t.Channel == "" || t.ChatID == "" {
			return "Error: no session context (channel/chat_id)", nil
		}
		everySeconds := 0
		if v, ok := args["every_seconds"].(float64); ok {
			everySeconds = int(v)
		}
		cronExpr, _ := args["cron_expr"].(string)
		at, _ := args["at"].(string)

		name := message
		if len(name) > 30 {
			name = name[:30]
		}
		return t.Cron.AddJob(name, message, t.Channel, t.ChatID, everySeconds, cronExpr, at)

	case CronList:
		return t.Cron.ListJobs()

	case CronRemove:
		jobID, _ := args["job_id"].(string)
		if jobID == "" {
			return "Error: job_id is required for remove", nil
		}
		return t.Cron.RemoveJob(jobID)

	default:
		return fmt.Sprintf("Unknown action: %s", action), nil
	}
}
