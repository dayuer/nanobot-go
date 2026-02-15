package cmd

import (
	"os"
	"strings"

	"github.com/dayuer/nanobot-go/internal/config"
	"github.com/dayuer/nanobot-go/internal/providers"
)

// makeProvider creates a Provider from the loaded config.
// It tries to detect the correct provider and API key from environment variables.
func makeProvider(cfg config.Config) *providers.Provider {
	model := cfg.Agent.Model
	if model == "" {
		model = "anthropic/claude-sonnet-4-5"
	}

	// Try to find API key: check env vars for matching provider
	apiKey := ""
	apiBase := ""
	providerName := ""

	spec := providers.FindByModel(model)
	if spec != nil {
		providerName = spec.Name
		apiKey = os.Getenv(spec.EnvKey)
		if spec.DefaultAPIBase != "" && apiBase == "" {
			apiBase = spec.DefaultAPIBase
		}
	}

	// Fallback: try common env vars
	if apiKey == "" {
		for _, envKey := range []string{"OPENROUTER_API_KEY", "OPENAI_API_KEY", "ANTHROPIC_API_KEY"} {
			if v := os.Getenv(envKey); v != "" {
				apiKey = v
				break
			}
		}
	}

	// Try to detect gateway from API key
	if apiKey != "" && strings.HasPrefix(apiKey, "sk-or-") {
		apiBase = "https://openrouter.ai/api/v1"
		providerName = "openrouter"
	}

	return providers.NewProvider(apiKey, apiBase, model, providerName)
}
