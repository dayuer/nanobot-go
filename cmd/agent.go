package cmd

import (
	"bufio"
	"context"
	"fmt"
	"log"
	"os"
	"os/signal"
	"strings"
	"syscall"

	"github.com/dayuer/nanobot-go/internal/agent"
	"github.com/dayuer/nanobot-go/internal/bus"
	"github.com/dayuer/nanobot-go/internal/config"
	"github.com/spf13/cobra"
)

var agentCmd = &cobra.Command{
	Use:   "agent",
	Short: "Interact with the agent directly",
	RunE:  runAgent,
}

var (
	agentMessage   string
	agentSessionID string
)

func init() {
	agentCmd.Flags().StringVarP(&agentMessage, "message", "m", "", "Message to send to the agent")
	agentCmd.Flags().StringVarP(&agentSessionID, "session", "s", "cli:direct", "Session ID")
	rootCmd.AddCommand(agentCmd)
}

func runAgent(cmd *cobra.Command, args []string) error {
	cfg, err := config.Load("")
	if err != nil {
		return fmt.Errorf("loading config: %w", err)
	}

	msgBus := bus.NewMessageBus()
	provider := makeProvider(cfg)

	loop := agent.NewAgentLoop(msgBus, provider, agent.AgentConfig{
		Workspace:     cfg.Agent.Workspace,
		Model:         cfg.Agent.Model,
		Temperature:   cfg.Agent.Temperature,
		MaxTokens:     cfg.Agent.MaxTokens,
		MaxIterations: cfg.Agent.MaxIterations,
	})

	if agentMessage != "" {
		// Single message mode
		resp, err := loop.ProcessDirect(context.Background(), agentMessage, agentSessionID, "cli", "direct")
		if err != nil {
			return err
		}
		fmt.Println(resp)
		return nil
	}

	// Interactive REPL mode
	fmt.Println("🤖 nanobot interactive mode (type 'exit' or Ctrl+C to quit)")
	fmt.Println()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		<-sigCh
		fmt.Println("\nGoodbye!")
		cancel()
		os.Exit(0)
	}()

	scanner := bufio.NewScanner(os.Stdin)
	exitCommands := map[string]bool{
		"exit": true, "quit": true, "/exit": true, "/quit": true, ":q": true,
	}

	for {
		fmt.Print("You: ")
		if !scanner.Scan() {
			break
		}
		input := strings.TrimSpace(scanner.Text())
		if input == "" {
			continue
		}
		if exitCommands[strings.ToLower(input)] {
			fmt.Println("Goodbye!")
			break
		}

		resp, err := loop.ProcessDirect(ctx, input, agentSessionID, "cli", "direct")
		if err != nil {
			log.Printf("Error: %v", err)
			continue
		}
		fmt.Println()
		fmt.Println("🤖 nanobot")
		fmt.Println(resp)
		fmt.Println()
	}

	return nil
}
