package provider

import (
	"context"
	"math/big"

	"github.com/ethereum/go-ethereum"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/ethclient"
)

// Provider wraps ethclient for RPC reads, gas estimation, and broadcast.
type Provider struct {
	client *ethclient.Client
}

// New creates a provider from an RPC URL.
func New(rpcURL string) (*Provider, error) {
	client, err := ethclient.Dial(rpcURL)
	if err != nil {
		return nil, err
	}
	return &Provider{client: client}, nil
}

// Close closes the underlying client.
func (p *Provider) Close() {
	if p.client != nil {
		p.client.Close()
	}
}

// BalanceAt returns the wei balance at the given block.
func (p *Provider) BalanceAt(ctx context.Context, account common.Address, block *big.Int) (*big.Int, error) {
	if block == nil {
		return p.client.BalanceAt(ctx, account, nil)
	}
	return p.client.BalanceAt(ctx, account, block)
}

// PendingNonceAt returns the pending nonce for the account.
func (p *Provider) PendingNonceAt(ctx context.Context, account common.Address) (uint64, error) {
	return p.client.PendingNonceAt(ctx, account)
}

// SuggestGasPrice returns a suggested gas price.
func (p *Provider) SuggestGasPrice(ctx context.Context) (*big.Int, error) {
	return p.client.SuggestGasPrice(ctx)
}

// EstimateGas estimates gas for a call. from, to can be nil.
func (p *Provider) EstimateGas(ctx context.Context, from, to *common.Address, value *big.Int, data []byte) (uint64, error) {
	var msg ethereum.CallMsg
	if from != nil {
		msg.From = *from
	}
	if to != nil {
		msg.To = to
	}
	msg.Value = value
	msg.Data = data
	return p.client.EstimateGas(ctx, msg)
}

// SendRawTransaction broadcasts a signed transaction. Returns the tx hash on success.
func (p *Provider) SendRawTransaction(ctx context.Context, rawTx []byte) (common.Hash, error) {
	tx := new(types.Transaction)
	if err := tx.UnmarshalBinary(rawTx); err != nil {
		return common.Hash{}, err
	}
	if err := p.client.SendTransaction(ctx, tx); err != nil {
		return common.Hash{}, err
	}
	return tx.Hash(), nil
}

// TransactionReceipt returns the receipt for a tx hash.
func (p *Provider) TransactionReceipt(ctx context.Context, txHash common.Hash) (*types.Receipt, error) {
	return p.client.TransactionReceipt(ctx, txHash)
}

// ChainID returns the chain ID.
func (p *Provider) ChainID(ctx context.Context) (*big.Int, error) {
	return p.client.ChainID(ctx)
}
