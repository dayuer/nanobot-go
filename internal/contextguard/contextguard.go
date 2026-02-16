// Package contextguard implements token pre-check, context compression,
// and memory flush to prevent token overflow in long conversations.
// Mirrors survival/nanobot/context_guard.py.
package contextguard

import (
	"fmt"
	"log"
	"strings"
)

// Action describes the pre-check result.
type Action string

const (
	ActionPass       Action = "pass"       // Token usage OK
	ActionWarn       Action = "warn"       // Approaching limit
	ActionCompressed Action = "compressed" // Context was compressed
	ActionReset      Action = "reset"      // Session was force-reset
)

// PreCheckResult holds the result of a token pre-check.
type PreCheckResult struct {
	Action        Action
	TokenEstimate int
	TokenLimit    int
	Ratio         float64
}

// ShouldNotifyUser returns true if the user should be informed (on reset).
func (r PreCheckResult) ShouldNotifyUser() bool {
	return r.Action == ActionReset
}

// NotificationMessage returns a user-visible message for resets.
func (r PreCheckResult) NotificationMessage() string {
	if r.Action != ActionReset {
		return ""
	}
	return fmt.Sprintf("⚠️ 对话上下文已超出模型限制 (%.0f%%)，已自动重置会话。之前的对话记忆已保存到知识库中，可通过 knowledge_search 工具检索。",
		r.Ratio*100)
}

// ModelTokenLimits maps model names to their context window sizes.
var ModelTokenLimits = map[string]int{
	// DeepSeek
	"deepseek/deepseek-chat":     64_000,
	"deepseek/deepseek-reasoner": 64_000,
	// OpenAI
	"gpt-4o":                          128_000,
	"gpt-4o-mini":                     128_000,
	"gpt-4-turbo":                     128_000,
	"openai/gpt-4o":                   128_000,
	// Anthropic
	"anthropic/claude-sonnet-4-20250514":  200_000,
	"anthropic/claude-opus-4-5":       200_000,
	// ZhipuAI
	"zhipuai/glm-5":                   128_000,
	"zhipuai/glm-4-flash":             128_000,
	// Default
	"_default": 64_000,
}

// GetModelLimit returns the token limit for a model.
func GetModelLimit(model string) int {
	if limit, ok := ModelTokenLimits[model]; ok {
		return limit
	}
	// Try prefix match
	for k, v := range ModelTokenLimits {
		if strings.HasPrefix(model, k) {
			return v
		}
	}
	return ModelTokenLimits["_default"]
}

// EstimateTokens estimates the token count for a list of messages.
// Uses a rough heuristic: 1 Chinese char ≈ 1.5 tokens, total chars / 2.
// Errs on the side of overestimation (safer).
func EstimateTokens(messages []map[string]any) int {
	total := 0
	for _, msg := range messages {
		if content, ok := msg["content"].(string); ok {
			total += len(content)
		}
		// Count tool calls
		if tc, ok := msg["tool_calls"].([]any); ok {
			for _, call := range tc {
				if callMap, ok := call.(map[string]any); ok {
					if fn, ok := callMap["function"].(map[string]any); ok {
						if args, ok := fn["arguments"].(string); ok {
							total += len(args)
						}
					}
				}
			}
		}
	}
	// Rough estimate: total chars / 2 (works well for Chinese-English mix)
	return total / 2
}

// Config holds ContextGuard thresholds.
type Config struct {
	WarnRatio     float64 // 0.70 → log warning
	CompressRatio float64 // 0.80 → trigger compression
	CriticalRatio float64 // 0.95 → force reset
}

// DefaultConfig returns sensible defaults.
func DefaultConfig() Config {
	return Config{
		WarnRatio:     0.70,
		CompressRatio: 0.80,
		CriticalRatio: 0.95,
	}
}

// Guard is the context guard that monitors token usage.
type Guard struct {
	cfg Config

	// Stats
	TotalChecks     int
	WarningCount    int
	CompressionCount int
	ResetCount      int
}

// NewGuard creates a new context guard.
func NewGuard(cfg Config) *Guard {
	return &Guard{cfg: cfg}
}

// PreCheck performs a token pre-check before calling the LLM.
// Returns the action to take based on current token usage.
func (g *Guard) PreCheck(messages []map[string]any, model string) PreCheckResult {
	g.TotalChecks++

	tokenEstimate := EstimateTokens(messages)
	tokenLimit := GetModelLimit(model)
	ratio := float64(tokenEstimate) / float64(tokenLimit)

	result := PreCheckResult{
		TokenEstimate: tokenEstimate,
		TokenLimit:    tokenLimit,
		Ratio:         ratio,
	}

	switch {
	case ratio >= g.cfg.CriticalRatio:
		result.Action = ActionReset
		g.ResetCount++
		log.Printf("[ContextGuard] 🔴 CRITICAL %.0f%% (%d/%d) → force reset",
			ratio*100, tokenEstimate, tokenLimit)

	case ratio >= g.cfg.CompressRatio:
		result.Action = ActionCompressed
		g.CompressionCount++
		log.Printf("[ContextGuard] 🟡 COMPRESS %.0f%% (%d/%d) → compressing",
			ratio*100, tokenEstimate, tokenLimit)

	case ratio >= g.cfg.WarnRatio:
		result.Action = ActionWarn
		g.WarningCount++
		log.Printf("[ContextGuard] 🟠 WARN %.0f%% (%d/%d)",
			ratio*100, tokenEstimate, tokenLimit)

	default:
		result.Action = ActionPass
	}

	return result
}

// Stats returns guard statistics.
func (g *Guard) Stats() map[string]any {
	return map[string]any{
		"totalChecks":      g.TotalChecks,
		"warningCount":     g.WarningCount,
		"compressionCount": g.CompressionCount,
		"resetCount":       g.ResetCount,
	}
}
