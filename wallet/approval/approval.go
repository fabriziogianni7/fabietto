package approval

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"custom-agent/wallet/account"
)

// Pending holds a pending approval request.
type Pending struct {
	ID          string           `json:"id"`
	Action      *account.Action  `json:"action"`
	ChainID     int64            `json:"chain_id"`
	Summary     string           `json:"summary"`
	Platform    string           `json:"platform"`
	UserID      string           `json:"user_id"`
	ChatID      string           `json:"chat_id"`
	CreatedAt   time.Time        `json:"created_at"`
	ExpiresAt   time.Time        `json:"expires_at"`
	Simulation  string           `json:"simulation,omitempty"`
}

// Store manages pending approvals with TTL.
type Store struct {
	mu       sync.RWMutex
	pending  map[string]*Pending
	dir      string
	ttl      time.Duration
}

// NewStore creates an approval store. dir is for persistence (optional); ttl is default expiry.
func NewStore(dir string, ttl time.Duration) *Store {
	if ttl <= 0 {
		ttl = 15 * time.Minute
	}
	s := &Store{
		pending: make(map[string]*Pending),
		dir:     dir,
		ttl:     ttl,
	}
	if dir != "" {
		_ = os.MkdirAll(dir, 0700)
	}
	return s
}

// Add adds a pending approval and returns its ID.
func (s *Store) Add(p *Pending) (string, error) {
	if p.ID == "" {
		p.ID = fmt.Sprintf("tx_%d", time.Now().UnixNano())
	}
	if p.ExpiresAt.IsZero() {
		p.ExpiresAt = time.Now().Add(s.ttl)
	}
	if p.CreatedAt.IsZero() {
		p.CreatedAt = time.Now()
	}
	s.mu.Lock()
	s.pending[p.ID] = p
	s.mu.Unlock()
	return p.ID, nil
}

// Get returns a pending approval by ID.
func (s *Store) Get(id string) (*Pending, bool) {
	s.mu.RLock()
	p, ok := s.pending[id]
	s.mu.RUnlock()
	if !ok {
		return nil, false
	}
	if time.Now().After(p.ExpiresAt) {
		s.Remove(id)
		return nil, false
	}
	return p, true
}

// Remove removes a pending approval.
func (s *Store) Remove(id string) {
	s.mu.Lock()
	delete(s.pending, id)
	s.mu.Unlock()
}

// List returns all non-expired pending approvals for a user.
func (s *Store) List(platform, userID string) []*Pending {
	s.mu.RLock()
	defer s.mu.RUnlock()
	now := time.Now()
	var out []*Pending
	for _, p := range s.pending {
		if p.Platform == platform && p.UserID == userID && p.ExpiresAt.After(now) {
			out = append(out, p)
		}
	}
	return out
}

// ParseApprovalMessage extracts an approval ID from "approve: tx_123" or "/approve_tx tx_123".
func ParseApprovalMessage(text string) (id string, ok bool) {
	text = strings.TrimSpace(text)
	if text == "" {
		return "", false
	}
	lower := strings.ToLower(text)
	if strings.HasPrefix(lower, "approve:") {
		return strings.TrimSpace(text[8:]), true
	}
	if strings.HasPrefix(lower, "/approve_tx ") {
		return strings.TrimSpace(text[12:]), true
	}
	return "", false
}

// FormatPendingForNotification returns a short message for the guardian.
func FormatPendingForNotification(p *Pending) string {
	return fmt.Sprintf("Transaction approval required: %s\nID: %s\nReply with: approve: %s", p.Summary, p.ID, p.ID)
}

// Save and Load for persistence (optional)
func (s *Store) save() error {
	if s.dir == "" {
		return nil
	}
	s.mu.RLock()
	data, err := json.MarshalIndent(s.pending, "", "  ")
	s.mu.RUnlock()
	if err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(s.dir, "approvals.json"), data, 0600)
}
