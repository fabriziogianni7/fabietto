package config

import (
	"fmt"
	"os"
	"strconv"
	"strings"

	"github.com/joho/godotenv"
)

func parseInt(s string, defaultVal int) int {
	s = strings.TrimSpace(s)
	if s == "" {
		return defaultVal
	}
	n, err := strconv.Atoi(s)
	if err != nil {
		return defaultVal
	}
	return n
}

func parseInt64(s string, defaultVal int64) int64 {
	s = strings.TrimSpace(s)
	if s == "" {
		return defaultVal
	}
	n, err := strconv.ParseInt(s, 10, 64)
	if err != nil {
		return defaultVal
	}
	return n
}

func parseBool(s string) bool {
	s = strings.TrimSpace(strings.ToLower(s))
	return s == "1" || s == "true" || s == "yes"
}

// Config holds validated application configuration.
type Config struct {
	TelegramBotToken     string
	GroqAPIKey           string
	BraveSearchAPIKey    string
	DiscordToken         string // optional
	HTTPPort             string // optional, e.g. "5000"
	SignalCliURL         string // optional, signal-cli-rest-api URL
	SignalNumber         string // optional, bot's Signal number
	CompactionThreshold  int    // optional, token count to trigger compaction (default 4000)
	OllamaURL            string // optional, e.g. "http://localhost:11434" for embeddings
	OllamaEmbedModel     string // optional, e.g. "nomic-embed-text" (default)

	// Wallet (optional). If EVM_RPC_URL and signer are set, wallet tools are enabled.
	EVM_RPC_URL          string // e.g. "https://eth-mainnet.g.alchemy.com/v2/..."
	ChainID              int64  // e.g. 1 for mainnet (used when WALLET_CHAINS not set)
	WalletSignerBackend  string // "env" | "kms" | "hsm" (default: env)
	WalletPrivateKeyEnv  string // env var name for key (default: WALLET_PRIVATE_KEY)
	WalletAccountMode    string // "eoa" | "smart" (default: eoa)
	WalletNativeSpendLimit string // wei string for auto-allow threshold, "0" = require approval for all
	WalletApprovalDir    string // dir for approval persistence (optional)

	// Multichain: JSON array of {chain_id, rpc_url, explorer, name}. If empty, use EVM_RPC_URL+CHAIN_ID.
	WalletChainsJSON     string // e.g. [{"chain_id":1,"rpc_url":"...","explorer":"https://etherscan.io","name":"Ethereum"}]
	WalletDefaultChainID int64  // default chain when chain_id omitted (default: from first chain or CHAIN_ID)

	// Skills: directory for user-installed skills (OpenClaw-style SKILL.md folders). Default: ./skills-data
	SkillsDir string

	// Autonomous mode: use x402 router for LLM instead of Groq, require wallet for permits.
	AutonomousMode bool   // when true, use X402_ROUTER_URL for LLM, GROQ_API_KEY optional
	X402RouterURL  string // default https://ai.xgate.run/v1
	X402PermitCap  string // optional session spend cap in USDC (default "50")
}

// Load reads environment variables from .env (if present) and validates required values.
// Returns an error if any required variable is missing or invalid.
func Load() (*Config, error) {
	_ = godotenv.Load() // ignore error if .env doesn't exist

	cfg := &Config{
		TelegramBotToken:    strings.TrimSpace(os.Getenv("TELEGRAM_BOT_TOKEN")),
		GroqAPIKey:          strings.TrimSpace(os.Getenv("GROQ_API_KEY")),
		BraveSearchAPIKey:   strings.TrimSpace(os.Getenv("BRAVE_SEARCH_API_KEY")),
		DiscordToken:        strings.TrimSpace(os.Getenv("DISCORD_BOT_TOKEN")),
		HTTPPort:            strings.TrimSpace(os.Getenv("HTTP_PORT")),
		SignalCliURL:        strings.TrimSpace(os.Getenv("SIGNAL_CLI_URL")),
		SignalNumber:        strings.TrimSpace(os.Getenv("SIGNAL_NUMBER")),
		CompactionThreshold: parseInt(os.Getenv("CONTEXT_COMPACTION_THRESHOLD"), 4000),
		OllamaURL:        strings.TrimSpace(os.Getenv("OLLAMA_URL")),
		OllamaEmbedModel: strings.TrimSpace(os.Getenv("OLLAMA_EMBED_MODEL")),

		EVM_RPC_URL:         strings.TrimSpace(os.Getenv("EVM_RPC_URL")),
		ChainID:             parseInt64(os.Getenv("CHAIN_ID"), 1),
		WalletSignerBackend: strings.TrimSpace(os.Getenv("WALLET_SIGNER_BACKEND")),
		WalletPrivateKeyEnv: strings.TrimSpace(os.Getenv("WALLET_PRIVATE_KEY_ENV")),
		WalletAccountMode:   strings.TrimSpace(os.Getenv("WALLET_ACCOUNT_MODE")),
		WalletNativeSpendLimit: strings.TrimSpace(os.Getenv("WALLET_NATIVE_SPEND_LIMIT")),
		WalletApprovalDir:   strings.TrimSpace(os.Getenv("WALLET_APPROVAL_DIR")),
		WalletChainsJSON:    strings.TrimSpace(os.Getenv("WALLET_CHAINS")),
		WalletDefaultChainID: parseInt64(os.Getenv("WALLET_DEFAULT_CHAIN_ID"), 0),
		SkillsDir:           strings.TrimSpace(os.Getenv("SKILLS_DIR")),
		AutonomousMode:      parseBool(os.Getenv("AUTONOMOUS_MODE")),
		X402RouterURL:       strings.TrimSpace(os.Getenv("X402_ROUTER_URL")),
		X402PermitCap:       strings.TrimSpace(os.Getenv("X402_PERMIT_CAP")),
	}
	if cfg.SkillsDir == "" {
		cfg.SkillsDir = "./skills-data"
	}
	if cfg.X402RouterURL == "" {
		cfg.X402RouterURL = "https://ai.xgate.run/v1"
	}
	if cfg.X402PermitCap == "" {
		cfg.X402PermitCap = "50"
	}

	if err := cfg.validate(); err != nil {
		return nil, err
	}

	return cfg, nil
}

func (c *Config) validate() error {
	var missing []string

	if c.AutonomousMode {
		// Autonomous mode: require wallet for x402 permits, GROQ_API_KEY optional
		if c.EVM_RPC_URL == "" {
			missing = append(missing, "EVM_RPC_URL (required for autonomous mode)")
		}
		if c.WalletSignerBackend == "" {
			c.WalletSignerBackend = "env"
		}
		if c.WalletPrivateKeyEnv == "" {
			c.WalletPrivateKeyEnv = "WALLET_PRIVATE_KEY"
		}
		if c.WalletSignerBackend == "env" && os.Getenv(c.WalletPrivateKeyEnv) == "" {
			missing = append(missing, c.WalletPrivateKeyEnv+" (required for autonomous mode)")
		}
		if c.X402RouterURL == "" {
			missing = append(missing, "X402_ROUTER_URL or set default")
		}
	} else {
		// Non-autonomous: require Groq API key
		if c.GroqAPIKey == "" {
			missing = append(missing, "GROQ_API_KEY")
		}
	}

	if c.BraveSearchAPIKey == "" {
		missing = append(missing, "BRAVE_SEARCH_API_KEY")
	}

	// At least one gateway must be enabled
	if c.TelegramBotToken == "" && c.DiscordToken == "" && c.HTTPPort == "" &&
		!(c.SignalCliURL != "" && c.SignalNumber != "") {
		missing = append(missing, "TELEGRAM_BOT_TOKEN, DISCORD_BOT_TOKEN, HTTP_PORT, or SIGNAL_CLI_URL+SIGNAL_NUMBER (at least one)")
	}

	// Signal requires both URL and number
	if (c.SignalCliURL != "" && c.SignalNumber == "") || (c.SignalCliURL == "" && c.SignalNumber != "") {
		missing = append(missing, "SIGNAL_CLI_URL and SIGNAL_NUMBER must be set together")
	}

	// Wallet: if EVM_RPC_URL set, require WALLET_PRIVATE_KEY (or backend-specific key)
	if c.EVM_RPC_URL != "" {
		if c.WalletSignerBackend == "" {
			c.WalletSignerBackend = "env"
		}
		if c.WalletPrivateKeyEnv == "" {
			c.WalletPrivateKeyEnv = "WALLET_PRIVATE_KEY"
		}
		if c.WalletAccountMode == "" {
			c.WalletAccountMode = "eoa"
		}
	}

	if len(missing) > 0 {
		return fmt.Errorf("missing required environment variables: %s (set them in .env or export)", strings.Join(missing, ", "))
	}

	return nil
}

// WalletEnabled returns true if wallet should be initialized (RPC URL and signer key set).
func (c *Config) WalletEnabled() bool {
	if c.EVM_RPC_URL == "" {
		return false
	}
	if c.WalletSignerBackend == "" {
		c.WalletSignerBackend = "env"
	}
	if c.WalletPrivateKeyEnv == "" {
		c.WalletPrivateKeyEnv = "WALLET_PRIVATE_KEY"
	}
	// For env backend, require the key to be set
	if c.WalletSignerBackend == "env" {
		return os.Getenv(c.WalletPrivateKeyEnv) != ""
	}
	return true
}
