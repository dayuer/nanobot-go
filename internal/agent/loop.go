package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"sync"

	"github.com/dayuer/nanobot-go/internal/bus"
	"github.com/dayuer/nanobot-go/internal/providers"
	"github.com/dayuer/nanobot-go/internal/session"
	"github.com/dayuer/nanobot-go/internal/tools"
)

// AgentLoop is the core processing engine.
// It receives messages, builds context, calls the LLM, executes tools, and sends responses.
type AgentLoop struct {
	Bus           *bus.MessageBus
	Provider      providers.LLMProvider
	Workspace     string
	Model         string
	MaxIterations int
	Temperature   float64
	MaxTokens     int
	MemoryWindow  int

	Context  *ContextBuilder
	Sessions *session.Manager
	Tools    *tools.Registry

	running bool
	mu      sync.Mutex
}

// AgentConfig holds configuration for creating an AgentLoop.
type AgentConfig struct {
	Workspace     string
	Model         string
	MaxIterations int
	Temperature   float64
	MaxTokens     int
	MemoryWindow  int
	BraveAPIKey   string
}

// NewAgentLoop creates and configures an agent loop.
func NewAgentLoop(msgBus *bus.MessageBus, provider providers.LLMProvider, cfg AgentConfig) *AgentLoop {
	model := cfg.Model
	if model == "" {
		model = provider.DefaultModel()
	}
	maxIter := cfg.MaxIterations
	if maxIter == 0 {
		maxIter = 20
	}
	maxTokens := cfg.MaxTokens
	if maxTokens == 0 {
		maxTokens = 4096
	}
	memWin := cfg.MemoryWindow
	if memWin == 0 {
		memWin = 50
	}

	loop := &AgentLoop{
		Bus:           msgBus,
		Provider:      provider,
		Workspace:     cfg.Workspace,
		Model:         model,
		MaxIterations: maxIter,
		Temperature:   cfg.Temperature,
		MaxTokens:     maxTokens,
		MemoryWindow:  memWin,
		Context:       NewContextBuilder(cfg.Workspace),
		Sessions:      session.NewManager(cfg.Workspace),
		Tools:         tools.NewRegistry(),
	}
	return loop
}

// RunAgentLoop executes the tool-calling loop until no more tool calls or max iterations.
func (a *AgentLoop) RunAgentLoop(ctx context.Context, messages []map[string]any) (string, []string, error) {
	iteration := 0
	var toolsUsed []string

	for iteration < a.MaxIterations {
		iteration++

		// Convert messages to providers.Message
		providerMsgs := make([]providers.Message, 0, len(messages))
		for _, m := range messages {
			role, _ := m["role"].(string)
			content, _ := m["content"].(string)
			providerMsgs = append(providerMsgs, providers.Message{Role: role, Content: content})
		}

		resp, err := a.Provider.Chat(ctx, providers.ChatRequest{
			Messages:    providerMsgs,
			Tools:       a.Tools.Schemas(),
			Model:       a.Model,
			MaxTokens:   a.MaxTokens,
			Temperature: a.Temperature,
		})
		if err != nil {
			return "", toolsUsed, fmt.Errorf("LLM chat: %w", err)
		}

		if resp.HasToolCalls() {
			// Build tool_calls for assistant message
			var toolCallDicts []map[string]any
			for _, tc := range resp.ToolCalls {
				argsJSON, _ := json.Marshal(tc.Arguments)
				toolCallDicts = append(toolCallDicts, map[string]any{
					"id":   tc.ID,
					"type": "function",
					"function": map[string]any{
						"name":      tc.Name,
						"arguments": string(argsJSON),
					},
				})
			}

			contentStr := ""
			if resp.Content != nil {
				contentStr = *resp.Content
			}
			rcStr := ""
			if resp.ReasoningContent != nil {
				rcStr = *resp.ReasoningContent
			}
			messages = a.Context.AddAssistantMessage(messages, contentStr, toolCallDicts, rcStr)

			// Execute tools
			for _, tc := range resp.ToolCalls {
				toolsUsed = append(toolsUsed, tc.Name)
				tool := a.Tools.Get(tc.Name)
				var result string
				if tool != nil {
					result, err = tool.Execute(ctx, tc.Arguments)
					if err != nil {
						result = fmt.Sprintf("Error: %v", err)
					}
				} else {
					result = fmt.Sprintf("Error: unknown tool %q", tc.Name)
				}
				messages = a.Context.AddToolResult(messages, tc.ID, tc.Name, result)
			}
		} else {
			content := ""
			if resp.Content != nil {
				content = *resp.Content
			}
			return content, toolsUsed, nil
		}
	}

	return "Max iterations reached", toolsUsed, nil
}

// ProcessDirect processes a message directly (CLI/cron usage).
func (a *AgentLoop) ProcessDirect(ctx context.Context, content, sessionKey, channel, chatID string) (string, error) {
	if sessionKey == "" {
		sessionKey = "cli:direct"
	}
	if channel == "" {
		channel = "cli"
	}
	if chatID == "" {
		chatID = "direct"
	}

	sess := a.Sessions.GetOrCreate(sessionKey)

	// Convert session history from []map[string]string to []map[string]any
	hist := sess.GetHistory(a.MemoryWindow)
	histAny := make([]map[string]any, len(hist))
	for i, h := range hist {
		histAny[i] = map[string]any{"role": h["role"], "content": h["content"]}
	}

	messages := a.Context.BuildMessages(histAny, content, channel, chatID)

	finalContent, _, err := a.RunAgentLoop(ctx, messages)
	if err != nil {
		return "", err
	}
	if finalContent == "" {
		finalContent = "Completed processing."
	}

	sess.AddMessage("user", content)
	sess.AddMessage("assistant", finalContent)
	a.Sessions.Save(sess)

	return finalContent, nil
}

// Stop signals the agent loop to stop.
func (a *AgentLoop) Stop() {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.running = false
	log.Println("Agent loop stopping")
}
