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
	"custom-agent/config"
	"custom-agent/conversation"
	"custom-agent/oneclaw"
	"custom-agent/embedding"
	"custom-agent/gateway"
	"custom-agent/memory"
	"custom-agent/reminders"
	"custom-agent/sessionqueue"
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
	cfg := config.LoadFromEnv()
	if err := oneclaw.ResolveConfig(context.Background(), cfg); err != nil {
		log.Fatalf("1claw resolve: %v", err)
	}
	if err := config.Validate(cfg); err != nil {
		log.Fatalf("config: %v", err)
	}

	personality, err := os.ReadFile("PERSONALITY.md")
	if err != nil {
		log.Fatalf("failed to load PERSONALITY.md: %v", err)
	}
	toolInstruction := "\n\nYou have access to tools. Use them when they help answer the user's question—for example, read files, run commands, search the web, use memory (save_memory, read_memory), schedule reminders (create_scheduled_reminder, list_reminders, delete_reminder), spawn parallel sub-agents (spawn_subagents), or http_request for HTTP APIs. When a task can be parallelized, use spawn_subagents."
	if cfg.WalletEnabled() {
		toolInstruction += " When the wallet is configured, you can use wallet_get_balance, wallet_execute_transfer, wallet_execute_contract_call, and wallet_list_transactions. You MUST call wallet_execute_transfer or wallet_execute_contract_call to send—never claim a transaction was sent without invoking the tool. Transactions may require user approval; reply with approve: <tx_id> when prompted. With wallet enabled, http_request can automatically pay for x402-protected APIs (402 Payment Required)."
	}
	toolInstruction += "\n"

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
		var sgn signer.Signer
		if cfg.WalletSignerBackend == "1claw" {
			oneClawClient, err := oneclaw.NewClient(cfg)
			if err != nil {
				log.Fatalf("1claw client: %v", err)
			}
			sgn, err = signer.NewFromBackend(cfg.WalletSignerBackend, map[string]string{
				"vault_id": cfg.OneClawVaultID,
				"key_path": oneclaw.PathWalletPrivateKey,
			}, signer.WithOneClawClient(oneClawClient))
		} else {
			sgn, err = signer.NewFromBackend(cfg.WalletSignerBackend, map[string]string{"env_key": cfg.WalletPrivateKeyEnv})
		}
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

	a := agent.New(llm, systemPrompt, cfg.CompactionThreshold, toolSet, convStore)

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
