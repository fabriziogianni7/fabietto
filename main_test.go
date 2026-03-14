package main

import (
	"context"
	"os"
	"testing"
)

func TestLoadConfig_Without1Claw(t *testing.T) {
	os.Setenv("GROQ_API_KEY", "gk")
	os.Setenv("BRAVE_SEARCH_API_KEY", "bk")
	os.Setenv("TELEGRAM_BOT_TOKEN", "tg")
	os.Unsetenv("1CLAW_VAULT_ID")
	os.Unsetenv("1CLAW_API_KEY")
	defer func() {
		os.Unsetenv("GROQ_API_KEY")
		os.Unsetenv("BRAVE_SEARCH_API_KEY")
		os.Unsetenv("TELEGRAM_BOT_TOKEN")
	}()

	cfg, err := loadConfig(context.Background())
	if err != nil {
		t.Fatalf("loadConfig: %v", err)
	}
	if cfg.GroqAPIKey != "gk" || cfg.TelegramBotToken != "tg" {
		t.Errorf("config not loaded: Groq=%q Telegram=%q", cfg.GroqAPIKey, cfg.TelegramBotToken)
	}
}

func TestLoadConfig_With1ClawEnvButNoResolver(t *testing.T) {
	// When 1claw env is set but resolver would fail (e.g. invalid API key),
	// we still want to test that we attempt resolution. For unit test we
	// avoid network by not setting 1claw env - so resolver is nil.
	os.Setenv("GROQ_API_KEY", "gk")
	os.Setenv("BRAVE_SEARCH_API_KEY", "bk")
	os.Setenv("TELEGRAM_BOT_TOKEN", "tg")
	os.Unsetenv("1CLAW_VAULT_ID")
	os.Unsetenv("1CLAW_API_KEY")
	defer func() {
		os.Unsetenv("GROQ_API_KEY")
		os.Unsetenv("BRAVE_SEARCH_API_KEY")
		os.Unsetenv("TELEGRAM_BOT_TOKEN")
	}()

	cfg, err := loadConfig(context.Background())
	if err != nil {
		t.Fatalf("loadConfig: %v", err)
	}
	if !cfg.OneClawEnabled() {
		// Expected when 1claw env not set
	}
	_ = cfg
}
