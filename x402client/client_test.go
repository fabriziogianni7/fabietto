package x402client

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"math/big"
	"net/http"
	"net/http/httptest"
	"testing"

	evm "github.com/coinbase/x402/go/mechanisms/evm"
	"github.com/coinbase/x402/go/types"
)

func TestNew_EmptyKey(t *testing.T) {
	c, err := New("")
	if err != nil {
		t.Fatalf("New(\"\"): unexpected error: %v", err)
	}
	if c != nil {
		t.Error("New(\"\"): expected nil client")
	}
}

func TestNew_InvalidKey(t *testing.T) {
	_, err := New("not-hex")
	if err == nil {
		t.Error("New(\"not-hex\"): expected error")
	}
}

func TestNew_ValidKey(t *testing.T) {
	// Use a valid hex private key (32 bytes) - test key only
	key := "0xac0974bec39a17e36ba4a6b4d238ff944bacb478cbed5efcae784d7bf4f2ff80"
	c, err := New(key)
	if err != nil {
		t.Fatalf("New(valid): %v", err)
	}
	if c == nil || c.Client == nil {
		t.Fatal("expected non-nil client")
	}
}

func TestClient_Do_FreeAPI(t *testing.T) {
	key := "0xac0974bec39a17e36ba4a6b4d238ff944bacb478cbed5efcae784d7bf4f2ff80"
	c, err := New(key)
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"ok":true}`))
	}))
	defer server.Close()

	req, _ := http.NewRequest("GET", server.URL, nil)
	resp, err := c.Do(req)
	if err != nil {
		t.Fatalf("Do: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("status: got %d, want 200", resp.StatusCode)
	}
}

func TestNewWithUpto_EmptyRPC_OnlyExact(t *testing.T) {
	key := "0xac0974bec39a17e36ba4a6b4d238ff944bacb478cbed5efcae784d7bf4f2ff80"
	c, err := NewWithUpto(key, "", "")
	if err != nil {
		t.Fatalf("NewWithUpto(empty rpc): %v", err)
	}
	if c == nil || c.Client == nil {
		t.Fatal("expected non-nil client")
	}
}

func TestNewWithUpto_NoUptoWhenPermitCapEmpty(t *testing.T) {
	key := "0xac0974bec39a17e36ba4a6b4d238ff944bacb478cbed5efcae784d7bf4f2ff80"
	// With empty permitCap, upto is not registered even if rpcURL is set
	c, err := NewWithUpto(key, "http://127.0.0.1:0", "")
	if err != nil {
		t.Fatalf("NewWithUpto(empty permitCap): %v", err)
	}
	if c == nil {
		t.Fatal("expected non-nil client")
	}
	// Client works (exact only); no upto registration
}

// mockUptoSigner implements evm.ClientEvmSignerWithReadContract for testing.
type mockUptoSigner struct {
	address string
}

func (m *mockUptoSigner) Address() string {
	if m.address == "" {
		return "0x1234567890123456789012345678901234567890"
	}
	return m.address
}

func (m *mockUptoSigner) SignTypedData(
	_ context.Context,
	_ evm.TypedDataDomain,
	_ map[string][]evm.TypedDataField,
	_ string,
	_ map[string]interface{},
) ([]byte, error) {
	sig := make([]byte, 65)
	sig[64] = 27
	return sig, nil
}

func (m *mockUptoSigner) ReadContract(
	_ context.Context,
	_ string,
	_ []byte,
	_ string,
	_ ...interface{},
) (interface{}, error) {
	return big.NewInt(0), nil
}

func TestUptoEvmScheme_CreatePaymentPayload_PayloadShape(t *testing.T) {
	signer := &mockUptoSigner{}
	scheme := NewUptoEvmScheme(signer, "50")

	requirements := types.PaymentRequirements{
		Scheme:  schemeUpto,
		Network: "eip155:8453",
		Asset:   "0x833589fCD6eDb6E08f4c7C32D4f71b54bdA02913",
		PayTo:   "0x1363C7Ff51CcCE10258A7F7bddd63bAaB6aAf678",
		Amount:  "",
	}

	payload, err := scheme.CreatePaymentPayload(context.Background(), requirements)
	if err != nil {
		t.Fatalf("CreatePaymentPayload: %v", err)
	}

	if payload.X402Version != 2 {
		t.Errorf("X402Version: got %d, want 2", payload.X402Version)
	}
	if payload.Accepted.Scheme != schemeUpto {
		t.Errorf("Accepted.Scheme: got %q, want %q", payload.Accepted.Scheme, schemeUpto)
	}
	if payload.Payload == nil {
		t.Fatal("Payload is nil")
	}
	auth, ok := payload.Payload["authorization"].(map[string]interface{})
	if !ok {
		t.Fatalf("Payload.authorization: got %T, want map", payload.Payload["authorization"])
	}
	for _, key := range []string{"from", "to", "value", "validBefore", "nonce"} {
		if _, exists := auth[key]; !exists {
			t.Errorf("Payload.authorization missing %q", key)
		}
	}
	if _, exists := payload.Payload["signature"]; !exists {
		t.Error("Payload missing signature")
	}
}

func TestClient_Do_402Upto_RetriesWithPayment(t *testing.T) {
	key := "0xac0974bec39a17e36ba4a6b4d238ff944bacb478cbed5efcae784d7bf4f2ff80"
	// Mock JSON-RPC: eth_chainId and eth_call (nonces) return valid responses
	rpcServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		// eth_call for nonces(owner) returns uint256 - 32 bytes hex, "0x" + 64 hex chars
		w.Write([]byte(`{"jsonrpc":"2.0","id":1,"result":"0x0000000000000000000000000000000000000000000000000000000000000000"}`))
	}))
	defer rpcServer.Close()

	c, err := NewWithUpto(key, rpcServer.URL, "50")
	if err != nil {
		t.Fatalf("NewWithUpto: %v", err)
	}

	// 402 with upto requirements; second request has PAYMENT-SIGNATURE
	var gotPaymentHeader bool
	apiServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("PAYMENT-SIGNATURE") != "" {
			gotPaymentHeader = true
			w.WriteHeader(http.StatusOK)
			w.Write([]byte(`{"ok":true}`))
			return
		}
		// 402 with PAYMENT-REQUIRED header (base64-encoded)
		pr := types.PaymentRequired{
			X402Version: 2,
			Accepts: []types.PaymentRequirements{{
				Scheme:  schemeUpto,
				Network: "eip155:8453",
				Asset:   "0x833589fCD6eDb6E08f4c7C32D4f71b54bdA02913",
				PayTo:   "0x1363C7Ff51CcCE10258A7F7bddd63bAaB6aAf678",
				Amount:  "50000000",
				Extra:   map[string]interface{}{"name": "USD Coin", "version": "2"},
			}},
		}
		b, _ := json.Marshal(pr)
		encoded := base64.StdEncoding.EncodeToString(b)
		w.Header().Set("PAYMENT-REQUIRED", encoded)
		w.WriteHeader(http.StatusPaymentRequired)
		w.Write(b)
	}))
	defer apiServer.Close()

	req, _ := http.NewRequest("GET", apiServer.URL, nil)
	resp, err := c.Do(req)
	if err != nil {
		t.Fatalf("Do: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("status: got %d, want 200", resp.StatusCode)
	}
	if !gotPaymentHeader {
		t.Error("expected retry with PAYMENT-SIGNATURE header")
	}
}
