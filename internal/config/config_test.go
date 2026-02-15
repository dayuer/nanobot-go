package config

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// --- Schema Tests ---

func TestDefaultConfig(t *testing.T) {
	cfg := DefaultConfig()
	assert.Equal(t, "anthropic/claude-sonnet-4-5", cfg.Agent.Model)
	assert.Equal(t, 4096, cfg.Agent.MaxTokens)
	assert.Equal(t, 0.7, cfg.Agent.Temperature)
	assert.Equal(t, 25, cfg.Agent.MaxIterations)
	assert.Equal(t, 18790, cfg.Gateway.Port)
	assert.True(t, cfg.Tools.RestrictToWorkspace)
}

func TestConfig_JSON_RoundTrip(t *testing.T) {
	original := Config{
		Channel: ChannelConfig{
			Telegram: &TelegramConfig{Token: "tok123", AllowFrom: []string{"u1"}},
		},
		Agent: AgentConfig{
			Model:       "openai/gpt-4",
			MaxTokens:   8192,
			Temperature: 0.5,
		},
	}

	data, err := json.Marshal(original)
	require.NoError(t, err)

	var decoded Config
	err = json.Unmarshal(data, &decoded)
	require.NoError(t, err)

	assert.Equal(t, "tok123", decoded.Channel.Telegram.Token)
	assert.Equal(t, []string{"u1"}, decoded.Channel.Telegram.AllowFrom)
	assert.Equal(t, "openai/gpt-4", decoded.Agent.Model)
	assert.Equal(t, 8192, decoded.Agent.MaxTokens)
}

func TestConfig_CamelCaseJSON(t *testing.T) {
	jsonStr := `{
		"channel": {
			"telegram": {"token": "abc", "allowFrom": ["user1"]},
			"feishu": {"appId": "f1", "appSecret": "s1"}
		},
		"agent": {"maxTokens": 2048, "maxIterations": 10},
		"tools": {"restrictToWorkspace": false},
		"gateway": {"port": 9090}
	}`

	var cfg Config
	err := json.Unmarshal([]byte(jsonStr), &cfg)
	require.NoError(t, err)

	assert.Equal(t, "abc", cfg.Channel.Telegram.Token)
	assert.Equal(t, []string{"user1"}, cfg.Channel.Telegram.AllowFrom)
	assert.Equal(t, "f1", cfg.Channel.Feishu.AppID)
	assert.Equal(t, 2048, cfg.Agent.MaxTokens)
	assert.Equal(t, 10, cfg.Agent.MaxIterations)
	assert.False(t, cfg.Tools.RestrictToWorkspace)
	assert.Equal(t, 9090, cfg.Gateway.Port)
}

func TestConfig_NilChannels(t *testing.T) {
	cfg := DefaultConfig()
	assert.Nil(t, cfg.Channel.Telegram)
	assert.Nil(t, cfg.Channel.Discord)
	assert.Nil(t, cfg.Channel.Feishu)
}

func TestConfig_MCPServers(t *testing.T) {
	jsonStr := `{
		"agent": {
			"mcpServers": [
				{"name": "fs", "command": "npx", "args": ["-y", "@modelcontextprotocol/server-filesystem"]},
				{"name": "remote", "url": "http://localhost:3000/mcp"}
			]
		}
	}`
	var cfg Config
	err := json.Unmarshal([]byte(jsonStr), &cfg)
	require.NoError(t, err)
	assert.Len(t, cfg.Agent.MCPServers, 2)
	assert.Equal(t, "fs", cfg.Agent.MCPServers[0].Name)
	assert.Equal(t, "npx", cfg.Agent.MCPServers[0].Command)
	assert.Equal(t, "http://localhost:3000/mcp", cfg.Agent.MCPServers[1].URL)
}

// --- Loader Tests ---

func TestLoad_FileNotExist(t *testing.T) {
	cfg, err := Load(filepath.Join(t.TempDir(), "nonexistent.json"))
	require.NoError(t, err)
	assert.Equal(t, DefaultConfig(), cfg)
}

func TestLoad_ValidFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.json")
	content := `{"agent": {"model": "deepseek/deepseek-chat", "maxTokens": 1024}}`
	err := os.WriteFile(path, []byte(content), 0644)
	require.NoError(t, err)

	cfg, err := Load(path)
	require.NoError(t, err)
	assert.Equal(t, "deepseek/deepseek-chat", cfg.Agent.Model)
	assert.Equal(t, 1024, cfg.Agent.MaxTokens)
	// Defaults should be preserved for unset fields
	assert.Equal(t, 0.7, cfg.Agent.Temperature)
}

func TestLoad_InvalidJSON(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "bad.json")
	err := os.WriteFile(path, []byte("{invalid json}"), 0644)
	require.NoError(t, err)

	cfg, err := Load(path)
	assert.Error(t, err)
	// Should return defaults on error
	assert.Equal(t, DefaultConfig(), cfg)
}

func TestSave_And_Load_RoundTrip(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "sub", "config.json")

	cfg := DefaultConfig()
	cfg.Channel.Telegram = &TelegramConfig{Token: "test-token"}
	cfg.Agent.Model = "openai/gpt-4o"

	err := Save(cfg, path)
	require.NoError(t, err)

	loaded, err := Load(path)
	require.NoError(t, err)
	assert.Equal(t, "test-token", loaded.Channel.Telegram.Token)
	assert.Equal(t, "openai/gpt-4o", loaded.Agent.Model)
}

func TestSave_CreatesParentDirs(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "a", "b", "config.json")

	err := Save(DefaultConfig(), path)
	require.NoError(t, err)

	_, err = os.Stat(path)
	assert.NoError(t, err)
}
