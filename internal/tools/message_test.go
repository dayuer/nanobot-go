package tools

import (
	"context"
	"fmt"
	"testing"

	"github.com/dayuer/nanobot-go/internal/bus"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// --- MessageTool Tests ---

func TestMessageTool_Contract(t *testing.T) {
	RunToolContractTests(t, &MessageTool{})
}

func TestMessageTool_Execute(t *testing.T) {
	var sent bus.OutboundMessage
	tool := &MessageTool{
		SendCallback: func(msg bus.OutboundMessage) error {
			sent = msg
			return nil
		},
		DefaultChannel: "telegram",
		DefaultChatID:  "123",
	}
	result, err := tool.Execute(context.Background(), map[string]any{
		"content": "hello!",
	})
	require.NoError(t, err)
	assert.Contains(t, result, "Message sent")
	assert.Equal(t, "telegram", sent.Channel)
	assert.Equal(t, "123", sent.ChatID)
	assert.Equal(t, "hello!", sent.Content)
}

func TestMessageTool_OverrideChannel(t *testing.T) {
	var sent bus.OutboundMessage
	tool := &MessageTool{
		SendCallback:   func(msg bus.OutboundMessage) error { sent = msg; return nil },
		DefaultChannel: "telegram",
		DefaultChatID:  "123",
	}
	tool.Execute(context.Background(), map[string]any{
		"content": "hi", "channel": "discord", "chat_id": "456",
	})
	assert.Equal(t, "discord", sent.Channel)
	assert.Equal(t, "456", sent.ChatID)
}

func TestMessageTool_NoTarget(t *testing.T) {
	tool := &MessageTool{}
	result, _ := tool.Execute(context.Background(), map[string]any{"content": "hi"})
	assert.Contains(t, result, "No target channel")
}

func TestMessageTool_NoCallback(t *testing.T) {
	tool := &MessageTool{DefaultChannel: "t", DefaultChatID: "1"}
	result, _ := tool.Execute(context.Background(), map[string]any{"content": "hi"})
	assert.Contains(t, result, "not configured")
}

func TestMessageTool_SendError(t *testing.T) {
	tool := &MessageTool{
		SendCallback:   func(msg bus.OutboundMessage) error { return fmt.Errorf("net error") },
		DefaultChannel: "t", DefaultChatID: "1",
	}
	result, _ := tool.Execute(context.Background(), map[string]any{"content": "hi"})
	assert.Contains(t, result, "Error sending message")
}

// --- SpawnTool Tests ---

func TestSpawnTool_Contract(t *testing.T) {
	RunToolContractTests(t, &SpawnTool{})
}

func TestSpawnTool_Execute(t *testing.T) {
	var capturedTask, capturedLabel, capturedCh, capturedID string
	tool := &SpawnTool{
		SpawnCallback: func(task, label, ch, id string) (string, error) {
			capturedTask = task
			capturedLabel = label
			capturedCh = ch
			capturedID = id
			return "Spawned subagent 'abc'", nil
		},
		OriginChannel: "cli",
		OriginChatID:  "direct",
	}

	result, err := tool.Execute(context.Background(), map[string]any{
		"task": "research topic", "label": "Research",
	})
	require.NoError(t, err)
	assert.Contains(t, result, "Spawned")
	assert.Equal(t, "research topic", capturedTask)
	assert.Equal(t, "Research", capturedLabel)
	assert.Equal(t, "cli", capturedCh)
	assert.Equal(t, "direct", capturedID)
}

func TestSpawnTool_NoCallback(t *testing.T) {
	tool := &SpawnTool{}
	result, _ := tool.Execute(context.Background(), map[string]any{"task": "x"})
	assert.Contains(t, result, "not configured")
}

// --- CronTool Tests ---

func TestCronTool_Contract(t *testing.T) {
	RunToolContractTests(t, &CronTool{})
}

type mockCron struct {
	jobs []string
}

func (m *mockCron) AddJob(name, message, channel, chatID string, every int, expr string, at string) (string, error) {
	m.jobs = append(m.jobs, name)
	return fmt.Sprintf("Created job '%s' (id: mock-1)", name), nil
}
func (m *mockCron) ListJobs() (string, error) {
	if len(m.jobs) == 0 {
		return "No scheduled jobs.", nil
	}
	return fmt.Sprintf("Scheduled jobs: %d", len(m.jobs)), nil
}
func (m *mockCron) RemoveJob(jobID string) (string, error) {
	return fmt.Sprintf("Removed job %s", jobID), nil
}

func TestCronTool_Add(t *testing.T) {
	mc := &mockCron{}
	tool := &CronTool{Cron: mc, Channel: "telegram", ChatID: "123"}

	result, err := tool.Execute(context.Background(), map[string]any{
		"action": "add", "message": "Drink water", "every_seconds": float64(3600),
	})
	require.NoError(t, err)
	assert.Contains(t, result, "Created job")
	assert.Len(t, mc.jobs, 1)
}

func TestCronTool_AddNoMessage(t *testing.T) {
	tool := &CronTool{Cron: &mockCron{}, Channel: "t", ChatID: "1"}
	result, _ := tool.Execute(context.Background(), map[string]any{"action": "add"})
	assert.Contains(t, result, "message is required")
}

func TestCronTool_AddNoContext(t *testing.T) {
	tool := &CronTool{Cron: &mockCron{}}
	result, _ := tool.Execute(context.Background(), map[string]any{
		"action": "add", "message": "hello",
	})
	assert.Contains(t, result, "no session context")
}

func TestCronTool_List(t *testing.T) {
	tool := &CronTool{Cron: &mockCron{}}
	result, _ := tool.Execute(context.Background(), map[string]any{"action": "list"})
	assert.Contains(t, result, "No scheduled jobs")
}

func TestCronTool_Remove(t *testing.T) {
	tool := &CronTool{Cron: &mockCron{}}
	result, _ := tool.Execute(context.Background(), map[string]any{
		"action": "remove", "job_id": "abc",
	})
	assert.Contains(t, result, "Removed job abc")
}

func TestCronTool_RemoveNoID(t *testing.T) {
	tool := &CronTool{Cron: &mockCron{}}
	result, _ := tool.Execute(context.Background(), map[string]any{"action": "remove"})
	assert.Contains(t, result, "job_id is required")
}

func TestCronTool_UnknownAction(t *testing.T) {
	tool := &CronTool{Cron: &mockCron{}}
	result, _ := tool.Execute(context.Background(), map[string]any{"action": "pause"})
	assert.Contains(t, result, "Unknown action")
}

func TestCronTool_NoCron(t *testing.T) {
	tool := &CronTool{}
	result, _ := tool.Execute(context.Background(), map[string]any{"action": "list"})
	assert.Contains(t, result, "not configured")
}
