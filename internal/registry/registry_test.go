package registry

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadAgentSpecs(t *testing.T) {
	yaml := `agents:
  - id: general
    description: "翔哥 — 十年老司机"
    tools: ["*"]
    is_default: true
    temperature: 0.7
    max_tokens: 1024
    max_iterations: 30

  - id: legal
    description: "叶律 — 法律纠纷"
    system_prompt_file: "team/roles/legal.md"
    tools:
      - knowledge_search
      - web_search
    temperature: 0.4
    max_tokens: 8192
`
	dir := t.TempDir()
	path := filepath.Join(dir, "agents.yaml")
	os.WriteFile(path, []byte(yaml), 0644)

	specs, err := LoadAgentSpecs(path)
	if err != nil {
		t.Fatalf("LoadAgentSpecs() error: %v", err)
	}
	if len(specs) != 2 {
		t.Fatalf("got %d specs, want 2", len(specs))
	}

	// Check general
	g := specs[0]
	if g.ID != "general" {
		t.Errorf("specs[0].ID = %q, want %q", g.ID, "general")
	}
	if !g.IsDefault {
		t.Error("specs[0].IsDefault should be true")
	}
	if g.Temperature != 0.7 {
		t.Errorf("specs[0].Temperature = %f, want 0.7", g.Temperature)
	}

	// Check legal
	l := specs[1]
	if l.ID != "legal" {
		t.Errorf("specs[1].ID = %q, want %q", l.ID, "legal")
	}
	if l.SystemPromptFile != "team/roles/legal.md" {
		t.Errorf("specs[1].SystemPromptFile = %q", l.SystemPromptFile)
	}
	if len(l.Tools) != 2 {
		t.Errorf("specs[1].Tools = %v, want 2 items", l.Tools)
	}
}

func TestLoadAgentSpecs_NotFound(t *testing.T) {
	specs, err := LoadAgentSpecs("/nonexistent/agents.yaml")
	if err != nil {
		t.Errorf("missing file should return nil, got error: %v", err)
	}
	if specs != nil {
		t.Errorf("missing file should return nil specs, got: %v", specs)
	}
}

func TestRegistry_RegisterAndGet(t *testing.T) {
	reg := NewRegistry(RegistryConfig{
		DefaultProvider: &mockProvider{model: "default-model"},
		Bus:             bus_stub(),
		Workspace:       t.TempDir(),
		DefaultModel:    "default-model",
	})

	err := reg.Register(AgentSpec{
		ID:          "general",
		Description: "General agent",
		IsDefault:   true,
		Temperature: 0.7,
	})
	if err != nil {
		t.Fatalf("Register() error: %v", err)
	}
	err = reg.Register(AgentSpec{
		ID:          "legal",
		Description: "Legal agent",
		Temperature: 0.4,
	})
	if err != nil {
		t.Fatalf("Register() error: %v", err)
	}

	if reg.Len() != 2 {
		t.Errorf("Len() = %d, want 2", reg.Len())
	}
	if !reg.Contains("general") {
		t.Error("should contain 'general'")
	}
	if !reg.Contains("legal") {
		t.Error("should contain 'legal'")
	}
	if reg.Contains("nonexistent") {
		t.Error("should not contain 'nonexistent'")
	}
}

func TestRegistry_GetDefault(t *testing.T) {
	reg := NewRegistry(RegistryConfig{
		DefaultProvider: &mockProvider{model: "m"},
		Bus:             bus_stub(),
		Workspace:       t.TempDir(),
		DefaultModel:    "m",
	})

	reg.Register(AgentSpec{ID: "a", IsDefault: false})
	reg.Register(AgentSpec{ID: "b", IsDefault: true})

	def := reg.GetDefault()
	if def == nil {
		t.Fatal("GetDefault() returned nil")
	}
	// The default agent should be "b"
	spec := reg.GetSpec("b")
	if spec == nil || !spec.IsDefault {
		t.Error("expected b to be default")
	}
}

func TestRegistry_ResolveForRole_Fallback(t *testing.T) {
	reg := NewRegistry(RegistryConfig{
		DefaultProvider: &mockProvider{model: "m"},
		Bus:             bus_stub(),
		Workspace:       t.TempDir(),
		DefaultModel:    "m",
	})

	reg.Register(AgentSpec{ID: "general", IsDefault: true})

	// Known role → exact match
	a := reg.ResolveForRole("general")
	if a == nil {
		t.Fatal("ResolveForRole('general') returned nil")
	}

	// Unknown role → fallback to default
	b := reg.ResolveForRole("unknown-role")
	if b == nil {
		t.Fatal("ResolveForRole('unknown-role') should fallback to default")
	}
}

func TestRegistry_ListAgents(t *testing.T) {
	reg := NewRegistry(RegistryConfig{
		DefaultProvider: &mockProvider{model: "m"},
		Bus:             bus_stub(),
		Workspace:       t.TempDir(),
		DefaultModel:    "m",
	})
	reg.Register(AgentSpec{ID: "a", Description: "Agent A"})
	reg.Register(AgentSpec{ID: "b", Description: "Agent B"})

	list := reg.ListAgents()
	if len(list) != 2 {
		t.Errorf("ListAgents() returned %d items, want 2", len(list))
	}
}
