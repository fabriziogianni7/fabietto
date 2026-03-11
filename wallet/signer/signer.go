package signer

import (
	"context"
	"math/big"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
)

// Signer abstracts key storage and signing. Implementations may use env, HSM, or KMS.
type Signer interface {
	// Address returns the Ethereum address for this signer.
	Address() common.Address
	// SignTransaction signs a legacy transaction. The signer may modify the tx (e.g. set chain ID).
	SignTransaction(ctx context.Context, tx *types.Transaction, chainID *big.Int) (*types.Transaction, error)
	// SignMessage signs an arbitrary message (EIP-191 personal_sign).
	SignMessage(ctx context.Context, msg []byte) ([]byte, error)
	// SignTypedData signs EIP-712 typed data.
	SignTypedData(ctx context.Context, domainSeparator, typedDataHash []byte) ([]byte, error)
	// PublicIdentifier returns a short identifier for logging (e.g. 0x1234...5678). Never the private key.
	PublicIdentifier() string
}
