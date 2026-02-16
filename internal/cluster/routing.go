package cluster

// routing.go — keyword routing, user memory injection, and route info formatting.
// Mirrors survival/nanobot/server.py: _route_superdriver, _inject_user_memory,
// _build_route_info, _format_route_header, _check_mention.

import (
	"context"
	"fmt"
	"log"
	"strings"
	"time"

	nanoredis "github.com/dayuer/nanobot-go/internal/redis"
	"github.com/dayuer/nanobot-go/internal/router"
)

// keywordRoutes maps role IDs to keyword lists for fast keyword-based routing.
// Mirrors Python server.py PARTNER_KEYWORDS (超级司机顾问团).
var keywordRoutes = map[string][]string{
	"legal": {"法律", "打官司", "起诉", "律师", "合同", "纠纷", "赔偿", "仲裁",
		"法院", "判决", "诉讼", "维权", "侵权", "违约"},
	"mechanic": {"修车", "维修", "保养", "4S店", "换胎", "发动机", "变速箱",
		"底盘", "刹车", "机油", "零件", "故障灯"},
	"driving": {"驾照", "违章", "扣分", "罚款", "行驶证", "年检", "审车",
		"科目", "路考", "交通规则"},
	"health": {"身体", "健康", "头痛", "腰痛", "失眠", "养生", "锻炼",
		"饮食", "体检", "疲劳", "颈椎"},
	"stockgod": {"股票", "A股", "涨停", "跌停", "基金", "持仓", "K线",
		"均线", "MACD", "量能", "板块", "龙头", "打板"},
	"insurance": {"保险", "理赔", "定损", "赔多少", "报销", "骗保",
		"交强险", "商业险", "三者险", "车损险", "严公估"},
	"food": {"吃饭", "饿了", "美食", "菜", "餐厅", "点餐", "外卖",
		"小吃", "火锅", "烧烤"},
	"rescue": {"拖车", "救援", "抛锚", "没电", "搭电", "轮胎", "爆胎",
		"事故", "碰撞", "翻车"},
}

// routeByKeyword scans message content and returns the role ID with the
// highest keyword match score. Empty string means no match.
func routeByKeyword(content string) (roleID string, score int) {
	var bestRole string
	var bestScore int

	for role, keywords := range keywordRoutes {
		s := 0
		for _, kw := range keywords {
			if strings.Contains(content, kw) {
				s++
			}
		}
		if s > bestScore {
			bestScore = s
			bestRole = role
		}
	}
	return bestRole, bestScore
}

// checkMention checks if the user @mentioned a specific role.
// Supports @翔哥 → general, @legal → legal, etc.
func checkMention(content string, mentionMap map[string]string) string {
	for mention, roleID := range mentionMap {
		if strings.Contains(content, "@"+mention) {
			return roleID
		}
	}
	return ""
}

// resolveRoute determines which agent should handle the message.
// Priority: explicit roleID → @mention → keyword → LLM router → general
func (s *Server) resolveRoute(ctx context.Context, content, roleID string) (resolved string, routeMethod string, routeResult *router.RouteResult) {
	// 1. Explicit role from request
	if roleID != "" && roleID != "general" {
		return roleID, "explicit", nil
	}

	// 2. @mention check
	if s.mentionMap != nil {
		if mentioned := checkMention(content, s.mentionMap); mentioned != "" {
			return mentioned, "mention", nil
		}
	}

	// 3. Keyword routing
	kwRole, kwScore := routeByKeyword(content)
	if kwRole != "" && kwScore >= 2 {
		return kwRole, "keyword", nil
	}

	// 4. LLM semantic routing (if router available)
	if s.router != nil {
		result := s.router.RouteMulti(ctx, content)
		if result.Primary != "" && result.Primary != "general" {
			return result.Primary, "llm", &result
		}
		// Even if primary is general, return the result (may have related)
		if len(result.Related) > 0 {
			return "general", "llm", &result
		}
	}

	// 5. Keyword with lower threshold (score >= 1)
	if kwRole != "" && kwScore >= 1 {
		return kwRole, "keyword", nil
	}

	return "general", "default", nil
}

// injectUserMemory reads user's personal memory from Redis and prepends
// it to the message content. Mirrors Python _inject_user_memory.
func injectUserMemory(ctx context.Context, personID, content string) string {
	if personID == "" || !nanoredis.IsAvailable() {
		return content
	}

	key := nanoredis.MemoryKey(personID)
	memory := nanoredis.CacheGet(ctx, key)
	if memory == "" {
		return content
	}

	// Prepend memory context to the message
	return fmt.Sprintf("[当前用户的个人记忆]\n%s\n\n---\n\n%s", memory, content)
}

// RouteInfo describes the routing decision for API responses.
type RouteInfo struct {
	AgentID     string   `json:"agentId"`
	AgentName   string   `json:"agentName,omitempty"`
	Description string   `json:"description,omitempty"`
	Method      string   `json:"method"`      // "keyword" | "llm" | "mention" | "explicit" | "default"
	Reason      string   `json:"reason,omitempty"`
	Related     []string `json:"related,omitempty"`
	Domains     []string `json:"domains,omitempty"`
	Summary     string   `json:"summary,omitempty"`
}

// buildRouteInfo constructs the route info for API responses.
func (s *Server) buildRouteInfo(roleID, method string, result *router.RouteResult) RouteInfo {
	info := RouteInfo{
		AgentID: roleID,
		Method:  method,
	}

	// Get agent description from registry
	if s.registry != nil {
		if spec := s.registry.GetSpec(roleID); spec != nil {
			info.Description = spec.Description
			info.AgentName = extractAgentName(roleID, spec.Description)
		}
	}

	if result != nil {
		info.Related = result.Related
		info.Domains = result.Domains
		info.Reason = result.Reason
	}

	// Build summary
	switch method {
	case "keyword":
		info.Summary = fmt.Sprintf("关键词匹配 → %s", info.AgentName)
	case "llm":
		info.Summary = fmt.Sprintf("AI 语义分析 → %s", info.AgentName)
		if info.Reason != "" {
			info.Summary += " | " + info.Reason
		}
	case "mention":
		info.Summary = fmt.Sprintf("@提及 → %s", info.AgentName)
	default:
		info.Summary = fmt.Sprintf("默认 → %s", info.AgentName)
	}

	return info
}

// formatRouteHeader formats the route info as a user-visible text block.
// Mirrors Python _format_route_header.
func (s *Server) formatRouteHeader(info RouteInfo) string {
	if info.AgentName == "" {
		info.AgentName = info.AgentID
	}

	var b strings.Builder
	b.WriteString(fmt.Sprintf("> 🎭 **%s** 为您服务", info.AgentName))
	if info.Description != "" {
		b.WriteString(" | " + info.Description)
	}
	b.WriteString("\n")
	b.WriteString(fmt.Sprintf("> 📍 路由: %s\n", info.Summary))

	// Show related agents for multi-domain routing
	if len(info.Related) > 0 {
		var names []string
		for _, rid := range info.Related {
			name := rid
			if s.registry != nil {
				if spec := s.registry.GetSpec(rid); spec != nil {
					name = extractAgentName(rid, spec.Description)
				}
			}
			names = append(names, "**"+name+"**")
		}
		b.WriteString(fmt.Sprintf("> 🔗 相关专家: %s\n", strings.Join(names, " · ")))
	}

	return b.String()
}

// extractAgentName extracts a short name from description.
// e.g. "叶律 — 法律纠纷处理专家" → "叶律"
func extractAgentName(roleID, desc string) string {
	if desc == "" {
		return roleID
	}
	// Try splitting by common separators
	for _, sep := range []string{" — ", " - ", "——", "：", ":"} {
		parts := strings.SplitN(desc, sep, 2)
		if len(parts) == 2 && len(parts[0]) <= 20 {
			return strings.TrimSpace(parts[0])
		}
	}
	// Use first few characters
	if len(desc) > 10 {
		return desc[:10]
	}
	return desc
}

// stripThinking removes leaked internal reasoning from LLM responses.
// Mirrors Python _strip_thinking.
func stripThinking(text string) string {
	// Remove common thinking patterns
	patterns := []string{
		"**Reflection**", "**Next Steps**", "**Analysis**",
		"**思考过程**", "**推理过程**", "**内部分析**",
	}

	for _, p := range patterns {
		idx := strings.Index(text, p)
		if idx >= 0 {
			// Find the section end (next double newline or end)
			rest := text[idx:]
			end := strings.Index(rest, "\n\n")
			if end >= 0 {
				text = text[:idx] + text[idx+end+2:]
			} else {
				text = text[:idx]
			}
		}
	}

	return strings.TrimSpace(text)
}

// saveUserMemoryUpdate stores updated user memory to Redis.
func saveUserMemoryUpdate(ctx context.Context, personID, memory string) {
	if personID == "" || memory == "" || !nanoredis.IsAvailable() {
		return
	}
	key := nanoredis.MemoryKey(personID)
	nanoredis.CacheSet(ctx, key, memory, 24*time.Hour)
	log.Printf("[Memory] Saved memory for %s (%d chars)", personID, len(memory))
}
