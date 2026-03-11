package conversation

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"custom-agent/embedding"
	"custom-agent/session"
)

const (
	storeDir = "conversation_embeddings"
)

// Entry is a conversation message with embedding for retrieval.
type Entry struct {
	Role      string    `json:"role"`
	Content   string    `json:"content"`
	Embedding []float32 `json:"embedding,omitempty"`
}

// Store holds conversation embeddings for semantic retrieval.
type Store struct {
	embedder embedding.Embedder
}

// NewStore creates a conversation store. embedder may be nil (no retrieval).
func NewStore(embedder embedding.Embedder) *Store {
	return &Store{embedder: embedder}
}

func storePath(platform, userID string) string {
	key := session.SessionKey(platform, userID)
	return filepath.Join(storeDir, key+".jsonl")
}

// Add stores a message with embedding for retrieval.
func (s *Store) Add(platform, userID, role, content string) error {
	if s.embedder == nil {
		return nil
	}
	content = strings.TrimSpace(content)
	if content == "" {
		return nil
	}

	emb, err := s.embedder.Embed(content)
	if err != nil {
		return nil // Skip embedding on failure; retrieval will fall back
	}

	if err := os.MkdirAll(storeDir, 0755); err != nil {
		return err
	}

	e := Entry{Role: role, Content: content, Embedding: emb}
	path := storePath(platform, userID)
	f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0600)
	if err != nil {
		return err
	}
	defer f.Close()
	return json.NewEncoder(f).Encode(e)
}

// Search returns the most relevant past messages for the query.
func (s *Store) Search(platform, userID, query string, limit int) ([]Entry, error) {
	if s.embedder == nil || limit <= 0 {
		return nil, nil
	}

	path := storePath(platform, userID)
	f, err := os.Open(path)
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
		if len(e.Embedding) > 0 {
			entries = append(entries, e)
		}
	}
	if err := scanner.Err(); err != nil {
		return nil, err
	}

	if len(entries) == 0 || strings.TrimSpace(query) == "" {
		return nil, nil
	}

	queryEmb, err := s.embedder.Embed(query)
	if err != nil {
		return nil, nil
	}

	type scored struct {
		e     Entry
		score float32
	}
	var scoredList []scored
	for _, e := range entries {
		score := embedding.CosineSimilarity(queryEmb, e.Embedding)
		scoredList = append(scoredList, scored{e, score})
	}
	// Sort by score descending
	for i := 0; i < len(scoredList)-1; i++ {
		for j := i + 1; j < len(scoredList); j++ {
			if scoredList[j].score > scoredList[i].score {
				scoredList[i], scoredList[j] = scoredList[j], scoredList[i]
			}
		}
	}

	result := make([]Entry, 0, limit)
	for i := 0; i < limit && i < len(scoredList); i++ {
		result = append(result, scoredList[i].e)
	}
	return result, nil
}

// Clear removes the conversation store for a user (e.g. on /new).
func Clear(platform, userID string) error {
	path := storePath(platform, userID)
	err := os.Remove(path)
	if err != nil && os.IsNotExist(err) {
		return nil
	}
	return err
}

// FormatEntries returns a string for prompt injection.
func FormatEntries(entries []Entry) string {
	if len(entries) == 0 {
		return ""
	}
	var b strings.Builder
	b.WriteString("--- Relevant past context ---\n")
	for _, e := range entries {
		b.WriteString(fmt.Sprintf("%s: %s\n", e.Role, e.Content))
	}
	b.WriteString("--- End relevant past context ---")
	return b.String()
}
