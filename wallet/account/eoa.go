package account

import (
	"context"
	"math/big"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"

	"custom-agent/wallet/provider"
	"custom-agent/wallet/signer"
)

// EOAAccount implements Account for an Externally Owned Account.
type EOAAccount struct {
	sgn     signer.Signer
	prov    *provider.Provider
	chainID *big.Int
}

// NewEOAAccount creates an EOA account backed by the given signer and provider.
func NewEOAAccount(sgn signer.Signer, prov *provider.Provider, chainID *big.Int) *EOAAccount {
	if chainID == nil {
		chainID = big.NewInt(1)
	}
	return &EOAAccount{sgn: sgn, prov: prov, chainID: chainID}
}

// Address implements Account.
func (e *EOAAccount) Address() common.Address {
	return e.sgn.Address()
}

// PrepareExecution implements Account. Returns RLP-encoded signed transaction.
func (e *EOAAccount) PrepareExecution(ctx context.Context, action *Action) ([]byte, error) {
	nonce, err := e.prov.PendingNonceAt(ctx, e.sgn.Address())
	if err != nil {
		return nil, err
	}
	gasPrice, err := e.prov.SuggestGasPrice(ctx)
	if err != nil {
		return nil, err
	}
	tx := types.NewTransaction(
		nonce,
		action.To,
		action.Value,
		action.GasLimit,
		gasPrice,
		action.Data,
	)
	signed, err := e.sgn.SignTransaction(ctx, tx, e.chainID)
	if err != nil {
		return nil, err
	}
	return signed.MarshalBinary()
}

// Estimate implements Account.
func (e *EOAAccount) Estimate(ctx context.Context, action *Action) (uint64, error) {
	from := e.sgn.Address()
	return e.prov.EstimateGas(ctx, &from, &action.To, action.Value, action.Data)
}
