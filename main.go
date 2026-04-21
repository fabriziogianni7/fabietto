package main

import (
	"context"
	"log"
	"math/big"
	"os"
	"os/signal"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"

	"custom-agent/agent"
	"custom-agent/agent/planning"
	"custom-agent/config"
	"custom-agent/conversation"
	"custom-agent/embedding"
	"custom-agent/gateway"
	"custom-agent/handbook"
	"custom-agent/memory"
	"custom-agent/reminders"
	"custom-agent/sessionqueue"
	"custom-agent/skills"
	"custom-agent/telemetry"
	"custom-agent/tools"
	"custom-agent/wallet"
	"custom-agent/wallet/approval"
	"custom-agent/wallet/chains"
	"custom-agent/wallet/history"
	"custom-agent/wallet/policy"
	"custom-agent/wallet/signer"
	"custom-agent/x402client"

	"github.com/sashabaranov/go-openai"
)

func main() {
	cfg, err := config.Load()
	if err != nil {
		log.Fatalf("config: %v", err)
	}

	handbookRoot := strings.TrimSpace(os.Getenv("AGENT_HANDBOOK_DIR"))
	if handbookRoot == "" {
		handbookRoot = "agent-handbook"
	}
	var exclude map[string]bool
	if !cfg.WalletEnabled() {
		exclude = map[string]bool{"capabilities/wallet.md": true}
	}
	handbookText, err := handbook.Load(handbookRoot, exclude)
	if err != nil {
		log.Fatalf("handbook: %v", err)
	}
	systemPrompt := handbookText
	if cfg.SkillsDir != "" {
		systemPrompt += "\n\nWhen the user asks to add, create, or install a skill (even without saying newSkill), compose the SKILL.md content with YAML frontmatter and body, then use write_skill. The tool automatically runs security and feasibility checks before saving."
	}

	llmConfig := openai.DefaultConfig(cfg.GroqAPIKey)
	llmConfig.BaseURL = "https://api.groq.com/openai/v1"
	llm := openai.NewClientWithConfig(llmConfig)

	// Optional: embedding client for memory and compaction (lazy - only used when needed)
	var memoryStore *memory.Store
	var convStore *conversation.Store
	ollamaURL := strings.TrimSpace(cfg.OllamaURL)
	if ollamaURL == "" {
		ollamaURL = "http://localhost:11434" // default; set OLLAMA_URL= to disable
	}
	var embedder embedding.Embedder
	if ollamaURL != "disabled" && ollamaURL != "false" {
		client := embedding.NewClient(ollamaURL)
		if cfg.OllamaEmbedModel != "" {
			client.SetModel(cfg.OllamaEmbedModel)
		}
		embedder = client
		log.Printf("[embedding] Ollama at %s (model: %s) - semantic memory enabled", ollamaURL, client.Model)
	} else {
		log.Printf("[embedding] Ollama disabled - using keyword search for memory")
	}
	memoryStore = memory.NewStore(embedder)
	convStore = conversation.NewStore(embedder)
	reminderStore := reminders.NewStore()

	toolSet := tools.NewToolsWithReminderStore(cfg.BraveSearchAPIKey, memoryStore, reminderStore)
	telemetryCfg := telemetry.Config{
		Enabled:               cfg.TelemetryEnabled,
		Verbosity:             cfg.TelemetryVerbosity,
		ArtifactsEnabled:      cfg.RunArtifactsEnabled,
		ArtifactsDir:          cfg.RunArtifactsDir,
		ArtifactRetentionDays: cfg.RunArtifactRetentionDays,
		MetricsEnabled:        cfg.TelemetryMetricsEnabled,
		LLMRoundsEnabled:      cfg.TelemetryLLMRounds,
	}
	telemetryRuntime := telemetry.NewRuntime(telemetryCfg)
	toolSet.Telemetry = telemetryRuntime
	if cfg.SkillsDir != "" {
		sm := skills.NewManager(cfg.SkillsDir)
		toolSet.SetSkills(sm)
		toolSet.SetLLMClient(llm)
		log.Printf("[skills] enabled, dir=%s", cfg.SkillsDir)
	}
	senderRegistry := gateway.NewSenderRegistry()

	// Build gateways and register Senders (for reminders and wallet approval notifications)
	type gwStarter struct {
		name string
		gw   gateway.Gateway
	}
	var starters []gwStarter
	if cfg.TelegramBotToken != "" {
		tg := gateway.NewTelegram(cfg.TelegramBotToken)
		senderRegistry.Register("telegram", tg)
		starters = append(starters, gwStarter{"telegram", tg})
	}
	if cfg.DiscordToken != "" {
		dc := gateway.NewDiscord(cfg.DiscordToken)
		senderRegistry.Register("discord", dc)
		starters = append(starters, gwStarter{"discord", dc})
	}
	if cfg.HTTPPort != "" {
		starters = append(starters, gwStarter{"http", gateway.NewHTTP(cfg.HTTPPort)})
	}
	if cfg.SignalCliURL != "" && cfg.SignalNumber != "" {
		sg := gateway.NewSignal(cfg.SignalCliURL, cfg.SignalNumber)
		senderRegistry.Register("signal", sg)
		starters = append(starters, gwStarter{"signal", sg})
	}

	// Wallet: optional. When enabled, create chain registry, signer, policy, approval, history, service.
	if cfg.WalletEnabled() {
		chainRegistry, err := chains.BuildFromConfig(cfg.WalletChainsJSON, cfg.EVM_RPC_URL, cfg.ChainID, cfg.WalletDefaultChainID)
		if err != nil {
			log.Fatalf("wallet chain registry: %v", err)
		}
		sgn, err := signer.NewFromBackend(cfg.WalletSignerBackend, map[string]string{"env_key": cfg.WalletPrivateKeyEnv})
		if err != nil {
			log.Fatalf("wallet signer: %v", err)
		}
		// Create x402 client from private key before unsetting env (env backend only)
		var x402Client *x402client.Client
		if cfg.WalletSignerBackend == "" || cfg.WalletSignerBackend == "env" {
			if pk := os.Getenv(cfg.WalletPrivateKeyEnv); pk != "" {
				x402Client, err = x402client.New(pk)
				if err != nil {
					log.Printf("[x402] signer init failed (http_request will use plain client): %v", err)
				} else {
					toolSet.SetX402Client(x402Client)
					log.Printf("[x402] buyer enabled for http_request")
				}
			}
		}
		signer.UnsetEnvKey(cfg.WalletPrivateKeyEnv)
		policyCfg := policy.DefaultConfig()
		if cfg.WalletNativeSpendLimit != "" {
			if n, ok := new(big.Int).SetString(cfg.WalletNativeSpendLimit, 10); ok {
				policyCfg.NativeSpendLimitWei = n
			}
		}
		policyEngine := policy.NewEngine(policyCfg)
		approvalDir := cfg.WalletApprovalDir
		if approvalDir == "" {
			approvalDir = "wallet-approvals"
		}
		approvalStore := approval.NewStore(approvalDir, 15*time.Minute)
		notifier := wallet.NewSenderNotifier(senderRegistry)
		historyDir := filepath.Join(filepath.Dir(approvalDir), "wallet-history")
		historyStore := history.NewStore(historyDir)
		walletSvc := wallet.NewService(chainRegistry, sgn, policyEngine, approvalStore, notifier, historyStore)
		toolSet.SetWallet(walletSvc)
		log.Printf("[wallet] enabled, address %s, default chain %d, backend=%s", walletSvc.WalletAddress(), walletSvc.DefaultChainID(), cfg.WalletSignerBackend)
	}

	if toolSet.Wallet != nil {
		systemPrompt = strings.Replace(systemPrompt, "{{WALLET_ADDRESS}}", toolSet.Wallet.WalletAddress(), 1)
		systemPrompt = strings.Replace(systemPrompt, "{{DEFAULT_CHAIN_ID}}", strconv.FormatInt(toolSet.Wallet.DefaultChainID(), 10), 1)
	}
	systemPrompt = strings.TrimSpace(systemPrompt)

	a := agent.New(llm, systemPrompt, cfg.CompactionThreshold, toolSet, convStore, cfg.SkillsDir, telemetryRuntime)
	plannerMode := cfg.EffectivePlannerMode()
	plannerCaps := planning.ParseCapabilities(cfg.PlannerCapabilities)
	if plannerMode != planning.PlannerModeOff && len(plannerCaps) == 0 {
		plannerCaps = planning.ParseCapabilities("wallet")
	}
	plannerOn := plannerMode != planning.PlannerModeOff
	if plannerOn && toolSet.Wallet == nil {
		log.Printf("[planner] mode=%s ignored (wallet not configured)", plannerMode)
		plannerOn = false
		plannerMode = planning.PlannerModeOff
	}
	orchMode := cfg.EffectiveOrchestrationMode()
	a.SetPlanner(planning.RuntimeConfig{
		Enabled:       plannerOn,
		Mode:          plannerMode,
		Capabilities:  plannerCaps,
		Orchestration: planning.OrchestrationRuntime{Mode: orchMode},
	})
	if plannerOn {
		log.Printf("[planner] mode=%s capabilities=%v", plannerMode, plannerCaps)
	}
	if orchMode != planning.OrchestrationModeOff {
		log.Printf("[orchestration] mode=%s", orchMode)
	}

	queue := sessionqueue.New(func(msg gateway.IncomingMessage) string {
		return a.HandleMessage(context.Background(), msg)
	})
	handler := func(msg gateway.IncomingMessage) string {
		return queue.Process(msg)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	for _, s := range starters {
		gw, name := s.gw, s.name
		go func() {
			if err := gw.Run(ctx, handler); err != nil && err != context.Canceled {
				log.Printf("[%s] %v", name, err)
			}
		}()
	}

	// Start reminders cron (sends scheduled messages via SenderRegistry)
	cronRunner := reminders.NewRunner(reminderStore, senderRegistry)
	go cronRunner.Start(ctx)

	sig := make(chan os.Signal, 1)
	signal.Notify(sig, syscall.SIGINT, syscall.SIGTERM)
	<-sig
	log.Println("shutting down...")
	cancel()
}
