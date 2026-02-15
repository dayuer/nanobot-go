package agent

import (
	"context"
	"testing"
	"time"

	"github.com/dayuer/nanobot-go/internal/bus"
	"github.com/dayuer/nanobot-go/internal/providers"
	"github.com/stretchr/testify/assert"
)

func TestSubagentManager_Spawn(t *testing.T) {
	mp := &mockProvider{
		responses: []*providers.LLMResponse{
			{Content: strP("Task done"), FinishReason: "stop"},
		},
	}
	msgBus := bus.NewMessageBus()
	sm := NewSubagentManager(mp, t.TempDir(), msgBus, "mock-model")

	result := sm.Spawn(context.Background(), "Search for Go docs", "go-docs", "telegram", "123")
	assert.Contains(t, result, "go-docs")
	assert.Contains(t, result, "started")

	// Wait for subagent to complete
	time.Sleep(200 * time.Millisecond)
	assert.Equal(t, 0, sm.RunningCount())
}

func TestSubagentManager_RunningCount(t *testing.T) {
	mp := &mockProvider{
		responses: []*providers.LLMResponse{
			{Content: strP("done"), FinishReason: "stop"},
		},
	}
	sm := NewSubagentManager(mp, t.TempDir(), nil, "mock-model")
	assert.Equal(t, 0, sm.RunningCount())
}

func TestSubagentManager_LabelTruncation(t *testing.T) {
	mp := &mockProvider{
		responses: []*providers.LLMResponse{
			{Content: strP("done"), FinishReason: "stop"},
		},
	}
	sm := NewSubagentManager(mp, t.TempDir(), nil, "mock-model")
	result := sm.Spawn(context.Background(), "This is a very long task description that should be truncated", "", "cli", "direct")
	assert.Contains(t, result, "This is a very long task descr...")
}

func TestSubagentManager_AnnouncesResult(t *testing.T) {
	mp := &mockProvider{
		responses: []*providers.LLMResponse{
			{Content: strP("Search complete: found 3 results"), FinishReason: "stop"},
		},
	}
	msgBus := bus.NewMessageBus()
	sm := NewSubagentManager(mp, t.TempDir(), msgBus, "mock-model")

	sm.Spawn(context.Background(), "Search task", "search", "telegram", "42")
	time.Sleep(200 * time.Millisecond)

	// Check bus for announcement
	select {
	case msg := <-msgBus.Inbound:
		assert.Equal(t, "system", msg.Channel)
		assert.Contains(t, msg.Content, "completed")
		assert.Equal(t, "telegram:42", msg.ChatID)
	case <-time.After(time.Second):
		t.Fatal("timeout waiting for bus message")
	}
}
