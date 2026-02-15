package agent

import (
	"context"
	"fmt"
	"sync"

	"github.com/dayuer/nanobot-go/internal/bus"
	"github.com/dayuer/nanobot-go/internal/providers"
	"github.com/dayuer/nanobot-go/internal/tools"
)

// SubagentManager manages background subagent execution.
type SubagentManager struct {
	Provider    providers.LLMProvider
	Workspace   string
	Bus         *bus.MessageBus
	Model       string
	MaxTokens   int
	Temperature float64

	mu      sync.Mutex
	running map[string]context.CancelFunc
}

// NewSubagentManager creates a SubagentManager.
func NewSubagentManager(provider providers.LLMProvider, workspace string, msgBus *bus.MessageBus, model string) *SubagentManager {
	return &SubagentManager{
		Provider:    provider,
		Workspace:   workspace,
		Bus:         msgBus,
		Model:       model,
		MaxTokens:   4096,
		Temperature: 0.7,
		running:     make(map[string]context.CancelFunc),
	}
}

// Spawn starts a subagent in the background.
func (sm *SubagentManager) Spawn(ctx context.Context, task, label, originChannel, originChatID string) string {
	taskID := fmt.Sprintf("sub-%d", sm.RunningCount()+1)
	if label == "" {
		if len(task) > 30 {
			label = task[:30] + "..."
		} else {
			label = task
		}
	}

	subCtx, cancel := context.WithCancel(ctx)
	sm.mu.Lock()
	sm.running[taskID] = cancel
	sm.mu.Unlock()

	go func() {
		defer func() {
			sm.mu.Lock()
			delete(sm.running, taskID)
			sm.mu.Unlock()
			cancel()
		}()
		sm.runSubagent(subCtx, taskID, task, label, originChannel, originChatID)
	}()

	return fmt.Sprintf("Subagent [%s] started (id: %s). I'll notify you when it completes.", label, taskID)
}

func (sm *SubagentManager) runSubagent(ctx context.Context, taskID, task, label, channel, chatID string) {
	// Build subagent tools (no message/spawn/cron)
	registry := tools.NewRegistry()
	registry.Register(&tools.ReadFileTool{})
	registry.Register(&tools.WriteFileTool{})
	registry.Register(&tools.ListDirTool{})
	registry.Register(&tools.WebFetchTool{})

	prompt := sm.buildSubagentPrompt(task)
	messages := []map[string]any{
		{"role": "system", "content": prompt},
		{"role": "user", "content": task},
	}

	maxIter := 15
	var finalResult string

	for i := 0; i < maxIter; i++ {
		providerMsgs := make([]providers.Message, 0, len(messages))
		for _, m := range messages {
			role, _ := m["role"].(string)
			content, _ := m["content"].(string)
			providerMsgs = append(providerMsgs, providers.Message{Role: role, Content: content})
		}

		resp, err := sm.Provider.Chat(ctx, providers.ChatRequest{
			Messages:    providerMsgs,
			Tools:       registry.Schemas(),
			Model:       sm.Model,
			MaxTokens:   sm.MaxTokens,
			Temperature: sm.Temperature,
		})
		if err != nil {
			finalResult = fmt.Sprintf("Error: %v", err)
			break
		}

		if !resp.HasToolCalls() {
			if resp.Content != nil {
				finalResult = *resp.Content
			} else {
				finalResult = "Task completed."
			}
			break
		}

		// Execute tools
		contentStr := ""
		if resp.Content != nil {
			contentStr = *resp.Content
		}
		messages = append(messages, map[string]any{
			"role":    "assistant",
			"content": contentStr,
		})

		for _, tc := range resp.ToolCalls {
			tool := registry.Get(tc.Name)
			var result string
			if tool != nil {
				result, err = tool.Execute(ctx, tc.Arguments)
				if err != nil {
					result = fmt.Sprintf("Error: %v", err)
				}
			} else {
				result = fmt.Sprintf("Error: unknown tool %q", tc.Name)
			}
			messages = append(messages, map[string]any{
				"role":         "tool",
				"tool_call_id": tc.ID,
				"name":         tc.Name,
				"content":      result,
			})
		}
	}

	if finalResult == "" {
		finalResult = "Task completed but no response was generated."
	}

	// Announce result back via bus
	if sm.Bus != nil {
		sm.Bus.PublishInbound(bus.InboundMessage{
			Channel:  "system",
			SenderID: "subagent",
			ChatID:   channel + ":" + chatID,
			Content:  fmt.Sprintf("[Subagent '%s' completed]\n\nTask: %s\n\nResult:\n%s", label, task, finalResult),
		})
	}
}

func (sm *SubagentManager) buildSubagentPrompt(task string) string {
	return fmt.Sprintf(`# Subagent

You are a subagent spawned by the main agent to complete a specific task.

## Rules
1. Stay focused - complete only the assigned task
2. Your final response will be reported back to the main agent
3. Be concise but informative

## What You Can Do
- Read and write files in the workspace
- Fetch web pages

## Workspace
%s`, sm.Workspace)
}

// RunningCount returns the number of active subagents.
func (sm *SubagentManager) RunningCount() int {
	sm.mu.Lock()
	defer sm.mu.Unlock()
	return len(sm.running)
}
