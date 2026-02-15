package cmd

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/dayuer/nanobot-go/internal/config"
	"github.com/spf13/cobra"
)

var onboardCmd = &cobra.Command{
	Use:   "onboard",
	Short: "Initialize nanobot configuration and workspace",
	RunE:  runOnboard,
}

func init() {
	rootCmd.AddCommand(onboardCmd)
}

func runOnboard(cmd *cobra.Command, args []string) error {
	configPath := config.GetConfigPath()

	if _, err := os.Stat(configPath); err == nil {
		fmt.Printf("Config already exists at %s\n", configPath)
	} else {
		// Create default config
		os.MkdirAll(filepath.Dir(configPath), 0755)
		if err := config.Save(config.DefaultConfig(), ""); err != nil {
			return fmt.Errorf("creating config: %w", err)
		}
		fmt.Printf("✓ Created config at %s\n", configPath)
	}

	// Load config to get workspace path
	cfg, err := config.Load("")
	if err != nil {
		return fmt.Errorf("loading config: %w", err)
	}

	// Create workspace
	workspace := cfg.Agent.Workspace
	if workspace == "" {
		workspace = filepath.Join(os.Getenv("HOME"), ".nanobot", "workspace")
	}
	os.MkdirAll(workspace, 0755)
	fmt.Printf("✓ Workspace at %s\n", workspace)

	// Create default bootstrap files
	templates := map[string]string{
		"AGENTS.md": "# Agent Instructions\n\nYou are a helpful AI assistant. Be concise, accurate, and friendly.\n\n## Guidelines\n\n- Always explain what you're doing before taking actions\n- Ask for clarification when the request is ambiguous\n- Use tools to help accomplish tasks\n- Remember important information in memory/MEMORY.md\n",
		"SOUL.md":   "# Soul\n\nI am nanobot, a lightweight AI assistant.\n\n## Personality\n\n- Helpful and friendly\n- Concise and to the point\n- Curious and eager to learn\n",
		"USER.md":   "# User\n\nInformation about the user goes here.\n\n## Preferences\n\n- Communication style: (casual/formal)\n- Timezone: (your timezone)\n- Language: (your preferred language)\n",
	}

	for filename, content := range templates {
		path := filepath.Join(workspace, filename)
		if _, err := os.Stat(path); os.IsNotExist(err) {
			os.WriteFile(path, []byte(content), 0644)
			fmt.Printf("  Created %s\n", filename)
		}
	}

	// Create memory directory
	memDir := filepath.Join(workspace, "memory")
	os.MkdirAll(memDir, 0755)
	memFile := filepath.Join(memDir, "MEMORY.md")
	if _, err := os.Stat(memFile); os.IsNotExist(err) {
		os.WriteFile(memFile, []byte("# Long-term Memory\n\n"), 0644)
		fmt.Println("  Created memory/MEMORY.md")
	}
	histFile := filepath.Join(memDir, "HISTORY.md")
	if _, err := os.Stat(histFile); os.IsNotExist(err) {
		os.WriteFile(histFile, []byte(""), 0644)
		fmt.Println("  Created memory/HISTORY.md")
	}

	// Create skills directory
	os.MkdirAll(filepath.Join(workspace, "skills"), 0755)

	fmt.Println("\n🤖 nanobot is ready!")
	fmt.Println("\nNext steps:")
	fmt.Println("  1. Add your API key to ~/.nanobot/config.json")
	fmt.Println("  2. Chat: nanobot agent -m \"Hello!\"")

	return nil
}
