package x402client

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	x402 "github.com/coinbase/x402/go"
	x402http "github.com/coinbase/x402/go/http"
	evm "github.com/coinbase/x402/go/mechanisms/evm"
	evmexact "github.com/coinbase/x402/go/mechanisms/evm/exact/client"
	evmsigners "github.com/coinbase/x402/go/signers/evm"

	"github.com/ethereum/go-ethereum/ethclient"
)

// Client wraps an HTTP client with x402 payment handling.
// When the underlying client receives 402 Payment Required, it automatically
// creates a payment payload and retries with the payment signature.
type Client struct {
	*http.Client
}

// New creates an x402-aware HTTP client from a hex-encoded EVM private key.
// Registers only the "exact" scheme. If privateKeyHex is empty, returns nil
// (caller should use plain http.DefaultClient).
func New(privateKeyHex string) (*Client, error) {
	return NewWithUpto(privateKeyHex, "", "")
}

// NewWithUpto creates an x402-aware HTTP client that supports both "exact" and "upto" schemes.
// When rpcURL and permitCapUSDC are non-empty, the upto scheme is registered for pay-as-you-go
// routers (e.g. ai.xgate.run). The client dynamically selects the scheme from the 402 response.
// If privateKeyHex is empty, returns nil.
func NewWithUpto(privateKeyHex, rpcURL, permitCapUSDC string) (*Client, error) {
	privateKeyHex = strings.TrimSpace(strings.TrimPrefix(privateKeyHex, "0x"))
	if privateKeyHex == "" {
		return nil, nil
	}

	signer, err := evmsigners.NewClientSignerFromPrivateKey(privateKeyHex)
	if err != nil {
		return nil, err
	}

	x402Client := x402.Newx402Client().
		Register("eip155:*", evmexact.NewExactEvmScheme(signer, nil))

	if rpcURL != "" && permitCapUSDC != "" {
		ethClient, err := ethclient.Dial(rpcURL)
		if err != nil {
			return nil, err
		}
		signerWithRead, err := evmsigners.NewClientSignerFromPrivateKeyWithClient(privateKeyHex, ethClient)
		if err != nil {
			ethClient.Close()
			return nil, err
		}
		uptoScheme := NewUptoEvmScheme(signerWithRead.(evm.ClientEvmSignerWithReadContract), permitCapUSDC)
		x402Client.Register("eip155:*", uptoScheme)
	}

	base := &http.Client{
		Timeout: 30 * time.Second,
	}
	wrapped := x402http.WrapHTTPClientWithPayment(base, x402http.Newx402HTTPClient(x402Client))

	return &Client{Client: wrapped}, nil
}

// Do sends an HTTP request. If the server responds with 402 Payment Required,
// the client creates a payment payload and retries automatically.
func (c *Client) Do(req *http.Request) (*http.Response, error) {
	return c.Client.Do(req)
}

// RouterStats is the response from GET /v1/stats (x402 router).
type RouterStats struct {
	TotalSpentUSD string `json:"total_spent_usd"`
	TotalTokens   int64  `json:"total_tokens"`
}

// FetchRouterStats fetches GET {baseURL}/stats and returns total_spent_usd and total_tokens.
// Use the x402 client so the request carries session context (if the router requires it).
func (c *Client) FetchRouterStats(ctx context.Context, baseURL string) (*RouterStats, error) {
	url := strings.TrimSuffix(baseURL, "/") + "/stats"
	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return nil, err
	}
	resp, err := c.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("stats: HTTP %d", resp.StatusCode)
	}
	var stats RouterStats
	if err := json.NewDecoder(resp.Body).Decode(&stats); err != nil {
		return nil, err
	}
	return &stats, nil
}
