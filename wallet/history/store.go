package history

import (
	"bufio"
	"encoding/json"
	"os"
	"path/filepath"
	"sort"
	"sync"
	"time"
)

const historyDir = "wallet-history"
const historyFile = "transactions.jsonl"

// Entry is a single transaction history record.
type Entry struct {
	ID            string    `json:"id"`
	Timestamp     time.Time `json:"timestamp"`
	ChainID       int64     `json:"chain_id"`
	ChainName     string    `json:"chain_name"`
	WalletAddress string    `json:"wallet_address"`
	TxHash        string    `json:"tx_hash"`
	ExplorerURL   string    `json:"explorer_url"`
	Status        string    `json:"status"` // submitted, confirmed, failed
	ActionType    string    `json:"action_type"`
	ToAddress     string    `json:"to_address"`
	ValueWei      string    `json:"value_wei"`
	Platform      string    `json:"platform,omitempty"`
	UserID        string    `json:"user_id,omitempty"`
	ChatID        string    `json:"chat_id,omitempty"`
	ApprovalID    string    `json:"approval_id,omitempty"`
}

// Store persists transaction history.
type Store struct {
	mu   sync.RWMutex
	dir  string
	path string
}

// NewStore creates a history store.
func NewStore(dir string) *Store {
	if dir == "" {
		dir = historyDir
	}
	return &Store{
		dir:  dir,
		path: filepath.Join(dir, historyFile),
	}
}

// Add appends an entry. ID is auto-generated if empty.
func (s *Store) Add(e *Entry) error {
	if e.ID == "" {
		suffix := e.TxHash
		if len(suffix) > 10 {
			suffix = suffix[:10]
		}
		if suffix == "" {
			suffix = "pending"
		}
		e.ID = "h_" + time.Now().Format("20060102150405") + "_" + suffix
	}
	if e.Timestamp.IsZero() {
		e.Timestamp = time.Now()
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := os.MkdirAll(s.dir, 0700); err != nil {
		return err
	}
	f, err := os.OpenFile(s.path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0600)
	if err != nil {
		return err
	}
	defer f.Close()
	enc := json.NewEncoder(f)
	if err := enc.Encode(e); err != nil {
		return err
	}
	return f.Sync()
}

// List returns entries, optionally filtered by chainID, limit. Sorted by timestamp desc.
func (s *Store) List(chainID int64, limit int) ([]Entry, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	f, err := os.Open(s.path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	defer f.Close()
	var entries []Entry
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		var e Entry
		if err := json.Unmarshal(scanner.Bytes(), &e); err != nil {
			continue
		}
		if chainID > 0 && e.ChainID != chainID {
			continue
		}
		entries = append(entries, e)
	}
	if err := scanner.Err(); err != nil {
		return nil, err
	}
	sort.Slice(entries, func(i, j int) bool {
		return entries[i].Timestamp.After(entries[j].Timestamp)
	})
	if limit > 0 && len(entries) > limit {
		entries = entries[:limit]
	}
	return entries, nil
}
