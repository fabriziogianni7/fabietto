package planning

import (
	"fmt"
	"strings"
)

// WalletTxTemplate returns a canonical wallet contract-call plan (steps only; goal/constraints set by caller).
func WalletTxTemplate(goal string, constraints map[string]string) *Plan {
	if constraints == nil {
		constraints = make(map[string]string)
	}
	steps := []Step{
		{ID: "s1", Type: StepParseIntent, Description: "Normalize to, data, value, chain from intent", AllowedTools: []string{}, Status: StepPending},
		{ID: "s2", Type: StepPolicyCheck, Description: "Policy preview and gas estimate", AllowedTools: []string{}, Status: StepPending},
		{ID: "s3", Type: StepQuoteRoute, Description: "Route/quote (optional; skipped if not configured)", AllowedTools: []string{}, Status: StepPending},
		{ID: "s4", Type: StepBuildTransaction, Description: "Finalize calldata and gas limit", AllowedTools: []string{}, Status: StepPending},
		{ID: "s5", Type: StepSimulate, Description: "eth_call preflight", AllowedTools: []string{}, Status: StepPending},
		{ID: "s6", Type: StepApprovalGate, Description: "User approval if policy requires", AllowedTools: []string{"wallet_execute_contract_call"}, Status: StepPending},
		{ID: "s7", Type: StepSignSend, Description: "Sign and broadcast", AllowedTools: []string{"wallet_execute_contract_call"}, Status: StepPending},
		{ID: "s8", Type: StepVerifyReceipt, Description: "Fetch receipt summary", AllowedTools: []string{}, Status: StepPending},
	}
	return &Plan{
		ID:          "",
		Goal:        goal,
		Capability:  CapabilityWalletTx,
		Constraints: constraints,
		Steps:       steps,
	}
}

// MergePlannerConstraints copies non-empty keys from src into dst.
func MergePlannerConstraints(dst, src map[string]string) {
	if dst == nil || src == nil {
		return
	}
	for k, v := range src {
		v = strings.TrimSpace(v)
		if v != "" {
			dst[strings.ToLower(strings.TrimSpace(k))] = v
		}
	}
}

// ValidateWalletConstraints checks required keys for wallet.tx execution.
func ValidateWalletConstraints(c map[string]string) error {
	if c == nil {
		return fmt.Errorf("constraints missing")
	}
	to := strings.TrimSpace(c["to"])
	data := strings.TrimSpace(c["data"])
	if to == "" || !strings.HasPrefix(to, "0x") {
		return fmt.Errorf("constraints.to must be a 0x-prefixed address")
	}
	if data == "" || !strings.HasPrefix(data, "0x") {
		return fmt.Errorf("constraints.data must be hex calldata")
	}
	return nil
}
