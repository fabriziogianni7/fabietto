package eval

import (
	"context"

	"custom-agent/tools"
)

// evalWalletStub implements tools.WalletService for eval tool_execute cases.
type evalWalletStub struct {
	erc20Out string
}

func newEvalWalletStub(erc20Out string) tools.WalletService {
	return &evalWalletStub{erc20Out: erc20Out}
}

func (e *evalWalletStub) WalletAddress() string {
	return "0x1111111111111111111111111111111111111111"
}

func (e *evalWalletStub) DefaultChainID() int64 { return 8453 }

func (e *evalWalletStub) GetBalanceString(ctx context.Context, chainID int64, block interface{}) (string, error) {
	return "0", nil
}

func (e *evalWalletStub) ERC20Balance(ctx context.Context, chainID int64, token string) (string, error) {
	return e.erc20Out, nil
}

func (e *evalWalletStub) ExecuteTransfer(ctx context.Context, chainID int64, to, valueWei, platform, userID, chatID string) (string, error) {
	return "", nil
}

func (e *evalWalletStub) ExecuteContractCall(ctx context.Context, chainID int64, to, dataHex, valueWei, platform, userID, chatID string) (string, error) {
	return "", nil
}

func (e *evalWalletStub) ExecuteApproved(ctx context.Context, approvalID, platform, userID, chatID string) (string, error) {
	return "", nil
}

func (e *evalWalletStub) ListTransactions(chainID int64, limit int) (string, error) {
	return "", nil
}

func (e *evalWalletStub) WalletPreviewContract(ctx context.Context, chainID int64, to, dataHex, valueWei string) (string, string, uint64, error) {
	return "", "allow", 0, nil
}

func (e *evalWalletStub) WalletSimulateContract(ctx context.Context, chainID int64, to, dataHex, valueWei string) error {
	return nil
}

func (e *evalWalletStub) WalletReceiptSummary(ctx context.Context, chainID int64, txHash string) (string, error) {
	return "", nil
}
