package alchemy

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestNew_NilWhenEmpty(t *testing.T) {
	c, err := New(Config{})
	if err != nil {
		t.Fatalf("New(empty): %v", err)
	}
	if c != nil {
		t.Error("expected nil client when config empty")
	}
}

func TestNew_WithAPIKey(t *testing.T) {
	c, err := New(Config{APIKey: "test-key"})
	if err != nil {
		t.Fatalf("New(apiKey): %v", err)
	}
	if c == nil {
		t.Fatal("expected non-nil client")
	}
	url, err := c.urlForChain(1)
	if err != nil {
		t.Fatalf("urlForChain(1): %v", err)
	}
	if url != "https://eth-mainnet.g.alchemy.com/v2/test-key" {
		t.Errorf("urlForChain(1) = %q", url)
	}
}

func TestNew_WithBaseURL(t *testing.T) {
	c, err := New(Config{BaseURL: "https://eth-mainnet.g.alchemy.com/v2/abc"})
	if err != nil {
		t.Fatalf("New(baseURL): %v", err)
	}
	if c == nil {
		t.Fatal("expected non-nil client")
	}
	url, err := c.urlForChain(999)
	if err != nil {
		t.Fatalf("urlForChain(999): %v", err)
	}
	if url != "https://eth-mainnet.g.alchemy.com/v2/abc" {
		t.Errorf("urlForChain(999) = %q", url)
	}
}

func TestGetTokenBalances_Mock(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "POST" {
			t.Errorf("method = %s", r.Method)
		}
		var body struct {
			Method string `json:"method"`
		}
		json.NewDecoder(r.Body).Decode(&body)
		if body.Method != "alchemy_getTokenBalances" {
			t.Errorf("method = %s", body.Method)
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"jsonrpc": "2.0",
			"id":      1,
			"result": map[string]interface{}{
				"address": "0x123",
				"tokenBalances": []map[string]interface{}{
					{"contractAddress": "0xabc", "tokenBalance": "0xde0b6b3a7640000", "error": ""},
				},
			},
		})
	}))
	defer server.Close()

	c, err := New(Config{ChainURLs: map[int64]string{1: server.URL}})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	res, err := c.GetTokenBalances(context.Background(), 1, "0x123", "erc20")
	if err != nil {
		t.Fatalf("GetTokenBalances: %v", err)
	}
	if res.Address != "0x123" {
		t.Errorf("address = %q", res.Address)
	}
	if len(res.TokenBalances) != 1 {
		t.Fatalf("len(tokenBalances) = %d", len(res.TokenBalances))
	}
	if res.TokenBalances[0].ContractAddress != "0xabc" {
		t.Errorf("contractAddress = %q", res.TokenBalances[0].ContractAddress)
	}
	if res.TokenBalances[0].TokenBalance != "0xde0b6b3a7640000" {
		t.Errorf("tokenBalance = %q", res.TokenBalances[0].TokenBalance)
	}
}

func TestSimulateAssetChanges_Mock(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"jsonrpc": "2.0",
			"id":      1,
			"result": map[string]interface{}{
				"changes": []map[string]interface{}{
					{"assetType": "ERC20", "changeType": "TRANSFER", "from": "0xa", "to": "0xb", "rawAmount": "1000"},
				},
			},
		})
	}))
	defer server.Close()

	c, err := New(Config{ChainURLs: map[int64]string{1: server.URL}})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	res, err := c.SimulateAssetChanges(context.Background(), 1, "0xa", "0xcontract", "0x1234", "0")
	if err != nil {
		t.Fatalf("SimulateAssetChanges: %v", err)
	}
	if len(res.Changes) != 1 {
		t.Fatalf("len(changes) = %d", len(res.Changes))
	}
	if res.Changes[0].AssetType != "ERC20" || res.Changes[0].ChangeType != "TRANSFER" {
		t.Errorf("change = %+v", res.Changes[0])
	}
}
