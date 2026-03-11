package signer

import (
	"context"
	"crypto/ecdsa"
	"fmt"
	"math/big"
	"os"
	"strings"

	"github.com/ethereum/go-ethereum/accounts"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/crypto"
)

// EnvSigner loads the private key from an environment variable at init.
// The key is kept in memory; unset the env var after init if desired.
type EnvSigner struct {
	key     *ecdsa.PrivateKey
	address common.Address
}

// NewEnvSigner creates a signer from WALLET_PRIVATE_KEY env var.
// Returns error if the var is empty or invalid. Does not unset the env var.
func NewEnvSigner(envKey string) (*EnvSigner, error) {
	if envKey == "" {
		envKey = "WALLET_PRIVATE_KEY"
	}
	hex := strings.TrimSpace(os.Getenv(envKey))
	if hex == "" {
		return nil, fmt.Errorf("wallet: %s not set", envKey)
	}
	key, err := crypto.HexToECDSA(strings.TrimPrefix(hex, "0x"))
	if err != nil {
		return nil, fmt.Errorf("wallet: invalid private key in %s: %w", envKey, err)
	}
	addr := crypto.PubkeyToAddress(key.PublicKey)
	return &EnvSigner{key: key, address: addr}, nil
}

// UnsetEnvKey clears the env var after loading. Call from main after NewEnvSigner.
func UnsetEnvKey(envKey string) {
	if envKey == "" {
		envKey = "WALLET_PRIVATE_KEY"
	}
	_ = os.Unsetenv(envKey)
}

// Address implements Signer.
func (e *EnvSigner) Address() common.Address {
	return e.address
}

// SignTransaction implements Signer.
func (e *EnvSigner) SignTransaction(ctx context.Context, tx *types.Transaction, chainID *big.Int) (*types.Transaction, error) {
	signer := types.NewEIP155Signer(chainID)
	return types.SignTx(tx, signer, e.key)
}

// SignMessage implements Signer (EIP-191 personal_sign).
func (e *EnvSigner) SignMessage(ctx context.Context, msg []byte) ([]byte, error) {
	hash := accounts.TextHash(msg)
	return crypto.Sign(hash, e.key)
}

// SignTypedData implements Signer.
func (e *EnvSigner) SignTypedData(ctx context.Context, domainSeparator, typedDataHash []byte) ([]byte, error) {
	hash := crypto.Keccak256Hash(append(append([]byte("\x19\x01"), domainSeparator...), typedDataHash...))
	return crypto.Sign(hash.Bytes(), e.key)
}

// PublicIdentifier implements Signer.
func (e *EnvSigner) PublicIdentifier() string {
	a := e.address.Hex()
	if len(a) > 10 {
		return a[:6] + "..." + a[len(a)-4:]
	}
	return a
}

// Ensure EnvSigner implements Signer at compile time.
var _ Signer = (*EnvSigner)(nil)
