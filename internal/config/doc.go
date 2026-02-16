// Package config handles configuration loading, saving, and schema definition.
package config

// Config is the top-level nanobot configuration.
// Uses json tags in camelCase to match the JSON config file format.
type Config struct {
	Channel   ChannelConfig   `json:"channel"`
	Agent     AgentConfig     `json:"agent"`
	Tools     ToolsConfig     `json:"tools"`
	Gateway   GatewayConfig   `json:"gateway"`
	WebSearch WebSearchConfig `json:"webSearch"`
	Survival  SurvivalConfig  `json:"survival"`
}

// ChannelConfig holds per-channel settings.
type ChannelConfig struct {
	Telegram *TelegramConfig `json:"telegram,omitempty"`
	Discord  *DiscordConfig  `json:"discord,omitempty"`
	Slack    *SlackConfig    `json:"slack,omitempty"`
	WhatsApp *WhatsAppConfig `json:"whatsapp,omitempty"`
	Feishu   *FeishuConfig   `json:"feishu,omitempty"`
	DingTalk *DingTalkConfig `json:"dingtalk,omitempty"`
	Email    *EmailConfig    `json:"email,omitempty"`
	QQ       *QQConfig       `json:"qq,omitempty"`
	Mochat   *MochatConfig   `json:"mochat,omitempty"`
}

// TelegramConfig holds Telegram bot settings.
type TelegramConfig struct {
	Token     string   `json:"token"`
	AllowFrom []string `json:"allowFrom,omitempty"`
}

// DiscordConfig holds Discord bot settings.
type DiscordConfig struct {
	Token     string   `json:"token"`
	AllowFrom []string `json:"allowFrom,omitempty"`
}

// SlackConfig holds Slack bot settings.
type SlackConfig struct {
	BotToken  string   `json:"botToken"`
	AppToken  string   `json:"appToken"`
	AllowFrom []string `json:"allowFrom,omitempty"`
}

// WhatsAppConfig holds WhatsApp settings.
type WhatsAppConfig struct {
	AllowFrom []string `json:"allowFrom,omitempty"`
}

// FeishuConfig holds Feishu/Lark settings.
type FeishuConfig struct {
	AppID     string   `json:"appId"`
	AppSecret string   `json:"appSecret"`
	AllowFrom []string `json:"allowFrom,omitempty"`
}

// DingTalkConfig holds DingTalk settings.
type DingTalkConfig struct {
	ClientID     string   `json:"clientId"`
	ClientSecret string   `json:"clientSecret"`
	AllowFrom    []string `json:"allowFrom,omitempty"`
}

// EmailConfig holds email channel settings.
type EmailConfig struct {
	IMAPServer   string   `json:"imapServer"`
	SMTPServer   string   `json:"smtpServer"`
	Email        string   `json:"email"`
	Password     string   `json:"password"`
	CheckInterval int    `json:"checkInterval,omitempty"`
	AllowFrom    []string `json:"allowFrom,omitempty"`
}

// QQConfig holds QQ bot settings.
type QQConfig struct {
	AppID     string   `json:"appId"`
	AppSecret string   `json:"appSecret"`
	AllowFrom []string `json:"allowFrom,omitempty"`
}

// MochatConfig holds Mochat settings.
type MochatConfig struct {
	ServerURL string   `json:"serverUrl"`
	AllowFrom []string `json:"allowFrom,omitempty"`
}

// AgentConfig holds agent behavior settings.
type AgentConfig struct {
	Model         string   `json:"model,omitempty"`
	MaxTokens     int      `json:"maxTokens,omitempty"`
	Temperature   float64  `json:"temperature,omitempty"`
	MaxIterations int      `json:"maxIterations,omitempty"`
	Workspace     string   `json:"workspace,omitempty"`
	AlwaysSkills  []string `json:"alwaysSkills,omitempty"`

	// MCP server configurations
	MCPServers []MCPServerConfig `json:"mcpServers,omitempty"`
}

// MCPServerConfig defines a Model Context Protocol server connection.
type MCPServerConfig struct {
	Name    string            `json:"name"`
	Command string            `json:"command,omitempty"`
	Args    []string          `json:"args,omitempty"`
	URL     string            `json:"url,omitempty"`
	Env     map[string]string `json:"env,omitempty"`
}

// ToolsConfig holds tool-related settings.
type ToolsConfig struct {
	RestrictToWorkspace bool       `json:"restrictToWorkspace,omitempty"`
	Exec                ExecConfig `json:"exec,omitempty"`
}

// ExecConfig holds shell execution settings.
type ExecConfig struct {
	DenyPatterns  []string `json:"denyPatterns,omitempty"`
	AllowPatterns []string `json:"allowPatterns,omitempty"`
	Timeout       int      `json:"timeout,omitempty"`
}

// GatewayConfig holds gateway/server settings.
type GatewayConfig struct {
	Port    int    `json:"port,omitempty"`
	Host    string `json:"host,omitempty"`
	Workers int    `json:"workers,omitempty"` // Number of worker processes (nginx-style)
}

// WebSearchConfig holds web search settings.
type WebSearchConfig struct {
	Provider string `json:"provider,omitempty"`
	APIKey   string `json:"apiKey,omitempty"`
}

// SurvivalConfig holds Survival backend connection settings.
type SurvivalConfig struct {
	APIURL string `json:"apiUrl,omitempty"` // Backend URL (e.g. http://host.docker.internal:3000)
	APIKey string `json:"apiKey,omitempty"` // Backend auth key (SURVIVAL_API_KEY)
	NanobotAPIKey string `json:"nanobotApiKey,omitempty"` // HTTP API auth key (NANOBOT_API_KEY)
}

// DefaultConfig returns a Config with sensible defaults.
func DefaultConfig() Config {
	return Config{
		Agent: AgentConfig{
			Model:         "anthropic/claude-sonnet-4-5",
			MaxTokens:     4096,
			Temperature:   0.7,
			MaxIterations: 25,
		},
		Tools: ToolsConfig{
			RestrictToWorkspace: true,
			Exec: ExecConfig{
				Timeout: 30,
			},
		},
		Gateway: GatewayConfig{
			Port: 18790,
			Host: "0.0.0.0",
		},
	}
}
