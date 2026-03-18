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
	"custom-agent/alchemy"
	"custom-agent/config"
	"custom-agent/conversation"
	"custom-agent/embedding"
	"custom-agent/gateway"
	"custom-agent/memory"
	"custom-agent/opportunity"
	"custom-agent/reminders"
	"custom-agent/sessionqueue"
	"custom-agent/skills"
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

	personalityPath := "PERSONALITY.md"
	if cfg.AutonomousMode {
		personalityPath = "PERSONALITY_AUTONOMOUS.md"
	}
	personality, err := os.ReadFile(personalityPath)
	if err != nil {
		log.Fatalf("failed to load %s: %v", personalityPath, err)
	}
	toolInstruction := "\n\nYou have access to tools. Use them when they help answer the user's question—for example, read files, run commands, search the web, use memory (save_memory, read_memory), schedule reminders (create_scheduled_reminder, list_reminders, delete_reminder), spawn parallel sub-agents (spawn_subagents), or http_request for HTTP APIs. When a task can be parallelized, use spawn_subagents."
	if cfg.AutonomousMode {
		toolInstruction += " Prioritize wallet and trading tools when seeking profitable opportunities. Use http_request with Tokenaru for onchain data before executing trades. Use x402_get_stats to check inference spend and runway before capital deployment."
		minBase := cfg.X402MinBaseUSDC
		if minBase == "" {
			minBase = "10"
		}
		toolInstruction += " Reserve at least $" + minBase + " USDC on Base (chain 8453) for inference; never trade below."
	}
	if cfg.SkillsDir != "" {
		toolInstruction += " When the user asks to add, create, or install a skill (even without saying newSkill), compose the SKILL.md content with YAML frontmatter and body, then use write_skill. The tool automatically runs security and feasibility checks before saving."
	}
	if cfg.WalletEnabled() {
		toolInstruction += " When the wallet is configured, you can use wallet_get_balance, wallet_execute_transfer, wallet_execute_contract_call, and wallet_list_transactions. You MUST call wallet_execute_transfer or wallet_execute_contract_call to send—never claim a transaction was sent without invoking the tool. Transactions may require user approval; reply with approve: <tx_id> when prompted. With wallet enabled, http_request can automatically pay for x402-protected APIs (402 Payment Required)."
		if cfg.AlchemyEnabled() {
			toolInstruction += " Use wallet_get_portfolio, wallet_get_portfolio_value, wallet_get_activity, and wallet_simulate_transaction for full holdings, USD valuation, activity history, and pre-trade simulation."
		}
	}
	toolInstruction += "\n"

	// Build x402 client early when autonomous (needed for LLM) or wallet-enabled (for http_request)
	var x402Client *x402client.Client
	if cfg.WalletEnabled() && (cfg.WalletSignerBackend == "" || cfg.WalletSignerBackend == "env") {
		if pk := os.Getenv(cfg.WalletPrivateKeyEnv); pk != "" {
			var errX402 error
			if cfg.AutonomousMode && cfg.EVM_RPC_URL != "" && cfg.X402PermitCap != "" {
				x402Client, errX402 = x402client.NewWithUpto(pk, cfg.EVM_RPC_URL, cfg.X402PermitCap)
			} else {
				x402Client, errX402 = x402client.New(pk)
			}
			if errX402 != nil {
				if cfg.AutonomousMode {
					log.Fatalf("[x402] autonomous mode requires x402 client: %v", errX402)
				}
				log.Printf("[x402] signer init failed (http_request will use plain client): %v", errX402)
			} else if cfg.AutonomousMode || cfg.WalletEnabled() {
				log.Printf("[x402] buyer enabled for http_request")
			}
		}
	}
	// LLM client: autonomous mode uses x402 router; else Groq
	var llm *openai.Client
	if cfg.AutonomousMode {
		if x402Client == nil {
			log.Fatal("[autonomous] x402 client required for LLM; ensure wallet is configured")
		}
		llmConfig := openai.DefaultConfig("x402") // auth via payment, not API key
		llmConfig.BaseURL = cfg.X402RouterURL
		llmConfig.HTTPClient = x402Client.Client
		llm = openai.NewClientWithConfig(llmConfig)
		log.Printf("[autonomous] LLM via x402 router %s", cfg.X402RouterURL)
	} else {
		llmConfig := openai.DefaultConfig(cfg.GroqAPIKey)
		llmConfig.BaseURL = "https://api.groq.com/openai/v1"
		llm = openai.NewClientWithConfig(llmConfig)
	}

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
	if x402Client != nil {
		toolSet.SetX402Client(x402Client)
		if cfg.AutonomousMode && cfg.X402RouterURL != "" {
			toolSet.SetX402StatsConfig(cfg.X402RouterURL, cfg.X402PermitCap)
		}
	}
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
		signer.UnsetEnvKey(cfg.WalletPrivateKeyEnv)
		// x402 client created earlier (before LLM) for autonomous mode or http_request
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

		// Alchemy: when enabled, build client and attach for portfolio tools
		if cfg.AlchemyEnabled() {
			alchemyCfg := alchemyConfig(cfg, chainRegistry)
			if alc, err := alchemy.New(alchemyCfg); err != nil {
				log.Printf("[alchemy] client init failed: %v", err)
			} else if alc != nil {
				toolSet.SetAlchemy(alc)
				log.Printf("[alchemy] portfolio tools enabled")
			}
		}
	}

	// Build system prompt: personality + tools, then append WALLET.md when wallet is enabled
	systemPrompt := strings.TrimSpace(string(personality)) + toolInstruction
	if toolSet.Wallet != nil {
		walletDoc, err := os.ReadFile("WALLET.md")
		if err != nil {
			log.Fatalf("failed to load WALLET.md: %v", err)
		}
		walletBlock := strings.Replace(string(walletDoc), "{{WALLET_ADDRESS}}", toolSet.Wallet.WalletAddress(), 1)
		walletBlock = strings.Replace(walletBlock, "{{DEFAULT_CHAIN_ID}}", strconv.FormatInt(toolSet.Wallet.DefaultChainID(), 10), 1)
		systemPrompt += "\n\n" + strings.TrimSpace(walletBlock)
	}
	parentModel := ""
	subagentModel := ""
	skipCompaction := false
	var modelForRole func(role string) string
	if cfg.AutonomousMode {
		parentModel = cfg.X402Model
		subagentModel = cfg.X402Model
		skipCompaction = true
		modelForRole = func(role string) string { return cfg.ModelForRole(role) }
	}
	a := agent.New(llm, parentModel, subagentModel, systemPrompt, cfg.CompactionThreshold, skipCompaction, toolSet, convStore, cfg.SkillsDir, modelForRole)

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

	// Start opportunity scan cron (autonomous mode only; sends synthetic message to agent)
	if cfg.AutonomousMode && cfg.OpportunityScanIntervalMinutes > 0 {
		scanInterval := time.Duration(cfg.OpportunityScanIntervalMinutes) * time.Minute
		oppRunner := opportunity.NewRunner(handler, senderRegistry, "telegram", cfg.TelegramOwnerChatID, cfg.TelegramOwnerChatID, scanInterval)
		go oppRunner.Start(ctx)
	}

	// Log x402 router spend stats periodically (autonomous mode only)
	if cfg.AutonomousMode && x402Client != nil {
		go logX402Stats(ctx, x402Client, cfg.X402RouterURL, cfg.X402PermitCap)
	}

	sig := make(chan os.Signal, 1)
	signal.Notify(sig, syscall.SIGINT, syscall.SIGTERM)
	<-sig
	log.Println("shutting down...")
	cancel()
}

// alchemyConfig builds Alchemy client config from app config and chain registry.
func alchemyConfig(cfg *config.Config, chainRegistry *chains.Registry) alchemy.Config {
	c := alchemy.Config{
		APIKey:  cfg.AlchemyAPIKey,
		BaseURL: cfg.AlchemyBaseURL,
	}
	if cfg.AlchemyAPIKey == "" && cfg.AlchemyBaseURL == "" && cfg.EVM_RPC_URL != "" && strings.Contains(cfg.EVM_RPC_URL, "alchemy.com") {
		c.BaseURL = strings.TrimSuffix(cfg.EVM_RPC_URL, "/")
	}
	if chainRegistry != nil {
		c.ChainURLs = chainRegistry.ChainURLs("alchemy.com")
	}
	return c
}

// logX402Stats periodically fetches /v1/stats and logs total_spent_usd, total_tokens, and remaining budget.
func logX402Stats(ctx context.Context, client *x402client.Client, routerURL, permitCap string) {
	capUSD, _ := strconv.ParseFloat(permitCap, 64)
	logOnce := func() {
		stats, err := client.FetchRouterStats(ctx, routerURL)
		if err != nil {
			log.Printf("[x402] stats: %v", err)
			return
		}
		spentUSD, _ := strconv.ParseFloat(stats.TotalSpentUSD, 64)
		remaining := capUSD - spentUSD
		if remaining < 0 {
			remaining = 0
		}
		log.Printf("[x402] stats: total_spent_usd=%s total_tokens=%d remaining_usd≈%.2f",
			stats.TotalSpentUSD, stats.TotalTokens, remaining)
	}
	logOnce() // first run immediately
	ticker := time.NewTicker(5 * time.Minute)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			logOnce()
		}
	}
}
