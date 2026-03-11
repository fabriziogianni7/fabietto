package chains

import (
	"encoding/json"
	"fmt"
	"math/big"
	"strings"
	"sync"

	"custom-agent/wallet/provider"
)

// Chain holds config for an EVM chain.
type Chain struct {
	ChainID  int64  `json:"chain_id"`
	RPCURL   string `json:"rpc_url"`
	Explorer string `json:"explorer"` // base URL, e.g. https://etherscan.io
	Name     string `json:"name"`
}

// Registry maps chain IDs to chain config and providers.
type Registry struct {
	mu             sync.RWMutex
	chains         map[int64]*Chain
	providers      map[int64]*provider.Provider
	defaultChainID int64
}

// NewRegistry creates an empty registry.
func NewRegistry() *Registry {
	return &Registry{
		chains:    make(map[int64]*Chain),
		providers: make(map[int64]*provider.Provider),
	}
}

// ChainConfig is the JSON shape for parsing WALLET_CHAINS.
type ChainConfig struct {
	ChainID  int64  `json:"chain_id"`
	RPCURL   string `json:"rpc_url"`
	Explorer string `json:"explorer"`
	Name     string `json:"name"`
}

// ParseChainsJSON parses a JSON array of chain configs.
func ParseChainsJSON(data string) ([]ChainConfig, error) {
	data = strings.TrimSpace(data)
	if data == "" {
		return nil, nil
	}
	var cfg []ChainConfig
	if err := json.Unmarshal([]byte(data), &cfg); err != nil {
		return nil, err
	}
	return cfg, nil
}

// AddChain adds a chain and lazily creates its provider on first use.
func (r *Registry) AddChain(c *Chain) error {
	if c.ChainID <= 0 {
		return fmt.Errorf("chain_id must be positive, got %d", c.ChainID)
	}
	if c.RPCURL == "" {
		return fmt.Errorf("rpc_url required for chain %d", c.ChainID)
	}
	c.Explorer = strings.TrimSuffix(c.Explorer, "/")
	if c.Name == "" {
		c.Name = fmt.Sprintf("Chain %d", c.ChainID)
	}
	r.mu.Lock()
	r.chains[c.ChainID] = c
	r.mu.Unlock()
	return nil
}

// SetDefaultChain sets the default chain ID.
func (r *Registry) SetDefaultChain(chainID int64) {
	r.mu.Lock()
	r.defaultChainID = chainID
	r.mu.Unlock()
}

// DefaultChainID returns the default chain ID.
func (r *Registry) DefaultChainID() int64 {
	r.mu.RLock()
	id := r.defaultChainID
	r.mu.RUnlock()
	return id
}

// ResolveChainID returns chainID if valid, else default. Returns 0 if neither is configured.
func (r *Registry) ResolveChainID(chainID int64) (int64, error) {
	if chainID > 0 {
		r.mu.RLock()
		_, ok := r.chains[chainID]
		r.mu.RUnlock()
		if !ok {
			return 0, fmt.Errorf("chain %d not configured", chainID)
		}
		return chainID, nil
	}
	def := r.DefaultChainID()
	if def <= 0 {
		return 0, fmt.Errorf("no default chain configured")
	}
	return def, nil
}

// GetProvider returns the provider for the chain, creating it lazily.
func (r *Registry) GetProvider(chainID int64) (*provider.Provider, error) {
	r.mu.RLock()
	c, ok := r.chains[chainID]
	p, hasProvider := r.providers[chainID]
	r.mu.RUnlock()
	if !ok {
		return nil, fmt.Errorf("chain %d not configured", chainID)
	}
	if hasProvider {
		return p, nil
	}
	prov, err := provider.New(c.RPCURL)
	if err != nil {
		return nil, err
	}
	r.mu.Lock()
	r.providers[chainID] = prov
	r.mu.Unlock()
	return prov, nil
}

// GetExplorerURL returns the block explorer URL for a tx.
func (r *Registry) GetExplorerURL(chainID int64, txHash string) string {
	r.mu.RLock()
	c, ok := r.chains[chainID]
	r.mu.RUnlock()
	if !ok || c.Explorer == "" {
		return ""
	}
	txHash = strings.TrimPrefix(txHash, "0x")
	return strings.TrimSuffix(c.Explorer, "/") + "/tx/0x" + txHash
}

// GetChainName returns the human-readable chain name.
func (r *Registry) GetChainName(chainID int64) string {
	r.mu.RLock()
	c, ok := r.chains[chainID]
	r.mu.RUnlock()
	if !ok {
		return fmt.Sprintf("Chain %d", chainID)
	}
	return c.Name
}

// ChainIDs returns all configured chain IDs.
func (r *Registry) ChainIDs() []int64 {
	r.mu.RLock()
	defer r.mu.RUnlock()
	ids := make([]int64, 0, len(r.chains))
	for id := range r.chains {
		ids = append(ids, id)
	}
	return ids
}

// BigInt returns chainID as *big.Int for signing.
func (r *Registry) BigInt(chainID int64) *big.Int {
	return big.NewInt(chainID)
}

// BuildFromConfig creates a registry from env-style config.
// chainsJSON: JSON array of chain configs. If empty, uses singleChainRPC+singleChainID.
// defaultChainID: 0 = use first chain or singleChainID.
func BuildFromConfig(chainsJSON, singleChainRPC string, singleChainID, defaultChainID int64) (*Registry, error) {
	r := NewRegistry()
	if chainsJSON != "" {
		cfgs, err := ParseChainsJSON(chainsJSON)
		if err != nil {
			return nil, err
		}
		for i := range cfgs {
			c := &Chain{
				ChainID:  cfgs[i].ChainID,
				RPCURL:   cfgs[i].RPCURL,
				Explorer: cfgs[i].Explorer,
				Name:     cfgs[i].Name,
			}
			if err := r.AddChain(c); err != nil {
				return nil, err
			}
		}
		if defaultChainID > 0 {
			r.SetDefaultChain(defaultChainID)
		} else if len(cfgs) > 0 {
			r.SetDefaultChain(cfgs[0].ChainID)
		}
	} else if singleChainRPC != "" && singleChainID > 0 {
		c := &Chain{
			ChainID:  singleChainID,
			RPCURL:   singleChainRPC,
			Explorer: explorerForKnownChain(singleChainID),
			Name:     nameForKnownChain(singleChainID),
		}
		if err := r.AddChain(c); err != nil {
			return nil, err
		}
		r.SetDefaultChain(singleChainID)
	}
	return r, nil
}

func explorerForKnownChain(chainID int64) string {
	switch chainID {
	case 1:
		return "https://etherscan.io"
	case 11155111:
		return "https://sepolia.etherscan.io"
	case 8453:
		return "https://basescan.org"
	case 10:
		return "https://optimistic.etherscan.io"
	case 42161:
		return "https://arbiscan.io"
	case 137:
		return "https://polygonscan.com"
	case 143:
		return "https://monadscan.com"
	default:
		return ""
	}
}

func nameForKnownChain(chainID int64) string {
	switch chainID {
	case 1:
		return "Ethereum"
	case 11155111:
		return "Sepolia"
	case 8453:
		return "Base"
	case 10:
		return "Optimism"
	case 42161:
		return "Arbitrum"
	case 137:
		return "Polygon"
	case 143:
		return "Monad"
	default:
		return ""
	}
}
