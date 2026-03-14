package oneclaw

import (
	"errors"

	sdk "github.com/1clawAI/1claw-go-sdk"

	"custom-agent/config"
)

// NewClient creates a 1claw SDK client from config.
// Requires cfg.OneClawEnabled() to be true (API key and vault ID).
func NewClient(cfg *config.Config) (*sdk.Client, error) {
	if !cfg.OneClawEnabled() {
		return nil, errors.New("1claw: not configured (API key and vault ID required)")
	}
	return newClientFromConfig(cfg)
}

// NewClientForIntents creates a 1claw SDK client for the Intents API.
// Requires API key and agent ID (vault ID not needed for transactions).
func NewClientForIntents(cfg *config.Config) (*sdk.Client, error) {
	if cfg.OneClawAPIKey == "" || cfg.OneClawAgentID == "" {
		return nil, errors.New("1claw: intents requires 1CLAW_API_KEY and 1CLAW_AGENT_ID")
	}
	return newClientFromConfig(cfg)
}

func newClientFromConfig(cfg *config.Config) (*sdk.Client, error) {
	opts := []sdk.Option{
		sdk.WithAPIKey(cfg.OneClawAPIKey),
	}
	if cfg.OneClawBaseURL != "" {
		opts = append(opts, sdk.WithBaseURL(cfg.OneClawBaseURL))
	}
	if cfg.OneClawAgentID != "" {
		opts = append(opts, sdk.WithAgentID(cfg.OneClawAgentID))
	}
	return sdk.New(opts...)
}
