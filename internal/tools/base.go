// Package tools defines the Tool interface and contract tests for agent tools.
package tools

import "context"

// Tool is the interface that all agent tools must implement.
// Mirrors Python's agent/tools/base.py Tool ABC.
type Tool interface {
	// Name returns the tool name used in LLM function calls.
	Name() string

	// Description returns what the tool does.
	Description() string

	// Parameters returns the JSON Schema for tool parameters.
	Parameters() map[string]any

	// Execute runs the tool with the given arguments.
	Execute(ctx context.Context, args map[string]any) (string, error)
}

// ToSchema converts a tool to OpenAI function calling format.
func ToSchema(t Tool) map[string]any {
	return map[string]any{
		"type": "function",
		"function": map[string]any{
			"name":        t.Name(),
			"description": t.Description(),
			"parameters":  t.Parameters(),
		},
	}
}
