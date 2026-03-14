package signer

import (
	"fmt"
	"strings"

	sdk "github.com/1clawAI/1claw-go-sdk"
)

// Backend identifies the signer implementation.
const (
	BackendEnv     = "env"
	BackendOneClaw = "1claw"
	BackendKMS     = "kms"
	BackendHSM     = "hsm"
)

// BackendOption is an optional parameter for NewFromBackend.
type BackendOption func(*backendOptions)

type backendOptions struct {
	oneclawClient *sdk.Client
}

// WithOneClawClient passes the 1claw SDK client for the "1claw" backend.
func WithOneClawClient(client *sdk.Client) BackendOption {
	return func(o *backendOptions) {
		o.oneclawClient = client
	}
}

// NewFromBackend creates a Signer from the given backend name and options.
// backend: "env", "1claw", "kms", "hsm"
// opts: backend-specific. For "env": opts["env_key"] (default "WALLET_PRIVATE_KEY").
//       For "1claw": opts["vault_id"], opts["key_path"] (default "WALLET_PRIVATE_KEY"), plus WithOneClawClient.
//       For "kms"/"hsm": key ID, region, etc. (to be implemented).
func NewFromBackend(backend string, opts map[string]string, options ...BackendOption) (Signer, error) {
	b := strings.ToLower(strings.TrimSpace(backend))
	if b == "" {
		b = BackendEnv
	}
	var bo backendOptions
	for _, opt := range options {
		opt(&bo)
	}
	switch b {
	case BackendEnv:
		envKey := opts["env_key"]
		return NewEnvSigner(envKey)
	case BackendOneClaw:
		vaultID := opts["vault_id"]
		keyPath := opts["key_path"]
		if keyPath == "" {
			keyPath = "WALLET_PRIVATE_KEY"
		}
		if bo.oneclawClient == nil {
			return nil, fmt.Errorf("wallet: 1claw backend requires WithOneClawClient")
		}
		return NewOneClawSigner(bo.oneclawClient, vaultID, keyPath)
	case BackendKMS, BackendHSM:
		return nil, fmt.Errorf("wallet: %s signer not yet implemented; use env backend", b)
	default:
		return nil, fmt.Errorf("wallet: unknown signer backend %q", backend)
	}
}
