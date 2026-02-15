package tools

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestWebSearchTool_Contract(t *testing.T) {
	RunToolContractTests(t, &WebSearchTool{})
}

func TestWebFetchTool_Contract(t *testing.T) {
	RunToolContractTests(t, &WebFetchTool{})
}

func TestValidateURL_Valid(t *testing.T) {
	ok, _ := validateURL("https://example.com")
	assert.True(t, ok)
}

func TestValidateURL_InvalidScheme(t *testing.T) {
	ok, msg := validateURL("ftp://example.com")
	assert.False(t, ok)
	assert.Contains(t, msg, "Only http/https")
}

func TestValidateURL_NoHost(t *testing.T) {
	ok, msg := validateURL("https://")
	assert.False(t, ok)
	assert.Contains(t, msg, "Missing domain")
}

func TestStripTags(t *testing.T) {
	input := `<html><head><script>alert(1)</script><style>body{}</style></head><body><h1>Hello</h1><p>World</p></body></html>`
	result := stripTags(input)
	assert.NotContains(t, result, "<")
	assert.Contains(t, result, "Hello")
	assert.Contains(t, result, "World")
	assert.NotContains(t, result, "alert")
	assert.NotContains(t, result, "body{}")
}

func TestNormalizeWhitespace(t *testing.T) {
	input := "  hello   world\n\n\n\n\nfoo  "
	result := normalizeWhitespace(input)
	assert.Equal(t, "hello world\n\nfoo", result)
}

func TestWebFetchTool_InvalidURL(t *testing.T) {
	tool := &WebFetchTool{}
	result, err := tool.Execute(context.Background(), map[string]any{"url": "ftp://bad"})
	assert.NoError(t, err)
	assert.Contains(t, result, "URL validation failed")
}

func TestWebSearchTool_NoAPIKey(t *testing.T) {
	tool := &WebSearchTool{}
	t.Setenv("BRAVE_API_KEY", "")
	result, err := tool.Execute(context.Background(), map[string]any{"query": "test"})
	assert.NoError(t, err)
	assert.Contains(t, result, "BRAVE_API_KEY not configured")
}
