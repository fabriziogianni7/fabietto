package chains

import (
	"testing"
)

func TestParseChainsJSON(t *testing.T) {
	tests := []struct {
		name    string
		json    string
		wantLen int
		wantErr bool
	}{
		{"empty", "", 0, false},
		{"empty array", "[]", 0, false},
		{"single chain", `[{"chain_id":1,"rpc_url":"https://eth.llamarpc.com","explorer":"https://etherscan.io","name":"Ethereum"}]`, 1, false},
		{"multiple", `[{"chain_id":1,"rpc_url":"u1","name":"Ethereum"},{"chain_id":137,"rpc_url":"u2","name":"Polygon"}]`, 2, false},
		{"invalid json", `[`, 0, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfgs, err := ParseChainsJSON(tt.json)
			if (err != nil) != tt.wantErr {
				t.Errorf("ParseChainsJSON() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if len(cfgs) != tt.wantLen {
				t.Errorf("ParseChainsJSON() len = %d, want %d", len(cfgs), tt.wantLen)
			}
		})
	}
}

func TestRegistry_ExplorerURL(t *testing.T) {
	r := NewRegistry()
	r.AddChain(&Chain{ChainID: 1, RPCURL: "https://x.com", Explorer: "https://etherscan.io", Name: "Ethereum"})

	url := r.GetExplorerURL(1, "0xabc123")
	if url != "https://etherscan.io/tx/0xabc123" {
		t.Errorf("GetExplorerURL = %q, want https://etherscan.io/tx/0xabc123", url)
	}
	url2 := r.GetExplorerURL(1, "abc123")
	if url2 != "https://etherscan.io/tx/0xabc123" {
		t.Errorf("GetExplorerURL(no 0x) = %q, want https://etherscan.io/tx/0xabc123", url2)
	}
}

func TestRegistry_ResolveChainID_Default(t *testing.T) {
	r := NewRegistry()
	r.AddChain(&Chain{ChainID: 1, RPCURL: "https://x.com", Name: "Ethereum"})
	r.SetDefaultChain(1)

	// 0 = use default
	got, err := r.ResolveChainID(0)
	if err != nil {
		t.Fatalf("ResolveChainID(0) err = %v", err)
	}
	if got != 1 {
		t.Errorf("ResolveChainID(0) = %d, want 1", got)
	}
}

func TestRegistry_ResolveChainID_Explicit(t *testing.T) {
	r := NewRegistry()
	r.AddChain(&Chain{ChainID: 1, RPCURL: "https://x.com", Name: "Ethereum"})
	r.AddChain(&Chain{ChainID: 137, RPCURL: "https://y.com", Name: "Polygon"})
	r.SetDefaultChain(1)

	got, err := r.ResolveChainID(137)
	if err != nil {
		t.Fatalf("ResolveChainID(137) err = %v", err)
	}
	if got != 137 {
		t.Errorf("ResolveChainID(137) = %d, want 137", got)
	}
}

func TestRegistry_ResolveChainID_Unknown(t *testing.T) {
	r := NewRegistry()
	r.AddChain(&Chain{ChainID: 1, RPCURL: "https://x.com", Name: "Ethereum"})
	r.SetDefaultChain(1)

	_, err := r.ResolveChainID(999)
	if err == nil {
		t.Error("ResolveChainID(999) expected error for unknown chain")
	}
}

func TestRegistry_ResolveChainID_NoDefault(t *testing.T) {
	r := NewRegistry()
	r.AddChain(&Chain{ChainID: 1, RPCURL: "https://x.com", Name: "Ethereum"})
	// no SetDefaultChain

	_, err := r.ResolveChainID(0)
	if err == nil {
		t.Error("ResolveChainID(0) expected error when no default")
	}
}

func TestBuildFromConfig_ChainsJSON(t *testing.T) {
	json := `[{"chain_id":1,"rpc_url":"https://eth.llamarpc.com","explorer":"https://etherscan.io","name":"Ethereum"}]`
	r, err := BuildFromConfig(json, "", 0, 0)
	if err != nil {
		t.Fatalf("BuildFromConfig err = %v", err)
	}
	if r.DefaultChainID() != 1 {
		t.Errorf("DefaultChainID = %d, want 1", r.DefaultChainID())
	}
	if r.GetChainName(1) != "Ethereum" {
		t.Errorf("GetChainName = %q, want Ethereum", r.GetChainName(1))
	}
}

func TestBuildFromConfig_SingleChain(t *testing.T) {
	r, err := BuildFromConfig("", "https://eth.llamarpc.com", 1, 0)
	if err != nil {
		t.Fatalf("BuildFromConfig err = %v", err)
	}
	if r.DefaultChainID() != 1 {
		t.Errorf("DefaultChainID = %d, want 1", r.DefaultChainID())
	}
	if r.GetExplorerURL(1, "0xabc") != "https://etherscan.io/tx/0xabc" {
		t.Errorf("explorer URL for chain 1 should be etherscan")
	}
}

func TestRegistry_MonadExplorer(t *testing.T) {
	r, err := BuildFromConfig("", "https://monad-rpc.example.com", 143, 0)
	if err != nil {
		t.Fatalf("BuildFromConfig err = %v", err)
	}
	url := r.GetExplorerURL(143, "0xabc123def456")
	if url != "https://monadscan.com/tx/0xabc123def456" {
		t.Errorf("Monad explorer URL = %q, want https://monadscan.com/tx/0xabc123def456", url)
	}
}

func TestRegistry_ChainURLs(t *testing.T) {
	json := `[
		{"chain_id":1,"rpc_url":"https://eth-mainnet.g.alchemy.com/v2/key1","explorer":"https://etherscan.io","name":"Ethereum"},
		{"chain_id":137,"rpc_url":"https://polygon-rpc.com","explorer":"https://polygonscan.com","name":"Polygon"}
	]`
	r, err := BuildFromConfig(json, "", 0, 0)
	if err != nil {
		t.Fatalf("BuildFromConfig err = %v", err)
	}
	urls := r.ChainURLs("alchemy.com")
	if len(urls) != 1 {
		t.Errorf("ChainURLs(alchemy.com) len = %d, want 1", len(urls))
	}
	if u, ok := urls[1]; !ok || u != "https://eth-mainnet.g.alchemy.com/v2/key1" {
		t.Errorf("ChainURLs(alchemy.com)[1] = %q", urls[1])
	}
	all := r.ChainURLs("")
	if len(all) != 2 {
		t.Errorf("ChainURLs(empty) len = %d, want 2", len(all))
	}
}
