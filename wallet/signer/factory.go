package signer

import (
	"fmt"
	"strings"
)

// Backend identifies the signer implementation.
const (
	BackendEnv = "env"
	BackendKMS = "kms"
	BackendHSM = "hsm"
)

// NewFromBackend creates a Signer from the given backend name and options.
// backend: "env", "kms", "hsm"
// opts: backend-specific. For "env": opts["env_key"] (default "WALLET_PRIVATE_KEY").
//       For "kms"/"hsm": key ID, region, etc. (to be implemented).
func NewFromBackend(backend string, opts map[string]string) (Signer, error) {
	b := strings.ToLower(strings.TrimSpace(backend))
	if b == "" {
		b = BackendEnv
	}
	switch b {
	case BackendEnv:
		envKey := opts["env_key"]
		return NewEnvSigner(envKey)
	case BackendKMS, BackendHSM:
		return nil, fmt.Errorf("wallet: %s signer not yet implemented; use env backend", b)
	default:
		return nil, fmt.Errorf("wallet: unknown signer backend %q", backend)
	}
}
