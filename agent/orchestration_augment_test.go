package agent

import (
	"testing"

	"custom-agent/tools"
)

func TestAugmentWalletReadToolsAllowlist(t *testing.T) {
	ts := tools.NewTools("", nil)
	if out := augmentWalletReadToolsAllowlist([]string{"web_search"}, ts); len(out) != 1 {
		t.Fatalf("no wallet tools: want unchanged, got %v", out)
	}

	ts.SetWallet(&mockWalletService{})
	out := augmentWalletReadToolsAllowlist([]string{"wallet_execute_contract_call"}, ts)
	if len(out) != 3 {
		t.Fatalf("want 3 tools, got %d: %v", len(out), out)
	}
	seen := map[string]bool{}
	for _, n := range out {
		seen[n] = true
	}
	if !seen["wallet_get_balance"] || !seen["wallet_erc20_balance"] || !seen["wallet_execute_contract_call"] {
		t.Fatalf("missing expected tools: %v", out)
	}

	// Idempotent when reads already present
	already := []string{"wallet_get_balance", "wallet_erc20_balance", "wallet_execute_transfer"}
	if out := augmentWalletReadToolsAllowlist(already, ts); len(out) != len(already) {
		t.Fatalf("want same length, got %v", out)
	}
}
