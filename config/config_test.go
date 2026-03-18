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
	os.Setenv("EVM_RPC_URL", "https://eth.llamarpc.com")
	os.Setenv("WALLET_PRIVATE_KEY", "0x0000000000000000000000000000000000000000000000000000000000000001")
	os.Unsetenv("GROQ_API_KEY")
	defer func() {
		os.Unsetenv("AUTONOMOUS_MODE")
		os.Unsetenv("TELEGRAM_BOT_TOKEN")
		os.Unsetenv("BRAVE_SEARCH_API_KEY")
		os.Unsetenv("EVM_RPC_URL")
		os.Unsetenv("WALLET_PRIVATE_KEY")
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

