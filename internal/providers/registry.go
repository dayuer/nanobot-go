// Package providers — registry.go
// Provider Registry: single source of truth for LLM provider metadata.
// Mirrors upstream/nanobot/providers/registry.py.
package providers

import "strings"

// ProviderSpec holds metadata for one LLM provider.
type ProviderSpec struct {
	Name               string            // config field name, e.g. "dashscope"
	Keywords           []string          // model-name keywords for matching (lowercase)
	EnvKey             string            // env var for API key, e.g. "DASHSCOPE_API_KEY"
	DisplayName        string            // shown in status
	LiteLLMPrefix      string            // prefix for model routing
	SkipPrefixes       []string          // don't add prefix if model already starts with these
	EnvExtras          [][2]string       // extra env vars: {name, value template}
	IsGateway          bool              // can route any model (OpenRouter, AiHubMix)
	IsLocal            bool              // local deployment (vLLM, Ollama)
	DetectByKeyPrefix  string            // match api_key prefix
	DetectByBaseKW     string            // match substring in api_base URL
	DefaultAPIBase     string            // fallback base URL
	StripModelPrefix   bool              // strip "provider/" before re-prefixing
	ModelOverrides     []ModelOverride   // per-model param overrides
}

// ModelOverride applies parameter overrides when a model name matches a pattern.
type ModelOverride struct {
	Pattern   string         // substring to match in model name (lowercase)
	Overrides map[string]any // params to override
}

// Label returns a display label.
func (s *ProviderSpec) Label() string {
	if s.DisplayName != "" {
		return s.DisplayName
	}
	return strings.Title(s.Name) //nolint:staticcheck
}

// Providers is the registry. Order = priority. Gateways first.
var Providers = []*ProviderSpec{
	// Custom (user-provided OpenAI-compatible endpoint)
	{
		Name: "custom", Keywords: nil, EnvKey: "OPENAI_API_KEY",
		DisplayName: "Custom", LiteLLMPrefix: "openai",
		SkipPrefixes: []string{"openai/"},
		IsGateway: true, StripModelPrefix: true,
	},
	// OpenRouter
	{
		Name: "openrouter", Keywords: []string{"openrouter"},
		EnvKey: "OPENROUTER_API_KEY", DisplayName: "OpenRouter",
		LiteLLMPrefix: "openrouter", IsGateway: true,
		DetectByKeyPrefix: "sk-or-", DetectByBaseKW: "openrouter",
		DefaultAPIBase: "https://openrouter.ai/api/v1",
	},
	// AiHubMix
	{
		Name: "aihubmix", Keywords: []string{"aihubmix"},
		EnvKey: "OPENAI_API_KEY", DisplayName: "AiHubMix",
		LiteLLMPrefix: "openai", IsGateway: true,
		DetectByBaseKW: "aihubmix",
		DefaultAPIBase: "https://aihubmix.com/v1",
		StripModelPrefix: true,
	},
	// Anthropic
	{
		Name: "anthropic", Keywords: []string{"anthropic", "claude"},
		EnvKey: "ANTHROPIC_API_KEY", DisplayName: "Anthropic",
	},
	// OpenAI
	{
		Name: "openai", Keywords: []string{"openai", "gpt"},
		EnvKey: "OPENAI_API_KEY", DisplayName: "OpenAI",
	},
	// DeepSeek
	{
		Name: "deepseek", Keywords: []string{"deepseek"},
		EnvKey: "DEEPSEEK_API_KEY", DisplayName: "DeepSeek",
		LiteLLMPrefix: "deepseek", SkipPrefixes: []string{"deepseek/"},
		DefaultAPIBase: "https://api.deepseek.com/v1",
	},
	// Gemini
	{
		Name: "gemini", Keywords: []string{"gemini"},
		EnvKey: "GEMINI_API_KEY", DisplayName: "Gemini",
		LiteLLMPrefix: "gemini", SkipPrefixes: []string{"gemini/"},
	},
	// Zhipu
	{
		Name: "zhipu", Keywords: []string{"zhipu", "glm", "zai"},
		EnvKey: "ZAI_API_KEY", DisplayName: "Zhipu AI",
		LiteLLMPrefix: "zai",
		SkipPrefixes: []string{"zhipu/", "zai/", "openrouter/", "hosted_vllm/"},
		EnvExtras: [][2]string{{"ZHIPUAI_API_KEY", "{api_key}"}},
	},
	// DashScope
	{
		Name: "dashscope", Keywords: []string{"qwen", "dashscope"},
		EnvKey: "DASHSCOPE_API_KEY", DisplayName: "DashScope",
		LiteLLMPrefix: "dashscope",
		SkipPrefixes: []string{"dashscope/", "openrouter/"},
	},
	// Moonshot
	{
		Name: "moonshot", Keywords: []string{"moonshot", "kimi"},
		EnvKey: "MOONSHOT_API_KEY", DisplayName: "Moonshot",
		LiteLLMPrefix: "moonshot",
		SkipPrefixes: []string{"moonshot/", "openrouter/"},
		EnvExtras:    [][2]string{{"MOONSHOT_API_BASE", "{api_base}"}},
		DefaultAPIBase: "https://api.moonshot.ai/v1",
		ModelOverrides: []ModelOverride{
			{Pattern: "kimi-k2.5", Overrides: map[string]any{"temperature": 1.0}},
		},
	},
	// MiniMax
	{
		Name: "minimax", Keywords: []string{"minimax"},
		EnvKey: "MINIMAX_API_KEY", DisplayName: "MiniMax",
		LiteLLMPrefix: "minimax",
		SkipPrefixes: []string{"minimax/", "openrouter/"},
		DefaultAPIBase: "https://api.minimax.io/v1",
	},
	// vLLM / Local
	{
		Name: "vllm", Keywords: []string{"vllm"},
		EnvKey: "HOSTED_VLLM_API_KEY", DisplayName: "vLLM/Local",
		LiteLLMPrefix: "hosted_vllm", IsLocal: true,
	},
	// Groq
	{
		Name: "groq", Keywords: []string{"groq"},
		EnvKey: "GROQ_API_KEY", DisplayName: "Groq",
		LiteLLMPrefix: "groq", SkipPrefixes: []string{"groq/"},
	},
}

// FindByModel returns a standard provider spec matching a model name keyword.
// Skips gateways and local providers.
func FindByModel(model string) *ProviderSpec {
	lower := strings.ToLower(model)
	for _, spec := range Providers {
		if spec.IsGateway || spec.IsLocal {
			continue
		}
		for _, kw := range spec.Keywords {
			if strings.Contains(lower, kw) {
				return spec
			}
		}
	}
	return nil
}

// FindGateway detects a gateway/local provider.
// Priority: 1) provider_name  2) api_key prefix  3) api_base keyword.
func FindGateway(providerName, apiKey, apiBase string) *ProviderSpec {
	// 1. Direct match by config key
	if providerName != "" {
		spec := FindByName(providerName)
		if spec != nil && (spec.IsGateway || spec.IsLocal) {
			return spec
		}
	}
	// 2. Auto-detect by api_key prefix / api_base keyword
	for _, spec := range Providers {
		if spec.DetectByKeyPrefix != "" && apiKey != "" &&
			strings.HasPrefix(apiKey, spec.DetectByKeyPrefix) {
			return spec
		}
		if spec.DetectByBaseKW != "" && apiBase != "" &&
			strings.Contains(apiBase, spec.DetectByBaseKW) {
			return spec
		}
	}
	return nil
}

// FindByName finds a provider spec by config field name.
func FindByName(name string) *ProviderSpec {
	for _, spec := range Providers {
		if spec.Name == name {
			return spec
		}
	}
	return nil
}
