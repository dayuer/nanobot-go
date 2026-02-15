// Package tools provides the ToolRegistry for managing agent tools.
package tools

import "sync"

// Registry holds all registered tools, keyed by name.
type Registry struct {
	mu    sync.RWMutex
	tools map[string]Tool
}

// NewRegistry creates an empty tool registry.
func NewRegistry() *Registry {
	return &Registry{tools: make(map[string]Tool)}
}

// Register adds a tool to the registry.
func (r *Registry) Register(tool Tool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.tools[tool.Name()] = tool
}

// Get returns a tool by name, or nil if not found.
func (r *Registry) Get(name string) Tool {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.tools[name]
}

// All returns all registered tools.
func (r *Registry) All() []Tool {
	r.mu.RLock()
	defer r.mu.RUnlock()
	result := make([]Tool, 0, len(r.tools))
	for _, t := range r.tools {
		result = append(result, t)
	}
	return result
}

// Schemas returns OpenAI function-call schemas for all registered tools.
func (r *Registry) Schemas() []map[string]any {
	tools := r.All()
	schemas := make([]map[string]any, len(tools))
	for i, t := range tools {
		schemas[i] = ToSchema(t)
	}
	return schemas
}
