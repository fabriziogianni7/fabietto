package policy

import (
	"math/big"
	"testing"

	"custom-agent/wallet/account"

	"github.com/ethereum/go-ethereum/common"
)

func TestEngine_DenyBlockedMethods(t *testing.T) {
	e := NewEngine(DefaultConfig())
	action := &account.Action{Type: "contract_call", Method: "approve", To: common.Address{}}
	if got := e.Evaluate(action); got != Deny {
		t.Errorf("approve should be denied, got %v", got)
	}
}

func TestEngine_AllowTransferUnderLimit(t *testing.T) {
	cfg := DefaultConfig()
	cfg.NativeSpendLimitWei = big.NewInt(1e18)
	e := NewEngine(cfg)
	action := &account.Action{Type: "transfer", Value: big.NewInt(1e17), To: common.Address{}}
	if got := e.Evaluate(action); got != Allow {
		t.Errorf("small transfer should be allowed, got %v", got)
	}
}

func TestEngine_RequireApprovalOverLimit(t *testing.T) {
	cfg := DefaultConfig()
	cfg.NativeSpendLimitWei = big.NewInt(1e18)
	e := NewEngine(cfg)
	action := &account.Action{Type: "transfer", Value: big.NewInt(2e18), To: common.Address{}}
	if got := e.Evaluate(action); got != RequireApproval {
		t.Errorf("large transfer should require approval, got %v", got)
	}
}
