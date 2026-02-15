package agent

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"
)

// BootstrapFiles are loaded into the system prompt when present.
var BootstrapFiles = []string{"AGENTS.md", "SOUL.md", "USER.md", "TOOLS.md", "IDENTITY.md"}

// ContextBuilder assembles system prompts and message lists for the agent.
type ContextBuilder struct {
	Workspace string
	Memory    *MemoryStore
	Skills    *SkillsLoader
}

// NewContextBuilder creates a ContextBuilder for a workspace.
func NewContextBuilder(workspace string) *ContextBuilder {
	return &ContextBuilder{
		Workspace: workspace,
		Memory:    NewMemoryStore(workspace),
		Skills:    NewSkillsLoader(workspace, ""),
	}
}

// BuildSystemPrompt builds the full system prompt from identity, bootstrap, memory, and skills.
func (c *ContextBuilder) BuildSystemPrompt(skillNames []string) string {
	var parts []string

	parts = append(parts, c.getIdentity())

	if bs := c.loadBootstrapFiles(); bs != "" {
		parts = append(parts, bs)
	}

	if mem := c.Memory.GetMemoryContext(); mem != "" {
		parts = append(parts, fmt.Sprintf("# Memory\n\n%s", mem))
	}

	if summary := c.Skills.BuildSkillsSummary(); summary != "" {
		parts = append(parts, fmt.Sprintf(`# Skills

The following skills extend your capabilities. To use a skill, read its SKILL.md file using the read_file tool.

%s`, summary))
	}

	return strings.Join(parts, "\n\n---\n\n")
}

func (c *ContextBuilder) getIdentity() string {
	now := time.Now().Format("2006-01-02 15:04 (Monday)")
	tz, _ := time.Now().Zone()
	sys := runtime.GOOS
	if sys == "darwin" {
		sys = "macOS"
	}
	rt := fmt.Sprintf("%s %s, Go %s", sys, runtime.GOARCH, runtime.Version())
	ws, _ := filepath.Abs(c.Workspace)

	return fmt.Sprintf(`# nanobot 🐈

You are nanobot, a helpful AI assistant. You have access to tools that allow you to:
- Read, write, and edit files
- Execute shell commands
- Search the web and fetch web pages
- Send messages to users on chat channels
- Spawn subagents for complex background tasks

## Current Time
%s (%s)

## Runtime
%s

## Workspace
Your workspace is at: %s
- Long-term memory: %s/memory/MEMORY.md
- History log: %s/memory/HISTORY.md (grep-searchable)
- Custom skills: %s/skills/{skill-name}/SKILL.md

Always be helpful, accurate, and concise.`, now, tz, rt, ws, ws, ws, ws)
}

func (c *ContextBuilder) loadBootstrapFiles() string {
	var parts []string
	for _, name := range BootstrapFiles {
		path := filepath.Join(c.Workspace, name)
		data, err := os.ReadFile(path)
		if err == nil {
			parts = append(parts, fmt.Sprintf("## %s\n\n%s", name, string(data)))
		}
	}
	if len(parts) == 0 {
		return ""
	}
	return strings.Join(parts, "\n\n")
}

// BuildMessages constructs the full message list for an LLM call.
func (c *ContextBuilder) BuildMessages(history []map[string]any, userMsg string, channel, chatID string) []map[string]any {
	systemPrompt := c.BuildSystemPrompt(nil)
	if channel != "" && chatID != "" {
		systemPrompt += fmt.Sprintf("\n\n## Current Session\nChannel: %s\nChat ID: %s", channel, chatID)
	}

	messages := []map[string]any{
		{"role": "system", "content": systemPrompt},
	}
	messages = append(messages, history...)
	messages = append(messages, map[string]any{"role": "user", "content": userMsg})
	return messages
}

// AddToolResult appends a tool result message.
func (c *ContextBuilder) AddToolResult(messages []map[string]any, toolCallID, toolName, result string) []map[string]any {
	return append(messages, map[string]any{
		"role":         "tool",
		"tool_call_id": toolCallID,
		"name":         toolName,
		"content":      result,
	})
}

// AddAssistantMessage appends an assistant message with optional tool calls.
func (c *ContextBuilder) AddAssistantMessage(messages []map[string]any, content string, toolCalls []map[string]any, reasoningContent string) []map[string]any {
	msg := map[string]any{"role": "assistant", "content": content}
	if len(toolCalls) > 0 {
		msg["tool_calls"] = toolCalls
	}
	if reasoningContent != "" {
		msg["reasoning_content"] = reasoningContent
	}
	return append(messages, msg)
}
