package config

import (
	"os"
	"strings"
	"testing"
)

func TestLoad_AutonomousMode_RequiresWallet(t *testing.T) {
	os.Setenv("AUTONOMOUS_MODE", "1")
	os.Setenv("TELEGRAM_BOT_TOKEN", "test")
	os.Setenv("BRAVE_SEARCH_API_KEY", "test")
	os.Unsetenv("GROQ_API_KEY")
	os.Unsetenv("EVM_RPC_URL")
	os.Unsetenv("WALLET_PRIVATE_KEY")
	defer func() {
		os.Unsetenv("AUTONOMOUS_MODE")
		os.Unsetenv("TELEGRAM_BOT_TOKEN")
		os.Unsetenv("BRAVE_SEARCH_API_KEY")
	}()

	_, err := Load()
	if err == nil {
		t.Error("expected error when autonomous mode without wallet")
	}
	if err != nil && !strings.Contains(err.Error(), "EVM_RPC_URL") && !strings.Contains(err.Error(), "WALLET_PRIVATE_KEY") {
		t.Errorf("expected wallet-related error, got: %v", err)
	}
}

func TestLoad_AutonomousMode_SucceedsWithWallet(t *testing.T) {
	os.Setenv("AUTONOMOUS_MODE", "1")
	os.Setenv("TELEGRAM_BOT_TOKEN", "test")
	os.Setenv("BRAVE_SEARCH_API_KEY", "test")
	os.Setenv("EVM_RPC_URL", "https://eth-mainnet.g.alchemy.com/v2/test")
	os.Setenv("WALLET_PRIVATE_KEY", "0x0000000000000000000000000000000000000000000000000000000000000001")
	os.Setenv("WALLET_CHAINS", `[{"chain_id":8453,"rpc_url":"https://base-mainnet.g.alchemy.com/v2/test","explorer":"https://basescan.org","name":"Base"}]`)
	os.Unsetenv("GROQ_API_KEY")
	defer func() {
		os.Unsetenv("AUTONOMOUS_MODE")
		os.Unsetenv("TELEGRAM_BOT_TOKEN")
		os.Unsetenv("BRAVE_SEARCH_API_KEY")
		os.Unsetenv("EVM_RPC_URL")
		os.Unsetenv("WALLET_PRIVATE_KEY")
		os.Unsetenv("WALLET_CHAINS")
	}()

	cfg, err := Load()
	if err != nil {
		t.Fatalf("autonomous mode with wallet should succeed: %v", err)
	}
	if !cfg.AutonomousMode {
		t.Error("expected AutonomousMode true")
	}
	if cfg.X402RouterURL == "" {
		t.Error("expected default X402RouterURL")
	}
	if cfg.X402PermitCap != "50" {
		t.Errorf("expected default X402PermitCap 50, got %q", cfg.X402PermitCap)
	}
	if cfg.X402MinBaseUSDC != "10" {
		t.Errorf("expected default X402MinBaseUSDC 10, got %q", cfg.X402MinBaseUSDC)
	}
}

func TestLoad_NonAutonomous_RequiresGroq(t *testing.T) {
	os.Unsetenv("AUTONOMOUS_MODE")
	os.Setenv("TELEGRAM_BOT_TOKEN", "test")
	os.Setenv("BRAVE_SEARCH_API_KEY", "test")
	os.Unsetenv("GROQ_API_KEY")
	defer func() {
		os.Unsetenv("TELEGRAM_BOT_TOKEN")
		os.Unsetenv("BRAVE_SEARCH_API_KEY")
	}()

	_, err := Load()
	if err == nil {
		t.Error("expected error when non-autonomous without GROQ_API_KEY")
	}
	if err != nil && !strings.Contains(err.Error(), "GROQ_API_KEY") {
		t.Errorf("expected GROQ_API_KEY error, got: %v", err)
	}
}

func TestLoad_OpportunityScan_RequiresTelegramOwnerWhenIntervalSet(t *testing.T) {
	os.Setenv("AUTONOMOUS_MODE", "1")
	os.Setenv("TELEGRAM_BOT_TOKEN", "test")
	os.Setenv("BRAVE_SEARCH_API_KEY", "test")
	os.Setenv("EVM_RPC_URL", "https://eth-mainnet.g.alchemy.com/v2/test")
	os.Setenv("WALLET_PRIVATE_KEY", "0x0000000000000000000000000000000000000000000000000000000000000001")
	os.Setenv("WALLET_CHAINS", `[{"chain_id":8453,"rpc_url":"https://base-mainnet.g.alchemy.com/v2/test","explorer":"https://basescan.org","name":"Base"}]`)
	os.Setenv("OPPORTUNITY_SCAN_INTERVAL_MINUTES", "15")
	os.Unsetenv("TELEGRAM_OWNER_CHAT_ID")
	defer func() {
		os.Unsetenv("AUTONOMOUS_MODE")
		os.Unsetenv("TELEGRAM_BOT_TOKEN")
		os.Unsetenv("BRAVE_SEARCH_API_KEY")
		os.Unsetenv("EVM_RPC_URL")
		os.Unsetenv("WALLET_PRIVATE_KEY")
		os.Unsetenv("WALLET_CHAINS")
		os.Unsetenv("OPPORTUNITY_SCAN_INTERVAL_MINUTES")
	}()

	_, err := Load()
	if err == nil {
		t.Error("expected error when opportunity scan interval set without TELEGRAM_OWNER_CHAT_ID")
	}
	if err != nil && !strings.Contains(err.Error(), "TELEGRAM_OWNER_CHAT_ID") {
		t.Errorf("expected TELEGRAM_OWNER_CHAT_ID error, got: %v", err)
	}
}

func TestLoad_OpportunityScan_SucceedsWithTelegramOwner(t *testing.T) {
	os.Setenv("AUTONOMOUS_MODE", "1")
	os.Setenv("TELEGRAM_BOT_TOKEN", "test")
	os.Setenv("BRAVE_SEARCH_API_KEY", "test")
	os.Setenv("EVM_RPC_URL", "https://eth-mainnet.g.alchemy.com/v2/test")
	os.Setenv("WALLET_PRIVATE_KEY", "0x0000000000000000000000000000000000000000000000000000000000000001")
	os.Setenv("WALLET_CHAINS", `[{"chain_id":8453,"rpc_url":"https://base-mainnet.g.alchemy.com/v2/test","explorer":"https://basescan.org","name":"Base"}]`)
	os.Setenv("OPPORTUNITY_SCAN_INTERVAL_MINUTES", "15")
	os.Setenv("TELEGRAM_OWNER_CHAT_ID", "123456")
	defer func() {
		os.Unsetenv("AUTONOMOUS_MODE")
		os.Unsetenv("TELEGRAM_BOT_TOKEN")
		os.Unsetenv("BRAVE_SEARCH_API_KEY")
		os.Unsetenv("EVM_RPC_URL")
		os.Unsetenv("WALLET_PRIVATE_KEY")
		os.Unsetenv("WALLET_CHAINS")
		os.Unsetenv("OPPORTUNITY_SCAN_INTERVAL_MINUTES")
		os.Unsetenv("TELEGRAM_OWNER_CHAT_ID")
	}()

	cfg, err := Load()
	if err != nil {
		t.Fatalf("opportunity scan with TELEGRAM_OWNER_CHAT_ID should succeed: %v", err)
	}
	if cfg.OpportunityScanIntervalMinutes != 15 {
		t.Errorf("expected interval 15, got %d", cfg.OpportunityScanIntervalMinutes)
	}
	if cfg.TelegramOwnerChatID != "123456" {
		t.Errorf("expected TelegramOwnerChatID 123456, got %q", cfg.TelegramOwnerChatID)
	}
}

func TestParseBool(t *testing.T) {
	for _, s := range []string{"1", "true", "yes", "TRUE", "Yes"} {
		if !parseBool(s) {
			t.Errorf("parseBool(%q) = false, want true", s)
		}
	}
	for _, s := range []string{"0", "false", "no", "", "x"} {
		if parseBool(s) {
			t.Errorf("parseBool(%q) = true, want false", s)
		}
	}
}

func TestAlchemyEnabled(t *testing.T) {
	os.Setenv("TELEGRAM_BOT_TOKEN", "test")
	os.Setenv("BRAVE_SEARCH_API_KEY", "test")
	os.Setenv("GROQ_API_KEY", "test")
	defer func() {
		os.Unsetenv("TELEGRAM_BOT_TOKEN")
		os.Unsetenv("BRAVE_SEARCH_API_KEY")
		os.Unsetenv("GROQ_API_KEY")
		os.Unsetenv("ALCHEMY_API_KEY")
		os.Unsetenv("ALCHEMY_BASE_URL")
		os.Unsetenv("EVM_RPC_URL")
	}()

	// No Alchemy config
	os.Unsetenv("ALCHEMY_API_KEY")
	os.Unsetenv("ALCHEMY_BASE_URL")
	os.Unsetenv("EVM_RPC_URL")
	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.AlchemyEnabled() {
		t.Error("AlchemyEnabled should be false when no Alchemy config")
	}

	// ALCHEMY_API_KEY set
	os.Setenv("ALCHEMY_API_KEY", "test-key")
	cfg, err = Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if !cfg.AlchemyEnabled() {
		t.Error("AlchemyEnabled should be true when ALCHEMY_API_KEY set")
	}
	os.Unsetenv("ALCHEMY_API_KEY")

	// EVM_RPC_URL with alchemy.com
	os.Setenv("EVM_RPC_URL", "https://eth-mainnet.g.alchemy.com/v2/abc")
	cfg, err = Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if !cfg.AlchemyEnabled() {
		t.Error("AlchemyEnabled should be true when EVM_RPC_URL contains alchemy.com")
	}
}
