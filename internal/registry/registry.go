// Package registry provides multi-agent registration and dispatch.
//
// It manages multiple independent AgentLoop instances, each with its own
// model, temperature, max_tokens, tools whitelist, and system prompt.
// Agents are defined in agents.yaml and registered at startup.
//
// This mirrors survival/nanobot/agent_registry.py.
package registry

import (
	"context"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"sync"

	"gopkg.in/yaml.v3"

	"github.com/dayuer/nanobot-go/internal/agent"
	"github.com/dayuer/nanobot-go/internal/bus"
	"github.com/dayuer/nanobot-go/internal/providers"
)

// AgentSpec defines a single agent's configuration (from agents.yaml).
type AgentSpec struct {
	ID               string   `yaml:"id" json:"id"`
	Description      string   `yaml:"description" json:"description"`
	Model            string   `yaml:"model,omitempty" json:"model,omitempty"`
	Temperature      float64  `yaml:"temperature,omitempty" json:"temperature,omitempty"`
	MaxTokens        int      `yaml:"max_tokens,omitempty" json:"maxTokens,omitempty"`
	MaxIterations    int      `yaml:"max_iterations,omitempty" json:"maxIterations,omitempty"`
	SystemPromptFile string   `yaml:"system_prompt_file,omitempty" json:"systemPromptFile,omitempty"`
	Tools            []string `yaml:"tools,omitempty" json:"tools,omitempty"`
	Skills           []string `yaml:"skills,omitempty" json:"skills,omitempty"`
	IsDefault        bool     `yaml:"is_default,omitempty" json:"isDefault,omitempty"`

	// Per-agent provider override (optional)
	ProviderConfig *ProviderConfig `yaml:"provider,omitempty" json:"provider,omitempty"`
}

// ProviderConfig holds per-agent provider overrides.
type ProviderConfig struct {
	APIKey       string `yaml:"api_key,omitempty" json:"apiKey,omitempty"`
	APIBase      string `yaml:"api_base,omitempty" json:"apiBase,omitempty"`
	ProviderName string `yaml:"provider_name,omitempty" json:"providerName,omitempty"`
}

// agentsFile is the top-level structure of agents.yaml.
type agentsFile struct {
	Agents []AgentSpec `yaml:"agents"`
}

// LoadAgentSpecs reads and parses an agents.yaml file.
func LoadAgentSpecs(path string) ([]AgentSpec, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil // no agents.yaml → single-agent mode
		}
		return nil, fmt.Errorf("read agents.yaml: %w", err)
	}

	var f agentsFile
	if err := yaml.Unmarshal(data, &f); err != nil {
		return nil, fmt.Errorf("parse agents.yaml: %w", err)
	}
	return f.Agents, nil
}

// registeredAgent holds a running agent and its spec.
type registeredAgent struct {
	spec  AgentSpec
	loop  *agent.AgentLoop
	prompt string // loaded system prompt content
}

// Registry manages multiple AgentLoop instances.
type Registry struct {
	mu              sync.RWMutex
	agents          map[string]*registeredAgent
	defaultID       string
	defaultProvider providers.LLMProvider
	bus             *bus.MessageBus
	workspace       string
	defaultModel    string
}

// RegistryConfig holds shared settings for all agents.
type RegistryConfig struct {
	DefaultProvider providers.LLMProvider
	Bus             *bus.MessageBus
	Workspace       string
	DefaultModel    string
}

// NewRegistry creates a new agent registry.
func NewRegistry(cfg RegistryConfig) *Registry {
	return &Registry{
		agents:          make(map[string]*registeredAgent),
		defaultProvider: cfg.DefaultProvider,
		bus:             cfg.Bus,
		workspace:       cfg.Workspace,
		defaultModel:    cfg.DefaultModel,
	}
}

// Register creates and registers an AgentLoop from an AgentSpec.
func (r *Registry) Register(spec AgentSpec) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	// Resolve provider
	provider := r.resolveProvider(spec)

	// Resolve model
	model := spec.Model
	if model == "" {
		model = r.defaultModel
	}

	// Resolve temperature
	temp := spec.Temperature
	if temp == 0 {
		temp = 0.7
	}

	// Resolve max tokens
	maxTokens := spec.MaxTokens
	if maxTokens == 0 {
		maxTokens = 4096
	}

	// Resolve max iterations
	maxIter := spec.MaxIterations
	if maxIter == 0 {
		maxIter = 25
	}

	// Create AgentLoop
	loop := agent.NewAgentLoop(r.bus, provider, agent.AgentConfig{
		Workspace:     r.workspace,
		Model:         model,
		Temperature:   temp,
		MaxTokens:     maxTokens,
		MaxIterations: maxIter,
	})

	// Load system prompt
	prompt := ""
	if spec.SystemPromptFile != "" {
		promptPath := filepath.Join(r.workspace, "..", spec.SystemPromptFile)
		if data, err := os.ReadFile(promptPath); err == nil {
			prompt = string(data)
		} else {
			log.Printf("[Registry] ⚠️ Prompt file not found for %s: %s", spec.ID, promptPath)
		}
	}

	r.agents[spec.ID] = &registeredAgent{
		spec:   spec,
		loop:   loop,
		prompt: prompt,
	}

	if spec.IsDefault {
		r.defaultID = spec.ID
	}

	log.Printf("[Registry] ✅ Registered agent: %s (model=%s, temp=%.1f)", spec.ID, model, temp)
	return nil
}

// RegisterOrUpdate dynamically registers a new agent or updates an existing one.
// Used for WS-pushed agent_config messages from the Backend.
func (r *Registry) RegisterOrUpdate(agentID string, config map[string]any) error {
	r.mu.RLock()
	_, exists := r.agents[agentID]
	r.mu.RUnlock()

	// Extract config fields
	model, _ := config["model"].(string)
	tempF, _ := config["temperature"].(float64)
	maxTokensF, _ := config["max_tokens"].(float64)
	maxIterF, _ := config["max_iterations"].(float64)

	if exists {
		// Update existing agent's spec (model/temp/tokens can be hot-swapped)
		r.mu.Lock()
		ra := r.agents[agentID]
		if model != "" {
			ra.spec.Model = model
		}
		if tempF > 0 {
			ra.spec.Temperature = tempF
		}
		if maxTokensF > 0 {
			ra.spec.MaxTokens = int(maxTokensF)
		}
		if maxIterF > 0 {
			ra.spec.MaxIterations = int(maxIterF)
		}
		r.mu.Unlock()

		log.Printf("[Registry] 🔄 Updated agent config: %s (model=%s, temp=%.1f, maxTokens=%d)",
			agentID, ra.spec.Model, ra.spec.Temperature, ra.spec.MaxTokens)
		return nil
	}

	// Register new agent
	spec := AgentSpec{
		ID:            agentID,
		Description:   fmt.Sprintf("Dynamic agent: %s", agentID),
		Model:         model,
		Temperature:   tempF,
		MaxTokens:     int(maxTokensF),
		MaxIterations: int(maxIterF),
	}

	return r.Register(spec)
}

// resolveProvider returns per-agent or default provider.
func (r *Registry) resolveProvider(spec AgentSpec) providers.LLMProvider {
	if spec.ProviderConfig != nil && spec.ProviderConfig.APIKey != "" {
		return providers.NewProvider(
			spec.ProviderConfig.APIKey,
			spec.ProviderConfig.APIBase,
			spec.Model,
			spec.ProviderConfig.ProviderName,
		)
	}
	return r.defaultProvider
}

// Get returns the AgentLoop for the given ID, or nil if not found.
func (r *Registry) Get(id string) *agent.AgentLoop {
	r.mu.RLock()
	defer r.mu.RUnlock()
	if ra, ok := r.agents[id]; ok {
		return ra.loop
	}
	return nil
}

// GetDefault returns the default agent.
func (r *Registry) GetDefault() *agent.AgentLoop {
	r.mu.RLock()
	defer r.mu.RUnlock()
	if r.defaultID != "" {
		if ra, ok := r.agents[r.defaultID]; ok {
			return ra.loop
		}
	}
	// Fallback: return first registered
	for _, ra := range r.agents {
		return ra.loop
	}
	return nil
}

// ResolveForRole maps a role_id to an agent_id.
// In the Survival model, role_id == agent_id.
// If the role isn't registered, falls back to default.
func (r *Registry) ResolveForRole(roleID string) *agent.AgentLoop {
	r.mu.RLock()
	defer r.mu.RUnlock()
	if ra, ok := r.agents[roleID]; ok {
		return ra.loop
	}
	// Fallback to default
	if r.defaultID != "" {
		if ra, ok := r.agents[r.defaultID]; ok {
			return ra.loop
		}
	}
	return nil
}

// ProcessDirect routes a message to the appropriate agent based on role.
func (r *Registry) ProcessDirect(ctx context.Context, content, sessionKey, channel, chatID, roleID string) (string, error) {
	agentLoop := r.ResolveForRole(roleID)
	if agentLoop == nil {
		return "", fmt.Errorf("no agent found for role %q", roleID)
	}
	return agentLoop.ProcessDirect(ctx, content, sessionKey, channel, chatID)
}

// ListAgents returns summary info for all registered agents.
func (r *Registry) ListAgents() []map[string]any {
	r.mu.RLock()
	defer r.mu.RUnlock()
	result := make([]map[string]any, 0, len(r.agents))
	for id, ra := range r.agents {
		result = append(result, map[string]any{
			"id":          id,
			"description": ra.spec.Description,
			"model":       ra.spec.Model,
			"is_default":  ra.spec.IsDefault,
			"tools":       ra.spec.Tools,
		})
	}
	return result
}

// GetSpec returns the AgentSpec for the given ID.
func (r *Registry) GetSpec(id string) *AgentSpec {
	r.mu.RLock()
	defer r.mu.RUnlock()
	if ra, ok := r.agents[id]; ok {
		spec := ra.spec
		return &spec
	}
	return nil
}

// Len returns the number of registered agents.
func (r *Registry) Len() int {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return len(r.agents)
}

// AgentIDs returns all registered agent IDs.
func (r *Registry) AgentIDs() []string {
	r.mu.RLock()
	defer r.mu.RUnlock()
	ids := make([]string, 0, len(r.agents))
	for id := range r.agents {
		ids = append(ids, id)
	}
	return ids
}

// Contains checks whether an agent ID is registered.
func (r *Registry) Contains(id string) bool {
	r.mu.RLock()
	defer r.mu.RUnlock()
	_, ok := r.agents[id]
	return ok
}

// GetPrompt returns the loaded system prompt for an agent.
func (r *Registry) GetPrompt(id string) string {
	r.mu.RLock()
	defer r.mu.RUnlock()
	if ra, ok := r.agents[id]; ok {
		return ra.prompt
	}
	return ""
}
