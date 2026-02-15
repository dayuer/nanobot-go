package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"regexp"
	"strings"
	"time"
)

const userAgent = "Mozilla/5.0 (Macintosh; Intel Mac OS X 14_7_2) AppleWebKit/537.36"

// WebSearchTool searches the web using Brave Search API.
type WebSearchTool struct {
	APIKey     string
	MaxResults int
}

func (t *WebSearchTool) Name() string        { return "web_search" }
func (t *WebSearchTool) Description() string  { return "Search the web. Returns titles, URLs, and snippets." }
func (t *WebSearchTool) Parameters() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"query": map[string]any{"type": "string", "description": "Search query"},
			"count": map[string]any{"type": "integer", "description": "Results (1-10)"},
		},
		"required": []string{"query"},
	}
}

func (t *WebSearchTool) Execute(ctx context.Context, args map[string]any) (string, error) {
	apiKey := t.APIKey
	if apiKey == "" {
		apiKey = os.Getenv("BRAVE_API_KEY")
	}
	if apiKey == "" {
		return "Error: BRAVE_API_KEY not configured", nil
	}

	query, _ := args["query"].(string)
	count := t.MaxResults
	if count == 0 {
		count = 5
	}
	if c, ok := args["count"].(float64); ok && c >= 1 && c <= 10 {
		count = int(c)
	}

	req, _ := http.NewRequestWithContext(ctx, "GET", "https://api.search.brave.com/res/v1/web/search", nil)
	q := req.URL.Query()
	q.Set("q", query)
	q.Set("count", fmt.Sprintf("%d", count))
	req.URL.RawQuery = q.Encode()
	req.Header.Set("Accept", "application/json")
	req.Header.Set("X-Subscription-Token", apiKey)

	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Sprintf("Error: %v", err), nil
	}
	defer resp.Body.Close()

	var data struct {
		Web struct {
			Results []struct {
				Title       string `json:"title"`
				URL         string `json:"url"`
				Description string `json:"description"`
			} `json:"results"`
		} `json:"web"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&data); err != nil {
		return fmt.Sprintf("Error: %v", err), nil
	}

	if len(data.Web.Results) == 0 {
		return fmt.Sprintf("No results for: %s", query), nil
	}

	lines := []string{fmt.Sprintf("Results for: %s\n", query)}
	for i, item := range data.Web.Results {
		if i >= count {
			break
		}
		lines = append(lines, fmt.Sprintf("%d. %s\n   %s", i+1, item.Title, item.URL))
		if item.Description != "" {
			lines = append(lines, "   "+item.Description)
		}
	}
	return strings.Join(lines, "\n"), nil
}

// WebFetchTool fetches and extracts content from a URL.
type WebFetchTool struct {
	MaxChars int
}

func (t *WebFetchTool) Name() string        { return "web_fetch" }
func (t *WebFetchTool) Description() string  { return "Fetch URL and extract readable content." }
func (t *WebFetchTool) Parameters() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"url":         map[string]any{"type": "string", "description": "URL to fetch"},
			"extractMode": map[string]any{"type": "string", "enum": []string{"markdown", "text"}},
			"maxChars":    map[string]any{"type": "integer", "minimum": 100},
		},
		"required": []string{"url"},
	}
}

func (t *WebFetchTool) Execute(ctx context.Context, args map[string]any) (string, error) {
	rawURL, _ := args["url"].(string)

	if valid, msg := validateURL(rawURL); !valid {
		result, _ := json.Marshal(map[string]string{"error": "URL validation failed: " + msg, "url": rawURL})
		return string(result), nil
	}

	maxChars := t.MaxChars
	if maxChars == 0 {
		maxChars = 50000
	}
	if mc, ok := args["maxChars"].(float64); ok && mc >= 100 {
		maxChars = int(mc)
	}

	client := &http.Client{
		Timeout: 30 * time.Second,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			if len(via) >= 5 {
				return fmt.Errorf("too many redirects")
			}
			return nil
		},
	}

	req, _ := http.NewRequestWithContext(ctx, "GET", rawURL, nil)
	req.Header.Set("User-Agent", userAgent)

	resp, err := client.Do(req)
	if err != nil {
		result, _ := json.Marshal(map[string]string{"error": err.Error(), "url": rawURL})
		return string(result), nil
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(io.LimitReader(resp.Body, int64(maxChars*2)))
	if err != nil {
		result, _ := json.Marshal(map[string]string{"error": err.Error(), "url": rawURL})
		return string(result), nil
	}

	text := stripTags(string(body))
	text = normalizeWhitespace(text)

	truncated := len(text) > maxChars
	if truncated {
		text = text[:maxChars]
	}

	result, _ := json.Marshal(map[string]any{
		"url":       rawURL,
		"finalUrl":  resp.Request.URL.String(),
		"status":    resp.StatusCode,
		"truncated": truncated,
		"length":    len(text),
		"text":      text,
	})
	return string(result), nil
}

func validateURL(rawURL string) (bool, string) {
	u, err := url.Parse(rawURL)
	if err != nil {
		return false, err.Error()
	}
	if u.Scheme != "http" && u.Scheme != "https" {
		return false, fmt.Sprintf("Only http/https allowed, got '%s'", u.Scheme)
	}
	if u.Host == "" {
		return false, "Missing domain"
	}
	return true, ""
}

var (
	reScript = regexp.MustCompile(`(?is)<script[\s\S]*?</script>`)
	reStyle  = regexp.MustCompile(`(?is)<style[\s\S]*?</style>`)
	reTag    = regexp.MustCompile(`<[^>]+>`)
	reSpaces = regexp.MustCompile(`[ \t]+`)
	reNL     = regexp.MustCompile(`\n{3,}`)
)

func stripTags(text string) string {
	text = reScript.ReplaceAllString(text, "")
	text = reStyle.ReplaceAllString(text, "")
	text = reTag.ReplaceAllString(text, "")
	return strings.TrimSpace(text)
}

func normalizeWhitespace(text string) string {
	text = reSpaces.ReplaceAllString(text, " ")
	return strings.TrimSpace(reNL.ReplaceAllString(text, "\n\n"))
}
