package signer

import (
	"context"
	"crypto/ecdsa"
	"errors"
	"math/big"
	"strings"

	"github.com/ethereum/go-ethereum/accounts"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/crypto"

	sdk "github.com/1clawAI/1claw-go-sdk"
)

// OneClawSecretGetter fetches a secret by path. Used for testing.
type OneClawSecretGetter interface {
	Get(ctx context.Context, vaultID, path string) (string, error)
}

// OneClawSigner fetches the private key from a 1claw vault once at init, then signs like EnvSigner.
type OneClawSigner struct {
	key     *ecdsa.PrivateKey
	address common.Address
}

// NewOneClawSigner creates a signer by fetching the key from the 1claw vault.
// client: 1claw SDK client (must be authenticated).
// vaultID: vault containing the secret.
// keyPath: secret path (e.g. "WALLET_PRIVATE_KEY").
func NewOneClawSigner(client *sdk.Client, vaultID, keyPath string) (*OneClawSigner, error) {
	if client == nil || vaultID == "" || keyPath == "" {
		return nil, errors.New("wallet: oneclaw signer requires client, vaultID, and keyPath")
	}
	g := &sdkSecretGetter{client: client}
	return newOneClawSignerFromGetter(context.Background(), g, vaultID, keyPath)
}

// NewOneClawSignerFromGetter creates a signer using a custom getter. Used for testing.
func NewOneClawSignerFromGetter(ctx context.Context, getter OneClawSecretGetter, vaultID, keyPath string) (*OneClawSigner, error) {
	if getter == nil || vaultID == "" || keyPath == "" {
		return nil, errors.New("wallet: oneclaw signer requires getter, vaultID, and keyPath")
	}
	return newOneClawSignerFromGetter(ctx, getter, vaultID, keyPath)
}

func newOneClawSignerFromGetter(ctx context.Context, getter OneClawSecretGetter, vaultID, keyPath string) (*OneClawSigner, error) {
	hex, err := getter.Get(ctx, vaultID, keyPath)
	if err != nil {
		return nil, err
	}
	hex = strings.TrimSpace(hex)
	if hex == "" {
		return nil, errors.New("wallet: secret value is empty")
	}
	key, err := crypto.HexToECDSA(strings.TrimPrefix(hex, "0x"))
	if err != nil {
		return nil, err
	}
	addr := crypto.PubkeyToAddress(key.PublicKey)
	return &OneClawSigner{key: key, address: addr}, nil
}

type sdkSecretGetter struct {
	client *sdk.Client
}

func (s *sdkSecretGetter) Get(ctx context.Context, vaultID, path string) (string, error) {
	secret, err := s.client.Secrets.Get(ctx, vaultID, path)
	if err != nil {
		return "", err
	}
	return secret.Value, nil
}

// Address implements Signer.
func (o *OneClawSigner) Address() common.Address {
	return o.address
}

// SignTransaction implements Signer.
func (o *OneClawSigner) SignTransaction(ctx context.Context, tx *types.Transaction, chainID *big.Int) (*types.Transaction, error) {
	s := types.NewEIP155Signer(chainID)
	return types.SignTx(tx, s, o.key)
}

// SignMessage implements Signer (EIP-191 personal_sign).
func (o *OneClawSigner) SignMessage(ctx context.Context, msg []byte) ([]byte, error) {
	hash := accounts.TextHash(msg)
	return crypto.Sign(hash, o.key)
}

// SignTypedData implements Signer.
func (o *OneClawSigner) SignTypedData(ctx context.Context, domainSeparator, typedDataHash []byte) ([]byte, error) {
	hash := crypto.Keccak256Hash(append(append([]byte("\x19\x01"), domainSeparator...), typedDataHash...))
	return crypto.Sign(hash.Bytes(), o.key)
}

// PublicIdentifier implements Signer.
func (o *OneClawSigner) PublicIdentifier() string {
	a := o.address.Hex()
	if len(a) > 10 {
		return a[:6] + "..." + a[len(a)-4:]
	}
	return a
}

var _ Signer = (*OneClawSigner)(nil)
