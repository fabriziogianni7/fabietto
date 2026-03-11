package reminders

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

const remindersDir = "reminders"

// Reminder is a scheduled message to send to a user.
type Reminder struct {
	ID        string    `json:"id"`
	Platform  string    `json:"platform"`
	UserID    string    `json:"user_id"`
	ChatID    string    `json:"chat_id"`
	Schedule  string    `json:"schedule"` // cron expr, e.g. "0 9 * * *"
	Message   string    `json:"message"`
	CreatedAt time.Time `json:"created_at"`
}

// Store persists reminders to disk.
type Store struct {
	mu sync.RWMutex
}

// NewStore creates a reminder store.
func NewStore() *Store {
	return &Store{}
}

func path() string {
	return filepath.Join(remindersDir, "reminders.jsonl")
}

// Add appends a reminder and returns its ID.
func (s *Store) Add(platform, userID, chatID, schedule, message string) (string, error) {
	if err := os.MkdirAll(remindersDir, 0755); err != nil {
		return "", err
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	r := Reminder{
		ID:        fmt.Sprintf("%d", time.Now().UnixNano()),
		Platform:  strings.TrimSpace(platform),
		UserID:    strings.TrimSpace(userID),
		ChatID:    strings.TrimSpace(chatID),
		Schedule:  strings.TrimSpace(schedule),
		Message:   strings.TrimSpace(message),
		CreatedAt: time.Now(),
	}
	if r.ChatID == "" {
		r.ChatID = r.UserID
	}

	f, err := os.OpenFile(path(), os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0600)
	if err != nil {
		return "", err
	}
	defer f.Close()

	enc := json.NewEncoder(f)
	if err := enc.Encode(r); err != nil {
		return "", err
	}
	return r.ID, nil
}

// List returns all reminders.
func (s *Store) List() ([]Reminder, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	f, err := os.Open(path())
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	defer f.Close()

	var out []Reminder
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		var r Reminder
		if err := json.Unmarshal(scanner.Bytes(), &r); err != nil {
			continue
		}
		out = append(out, r)
	}
	return out, scanner.Err()
}

// Delete removes a reminder by ID. If platform and userID are non-empty, only deletes if they match (ownership check).
func (s *Store) Delete(id string, platform, userID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	f, err := os.Open(path())
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	defer f.Close()

	var kept []Reminder
	found := false
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		var r Reminder
		if err := json.Unmarshal(scanner.Bytes(), &r); err != nil {
			continue
		}
		if r.ID == id {
			found = true
			if platform != "" && userID != "" && (r.Platform != platform || r.UserID != userID) {
				kept = append(kept, r) // keep it, not owned by caller
			}
			continue
		}
		kept = append(kept, r)
	}
	if err := scanner.Err(); err != nil {
		return err
	}
	if !found {
		return nil // id not found, nothing to delete
	}

	// Rewrite file
	if err := f.Close(); err != nil {
		return err
	}
	tf, err := os.Create(path())
	if err != nil {
		return err
	}
	enc := json.NewEncoder(tf)
	for _, r := range kept {
		_ = enc.Encode(r)
	}
	return tf.Close()
}

// ListForPlatform returns reminders for a given platform and user.
func (s *Store) ListForPlatform(platform, userID string) ([]Reminder, error) {
	all, err := s.List()
	if err != nil {
		return nil, err
	}
	var out []Reminder
	for _, r := range all {
		if r.Platform == platform && r.UserID == userID {
			out = append(out, r)
		}
	}
	return out, nil
}
