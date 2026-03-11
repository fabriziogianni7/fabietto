package history

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestStore_AddAndList(t *testing.T) {
	dir := t.TempDir()
	s := NewStore(dir)

	e := &Entry{
		ChainID:       1,
		ChainName:     "Ethereum",
		WalletAddress: "0x123",
		TxHash:        "0xabc",
		ExplorerURL:   "https://etherscan.io/tx/0xabc",
		Status:        "submitted",
		ActionType:    "transfer",
		ToAddress:     "0x456",
		ValueWei:      "1000000",
	}
	if err := s.Add(e); err != nil {
		t.Fatalf("Add err = %v", err)
	}

	entries, err := s.List(0, 10)
	if err != nil {
		t.Fatalf("List err = %v", err)
	}
	if len(entries) != 1 {
		t.Fatalf("List len = %d, want 1", len(entries))
	}
	if entries[0].TxHash != "0xabc" || entries[0].ChainName != "Ethereum" {
		t.Errorf("entry = %+v", entries[0])
	}
}

func TestStore_ListFilterByChain(t *testing.T) {
	dir := t.TempDir()
	s := NewStore(dir)

	s.Add(&Entry{ChainID: 1, TxHash: "0xa", Timestamp: time.Now()})
	s.Add(&Entry{ChainID: 137, TxHash: "0xb", Timestamp: time.Now()})
	s.Add(&Entry{ChainID: 1, TxHash: "0xc", Timestamp: time.Now()})

	entries, err := s.List(1, 0)
	if err != nil {
		t.Fatalf("List err = %v", err)
	}
	if len(entries) != 2 {
		t.Errorf("List(chain 1) len = %d, want 2", len(entries))
	}
	for _, e := range entries {
		if e.ChainID != 1 {
			t.Errorf("entry chain = %d, want 1", e.ChainID)
		}
	}
}

func TestStore_ListLimit(t *testing.T) {
	dir := t.TempDir()
	s := NewStore(dir)

	for i := 0; i < 5; i++ {
		s.Add(&Entry{ChainID: 1, TxHash: fmt.Sprintf("0x%x", i), Timestamp: time.Now()})
	}

	entries, err := s.List(0, 2)
	if err != nil {
		t.Fatalf("List err = %v", err)
	}
	if len(entries) != 2 {
		t.Errorf("List(limit 2) len = %d, want 2", len(entries))
	}
}

func TestStore_EmptyFile(t *testing.T) {
	dir := t.TempDir()
	s := NewStore(dir)

	entries, err := s.List(0, 10)
	if err != nil {
		t.Fatalf("List err = %v", err)
	}
	if len(entries) != 0 {
		t.Errorf("List empty = %d, want 0", len(entries))
	}
}

func TestStore_NoDir(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "sub", "wallet-history")
	s := NewStore(dir)

	e := &Entry{ChainID: 1, TxHash: "0xabc"}
	if err := s.Add(e); err != nil {
		t.Fatalf("Add should create dir, err = %v", err)
	}
	if _, err := os.Stat(dir); err != nil {
		t.Errorf("dir not created: %v", err)
	}
}
