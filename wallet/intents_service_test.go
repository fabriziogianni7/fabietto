package wallet

import (
	"context"
	"math/big"
	"testing"

	"github.com/ethereum/go-ethereum/common"

	sdk "github.com/1clawAI/1claw-go-sdk"

	"custom-agent/wallet/approval"
	"custom-agent/wallet/chains"
	"custom-agent/wallet/history"
	"custom-agent/wallet/policy"
)

func TestWeiToEth(t *testing.T) {
	tests := []struct {
		wei string
		eth string
	}{
		{"0", "0.000000000000000000"},
		{"1000000000000000000", "1.000000000000000000"},
		{"100000000000000000", "0.100000000000000000"},
	}
	for _, tt := range tests {
		got, err := weiToEth(tt.wei)
		if err != nil {
			t.Errorf("weiToEth(%q): %v", tt.wei, err)
			continue
		}
		if got != tt.eth {
			t.Errorf("weiToEth(%q) = %q, want %q", tt.wei, got, tt.eth)
		}
	}
}

func TestIntentsService_WalletAddress(t *testing.T) {
	client, _ := sdk.New(sdk.WithToken("test"))
	chainReg := chains.NewRegistry()
	chainReg.AddChain(&chains.Chain{ChainID: 1, RPCURL: "https://eth.llamarpc.com", Name: "Ethereum"})
	chainReg.SetDefaultChain(1)
	svc := NewIntentsService(client, "agent-1", "0x1234567890123456789012345678901234567890", chainReg, policy.NewEngine(policy.DefaultConfig()), nil, nil, history.NewStore(""))
	if got := svc.WalletAddress(); got != "0x1234567890123456789012345678901234567890" {
		t.Errorf("WalletAddress = %q", got)
	}
	if got := svc.DefaultChainID(); got != 1 {
		t.Errorf("DefaultChainID = %d", got)
	}
}

func TestIntentsService_ExecuteApproved_NoApprovalStore(t *testing.T) {
	client, _ := sdk.New(sdk.WithToken("test"))
	chainReg := chains.NewRegistry()
	chainReg.AddChain(&chains.Chain{ChainID: 1, RPCURL: "https://eth.llamarpc.com", Name: "Ethereum"})
	chainReg.SetDefaultChain(1)
	svc := NewIntentsService(client, "agent-1", "0x1234", chainReg, policy.NewEngine(policy.DefaultConfig()), nil, nil, history.NewStore(""))
	result, err := svc.ExecuteApproved(context.Background(), "tx_1", "telegram", "user1", "chat1")
	if err != nil {
		t.Fatalf("ExecuteApproved: %v", err)
	}
	if result != "Error: approval store not configured." {
		t.Errorf("ExecuteApproved = %q", result)
	}
}

func TestIntentsService_ExecuteApproved_NotFound(t *testing.T) {
	client, _ := sdk.New(sdk.WithToken("test"))
	chainReg := chains.NewRegistry()
	chainReg.AddChain(&chains.Chain{ChainID: 1, RPCURL: "https://eth.llamarpc.com", Name: "Ethereum"})
	chainReg.SetDefaultChain(1)
	approvalStore := approval.NewStore("", 0)
	svc := NewIntentsService(client, "agent-1", "0x1234", chainReg, policy.NewEngine(policy.DefaultConfig()), approvalStore, nil, history.NewStore(""))
	result, err := svc.ExecuteApproved(context.Background(), "tx_nonexistent", "telegram", "user1", "chat1")
	if err != nil {
		t.Fatalf("ExecuteApproved: %v", err)
	}
	if result != "Error: approval not found or expired." {
		t.Errorf("ExecuteApproved = %q", result)
	}
}

func TestToAccountAction(t *testing.T) {
	a := &accountAction{
		Type:  "transfer",
		To:    common.HexToAddress("0xdead"),
		Value: big.NewInt(1e18),
		Data:  nil,
	}
	acc := toAccountAction(a)
	if acc.Type != "transfer" {
		t.Errorf("Type = %q", acc.Type)
	}
	if acc.To != a.To {
		t.Errorf("To mismatch")
	}
	if acc.Value.Cmp(a.Value) != 0 {
		t.Errorf("Value mismatch")
	}
	if acc.GasLimit != 21000 {
		t.Errorf("GasLimit = %d for transfer", acc.GasLimit)
	}
	// Contract call should get higher gas
	a2 := &accountAction{Type: "contract_call", Data: []byte{0x12, 0x34}}
	acc2 := toAccountAction(a2)
	if acc2.GasLimit != 300000 {
		t.Errorf("GasLimit = %d for contract call", acc2.GasLimit)
	}
}
