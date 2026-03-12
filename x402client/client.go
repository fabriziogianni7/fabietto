package x402client

import (
	"net/http"
	"strings"
	"time"

	x402 "github.com/coinbase/x402/go"
	x402http "github.com/coinbase/x402/go/http"
	evm "github.com/coinbase/x402/go/mechanisms/evm/exact/client"
	evmsigners "github.com/coinbase/x402/go/signers/evm"
)

// Client wraps an HTTP client with x402 payment handling.
// When the underlying client receives 402 Payment Required, it automatically
// creates a payment payload and retries with the payment signature.
type Client struct {
	*http.Client
}

// New creates an x402-aware HTTP client from a hex-encoded EVM private key.
// If privateKeyHex is empty, returns nil (caller should use plain http.DefaultClient).
// The returned client handles both paid (402) and free requests transparently.
func New(privateKeyHex string) (*Client, error) {
	privateKeyHex = strings.TrimSpace(strings.TrimPrefix(privateKeyHex, "0x"))
	if privateKeyHex == "" {
		return nil, nil
	}

	signer, err := evmsigners.NewClientSignerFromPrivateKey(privateKeyHex)
	if err != nil {
		return nil, err
	}

	x402Client := x402.Newx402Client().
		Register("eip155:*", evm.NewExactEvmScheme(signer, nil))

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
