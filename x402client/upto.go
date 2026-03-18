package x402client

import (
	"context"
	"fmt"
	"math/big"
	"strconv"
	"strings"
	"time"

	"github.com/ethereum/go-ethereum/common"

	evm "github.com/coinbase/x402/go/mechanisms/evm"
	"github.com/coinbase/x402/go/types"
)

const schemeUpto = "upto"

// UptoEvmScheme implements SchemeNetworkClient for the x402 "upto" scheme.
// It builds ERC-2612 permits for pay-as-you-go (upto cap) payments.
type UptoEvmScheme struct {
	signer    evm.ClientEvmSignerWithReadContract
	permitCap string // default USDC cap when requirements.Amount is empty, e.g. "50"
}

// NewUptoEvmScheme creates an upto scheme client.
// permitCapUSDC is the default spend cap in USDC when the 402 response does not specify amount (e.g. "50").
func NewUptoEvmScheme(signer evm.ClientEvmSignerWithReadContract, permitCapUSDC string) *UptoEvmScheme {
	cap := permitCapUSDC
	if cap == "" {
		cap = "50"
	}
	return &UptoEvmScheme{signer: signer, permitCap: cap}
}

// Scheme returns the scheme identifier.
func (c *UptoEvmScheme) Scheme() string {
	return schemeUpto
}

// CreatePaymentPayload builds a V2 payment payload for the upto scheme.
// It reads network, asset, payTo from requirements and optionally amount/extra.
func (c *UptoEvmScheme) CreatePaymentPayload(
	ctx context.Context,
	requirements types.PaymentRequirements,
) (types.PaymentPayload, error) {
	asset := evm.NormalizeAddress(requirements.Asset)
	payTo := evm.NormalizeAddress(requirements.PayTo)
	if asset == "" || payTo == "" {
		return types.PaymentPayload{}, fmt.Errorf("upto: missing asset or payTo in requirements")
	}

	chainID, err := evm.GetEvmChainId(requirements.Network)
	if err != nil {
		return types.PaymentPayload{}, fmt.Errorf("upto: %w", err)
	}

	// Permit cap: use requirements.Amount if set and valid, else permitCap
	amountStr := strings.TrimSpace(requirements.Amount)
	if amountStr == "" {
		amountStr = c.permitCap
	}
	// Parse as USDC (6 decimals): "50" -> 50_000_000
	capFloat, err := strconv.ParseFloat(amountStr, 64)
	if err != nil {
		return types.PaymentPayload{}, fmt.Errorf("upto: invalid amount %q: %w", amountStr, err)
	}
	capUnits := new(big.Int).SetInt64(int64(capFloat * 1e6))
	valueStr := capUnits.String()

	// Deadline: 1 hour from now (or MaxTimeoutSeconds if smaller)
	deadlineSec := int64(3600)
	if requirements.MaxTimeoutSeconds > 0 && requirements.MaxTimeoutSeconds < 3600 {
		deadlineSec = int64(requirements.MaxTimeoutSeconds)
	}
	deadline := time.Now().Add(time.Duration(deadlineSec) * time.Second).Unix()
	deadlineStr := strconv.FormatInt(deadline, 10)

	// EIP-712 domain from requirements.Extra or defaults for USDC on Base
	domainName := "USD Coin"
	domainVersion := "2"
	if requirements.Extra != nil {
		if n, ok := requirements.Extra["name"].(string); ok && n != "" {
			domainName = n
		}
		if v, ok := requirements.Extra["version"].(string); ok && v != "" {
			domainVersion = v
		}
	}

	owner := c.signer.Address()

	// Get EIP-2612 nonce from token contract
	nonceResult, err := c.signer.ReadContract(ctx, asset, evm.EIP2612NoncesABI, "nonces", common.HexToAddress(owner))
	if err != nil {
		return types.PaymentPayload{}, fmt.Errorf("upto: nonce: %w", err)
	}
	nonce, ok := nonceResult.(*big.Int)
	if !ok {
		return types.PaymentPayload{}, fmt.Errorf("upto: unexpected nonce type: %T", nonceResult)
	}

	domain := evm.TypedDataDomain{
		Name:              domainName,
		Version:           domainVersion,
		ChainID:           chainID,
		VerifyingContract: asset,
	}
	typesMap := evm.GetEIP2612EIP712Types()
	message := map[string]interface{}{
		"owner":    common.HexToAddress(owner).Hex(),
		"spender":  common.HexToAddress(payTo).Hex(),
		"value":    capUnits,
		"nonce":    nonce,
		"deadline": big.NewInt(deadline),
	}

	sigBytes, err := c.signer.SignTypedData(ctx, domain, typesMap, "Permit", message)
	if err != nil {
		return types.PaymentPayload{}, fmt.Errorf("upto: sign: %w", err)
	}
	sigHex := evm.BytesToHex(sigBytes)

	payload := types.PaymentPayload{
		X402Version: 2,
		Accepted:    requirements,
		Payload: map[string]interface{}{
			"authorization": map[string]interface{}{
				"from":        owner,
				"to":          payTo,
				"value":       valueStr,
				"validBefore": deadlineStr,
				"nonce":       nonce.String(),
			},
			"signature": sigHex,
		},
	}
	return payload, nil
}
