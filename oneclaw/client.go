package oneclaw

import (
	"errors"

	sdk "github.com/1clawAI/1claw-go-sdk"

	"custom-agent/config"
)

// NewClient creates a 1claw SDK client from config.
// Requires cfg.OneClawEnabled() to be true.
func NewClient(cfg *config.Config) (*sdk.Client, error) {
	if !cfg.OneClawEnabled() {
		return nil, errors.New("1claw: not configured (API key and vault ID required)")
	}
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
