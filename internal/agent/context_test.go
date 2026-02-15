package agent

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestContextBuilder_BuildSystemPrompt_Basic(t *testing.T) {
	ws := t.TempDir()
	cb := NewContextBuilder(ws)
	prompt := cb.BuildSystemPrompt(nil)
	assert.Contains(t, prompt, "# nanobot")
	assert.Contains(t, prompt, "workspace")
}

func TestContextBuilder_BuildSystemPrompt_WithMemory(t *testing.T) {
	ws := t.TempDir()
	cb := NewContextBuilder(ws)
	cb.Memory.WriteLongTerm("User is a Go developer")
	prompt := cb.BuildSystemPrompt(nil)
	assert.Contains(t, prompt, "# Memory")
	assert.Contains(t, prompt, "User is a Go developer")
}

func TestContextBuilder_BuildSystemPrompt_WithBootstrap(t *testing.T) {
	ws := t.TempDir()
	os.WriteFile(filepath.Join(ws, "AGENTS.md"), []byte("I am an agent"), 0o644)
	cb := NewContextBuilder(ws)
	prompt := cb.BuildSystemPrompt(nil)
	assert.Contains(t, prompt, "## AGENTS.md")
	assert.Contains(t, prompt, "I am an agent")
}

func TestContextBuilder_BuildMessages(t *testing.T) {
	ws := t.TempDir()
	cb := NewContextBuilder(ws)
	history := []map[string]any{
		{"role": "user", "content": "Hello"},
		{"role": "assistant", "content": "Hi there!"},
	}
	msgs := cb.BuildMessages(history, "What's 2+2?", "telegram", "123")

	require.Len(t, msgs, 4) // system + 2 history + user
	assert.Equal(t, "system", msgs[0]["role"])
	assert.Contains(t, msgs[0]["content"], "Channel: telegram")
	assert.Equal(t, "user", msgs[1]["role"])
	assert.Equal(t, "Hello", msgs[1]["content"])
	assert.Equal(t, "What's 2+2?", msgs[3]["content"])
}

func TestContextBuilder_BuildMessages_NoChannel(t *testing.T) {
	ws := t.TempDir()
	cb := NewContextBuilder(ws)
	msgs := cb.BuildMessages(nil, "Hi", "", "")
	require.Len(t, msgs, 2) // system + user
	assert.NotContains(t, msgs[0]["content"], "Channel:")
}

func TestContextBuilder_AddToolResult(t *testing.T) {
	cb := NewContextBuilder(t.TempDir())
	msgs := cb.AddToolResult(nil, "call_1", "read_file", "file content")
	require.Len(t, msgs, 1)
	assert.Equal(t, "tool", msgs[0]["role"])
	assert.Equal(t, "call_1", msgs[0]["tool_call_id"])
	assert.Equal(t, "read_file", msgs[0]["name"])
}

func TestContextBuilder_AddAssistantMessage(t *testing.T) {
	cb := NewContextBuilder(t.TempDir())
	toolCalls := []map[string]any{
		{"id": "call_1", "type": "function", "function": map[string]any{"name": "exec"}},
	}
	msgs := cb.AddAssistantMessage(nil, "Let me run that", toolCalls, "thinking...")
	require.Len(t, msgs, 1)
	assert.Equal(t, "assistant", msgs[0]["role"])
	assert.Equal(t, "Let me run that", msgs[0]["content"])
	assert.NotNil(t, msgs[0]["tool_calls"])
	assert.Equal(t, "thinking...", msgs[0]["reasoning_content"])
}

func TestContextBuilder_AddAssistantMessage_NoToolCalls(t *testing.T) {
	cb := NewContextBuilder(t.TempDir())
	msgs := cb.AddAssistantMessage(nil, "Just text", nil, "")
	assert.Nil(t, msgs[0]["tool_calls"])
	_, hasRC := msgs[0]["reasoning_content"]
	assert.False(t, hasRC) // no reasoning_content key when empty
}
