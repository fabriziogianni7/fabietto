package oneclaw

import (
	"context"
	"errors"
	"os"
	"testing"

	"custom-agent/config"
)

type mockGetter struct {
	secrets map[string]string
	err    error
}

func (m *mockGetter) Get(ctx context.Context, vaultID, path string) (string, error) {
	if m.err != nil {
		return "", m.err
	}
	if v, ok := m.secrets[path]; ok {
		return v, nil
	}
	return "", errors.New("not found")
}

func TestResolveConfig_NoOpWhen1ClawDisabled(t *testing.T) {
	cfg := &config.Config{
		OneClawAPIKey:  "",
		OneClawVaultID: "",
	}
	// Mock getter would panic if called; with 1claw disabled it should never be called
	err := ResolveConfigWithGetter(context.Background(), cfg, &mockGetter{secrets: map[string]string{"GROQ_API_KEY": "from-vault"}})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.GroqAPIKey != "" {
		t.Errorf("expected GroqAPIKey empty when 1claw disabled, got %q", cfg.GroqAPIKey)
	}
}

func TestResolveConfig_EnvOverridesVault(t *testing.T) {
	cfg := &config.Config{
		OneClawAPIKey:  "ocv_test",
		OneClawVaultID: "vault-1",
		GroqAPIKey:     "from-env",
	}
	getter := &mockGetter{
		secrets: map[string]string{
			PathGroqAPIKey: "from-vault",
		},
	}
	err := ResolveConfigWithGetter(context.Background(), cfg, getter)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.GroqAPIKey != "from-env" {
		t.Errorf("expected env to override vault, got GroqAPIKey=%q", cfg.GroqAPIKey)
	}
}

func TestResolveConfig_EmptyFieldFilledFromVault(t *testing.T) {
	cfg := &config.Config{
		OneClawAPIKey:     "ocv_test",
		OneClawVaultID:    "vault-1",
		GroqAPIKey:        "",
		BraveSearchAPIKey: "",
	}
	getter := &mockGetter{
		secrets: map[string]string{
			PathGroqAPIKey:        "groq-from-vault",
			PathBraveSearchAPIKey: "brave-from-vault",
		},
	}
	err := ResolveConfigWithGetter(context.Background(), cfg, getter)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.GroqAPIKey != "groq-from-vault" {
		t.Errorf("expected GroqAPIKey from vault, got %q", cfg.GroqAPIKey)
	}
	if cfg.BraveSearchAPIKey != "brave-from-vault" {
		t.Errorf("expected BraveSearchAPIKey from vault, got %q", cfg.BraveSearchAPIKey)
	}
}

func TestResolveConfig_MissingVaultSecretLeavesFieldEmpty(t *testing.T) {
	cfg := &config.Config{
		OneClawAPIKey:     "ocv_test",
		OneClawVaultID:    "vault-1",
		GroqAPIKey:        "",
		BraveSearchAPIKey: "",
	}
	getter := &mockGetter{
		secrets: map[string]string{
			PathGroqAPIKey: "groq-from-vault",
			// BraveSearchAPIKey not in vault
		},
	}
	err := ResolveConfigWithGetter(context.Background(), cfg, getter)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.GroqAPIKey != "groq-from-vault" {
		t.Errorf("expected GroqAPIKey from vault, got %q", cfg.GroqAPIKey)
	}
	if cfg.BraveSearchAPIKey != "" {
		t.Errorf("expected BraveSearchAPIKey empty when missing from vault, got %q", cfg.BraveSearchAPIKey)
	}
}

func TestResolveConfig_WalletPrivateKeySetInEnv(t *testing.T) {
	envKey := "WALLET_PRIVATE_KEY_TEST"
	os.Unsetenv(envKey)
	defer os.Unsetenv(envKey)

	cfg := &config.Config{
		OneClawAPIKey:       "ocv_test",
		OneClawVaultID:      "vault-1",
		WalletPrivateKeyEnv: envKey,
	}
	getter := &mockGetter{
		secrets: map[string]string{
			PathWalletPrivateKey: "0x1234abcd",
		},
	}
	err := ResolveConfigWithGetter(context.Background(), cfg, getter)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got := os.Getenv(envKey); got != "0x1234abcd" {
		t.Errorf("expected WALLET_PRIVATE_KEY in env, got %q", got)
	}
}

func TestResolveConfig_WalletPrivateKeyEnvNotOverwrittenIfSet(t *testing.T) {
	envKey := "WALLET_PRIVATE_KEY_TEST"
	os.Setenv(envKey, "already-set")
	defer os.Unsetenv(envKey)

	cfg := &config.Config{
		OneClawAPIKey:       "ocv_test",
		OneClawVaultID:      "vault-1",
		WalletPrivateKeyEnv: envKey,
	}
	getter := &mockGetter{
		secrets: map[string]string{
			PathWalletPrivateKey: "0x1234abcd",
		},
	}
	err := ResolveConfigWithGetter(context.Background(), cfg, getter)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got := os.Getenv(envKey); got != "already-set" {
		t.Errorf("expected env to override vault, got %q", got)
	}
}
