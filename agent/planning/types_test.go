package planning

import "testing"

func TestValidatePlan_OK(t *testing.T) {
	p := WalletTxTemplate("swap ETH to USDC", map[string]string{"to": "0xabc", "data": "0x1234"})
	if err := ValidatePlan(p); err != nil {
		t.Fatal(err)
	}
}

func TestValidatePlan_EmptyGoal(t *testing.T) {
	p := &Plan{Capability: CapabilityWalletTx, Steps: []Step{{ID: "s1"}}}
	if err := ValidatePlan(p); err == nil {
		t.Fatal("expected error")
	}
}

func TestValidateWalletConstraints(t *testing.T) {
	if err := ValidateWalletConstraints(map[string]string{"to": "nothex", "data": "0xabcd"}); err == nil {
		t.Fatal("expected error for invalid to")
	}
	if err := ValidateWalletConstraints(map[string]string{"to": "0x1234567890123456789012345678901234567890", "data": "abcd"}); err == nil {
		t.Fatal("expected error for data without 0x")
	}
	if err := ValidateWalletConstraints(map[string]string{"to": "0x1234567890123456789012345678901234567890", "data": "0xabcd"}); err != nil {
		t.Fatal(err)
	}
}

func TestParseCapabilities(t *testing.T) {
	m := ParseCapabilities("wallet, files ")
	if !m["wallet"] || !m["files"] {
		t.Fatalf("got %v", m)
	}
}

func TestValidator_AllowedCapability(t *testing.T) {
	p := WalletTxTemplate("x", map[string]string{"to": "0x1234567890123456789012345678901234567890", "data": "0x00"})
	v := NewValidator(CapabilityWalletTx)
	if err := v.Validate(p); err != nil {
		t.Fatal(err)
	}
}

func TestValidator_WrongCapability(t *testing.T) {
	p := WalletTxTemplate("x", map[string]string{"to": "0x1234567890123456789012345678901234567890", "data": "0x00"})
	p.Capability = "other"
	v := NewValidator(CapabilityWalletTx)
	if err := v.Validate(p); err == nil {
		t.Fatal("expected error")
	}
}
