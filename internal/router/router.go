// Package router provides LLM-based semantic intent routing.
// It analyzes user messages and routes them to the most appropriate agent.
// Mirrors survival/nanobot/llm_router.py.
package router

import (
	"context"
	"crypto/md5"
	"encoding/json"
	"fmt"
	"log"
	"strings"
	"sync"
	"time"

	"github.com/dayuer/nanobot-go/internal/providers"
)

// RouteResult holds the routing analysis result.
type RouteResult struct {
	Primary  string            `json:"primary"`  // Primary agent ID
	Related  []string          `json:"related"`  // Related agent IDs
	Reason   string            `json:"reason"`   // Routing reason
	Domains  []string          `json:"domains"`  // Involved domains
	SubTasks map[string]string `json:"sub_tasks"` // Focused sub-questions for each related agent
}

// AllAgents returns all involved agents (primary + related, deduplicated).
func (r *RouteResult) AllAgents() []string {
	seen := map[string]bool{r.Primary: true}
	result := []string{r.Primary}
	for _, id := range r.Related {
		if !seen[id] {
			seen[id] = true
			result = append(result, id)
		}
	}
	return result
}

// Role describes an available agent for routing.
type Role struct {
	ID          string `json:"id"`
	Description string `json:"description"`
}

// LLMRouter is the semantic intent router using a lightweight LLM model.
type LLMRouter struct {
	roles       []Role
	validIDs    map[string]bool
	model       string
	provider    providers.LLMProvider
	systemPrompt string

	mu    sync.RWMutex
	cache map[string]cacheEntry
}

type cacheEntry struct {
	result RouteResult
	ts     time.Time
}

const (
	cacheTTL = 60 * time.Second
	cacheMax = 256
)

// routerSystemPrompt is the system prompt template for the router model.
const routerSystemPrompt = `你是一个消息路由器。根据用户消息的语义意图，分析最适合回答的专家，并为相关专家生成聚焦子问题。

## 可用专家角色

%s

## 分析规则
1. 判断用户消息涉及的所有领域
2. 选择最紧迫/最核心的领域作为主角色 (primary)
3. 列出所有相关的角色 (related)，按相关度排序
4. 为每个 related 角色生成一个聚焦子问题 (sub_tasks)，让该专家从自己的视角给出简要建议
5. 用一句极简中文说明路由理由

## 返回格式 (严格 JSON)
{"primary":"角色ID","related":["角色ID1","角色ID2"],"sub_tasks":{"角色ID1":"聚焦子问题1","角色ID2":"聚焦子问题2"},"reason":"一句话","domains":["领域1","领域2"]}

## 注意
- 闲聊、打招呼、不确定的内容: primary 设为 general, related 为空, sub_tasks 为空
- related 只放确实相关的角色，不要凑数
- sub_tasks 的子问题要具体且聚焦该专家领域，100字以内
- 如果只涉及一个领域，related 和 sub_tasks 可以为空`

// NewLLMRouter creates a new semantic intent router.
func NewLLMRouter(roles []Role, model string, provider providers.LLMProvider) *LLMRouter {
	// Build roles block
	var rolesBlock strings.Builder
	for _, r := range roles {
		fmt.Fprintf(&rolesBlock, "- **%s**: %s\n", r.ID, r.Description)
	}

	validIDs := make(map[string]bool, len(roles))
	for _, r := range roles {
		validIDs[r.ID] = true
	}

	return &LLMRouter{
		roles:        roles,
		validIDs:     validIDs,
		model:        model,
		provider:     provider,
		systemPrompt: fmt.Sprintf(routerSystemPrompt, rolesBlock.String()),
		cache:        make(map[string]cacheEntry, cacheMax),
	}
}

// RouteMulti analyzes a user message for multi-domain routing.
func (r *LLMRouter) RouteMulti(ctx context.Context, content string) RouteResult {
	content = strings.TrimSpace(content)
	if content == "" {
		return RouteResult{Primary: "general"}
	}

	// Cache lookup
	cacheKey := contentHash(content)
	r.mu.RLock()
	if entry, ok := r.cache[cacheKey]; ok && time.Since(entry.ts) < cacheTTL {
		r.mu.RUnlock()
		log.Printf("  🧠 LLM Router cache hit: %s", entry.result.Primary)
		return entry.result
	}
	r.mu.RUnlock()

	// Call LLM
	result, err := r.callLLM(ctx, content)
	if err != nil {
		log.Printf("  ⚠️ LLM Router failed: %v", err)
		return RouteResult{Primary: "general"}
	}

	// Validate
	if !r.validIDs[result.Primary] {
		log.Printf("  ⚠️ LLM Router returned invalid primary: '%s', fallback to general", result.Primary)
		result.Primary = "general"
	}
	validRelated := make([]string, 0, len(result.Related))
	for _, id := range result.Related {
		if r.validIDs[id] {
			validRelated = append(validRelated, id)
		}
	}
	result.Related = validRelated

	// Write cache
	r.mu.Lock()
	if len(r.cache) >= cacheMax {
		// Evict oldest
		var oldestKey string
		var oldestTime time.Time
		for k, v := range r.cache {
			if oldestKey == "" || v.ts.Before(oldestTime) {
				oldestKey = k
				oldestTime = v.ts
			}
		}
		delete(r.cache, oldestKey)
	}
	r.cache[cacheKey] = cacheEntry{result: result, ts: time.Now()}
	r.mu.Unlock()

	log.Printf("  🧠 LLM Router: primary=%s, related=%v, reason=%s",
		result.Primary, result.Related, result.Reason)
	return result
}

// callLLM calls the router-level model for multi-domain classification.
func (r *LLMRouter) callLLM(ctx context.Context, content string) (RouteResult, error) {
	msgs := []providers.Message{
		{Role: "system", Content: r.systemPrompt},
		{Role: "user", Content: content},
	}

	resp, err := r.provider.Chat(ctx, providers.ChatRequest{
		Messages:    msgs,
		Model:       r.model,
		MaxTokens:   300,
		Temperature: 0.1,
	})
	if err != nil {
		return RouteResult{Primary: "general"}, err
	}

	raw := ""
	if resp.Content != nil {
		raw = strings.TrimSpace(*resp.Content)
	}
	if raw == "" {
		return RouteResult{Primary: "general"}, fmt.Errorf("empty response")
	}

	// Clean markdown code blocks
	if strings.HasPrefix(raw, "```") {
		lines := strings.SplitN(raw, "\n", 2)
		if len(lines) > 1 {
			raw = lines[1]
		}
		if idx := strings.LastIndex(raw, "```"); idx >= 0 {
			raw = strings.TrimSpace(raw[:idx])
		}
	}

	var result RouteResult
	if err := json.Unmarshal([]byte(raw), &result); err != nil {
		// Fallback: try as plain text ID
		roleID := strings.ToLower(strings.TrimSpace(raw))
		if r.validIDs[roleID] {
			return RouteResult{Primary: roleID, Reason: "single-id fallback"}, nil
		}
		log.Printf("  ⚠️ LLM Router parse failed: %.100s", raw)
		return RouteResult{Primary: "general"}, nil
	}

	return result, nil
}

// contentHash returns a short hash of the content for caching.
func contentHash(content string) string {
	text := strings.ToLower(strings.TrimSpace(content))
	if len(text) > 200 {
		text = text[:200]
	}
	h := md5.Sum([]byte(text))
	return fmt.Sprintf("%x", h[:6])
}
