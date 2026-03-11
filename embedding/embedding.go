package embedding

import (
	"bytes"
	"encoding/json"
	"fmt"
	"math"
	"net/http"
	"strings"
)

const defaultModel = "nomic-embed-text"

// Embedder produces vector embeddings for text.
type Embedder interface {
	Embed(text string) ([]float32, error)
}

// Client embeds text via local Ollama.
type Client struct {
	BaseURL string
	Model   string
	client  *http.Client
}

// NewClient creates an embedding client. baseURL is e.g. "http://localhost:11434".
func NewClient(baseURL string) *Client {
	baseURL = strings.TrimSuffix(baseURL, "/")
	if baseURL == "" {
		baseURL = "http://localhost:11434"
	}
	return &Client{
		BaseURL: baseURL,
		Model:   defaultModel,
		client:  &http.Client{},
	}
}

// SetModel sets the embedding model (e.g. "nomic-embed-text", "mxbai-embed-large").
func (c *Client) SetModel(model string) {
	if model != "" {
		c.Model = model
	}
}

// Embed returns the embedding vector for the given text.
func (c *Client) Embed(text string) ([]float32, error) {
	if strings.TrimSpace(text) == "" {
		return nil, fmt.Errorf("empty text")
	}

	body, err := json.Marshal(map[string]string{
		"model":  c.Model,
		"prompt": text,
	})
	if err != nil {
		return nil, err
	}

	req, err := http.NewRequest("POST", c.BaseURL+"/api/embeddings", bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("ollama returned status %d", resp.StatusCode)
	}

	var result struct {
		Embedding []float32 `json:"embedding"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, err
	}
	return result.Embedding, nil
}

// CosineSimilarity returns cosine similarity between two vectors (-1 to 1, higher = more similar).
func CosineSimilarity(a, b []float32) float32 {
	if len(a) != len(b) || len(a) == 0 {
		return 0
	}
	var dot, normA, normB float32
	for i := range a {
		dot += a[i] * b[i]
		normA += a[i] * a[i]
		normB += b[i] * b[i]
	}
	if normA == 0 || normB == 0 {
		return 0
	}
	return dot / (float32(math.Sqrt(float64(normA))) * float32(math.Sqrt(float64(normB))))
}
