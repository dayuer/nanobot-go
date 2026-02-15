package cmd

import (
	"context"
	"fmt"
	"log"
	"os"
	"os/signal"
	"syscall"

	"github.com/dayuer/nanobot-go/internal/agent"
	"github.com/dayuer/nanobot-go/internal/bus"
	"github.com/dayuer/nanobot-go/internal/channels"
	"github.com/dayuer/nanobot-go/internal/config"
	"github.com/spf13/cobra"
)

var gatewayCmd = &cobra.Command{
	Use:   "gateway",
	Short: "Start the nanobot gateway (channels + agent)",
	RunE:  runGateway,
}

var gatewayPort int

func init() {
	gatewayCmd.Flags().IntVarP(&gatewayPort, "port", "p", 18790, "Gateway port")
	rootCmd.AddCommand(gatewayCmd)
}

func runGateway(cmd *cobra.Command, args []string) error {
	cfg, err := config.Load("")
	if err != nil {
		return fmt.Errorf("loading config: %w", err)
	}

	fmt.Printf("🤖 Starting nanobot gateway on port %d...\n", gatewayPort)

	msgBus := bus.NewMessageBus()
	provider := makeProvider(cfg)

	loop := agent.NewAgentLoop(msgBus, provider, agent.AgentConfig{
		Workspace:     cfg.Agent.Workspace,
		Model:         cfg.Agent.Model,
		Temperature:   cfg.Agent.Temperature,
		MaxTokens:     cfg.Agent.MaxTokens,
		MaxIterations: cfg.Agent.MaxIterations,
	})

	// Create channel manager
	chMgr := channels.NewManager(msgBus)

	// Register enabled channels
	if tg := cfg.Channel.Telegram; tg != nil && tg.Token != "" {
		chMgr.Register(channels.NewTelegramChannel(tg.Token, tg.AllowFrom, msgBus))
		log.Println("Telegram channel enabled")
	}

	if enabled := chMgr.EnabledChannels(); len(enabled) > 0 {
		fmt.Printf("✓ Channels enabled: %v\n", enabled)
	} else {
		fmt.Println("⚠ No channels enabled")
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)

	go func() {
		<-sigCh
		fmt.Println("\nShutting down...")
		chMgr.StopAll()
		loop.Stop()
		cancel()
	}()

	// Run agent loop and channels concurrently
	errCh := make(chan error, 2)
	go func() {
		// Agent loop listens for inbound messages from bus
		for {
			select {
			case <-ctx.Done():
				errCh <- nil
				return
			case msg := <-msgBus.Inbound:
				resp, err := loop.ProcessDirect(ctx, msg.Content, msg.SessionKey(), msg.Channel, msg.ChatID)
				if err != nil {
					log.Printf("Agent error: %v", err)
					continue
				}
				msgBus.PublishOutbound(bus.OutboundMessage{
					Channel: msg.Channel,
					ChatID:  msg.ChatID,
					Content: resp,
				})
			}
		}
	}()
	go func() { errCh <- chMgr.StartAll(ctx) }()

	return <-errCh
}
