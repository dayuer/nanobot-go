// Package events provides a business event engine that matches YAML rules
// against incoming events and dispatches them to agents.
// Mirrors survival/nanobot/event_engine.py.
package events

import (
	"context"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"

	"gopkg.in/yaml.v3"
)

// Rule defines a single event matching rule (from YAML).
type Rule struct {
	EventType   string         `yaml:"event_type"`
	AgentID     string         `yaml:"agent_id"`
	Template    string         `yaml:"template"`
	Channel     string         `yaml:"channel"`
	TargetField string         `yaml:"target_field"`
	Conditions  map[string]any `yaml:"conditions"`
	Enabled     *bool          `yaml:"enabled,omitempty"`
	Priority    int            `yaml:"priority"`
	sourceFile  string
}

// IsEnabled returns whether the rule is enabled (default true).
func (r Rule) IsEnabled() bool {
	if r.Enabled == nil {
		return true
	}
	return *r.Enabled
}

// DispatchResult holds the result of dispatching an event.
type DispatchResult struct {
	RuleType string `json:"ruleType"`
	AgentID  string `json:"agentId"`
	Response string `json:"response,omitempty"`
	Error    string `json:"error,omitempty"`
}

// AgentHandler is a function that processes a message for an agent.
type AgentHandler func(ctx context.Context, content, sessionKey, channel, chatID, roleID string) (string, error)

// Engine processes business events using YAML rules.
type Engine struct {
	mu       sync.RWMutex
	rules    []Rule
	handler  AgentHandler

	// Stats
	totalEvents    int
	totalDispatches int
	totalErrors    int
}

// NewEngine creates a new event engine.
func NewEngine(handler AgentHandler) *Engine {
	return &Engine{
		handler: handler,
	}
}

// LoadRules loads event rules from all YAML files in the given directory.
func (e *Engine) LoadRules(dir string) error {
	e.mu.Lock()
	defer e.mu.Unlock()

	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			log.Printf("[EventEngine] No events directory: %s", dir)
			return nil
		}
		return fmt.Errorf("read events dir: %w", err)
	}

	var rules []Rule
	for _, entry := range entries {
		if entry.IsDir() || (!strings.HasSuffix(entry.Name(), ".yaml") && !strings.HasSuffix(entry.Name(), ".yml")) {
			continue
		}

		path := filepath.Join(dir, entry.Name())
		data, err := os.ReadFile(path)
		if err != nil {
			log.Printf("[EventEngine] ⚠️ Failed to read %s: %v", path, err)
			continue
		}

		var fileRules []Rule
		if err := yaml.Unmarshal(data, &fileRules); err != nil {
			log.Printf("[EventEngine] ⚠️ Failed to parse %s: %v", path, err)
			continue
		}

		for i := range fileRules {
			fileRules[i].sourceFile = entry.Name()
		}
		rules = append(rules, fileRules...)
	}

	e.rules = rules
	log.Printf("[EventEngine] ✅ Loaded %d rules from %s", len(rules), dir)
	return nil
}

// Ingest processes a business event, matching all rules and dispatching.
func (e *Engine) Ingest(ctx context.Context, event map[string]any) []DispatchResult {
	e.mu.RLock()
	defer e.mu.RUnlock()

	e.totalEvents++

	eventType, _ := event["type"].(string)
	if eventType == "" {
		return []DispatchResult{{Error: "event missing 'type' field"}}
	}

	// Find matching rules (sorted by priority, highest first)
	var matched []Rule
	for _, rule := range e.rules {
		if !rule.IsEnabled() {
			continue
		}
		if matchType(rule.EventType, eventType) && e.matchConditions(rule, event) {
			matched = append(matched, rule)
		}
	}

	if len(matched) == 0 {
		log.Printf("[EventEngine] No rules matched for event type: %s", eventType)
		return nil
	}

	// Sort by priority (highest first) — simple insertion sort
	for i := 1; i < len(matched); i++ {
		for j := i; j > 0 && matched[j].Priority > matched[j-1].Priority; j-- {
			matched[j], matched[j-1] = matched[j-1], matched[j]
		}
	}

	// Dispatch to each matching rule
	var results []DispatchResult
	for _, rule := range matched {
		result := e.dispatch(ctx, rule, event)
		results = append(results, result)
	}

	return results
}

// dispatch sends the rendered message to the agent.
func (e *Engine) dispatch(ctx context.Context, rule Rule, event map[string]any) DispatchResult {
	// Render template
	content := RenderTemplate(rule.Template, event)

	// Resolve target
	target := ""
	if rule.TargetField != "" {
		if v, ok := event[rule.TargetField]; ok {
			target = fmt.Sprintf("%v", v)
		}
	}

	sessionKey := fmt.Sprintf("event:%s:%s", rule.EventType, target)
	channel := rule.Channel
	if channel == "" {
		channel = "none"
	}

	e.totalDispatches++

	resp, err := e.handler(ctx, content, sessionKey, channel, "", rule.AgentID)
	if err != nil {
		e.totalErrors++
		log.Printf("[EventEngine] ❌ Dispatch failed for %s→%s: %v", rule.EventType, rule.AgentID, err)
		return DispatchResult{
			RuleType: rule.EventType,
			AgentID:  rule.AgentID,
			Error:    err.Error(),
		}
	}

	log.Printf("[EventEngine] ✅ %s → %s dispatched (%d chars)", rule.EventType, rule.AgentID, len(resp))
	return DispatchResult{
		RuleType: rule.EventType,
		AgentID:  rule.AgentID,
		Response: resp,
	}
}

// matchType checks if event type matches rule pattern (supports wildcards).
func matchType(pattern, eventType string) bool {
	if pattern == "*" {
		return true
	}
	if strings.HasSuffix(pattern, ".*") {
		prefix := strings.TrimSuffix(pattern, ".*")
		return strings.HasPrefix(eventType, prefix+".")
	}
	return pattern == eventType
}

// matchConditions checks if event satisfies all rule conditions.
func (e *Engine) matchConditions(rule Rule, event map[string]any) bool {
	for key, expected := range rule.Conditions {
		actual, ok := event[key]
		if !ok {
			return false
		}

		switch exp := expected.(type) {
		case float64:
			// Numeric comparison: min_amount, max_amount etc.
			if strings.HasPrefix(key, "min_") {
				if val, ok := toFloat64(actual); !ok || val < exp {
					return false
				}
			} else if strings.HasPrefix(key, "max_") {
				if val, ok := toFloat64(actual); !ok || val > exp {
					return false
				}
			} else {
				if val, ok := toFloat64(actual); !ok || val != exp {
					return false
				}
			}
		case string:
			if fmt.Sprintf("%v", actual) != exp {
				return false
			}
		}
	}
	return true
}

// toFloat64 converts an interface value to float64.
func toFloat64(v any) (float64, bool) {
	switch n := v.(type) {
	case float64:
		return n, true
	case int:
		return float64(n), true
	case int64:
		return float64(n), true
	case float32:
		return float64(n), true
	default:
		return 0, false
	}
}

// RenderTemplate renders a template with event data.
// Supports {key} and nested {user.name} access.
var templatePattern = regexp.MustCompile(`\{([^}]+)\}`)

func RenderTemplate(template string, data map[string]any) string {
	return templatePattern.ReplaceAllStringFunc(template, func(match string) string {
		key := match[1 : len(match)-1] // strip { }
		val := getNestedValue(data, key)
		if val == nil {
			return match // preserve unmatched
		}
		return fmt.Sprintf("%v", val)
	})
}

// getNestedValue accesses nested map values by dot-separated path.
func getNestedValue(data map[string]any, path string) any {
	parts := strings.Split(path, ".")
	current := any(data)

	for _, part := range parts {
		m, ok := current.(map[string]any)
		if !ok {
			return nil
		}
		current, ok = m[part]
		if !ok {
			return nil
		}
	}
	return current
}

// Stats returns engine statistics.
func (e *Engine) Stats() map[string]any {
	e.mu.RLock()
	defer e.mu.RUnlock()

	typeCounts := make(map[string]int)
	for _, rule := range e.rules {
		typeCounts[rule.EventType]++
	}

	return map[string]any{
		"totalRules":      len(e.rules),
		"totalEvents":     e.totalEvents,
		"totalDispatches": e.totalDispatches,
		"totalErrors":     e.totalErrors,
		"rulesByType":     typeCounts,
	}
}

// RuleCount returns the number of loaded rules.
func (e *Engine) RuleCount() int {
	e.mu.RLock()
	defer e.mu.RUnlock()
	return len(e.rules)
}
