package main

import (
	"context"
	"os"
	"testing"

	"custom-agent/config"
	"custom-agent/oneclaw"
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

	cfg := config.LoadFromEnv()
	if err := oneclaw.ResolveConfig(context.Background(), cfg); err != nil {
		t.Fatalf("ResolveConfig: %v", err)
	}
	if err := config.Validate(cfg); err != nil {
		t.Fatalf("Validate: %v", err)
	}
	if cfg.GroqAPIKey != "gk" || cfg.TelegramBotToken != "tg" {
		t.Errorf("config not loaded: Groq=%q Telegram=%q", cfg.GroqAPIKey, cfg.TelegramBotToken)
	}
}

func TestLoadConfig_With1ClawEnvButNoResolver(t *testing.T) {
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

	cfg := config.LoadFromEnv()
	if err := oneclaw.ResolveConfig(context.Background(), cfg); err != nil {
		t.Fatalf("ResolveConfig: %v", err)
	}
	if err := config.Validate(cfg); err != nil {
		t.Fatalf("Validate: %v", err)
	}
	if cfg.OneClawEnabled() {
		t.Error("expected OneClawEnabled false when 1claw env not set")
	}
}
