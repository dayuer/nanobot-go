package cmd

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"log"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"

	"github.com/spf13/cobra"

	"github.com/dayuer/nanobot-go/internal/bus"
	"github.com/dayuer/nanobot-go/internal/channels"
	"github.com/dayuer/nanobot-go/internal/cluster"
	"github.com/dayuer/nanobot-go/internal/config"
	"github.com/dayuer/nanobot-go/internal/confighub"
	"github.com/dayuer/nanobot-go/internal/providers"
	nanoredis "github.com/dayuer/nanobot-go/internal/redis"
	"github.com/dayuer/nanobot-go/internal/registry"
	"github.com/dayuer/nanobot-go/internal/router"
)

var (
	serverPort    int
	serverAPIKey  string
	registryURL   string
	agentsFile    string
)

var serverCmd = &cobra.Command{
	Use:   "server",
	Short: "Start the Survival server (multi-agent, HTTP API, dynamic config)",
	Long: `Start the nanobot Survival server with:
  - Dynamic LLM config from registry center (hot-switchable)
  - Multi-agent dispatch (agents.yaml)
  - Session lane concurrency control
  - HTTP API endpoints (/api/chat, /api/status, /api/agents, etc.)`,
	RunE: runServer,
}

func init() {
	rootCmd.AddCommand(serverCmd)
	serverCmd.Flags().IntVarP(&serverPort, "port", "p", 18790, "HTTP API port")
	serverCmd.Flags().StringVar(&serverAPIKey, "api-key", "", "API key for auth (or NANOBOT_API_KEY env)")
	serverCmd.Flags().StringVar(&registryURL, "registry", "", "Registry center URL (or NANOBOT_REGISTRY_URL env)")
	serverCmd.Flags().StringVar(&agentsFile, "agents", "", "Path to agents.yaml (default: workspace/agents.yaml)")
}

func runServer(cmd *cobra.Command, args []string) error {
	// 1. Load local config (Layer 1: fallback)
	cfg, err := config.Load("")
	if err != nil {
		return fmt.Errorf("loading config: %w", err)
	}
	cfg.ExportEnv() // Export config values as environment variables

	// --- Resolve settings: CLI flag → config.json → env var ---

	// Port
	port := serverPort
	if cfg.Gateway.Port != 0 && serverPort == 18790 {
		port = cfg.Gateway.Port
	}
	if p := os.Getenv("NANOBOT_PORT"); p != "" && serverPort == 18790 {
		if pv, err := fmt.Sscanf(p, "%d", &port); err == nil && pv == 1 {
			// use parsed port
		}
	}

	// Workspace
	workspace := cfg.Agent.Workspace
	if workspace == "" {
		workspace = os.Getenv("NANOBOT_WORKSPACE")
	}
	cfg.Agent.Workspace = workspace

	// NANOBOT_API_KEY — for HTTP API auth
	apiKey := serverAPIKey
	if apiKey == "" && cfg.Survival.NanobotAPIKey != "" {
		apiKey = cfg.Survival.NanobotAPIKey
	}
	if apiKey == "" {
		apiKey = os.Getenv("NANOBOT_API_KEY")
	}

	// SURVIVAL_API_URL — backend/registry URL
	regURL := registryURL
	if regURL == "" && cfg.Survival.APIURL != "" {
		regURL = cfg.Survival.APIURL
	}
	if regURL == "" {
		regURL = os.Getenv("SURVIVAL_API_URL")
	}

	// SURVIVAL_API_KEY — backend auth (defaults to NANOBOT_API_KEY if not set)
	regKey := cfg.Survival.APIKey
	if regKey == "" {
		regKey = os.Getenv("SURVIVAL_API_KEY")
	}
	if regKey == "" {
		regKey = apiKey // fallback: same as NANOBOT_API_KEY
	}

	// --- 🌐 Pool bootstrap: get instanceId from backend ---
	wsFingerprint := generateFingerprint()

	poolClient := cluster.NewPoolClient(cluster.PoolConfig{
		BackendURL:  regURL,
		APIKey:      apiKey, // NANOBOT_API_KEY for Bearer auth
		Port:        port,
		Model:       cfg.Agent.Model,
		ToolCount:   0, // updated after agent registration
		Fingerprint: wsFingerprint,
	})

	var instanceID string
	if envID := os.Getenv("NANOBOT_INSTANCE_ID"); envID != "" {
		instanceID = envID
		poolClient.SetInstanceID(envID)
	} else if regURL != "" {
		fmt.Println("   🌐 Requesting instanceId from backend...")
		instanceID, _ = poolClient.Bootstrap(port)
	} else {
		instanceID = fmt.Sprintf("nanobot-%d", port)
		poolClient.SetInstanceID(instanceID)
	}

	fmt.Println("🚀 Starting nanobot Survival server...")
	fmt.Printf("   Instance: %s\n", instanceID)
	fmt.Printf("   Self URL: %s\n", poolClient.SelfURL())
	fmt.Printf("   🔑 WS Fingerprint: %s\n", wsFingerprint)
	if workspace != "" {
		fmt.Printf("   Workspace: %s\n", workspace)
	}

	// 2. Create ConfigHub (dynamic config center)
	hub := confighub.New(
		confighub.LLMConfig{
			Model:       cfg.Agent.Model,
			Temperature: cfg.Agent.Temperature,
			MaxTokens:   cfg.Agent.MaxTokens,
		},
		confighub.WithRegistryURL(regURL),
		confighub.WithInstanceID(instanceID),
		confighub.WithAPIKey(regKey),
	)

	// 3. Fetch config from registry (Layer 2)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	if regURL != "" {
		fmt.Printf("   Registry: %s\n", regURL)
		if err := hub.Fetch(ctx); err != nil {
			log.Printf("⚠️ Registry fetch failed, using local config: %v", err)
		} else {
			fmt.Println("   ✅ Config fetched from registry")
		}
	} else {
		fmt.Println("   📋 Using local config (no registry URL)")
	}

	// 4. Create provider with dynamic config
	llmCfg := hub.Current()
	baseProvider := makeProviderFromLLMConfig(llmCfg)
	dynProvider := providers.NewDynamicProvider(baseProvider)

	// Register config change callback: hot-swap provider
	hub.OnChange(func(newCfg *confighub.LLMConfig) {
		newProvider := makeProviderFromLLMConfig(newCfg)
		dynProvider.Swap(newProvider)
		log.Printf("[Server] 🔄 Provider hot-swapped → model=%s", newCfg.Model)
	})

	// 5. Create message bus
	msgBus := bus.NewMessageBus()

	// 6. Register agents
	reg := registry.NewRegistry(registry.RegistryConfig{
		DefaultProvider: dynProvider,
		Bus:             msgBus,
		Workspace:       cfg.Agent.Workspace,
		DefaultModel:    llmCfg.Model,
	})

	// Load agents.yaml
	agentsPath := agentsFile
	if agentsPath == "" {
		agentsPath = filepath.Join(cfg.Agent.Workspace, "agents.yaml")
	}
	specs, err := registry.LoadAgentSpecs(agentsPath)
	if err != nil {
		log.Printf("⚠️ Could not load agents.yaml: %v", err)
	}

	if len(specs) > 0 {
		for _, spec := range specs {
			if err := reg.Register(spec); err != nil {
				log.Printf("⚠️ Failed to register agent %s: %v", spec.ID, err)
			}
		}
		fmt.Printf("   ✅ %d agents registered\n", reg.Len())
	} else {
		// Register a default agent
		reg.Register(registry.AgentSpec{
			ID:          "general",
			Description: "Default agent",
			IsDefault:   true,
		})
		fmt.Println("   📋 Single-agent mode (no agents.yaml)")
	}

	// 7. Init Redis (optional, graceful fallback)
	if cfg.Redis.URL != "" {
		if nanoredis.Init(nanoredis.Config{
			URL:      cfg.Redis.URL,
			Password: cfg.Redis.Password,
			DB:       cfg.Redis.DB,
		}) {
			fmt.Println("   ✅ Redis connected")
		} else {
			fmt.Println("   ⚠️ Redis unavailable (memory/cache features disabled)")
		}
	}

	// 8. Create LLM Router (if router model configured and agents > 1)
	var llmRouter *router.LLMRouter
	if cfg.RouterModel.Model != "" && reg.Len() > 1 {
		// Build role list from registered agents
		var roles []router.Role
		for _, id := range reg.AgentIDs() {
			if spec := reg.GetSpec(id); spec != nil {
				roles = append(roles, router.Role{
					ID:          id,
					Description: spec.Description,
				})
			}
		}
		if len(roles) > 0 {
			routerProvider := providers.NewProvider("", "", cfg.RouterModel.Model, "")
			llmRouter = router.NewLLMRouter(roles, cfg.RouterModel.Model, routerProvider)
			fmt.Printf("   ✅ LLM Router enabled (model=%s, %d roles)\n", cfg.RouterModel.Model, len(roles))
		}
	}

	// 9. Create channel manager
	chMgr := channels.NewManager(msgBus)
	if tg := cfg.Channel.Telegram; tg != nil && tg.Token != "" {
		chMgr.Register(channels.NewTelegramChannel(tg.Token, tg.AllowFrom, msgBus))
		log.Println("   Telegram channel enabled")
	}
	if enabled := chMgr.EnabledChannels(); len(enabled) > 0 {
		fmt.Printf("   ✅ Channels: %v\n", enabled)
	}

	// 10. Start cluster HTTP + WS server
	srv := cluster.NewServer(cluster.ServerConfig{
		Port:          port,
		APIKey:        apiKey,
		InstanceID:    instanceID,
		WSFingerprint: wsFingerprint,
		Registry:      reg,
		ConfigHub:     hub,
		Router:        llmRouter,
	})

	// WS disconnect → auto re-register to backend pool (with retry)
	srv.ReRegisterFn = func() { poolClient.RegisterWithRetry(ctx) }

	fmt.Printf("   ✅ HTTP API → http://0.0.0.0:%d\n", port)
	fmt.Printf("   ✅ WebSocket → ws://0.0.0.0:%d/ws\n", port)
	fmt.Println("────────────────────────────────────────")

	// 9. Register to backend pool with retry (non-blocking)
	go poolClient.RegisterWithRetry(ctx)

	// Write PID file only in direct foreground mode (not when spawned by daemon).
	// The daemon manages the multi-PID file itself.
	isForeground := false
	if _, err := os.Stat(pidFilePath()); os.IsNotExist(err) {
		writePID(os.Getpid())
		isForeground = true
	}
	defer func() {
		if isForeground {
			removePID()
		}
	}()

	// 10. Graceful shutdown + SIGHUP reload
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM, syscall.SIGHUP)

	go func() {
		for sig := range sigCh {
			switch sig {
			case syscall.SIGHUP:
				// Reload: re-fetch config from registry
				log.Println("🔄 SIGHUP received — reloading config...")
				if regURL != "" {
					if err := hub.Fetch(ctx); err != nil {
						log.Printf("⚠️ Reload failed: %v", err)
					} else {
						log.Println("✅ Config reloaded from registry")
					}
				}
			case syscall.SIGINT, syscall.SIGTERM:
				fmt.Println("\n🛑 Shutting down...")
				poolClient.Unregister()
				srv.Stop()
				chMgr.StopAll()
				nanoredis.Close()
				cancel()
				return
			}
		}
	}()

	// 11. Start server (blocks)
	go chMgr.StartAll(ctx)
	return srv.Start(ctx)
}

// makeProviderFromLLMConfig creates a Provider from ConfigHub's LLMConfig.
func makeProviderFromLLMConfig(cfg *confighub.LLMConfig) *providers.Provider {
	apiKey := cfg.APIKey
	if apiKey == "" {
		// Fallback to env
		spec := providers.FindByModel(cfg.Model)
		if spec != nil {
			apiKey = os.Getenv(spec.EnvKey)
		}
	}
	if apiKey == "" {
		for _, envKey := range []string{"OPENROUTER_API_KEY", "OPENAI_API_KEY", "ANTHROPIC_API_KEY"} {
			if v := os.Getenv(envKey); v != "" {
				apiKey = v
				break
			}
		}
	}
	return providers.NewProvider(apiKey, cfg.APIBase, cfg.Model, cfg.Provider)
}

// generateFingerprint creates a random 16-char hex string.
// Mirrors Python's uuid4().hex[:16] used for WS fingerprint.
func generateFingerprint() string {
	b := make([]byte, 8)
	rand.Read(b)
	return hex.EncodeToString(b)
}
