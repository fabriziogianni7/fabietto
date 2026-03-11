package memory

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"custom-agent/embedding"
	"custom-agent/session"
)

const (
	memoryDir = "memories"
)

// Memory is a stored memory with optional embedding.
type Memory struct {
	ID        string    `json:"id"`
	Content   string    `json:"content"`
	Tags      string    `json:"tags,omitempty"`
	CreatedAt time.Time `json:"created_at"`
	Embedding []float32 `json:"embedding,omitempty"` // cached; empty if embeddings unavailable
}

// Store handles long-term memory with optional embedding-based search.
type Store struct {
	embedder embedding.Embedder
}

// NewStore creates a memory store. embedder may be nil for keyword-only search.
func NewStore(embedder embedding.Embedder) *Store {
	return &Store{embedder: embedder}
}

func memoryPath(platform, userID string) string {
	key := session.SessionKey(platform, userID)
	return filepath.Join(memoryDir, key+".jsonl")
}

// Save stores a memory. Embeds and caches if embedder is available.
func (s *Store) Save(platform, userID, content, tags string) error {
	if err := os.MkdirAll(memoryDir, 0755); err != nil {
		return err
	}

	m := Memory{
		ID:        fmt.Sprintf("%d", time.Now().UnixNano()),
		Content:   strings.TrimSpace(content),
		Tags:      strings.TrimSpace(tags),
		CreatedAt: time.Now(),
	}

	if s.embedder != nil {
		emb, err := s.embedder.Embed(m.Content)
		if err == nil {
			m.Embedding = emb
		}
		// If embed fails (Ollama down), save without embedding; keyword search will work
	}

	path := memoryPath(platform, userID)
	f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0600)
	if err != nil {
		return err
	}
	defer f.Close()

	enc := json.NewEncoder(f)
	return enc.Encode(m)
}

// Search returns memories matching the query. Uses embeddings if available, else keyword search.
func (s *Store) Search(platform, userID, query string, limit int) ([]Memory, error) {
	if limit <= 0 {
		limit = 5
	}

	path := memoryPath(platform, userID)
	f, err := os.Open(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	defer f.Close()

	var memories []Memory
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		var m Memory
		if err := json.Unmarshal(scanner.Bytes(), &m); err != nil {
			continue
		}
		memories = append(memories, m)
	}
	if err := scanner.Err(); err != nil {
		return nil, err
	}

	if len(memories) == 0 {
		return nil, nil
	}

	query = strings.ToLower(strings.TrimSpace(query))

	// Try embedding search first
	if s.embedder != nil && query != "" {
		queryEmb, err := s.embedder.Embed(query)
		if err == nil {
			// Score each memory by cosine similarity (only those with embeddings)
			type scored struct {
				m     Memory
				score float32
			}
			var scoredList []scored
			for _, m := range memories {
				if len(m.Embedding) > 0 {
					score := embedding.CosineSimilarity(queryEmb, m.Embedding)
					scoredList = append(scoredList, scored{m, score})
				} else {
					// No embedding: use keyword match as fallback score
					content := strings.ToLower(m.Content)
					tags := strings.ToLower(m.Tags)
					score := float32(0)
					if strings.Contains(content, query) || strings.Contains(tags, query) {
						score = 0.5
					}
					scoredList = append(scoredList, scored{m, score})
				}
			}
			// Sort by score descending (simple bubble for small n)
			for i := 0; i < len(scoredList)-1; i++ {
				for j := i + 1; j < len(scoredList); j++ {
					if scoredList[j].score > scoredList[i].score {
						scoredList[i], scoredList[j] = scoredList[j], scoredList[i]
					}
				}
			}
			// Take top limit
			result := make([]Memory, 0, limit)
			for i := 0; i < limit && i < len(scoredList); i++ {
				result = append(result, scoredList[i].m)
			}
			return result, nil
		}
		// Embed failed: fall through to keyword search
	}

	// Keyword search fallback
	queryWords := strings.Fields(query)
	var matched []Memory
	for _, m := range memories {
		content := strings.ToLower(m.Content)
		tags := strings.ToLower(m.Tags)
		matches := 0
		for _, w := range queryWords {
			if len(w) < 2 {
				continue
			}
			if strings.Contains(content, w) || strings.Contains(tags, w) {
				matches++
			}
		}
		if matches > 0 {
			matched = append(matched, m)
		}
	}
	// Return most recent matches, up to limit
	start := len(matched) - limit
	if start < 0 {
		start = 0
	}
	result := make([]Memory, 0, limit)
	for i := len(matched) - 1; i >= start && len(result) < limit; i-- {
		result = append(result, matched[i])
	}
	return result, nil
}
