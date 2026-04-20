package planning

import (
	"context"
	"strings"
	"testing"
)

type stubWallet struct {
	previewMethod   string
	previewDecision string
	previewGas      uint64
	simulateErr     error
	callCount       int
	lastOut         string
}

func (s *stubWallet) WalletAddress() string { return "0x1" }
func (s *stubWallet) DefaultChainID() int64 { return 1 }
func (s *stubWallet) GetBalanceString(ctx context.Context, chainID int64, block interface{}) (string, error) {
	return "0", nil
}
func (s *stubWallet) ExecuteTransfer(ctx context.Context, chainID int64, to, valueWei, platform, userID, chatID string) (string, error) {
	return "", nil
}
func (s *stubWallet) ExecuteContractCall(ctx context.Context, chainID int64, to, dataHex, valueWei, platform, userID, chatID string) (string, error) {
	s.callCount++
	s.lastOut = "Transaction sent.\nHash: 0xdeadbeef\nChain: Test"
	return s.lastOut, nil
}
func (s *stubWallet) ExecuteApproved(ctx context.Context, approvalID, platform, userID, chatID string) (string, error) {
	return "", nil
}
func (s *stubWallet) ListTransactions(chainID int64, limit int) (string, error) {
	return "", nil
}
func (s *stubWallet) WalletPreviewContract(ctx context.Context, chainID int64, to, dataHex, valueWei string) (string, string, uint64, error) {
	return s.previewMethod, s.previewDecision, s.previewGas, nil
}
func (s *stubWallet) WalletSimulateContract(ctx context.Context, chainID int64, to, dataHex, valueWei string) error {
	return s.simulateErr
}
func (s *stubWallet) WalletReceiptSummary(ctx context.Context, chainID int64, txHash string) (string, error) {
	return "success block=1 gas_used=21000", nil
}

func TestWalletExecutor_HappyPath_SingleSend(t *testing.T) {
	st := &stubWallet{previewMethod: "swap", previewDecision: "allow", previewGas: 100000}
	p := WalletTxTemplate("swap", map[string]string{
		"to":   "0x1234567890123456789012345678901234567890",
		"data": "0x12345678",
	})
	ex := &WalletExecutor{Wallet: st}
	res, err := ex.Execute(context.Background(), p)
	if err != nil {
		t.Fatal(err)
	}
	if st.callCount != 1 {
		t.Fatalf("expected 1 contract call, got %d", st.callCount)
	}
	if !strings.Contains(res.FinalReply, "Transaction sent") {
		t.Fatalf("final: %q", res.FinalReply)
	}
}

func TestWalletExecutor_SimulationFails(t *testing.T) {
	st := &stubWallet{previewMethod: "x", previewDecision: "allow", simulateErr: context.Canceled}
	p := WalletTxTemplate("x", map[string]string{
		"to":   "0x1234567890123456789012345678901234567890",
		"data": "0x00",
	})
	ex := &WalletExecutor{Wallet: st}
	res, err := ex.Execute(context.Background(), p)
	if err != nil {
		t.Fatal(err)
	}
	if !res.StoppedEarly {
		t.Fatal("expected stopped early")
	}
	if st.callCount != 0 {
		t.Fatal("should not broadcast")
	}
}

func TestWalletExecutor_PolicyDeny(t *testing.T) {
	st := &stubWallet{previewMethod: "approve", previewDecision: "deny"}
	p := WalletTxTemplate("x", map[string]string{
		"to":   "0x1234567890123456789012345678901234567890",
		"data": "0x095ea7b3",
	})
	ex := &WalletExecutor{Wallet: st}
	res, err := ex.Execute(context.Background(), p)
	if err != nil {
		t.Fatal(err)
	}
	if !res.StoppedEarly {
		t.Fatal("expected stopped")
	}
	if st.callCount != 0 {
		t.Fatal("should not broadcast")
	}
}
