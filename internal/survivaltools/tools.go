// Package survivaltools provides Survival-specific tools that bridge
// the Go nanobot with the Survival Backend API.
// Mirrors survival/nanobot/tools/survival_*.py
package survivaltools

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	nanoredis "github.com/dayuer/nanobot-go/internal/redis"
)

// backendClient is a shared HTTP client for Survival Backend API calls.
type backendClient struct {
	baseURL    string
	apiKey     string
	httpClient *http.Client
}

func newBackendClient(baseURL, apiKey string) *backendClient {
	return &backendClient{
		baseURL:    strings.TrimRight(baseURL, "/"),
		apiKey:     apiKey,
		httpClient: &http.Client{Timeout: 30 * time.Second},
	}
}

func (c *backendClient) get(ctx context.Context, path string, params map[string]string) ([]byte, error) {
	u, _ := url.Parse(c.baseURL + path)
	q := u.Query()
	for k, v := range params {
		q.Set(k, v)
	}
	u.RawQuery = q.Encode()

	req, err := http.NewRequestWithContext(ctx, "GET", u.String(), nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+c.apiKey)
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode >= 400 {
		return nil, fmt.Errorf("HTTP %d: %s", resp.StatusCode, string(body[:min(len(body), 500)]))
	}
	return body, nil
}

func (c *backendClient) post(ctx context.Context, path string, data any) ([]byte, error) {
	jsonData, _ := json.Marshal(data)
	req, err := http.NewRequestWithContext(ctx, "POST", c.baseURL+path, bytes.NewReader(jsonData))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+c.apiKey)
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode >= 400 {
		return nil, fmt.Errorf("HTTP %d: %s", resp.StatusCode, string(body[:min(len(body), 500)]))
	}
	return body, nil
}

func (c *backendClient) put(ctx context.Context, path string, data any) ([]byte, error) {
	jsonData, _ := json.Marshal(data)
	req, err := http.NewRequestWithContext(ctx, "PUT", c.baseURL+path, bytes.NewReader(jsonData))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+c.apiKey)
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode >= 400 {
		return nil, fmt.Errorf("HTTP %d: %s", resp.StatusCode, string(body[:min(len(body), 500)]))
	}
	return body, nil
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func errJSON(msg string) string {
	b, _ := json.Marshal(map[string]string{"error": msg})
	return string(b)
}

// ─────────────────────────────────────────────────────────────
// SurvivalDataTool — 查询运营数据和生成报表
// ─────────────────────────────────────────────────────────────

type DataTool struct {
	client *backendClient
}

func NewDataTool(baseURL, apiKey string) *DataTool {
	return &DataTool{client: newBackendClient(baseURL, apiKey)}
}

func (t *DataTool) Name() string        { return "survival_data" }
func (t *DataTool) Description() string {
	return "查询 Survival 运营数据。支持查询：今日收入、订单统计、用户增长、工具使用率、月度报表、环比数据等。"
}
func (t *DataTool) Parameters() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"query": map[string]any{
				"type":        "string",
				"description": "数据查询类型",
				"enum":        []string{"dashboard", "daily_summary", "weekly_summary", "monthly_summary", "user_growth", "tool_usage", "revenue", "order_stats"},
			},
			"dateRange": map[string]any{
				"type":        "object",
				"description": "时间范围 (可选)，格式 {from: 'YYYY-MM-DD', to: 'YYYY-MM-DD'}",
				"properties": map[string]any{
					"from": map[string]any{"type": "string"},
					"to":   map[string]any{"type": "string"},
				},
			},
		},
		"required": []string{"query"},
	}
}
func (t *DataTool) Execute(ctx context.Context, args map[string]any) (string, error) {
	query, _ := args["query"].(string)
	if query == "" {
		return errJSON("missing 'query' parameter"), nil
	}

	params := map[string]string{"type": query}
	if dr, ok := args["dateRange"].(map[string]any); ok {
		if from, ok := dr["from"].(string); ok {
			params["from"] = from
		}
		if to, ok := dr["to"].(string); ok {
			params["to"] = to
		}
	}

	data, err := t.client.get(ctx, "/api/stats/query", params)
	if err != nil {
		return errJSON(err.Error()), nil
	}
	return string(data), nil
}

// ─────────────────────────────────────────────────────────────
// SurvivalStockTool — 股票数据 & 量化分析 (14 endpoints)
// ─────────────────────────────────────────────────────────────

type StockTool struct {
	client   *backendClient
	personID string // set per request
}

func NewStockTool(baseURL, apiKey string) *StockTool {
	return &StockTool{client: newBackendClient(baseURL, apiKey)}
}

func (t *StockTool) SetPersonID(id string) { t.personID = id }
func (t *StockTool) Name() string          { return "survival_stock" }
func (t *StockTool) Description() string {
	return "桥接 Survival Backend 的股票/量化分析 API。支持14个端点：list_stocks (股票列表), get_ticks (分笔数据), get_snapshots (实时快照), get_predictions (AI预测), get_stats (统计指标), get_sentiment (市场情绪), get_risk (风险评估), get_performance (收益分析), get_portfolios (组合管理), get_positions (持仓查询), get_strategies (策略列表), generate_prediction (生成预测), get_backtest (回测结果), run_backtest (执行回测)。"
}
func (t *StockTool) Parameters() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"action": map[string]any{
				"type":        "string",
				"description": "操作类型",
				"enum":        []string{"list_stocks", "get_ticks", "get_snapshots", "get_predictions", "get_stats", "get_sentiment", "get_risk", "get_performance", "get_portfolios", "get_positions", "get_strategies", "generate_prediction", "get_backtest", "run_backtest"},
			},
			"symbol":    map[string]any{"type": "string", "description": "股票代码 (如: 000001.SZ)"},
			"period":    map[string]any{"type": "string", "description": "时间周期 (1min/5min/15min/30min/60min/daily)"},
			"limit":     map[string]any{"type": "integer", "description": "返回条数，默认 50"},
			"portfolioId": map[string]any{"type": "string", "description": "组合 ID"},
		},
		"required": []string{"action"},
	}
}
func (t *StockTool) Execute(ctx context.Context, args map[string]any) (string, error) {
	action, _ := args["action"].(string)
	symbol, _ := args["symbol"].(string)
	period, _ := args["period"].(string)
	limitF, _ := args["limit"].(float64)
	limit := int(limitF)
	if limit == 0 {
		limit = 50
	}
	portfolioID, _ := args["portfolioId"].(string)

	params := map[string]string{}
	if symbol != "" {
		params["symbol"] = symbol
	}
	if period != "" {
		params["period"] = period
	}
	params["limit"] = fmt.Sprintf("%d", limit)

	var path string
	var method string = "GET"
	var postData any

	switch action {
	case "list_stocks":
		path = "/api/stock/masters"
	case "get_ticks":
		path = "/api/stock/ticks"
	case "get_snapshots":
		path = "/api/stock/snapshots"
	case "get_predictions":
		path = "/api/stock/predictions"
	case "get_stats":
		path = "/api/stock/stats"
	case "get_sentiment":
		path = "/api/stock/sentiment"
	case "get_risk":
		path = "/api/stock/risk"
	case "get_performance":
		path = "/api/stock/performance"
	case "get_portfolios":
		path = "/api/stock/portfolio"
	case "get_positions":
		path = "/api/stock/positions"
		if portfolioID != "" {
			params["portfolioId"] = portfolioID
		}
	case "get_strategies":
		path = "/api/stock/strategies"
	case "generate_prediction":
		method = "POST"
		path = "/api/stock/predictions/generate"
		postData = map[string]any{"symbol": symbol, "period": period}
	case "get_backtest":
		path = "/api/stock/backtest"
	case "run_backtest":
		method = "POST"
		path = "/api/stock/backtest"
		postData = args
	default:
		return errJSON(fmt.Sprintf("unknown action: %s", action)), nil
	}

	var data []byte
	var err error
	if method == "POST" {
		data, err = t.client.post(ctx, path, postData)
	} else {
		data, err = t.client.get(ctx, path, params)
	}
	if err != nil {
		return errJSON(err.Error()), nil
	}
	return string(data), nil
}

// ─────────────────────────────────────────────────────────────
// SurvivalMemoryTool — 每用户独立记忆管理
// ─────────────────────────────────────────────────────────────

type MemoryTool struct {
	client   *backendClient
	personID string
}

func NewMemoryTool(baseURL, apiKey string) *MemoryTool {
	return &MemoryTool{client: newBackendClient(baseURL, apiKey)}
}

func (t *MemoryTool) SetPersonID(id string) { t.personID = id }
func (t *MemoryTool) Name() string          { return "user_memory" }
func (t *MemoryTool) Description() string {
	return "管理每个用户的独立记忆 (长期偏好 + 每日笔记)。支持操作：read (读取记忆), save (保存/覆盖), append (追加), note (添加每日笔记)。记忆存储三级架构：Redis缓存 → Backend API → 本地文件。"
}
func (t *MemoryTool) Parameters() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"action": map[string]any{
				"type":        "string",
				"description": "操作类型",
				"enum":        []string{"read", "save", "append", "note"},
			},
			"content": map[string]any{
				"type":        "string",
				"description": "记忆内容 (save/append/note 时必填)",
			},
		},
		"required": []string{"action"},
	}
}
func (t *MemoryTool) Execute(ctx context.Context, args map[string]any) (string, error) {
	action, _ := args["action"].(string)
	content, _ := args["content"].(string)
	personID := t.personID

	if personID == "" {
		return errJSON("person ID not available"), nil
	}

	switch action {
	case "read":
		return t.readMemory(ctx, personID)
	case "save":
		if content == "" {
			return errJSON("content is required for save"), nil
		}
		return t.saveMemory(ctx, personID, content)
	case "append":
		if content == "" {
			return errJSON("content is required for append"), nil
		}
		return t.appendMemory(ctx, personID, content)
	case "note":
		if content == "" {
			return errJSON("content is required for note"), nil
		}
		return t.addNote(ctx, personID, content)
	default:
		return errJSON(fmt.Sprintf("unknown action: %s", action)), nil
	}
}

func (t *MemoryTool) readMemory(ctx context.Context, personID string) (string, error) {
	// 1. Try Redis cache
	if nanoredis.IsAvailable() {
		key := nanoredis.MemoryKey(personID)
		if cached := nanoredis.CacheGet(ctx, key); cached != "" {
			return cached, nil
		}
	}
	// 2. Try Backend API
	data, err := t.client.get(ctx, "/api/memory/"+personID, nil)
	if err == nil {
		// Cache for 1 hour
		if nanoredis.IsAvailable() {
			nanoredis.CacheSet(ctx, nanoredis.MemoryKey(personID), string(data), time.Hour)
		}
		return string(data), nil
	}
	return errJSON("no memory found"), nil
}

func (t *MemoryTool) saveMemory(ctx context.Context, personID, content string) (string, error) {
	// Save to Redis
	if nanoredis.IsAvailable() {
		nanoredis.CacheSet(ctx, nanoredis.MemoryKey(personID), content, time.Hour)
	}
	// Persist to Backend
	_, err := t.client.put(ctx, "/api/memory/"+personID, map[string]string{"content": content})
	if err != nil {
		return errJSON("saved to cache only: " + err.Error()), nil
	}
	result, _ := json.Marshal(map[string]any{"status": "saved", "personId": personID, "length": len(content)})
	return string(result), nil
}

func (t *MemoryTool) appendMemory(ctx context.Context, personID, content string) (string, error) {
	existing, _ := t.readMemory(ctx, personID)
	var combined string
	if existing != "" && existing != errJSON("no memory found") {
		combined = existing + "\n\n" + content
	} else {
		combined = content
	}
	return t.saveMemory(ctx, personID, combined)
}

func (t *MemoryTool) addNote(ctx context.Context, personID, content string) (string, error) {
	note := fmt.Sprintf("\n[%s] %s", time.Now().Format("2006-01-02 15:04"), content)
	return t.appendMemory(ctx, personID, note)
}

// ─────────────────────────────────────────────────────────────
// SurvivalNotifyTool — 通过 Survival IM 发送通知
// ─────────────────────────────────────────────────────────────

type NotifyTool struct {
	client *backendClient
}

func NewNotifyTool(baseURL, apiKey string) *NotifyTool {
	return &NotifyTool{client: newBackendClient(baseURL, apiKey)}
}

func (t *NotifyTool) Name() string        { return "survival_notify" }
func (t *NotifyTool) Description() string {
	return "通过 Survival 内置 IM 向用户发送通知消息。用于：工单状态更新、系统提醒、活动通知、AI 主动推送等。"
}
func (t *NotifyTool) Parameters() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"personId": map[string]any{"type": "string", "description": "目标用户的 Person ID"},
			"content":  map[string]any{"type": "string", "description": "消息内容 (支持 Markdown 格式)"},
			"agentId":  map[string]any{"type": "string", "description": "以哪个 AI Agent 身份发送，默认 system"},
		},
		"required": []string{"personId", "content"},
	}
}
func (t *NotifyTool) Execute(ctx context.Context, args map[string]any) (string, error) {
	personID, _ := args["personId"].(string)
	content, _ := args["content"].(string)
	agentID, _ := args["agentId"].(string)
	if agentID == "" {
		agentID = "system"
	}
	if personID == "" || content == "" {
		return errJSON("personId and content are required"), nil
	}

	data, err := t.client.post(ctx, "/api/im/send", map[string]any{
		"personId": personID,
		"agentId":  agentID,
		"content":  content,
	})
	if err != nil {
		return errJSON(err.Error()), nil
	}
	return string(data), nil
}

// ─────────────────────────────────────────────────────────────
// SurvivalToolsBridge — 桥接 Backend 55+ 专业计算工具
// ─────────────────────────────────────────────────────────────

type ToolsBridge struct {
	client   *backendClient
	personID string
}

func NewToolsBridge(baseURL, apiKey string) *ToolsBridge {
	return &ToolsBridge{client: newBackendClient(baseURL, apiKey)}
}

func (t *ToolsBridge) SetPersonID(id string) { t.personID = id }
func (t *ToolsBridge) Name() string          { return "survival_tools" }
func (t *ToolsBridge) Description() string {
	return "桥接 Survival Backend 的全部专业计算工具 (电气/水暖/建筑/诊断等 39 个行业, 55+ 工具)。支持操作：list (查看可用工具), schema (查看工具参数), execute (执行计算)。先 list 查看可用工具，再 schema 了解参数，最后 execute 执行。"
}
func (t *ToolsBridge) Parameters() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"action": map[string]any{
				"type":        "string",
				"description": "操作类型",
				"enum":        []string{"list", "schema", "execute"},
			},
			"toolId":   map[string]any{"type": "string", "description": "工具 ID (schema/execute 时必填)"},
			"category": map[string]any{"type": "string", "description": "工具分类 (list 时可选过滤)"},
			"params": map[string]any{
				"type":        "object",
				"description": "工具执行参数 (execute 时必填)",
			},
		},
		"required": []string{"action"},
	}
}
func (t *ToolsBridge) Execute(ctx context.Context, args map[string]any) (string, error) {
	action, _ := args["action"].(string)
	toolID, _ := args["toolId"].(string)
	category, _ := args["category"].(string)

	switch action {
	case "list":
		params := map[string]string{}
		if category != "" {
			params["category"] = category
		}
		data, err := t.client.get(ctx, "/api/tools", params)
		if err != nil {
			return errJSON(err.Error()), nil
		}
		return string(data), nil

	case "schema":
		if toolID == "" {
			return errJSON("toolId is required for schema"), nil
		}
		data, err := t.client.get(ctx, "/api/tools/"+toolID+"/schema", nil)
		if err != nil {
			return errJSON(err.Error()), nil
		}
		return string(data), nil

	case "execute":
		if toolID == "" {
			return errJSON("toolId is required for execute"), nil
		}
		params, _ := args["params"].(map[string]any)
		if params == nil {
			params = map[string]any{}
		}
		data, err := t.client.post(ctx, "/api/tools/"+toolID+"/execute", map[string]any{
			"params":   params,
			"personId": t.personID,
		})
		if err != nil {
			return errJSON(err.Error()), nil
		}
		return string(data), nil

	default:
		return errJSON(fmt.Sprintf("unknown action: %s", action)), nil
	}
}

// ─────────────────────────────────────────────────────────────
// KnowledgeSearchTool — RAG 知识库检索
// ─────────────────────────────────────────────────────────────

// RAGQuerier is the interface for RAG search.
type RAGQuerier interface {
	Query(ctx context.Context, text string, topK int) ([]SearchResult, error)
}

// SearchResult is imported from rag package (re-defined to avoid circular import).
type SearchResult struct {
	Text     string         `json:"text"`
	Source   string         `json:"source"`
	Distance float64       `json:"distance"`
}

type KnowledgeSearchTool struct {
	rag RAGQuerier
}

func NewKnowledgeSearchTool(rag RAGQuerier) *KnowledgeSearchTool {
	return &KnowledgeSearchTool{rag: rag}
}

func (t *KnowledgeSearchTool) Name() string        { return "knowledge_search" }
func (t *KnowledgeSearchTool) Description() string {
	return "在知识库中进行语义搜索，查找与查询最相关的文档片段。适用于查找 SOP、产品文档、FAQ、历史案例等。返回最相关的文档片段及其来源。"
}
func (t *KnowledgeSearchTool) Parameters() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"query": map[string]any{"type": "string", "description": "搜索查询文本，用自然语言描述你要查找的信息"},
			"top_k": map[string]any{"type": "integer", "description": "返回结果数量，默认 5", "default": 5},
		},
		"required": []string{"query"},
	}
}
func (t *KnowledgeSearchTool) Execute(ctx context.Context, args map[string]any) (string, error) {
	if t.rag == nil {
		return errJSON("知识库未初始化"), nil
	}

	query, _ := args["query"].(string)
	if query == "" {
		return errJSON("query is required"), nil
	}

	topK := 5
	if k, ok := args["top_k"].(float64); ok && k > 0 {
		topK = int(k)
	}

	results, err := t.rag.Query(ctx, query, topK)
	if err != nil {
		return errJSON(fmt.Sprintf("检索失败: %v", err)), nil
	}

	if len(results) == 0 {
		result, _ := json.Marshal(map[string]any{"message": "未找到相关文档", "query": query})
		return string(result), nil
	}

	formatted := make([]map[string]any, len(results))
	for i, r := range results {
		relevance := 1 - r.Distance
		if relevance < 0 {
			relevance = 0
		}
		formatted[i] = map[string]any{
			"content":   r.Text,
			"source":    r.Source,
			"relevance": fmt.Sprintf("%.3f", relevance),
		}
	}

	output, _ := json.Marshal(map[string]any{
		"query":   query,
		"results": formatted,
		"total":   len(results),
	})
	return string(output), nil
}
