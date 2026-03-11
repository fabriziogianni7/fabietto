package account

import (
	"context"
	"fmt"

	"github.com/ethereum/go-ethereum/common"
)

// SmartAccount is a placeholder for ERC-4337 smart account support.
// When WALLET_ACCOUNT_MODE=smart, the WalletService will use this instead of EOAAccount.
type SmartAccount struct {
	address common.Address
}

// NewSmartAccount creates a smart account placeholder. Full implementation will add
// bundler/paymaster clients and userOp construction.
func NewSmartAccount(address common.Address) *SmartAccount {
	return &SmartAccount{address: address}
}

// Address implements Account.
func (s *SmartAccount) Address() common.Address {
	return s.address
}

// PrepareExecution implements Account. Not yet implemented for smart accounts.
func (s *SmartAccount) PrepareExecution(ctx context.Context, action *Action) ([]byte, error) {
	return nil, fmt.Errorf("wallet: smart account (ERC-4337) not yet implemented; use eoa mode")
}

// Estimate implements Account.
func (s *SmartAccount) Estimate(ctx context.Context, action *Action) (uint64, error) {
	return 0, fmt.Errorf("wallet: smart account (ERC-4337) not yet implemented; use eoa mode")
}
