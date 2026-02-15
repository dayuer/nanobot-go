package cmd

import (
	"fmt"

	"github.com/dayuer/nanobot-go/internal/config"
	"github.com/dayuer/nanobot-go/internal/providers"
	"github.com/spf13/cobra"
)

var statusCmd = &cobra.Command{
	Use:   "status",
	Short: "Show nanobot status",
	RunE:  runStatus,
}

func init() {
	rootCmd.AddCommand(statusCmd)
}

func runStatus(cmd *cobra.Command, args []string) error {
	configPath := config.GetConfigPath()
	cfg, err := config.Load("")
	if err != nil {
		return fmt.Errorf("loading config: %w", err)
	}

	fmt.Println("🤖 nanobot Status")
	fmt.Println()
	fmt.Printf("Config: %s\n", configPath)
	fmt.Printf("Workspace: %s\n", cfg.Agent.Workspace)
	fmt.Printf("Model: %s\n", cfg.Agent.Model)

	// Provider status
	spec := providers.FindByModel(cfg.Agent.Model)
	if spec != nil {
		fmt.Printf("Provider: %s\n", spec.Label())
	}

	// Channel status
	fmt.Println("\nChannels:")
	if tg := cfg.Channel.Telegram; tg != nil && tg.Token != "" {
		fmt.Println("  Telegram: ✓")
	}
	if sl := cfg.Channel.Slack; sl != nil && sl.BotToken != "" {
		fmt.Println("  Slack: ✓")
	}
	if fs := cfg.Channel.Feishu; fs != nil && fs.AppID != "" {
		fmt.Println("  Feishu: ✓")
	}

	return nil
}
