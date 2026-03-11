package policy

import (
	"math/big"
	"strings"

	"custom-agent/wallet/account"
)

// Decision is the policy result for an action.
type Decision int

const (
	Allow Decision = iota
	RequireApproval
	Deny
)

func (d Decision) String() string {
	switch d {
	case Allow:
		return "allow"
	case RequireApproval:
		return "require_approval"
	case Deny:
		return "deny"
	default:
		return "unknown"
	}
}

// Config holds policy limits and rules.
type Config struct {
	// NativeSpendLimitWei: auto-allow native transfers below this; above requires approval.
	NativeSpendLimitWei *big.Int
	// TokenSpendLimitWei: same for ERC20 transfers (value in token units, or 0 to use raw).
	TokenSpendLimitWei *big.Int
	// BlockedMethods: contract methods that are always denied (e.g. "approve", "transferFrom" with unlimited).
	BlockedMethods []string
	// AllowedMethods: if non-empty, only these methods are allowed (allowlist).
	AllowedMethods []string
	// RequireApprovalMethods: methods that always require approval.
	RequireApprovalMethods []string
}

// DefaultConfig returns a conservative default policy.
func DefaultConfig() *Config {
	return &Config{
		NativeSpendLimitWei:    big.NewInt(0),
		TokenSpendLimitWei:    big.NewInt(0),
		BlockedMethods:        []string{"approve", "increaseAllowance", "permit"},
		RequireApprovalMethods: []string{"transfer", "transferFrom", "mint", "burn"},
	}
}

// Engine evaluates policy for actions.
type Engine struct {
	cfg *Config
}

// NewEngine creates a policy engine with the given config.
func NewEngine(cfg *Config) *Engine {
	if cfg == nil {
		cfg = DefaultConfig()
	}
	return &Engine{cfg: cfg}
}

// Evaluate returns the decision for the action.
func (e *Engine) Evaluate(action *account.Action) Decision {
	// Blocked methods
	for _, m := range e.cfg.BlockedMethods {
		if strings.EqualFold(action.Method, m) {
			return Deny
		}
	}
	// Allowlist: if set, only these methods allowed
	if len(e.cfg.AllowedMethods) > 0 {
		found := false
		for _, m := range e.cfg.AllowedMethods {
			if strings.EqualFold(action.Method, m) {
				found = true
				break
			}
		}
		if !found && action.Type == "contract_call" {
			return Deny
		}
	}
	// Require approval for specific methods
	for _, m := range e.cfg.RequireApprovalMethods {
		if strings.EqualFold(action.Method, m) {
			return RequireApproval
		}
	}
	// Transfer: check native spend limit
	if action.Type == "transfer" && action.Value != nil {
		if e.cfg.NativeSpendLimitWei != nil && e.cfg.NativeSpendLimitWei.Sign() > 0 {
			if action.Value.Cmp(e.cfg.NativeSpendLimitWei) > 0 {
				return RequireApproval
			}
		}
		return Allow
	}
	// Contract call with value
	if action.Value != nil && action.Value.Sign() > 0 {
		if e.cfg.NativeSpendLimitWei != nil && e.cfg.NativeSpendLimitWei.Sign() > 0 {
			if action.Value.Cmp(e.cfg.NativeSpendLimitWei) > 0 {
				return RequireApproval
			}
		}
	}
	return Allow
}
