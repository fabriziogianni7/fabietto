package alchemy

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"strings"
	"time"
)

// chainToAlchemyNetwork maps chain ID to Alchemy network subdomain.
var chainToAlchemyNetwork = map[int64]string{
	1:        "eth-mainnet",
	11155111: "eth-sepolia",
	8453:     "base-mainnet",
	10:       "opt-mainnet",
	42161:    "arb-mainnet",
	137:      "polygon-mainnet",
}

// Client calls Alchemy Data API (Token, Transfers, Simulation).
type Client struct {
	httpClient *http.Client
	apiKey     string
	baseURL    string // when set, use for all chains (single-chain mode)
	chainURLs  map[int64]string
}

// Config for creating an Alchemy client.
type Config struct {
	APIKey    string
	BaseURL   string // optional; when set, use for default chain
	ChainURLs map[int64]string
}

// New creates an Alchemy client. chainURLs maps chain ID to Alchemy RPC URL (e.g. from WALLET_CHAINS).
// When apiKey is set and chainURLs is empty, URLs are derived as https://{network}.g.alchemy.com/v2/{apiKey}.
func New(cfg Config) (*Client, error) {
	if cfg.APIKey == "" && cfg.BaseURL == "" && len(cfg.ChainURLs) == 0 {
		return nil, nil
	}
	client := &Client{
		httpClient: &http.Client{Timeout: 30 * time.Second},
		apiKey:     cfg.APIKey,
		baseURL:    strings.TrimSuffix(cfg.BaseURL, "/"),
		chainURLs:  make(map[int64]string),
	}
	for id, u := range cfg.ChainURLs {
		client.chainURLs[id] = strings.TrimSuffix(u, "/")
	}
	return client, nil
}

// urlForChain returns the Alchemy RPC URL for the given chain.
func (c *Client) urlForChain(chainID int64) (string, error) {
	var url string
	if u, ok := c.chainURLs[chainID]; ok && u != "" {
		url = u
	} else if network, ok := chainToAlchemyNetwork[chainID]; ok && c.apiKey != "" {
		// Prefer chain-specific URL from apiKey over baseURL, so Base (8453) doesn't
		// incorrectly use eth-mainnet when baseURL is from EVM_RPC_URL.
		url = fmt.Sprintf("https://%s.g.alchemy.com/v2/%s", network, c.apiKey)
	} else if c.baseURL != "" {
		url = c.baseURL
	} else {
		return "", fmt.Errorf("alchemy: no URL for chain %d", chainID)
	}
	log.Printf("[alchemy] chain %d -> %s", chainID, redactURL(url))
	return url, nil
}

// redactURL masks the API key in Alchemy URLs for safe logging.
func redactURL(u string) string {
	if idx := strings.Index(u, "/v2/"); idx >= 0 && idx+4 < len(u) {
		return u[:idx+4] + "***"
	}
	return u
}

// jsonRPC sends a JSON-RPC request and decodes the result into v.
func (c *Client) jsonRPC(ctx context.Context, url, method string, params interface{}, v interface{}) error {
	body := map[string]interface{}{
		"jsonrpc": "2.0",
		"id":      1,
		"method":  method,
		"params":  params,
	}
	if params == nil {
		body["params"] = []interface{}{}
	}
	enc, err := json.Marshal(body)
	if err != nil {
		return err
	}
	log.Printf("[alchemy] %s %s body=%s", "POST", redactURL(url), string(enc))
	req, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewReader(enc))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("alchemy: HTTP %d", resp.StatusCode)
	}
	var out struct {
		Error *struct {
			Message string `json:"message"`
		} `json:"error"`
		Result interface{} `json:"result"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return err
	}
	if out.Error != nil {
		return fmt.Errorf("alchemy: %s", out.Error.Message)
	}
	if v != nil && out.Result != nil {
		enc, _ := json.Marshal(out.Result)
		return json.Unmarshal(enc, v)
	}
	return nil
}

// TokenBalance from alchemy_getTokenBalances.
type TokenBalance struct {
	ContractAddress string `json:"contractAddress"`
	TokenBalance    string `json:"tokenBalance"`
	Error           string `json:"error,omitempty"`
}

// TokenBalancesResult from alchemy_getTokenBalances.
type TokenBalancesResult struct {
	Address       string         `json:"address"`
	TokenBalances []TokenBalance `json:"tokenBalances"`
}

// GetTokenBalances returns ERC-20 token balances for the address on the given chain.
// tokenSpec: "erc20" for all, "NATIVE_TOKEN" for native, or single contract address.
// Alchemy expects params as [address, tokenSpec] — two positional args, not an object.
func (c *Client) GetTokenBalances(ctx context.Context, chainID int64, address, tokenSpec string) (*TokenBalancesResult, error) {
	url, err := c.urlForChain(chainID)
	if err != nil {
		return nil, err
	}
	var params []interface{}
	spec := tokenSpec
	if spec == "" {
		spec = "erc20"
	}
	if spec == "erc20" || spec == "NATIVE_TOKEN" {
		params = []interface{}{address, spec}
	} else {
		// Specific tokens: [address, [contractAddrs], {pageKey?, maxCount?}]
		params = []interface{}{address, []string{spec}, map[string]interface{}{}}
	}
	var result TokenBalancesResult
	if err := c.jsonRPC(ctx, url, "alchemy_getTokenBalances", params, &result); err != nil {
		return nil, err
	}
	return &result, nil
}

// TokenMetadata from alchemy_getTokenMetadata.
type TokenMetadata struct {
	Name     string `json:"name"`
	Symbol   string `json:"symbol"`
	Decimals int    `json:"decimals"`
	Logo     string `json:"logo,omitempty"`
}

// GetTokenMetadata returns metadata for a token contract.
func (c *Client) GetTokenMetadata(ctx context.Context, chainID int64, contractAddress string) (*TokenMetadata, error) {
	url, err := c.urlForChain(chainID)
	if err != nil {
		return nil, err
	}
	var result TokenMetadata
	if err := c.jsonRPC(ctx, url, "alchemy_getTokenMetadata", []interface{}{contractAddress}, &result); err != nil {
		return nil, err
	}
	return &result, nil
}

// AssetTransfer from alchemy_getAssetTransfers.
type AssetTransfer struct {
	BlockNum string `json:"blockNum"`
	Hash     string `json:"hash"`
	From     string `json:"from"`
	To       string `json:"to"`
	Value    string `json:"value"`
	Asset    string `json:"asset"`
	Category string `json:"category"`
	Metadata *struct {
		BlockTimestamp string `json:"blockTimestamp"`
	} `json:"metadata,omitempty"`
}

// AssetTransfersResult from alchemy_getAssetTransfers.
type AssetTransfersResult struct {
	Transfers []AssetTransfer `json:"transfers"`
	PageKey   string          `json:"pageKey,omitempty"`
}

// GetAssetTransfers fetches historical transfers for an address.
// Params follow Alchemy docs: fromBlock, fromAddress, toAddress, excludeZeroValue, withMetadata, category, maxCount, pageKey.
// "order" is not a valid param; use fromBlock for range. withMetadata required for BlockTimestamp.
func (c *Client) GetAssetTransfers(ctx context.Context, chainID int64, fromAddress, toAddress string, categories []string, maxCount int, pageKey string) (*AssetTransfersResult, error) {
	url, err := c.urlForChain(chainID)
	if err != nil {
		return nil, err
	}
	params := map[string]interface{}{
		"fromBlock":        "0x0",
		"excludeZeroValue": true,
		"withMetadata":     true,
	}
	if fromAddress != "" {
		params["fromAddress"] = fromAddress
	}
	if toAddress != "" {
		params["toAddress"] = toAddress
	}
	if len(categories) > 0 {
		params["category"] = categories
	}
	if pageKey != "" {
		params["pageKey"] = pageKey
	}
	var result AssetTransfersResult
	if err := c.jsonRPC(ctx, url, "alchemy_getAssetTransfers", []interface{}{params}, &result); err != nil {
		return nil, err
	}
	return &result, nil
}

// SimulateAssetChangesRequest for alchemy_simulateAssetChanges.
type SimulateAssetChangesRequest struct {
	From  string `json:"from,omitempty"`
	To    string `json:"to"`
	Data  string `json:"data"`
	Value string `json:"value,omitempty"`
}

// AssetChange from simulation result.
type AssetChange struct {
	AssetType  string `json:"assetType"`
	ChangeType string `json:"changeType"`
	From       string `json:"from,omitempty"`
	To         string `json:"to,omitempty"`
	RawAmount  string `json:"rawAmount,omitempty"`
	Contract   string `json:"contract,omitempty"`
	Symbol     string `json:"symbol,omitempty"`
	Decimals   int    `json:"decimals,omitempty"`
}

// SimulateAssetChangesResult from alchemy_simulateAssetChanges.
type SimulateAssetChangesResult struct {
	Changes []AssetChange `json:"changes"`
	Error   *struct {
		Message string `json:"message"`
	} `json:"error,omitempty"`
}

// SimulateAssetChanges simulates a transaction and returns asset changes.
// Alchemy expects params as [transaction_object] — transaction is the sole element of the params array.
func (c *Client) SimulateAssetChanges(ctx context.Context, chainID int64, from, to, data, valueWei string) (*SimulateAssetChangesResult, error) {
	url, err := c.urlForChain(chainID)
	if err != nil {
		return nil, err
	}
	tx := map[string]interface{}{
		"to":   to,
		"data": data,
	}
	if from != "" {
		tx["from"] = from
	}
	if valueWei != "" && valueWei != "0" {
		tx["value"] = valueWei
	}
	var result SimulateAssetChangesResult
	if err := c.jsonRPC(ctx, url, "alchemy_simulateAssetChanges", []interface{}{tx}, &result); err != nil {
		return nil, err
	}
	if result.Error != nil {
		return &result, fmt.Errorf("simulation: %s", result.Error.Message)
	}
	return &result, nil
}

// GetTokenPrice returns USD price for a token. Alchemy Prices API uses REST.
// Endpoint: GET https://{network}.g.alchemy.com/v2/{key}/getTokenPrice?contractAddress=0x...
func (c *Client) GetTokenPrice(ctx context.Context, chainID int64, contractAddress string) (float64, error) {
	base, err := c.urlForChain(chainID)
	if err != nil {
		return 0, err
	}
	u := base + "/getTokenPrice?contractAddress=" + contractAddress
	log.Printf("[alchemy] GET %s", redactURL(u))
	req, err := http.NewRequestWithContext(ctx, "GET", u, nil)
	if err != nil {
		return 0, err
	}
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return 0, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return 0, fmt.Errorf("getTokenPrice: HTTP %d", resp.StatusCode)
	}
	var out struct {
		USD float64 `json:"usdPrice"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return 0, err
	}
	return out.USD, nil
}
