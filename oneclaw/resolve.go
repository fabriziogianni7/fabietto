package oneclaw

import (
	"context"
	"os"
	"strings"

	sdk "github.com/1clawAI/1claw-go-sdk"

	"custom-agent/config"
)

// Vault path convention for secrets.
const (
	PathGroqAPIKey        = "GROQ_API_KEY"
	PathBraveSearchAPIKey = "BRAVE_SEARCH_API_KEY"
	PathTelegramBotToken  = "TELEGRAM_BOT_TOKEN"
	PathDiscordBotToken   = "DISCORD_BOT_TOKEN"
	PathWalletPrivateKey  = "WALLET_PRIVATE_KEY"
)

// SecretGetter fetches a secret by path. Used for testing.
type SecretGetter interface {
	Get(ctx context.Context, vaultID, path string) (string, error)
}

// ResolveConfig fills missing config values from the 1claw vault.
// Env vars take precedence; only empty fields are filled from the vault.
// No-op if 1claw is not configured (OneClawAPIKey and OneClawVaultID empty).
// For tests, pass a SecretGetter via ResolveConfigWithGetter.
func ResolveConfig(ctx context.Context, cfg *config.Config) error {
	return ResolveConfigWithGetter(ctx, cfg, nil)
}

// ResolveConfigWithGetter is like ResolveConfig but accepts an optional SecretGetter for testing.
// If getter is nil, a real 1claw client is created.
func ResolveConfigWithGetter(ctx context.Context, cfg *config.Config, getter SecretGetter) error {
	if !cfg.OneClawEnabled() {
		return nil
	}

	var g SecretGetter
	if getter != nil {
		g = getter
	} else {
		opts := []sdk.Option{
			sdk.WithAPIKey(cfg.OneClawAPIKey),
		}
		if cfg.OneClawBaseURL != "" {
			opts = append(opts, sdk.WithBaseURL(cfg.OneClawBaseURL))
		}
		if cfg.OneClawAgentID != "" {
			opts = append(opts, sdk.WithAgentID(cfg.OneClawAgentID))
		}
		client, err := sdk.New(opts...)
		if err != nil {
			return err
		}
		g = &sdkSecretGetter{client: client}
	}

	vaultID := cfg.OneClawVaultID

	type fill struct {
		path string
		set  func(string)
	}
	fills := []fill{
		{PathGroqAPIKey, func(v string) {
			if cfg.GroqAPIKey == "" {
				cfg.GroqAPIKey = v
			}
		}},
		{PathBraveSearchAPIKey, func(v string) {
			if cfg.BraveSearchAPIKey == "" {
				cfg.BraveSearchAPIKey = v
			}
		}},
		{PathTelegramBotToken, func(v string) {
			if cfg.TelegramBotToken == "" {
				cfg.TelegramBotToken = v
			}
		}},
		{PathDiscordBotToken, func(v string) {
			if cfg.DiscordToken == "" {
				cfg.DiscordToken = v
			}
		}},
		{PathWalletPrivateKey, func(v string) {
			envKey := cfg.WalletPrivateKeyEnv
			if envKey == "" {
				envKey = "WALLET_PRIVATE_KEY"
			}
			if os.Getenv(envKey) == "" {
				_ = os.Setenv(envKey, v)
			}
		}},
	}

	for _, f := range fills {
		v, err := g.Get(ctx, vaultID, f.path)
		if err != nil {
			continue
		}
		v = strings.TrimSpace(v)
		if v != "" {
			f.set(v)
		}
	}

	return nil
}

type sdkSecretGetter struct {
	client *sdk.Client
}

func (s *sdkSecretGetter) Get(ctx context.Context, vaultID, path string) (string, error) {
	secret, err := s.client.Secrets.Get(ctx, vaultID, path)
	if err != nil {
		return "", err
	}
	return secret.Value, nil
}
