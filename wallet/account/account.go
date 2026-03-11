package account

import (
	"context"
	"math/big"

	"github.com/ethereum/go-ethereum/common"
)

// Action represents a normalized execution intent (transfer or contract call).
// Policy and approval flow operate on Actions, so they work for both EOA and ERC-4337.
type Action struct {
	Type       string         // "transfer" | "contract_call"
	To         common.Address
	Value      *big.Int
	Data       []byte
	GasLimit   uint64
	Method     string   // optional, for contract_call
	MethodArgs []string // optional, human-readable args for display
}

// Account abstracts EOA and smart accounts. Execute passes through policy before signing.
type Account interface {
	// Address returns the account address (EOA or smart account).
	Address() common.Address
	// PrepareExecution builds a signed transaction or userOp from an Action.
	// Does not broadcast; caller is responsible for policy checks and approval.
	PrepareExecution(ctx context.Context, action *Action) ([]byte, error)
	// Estimate returns estimated gas for the action.
	Estimate(ctx context.Context, action *Action) (uint64, error)
}
