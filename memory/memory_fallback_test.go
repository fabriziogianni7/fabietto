package memory

import (
	"errors"
	"os"
	"testing"
)

type failingEmbedder struct{}

func (f *failingEmbedder) Embed(text string) ([]float32, error) {
	return nil, errors.New("embedder unavailable")
}

func TestSearchFallsBackToKeywordWhenEmbeddingUnavailable(t *testing.T) {
	origWD, err := os.Getwd()
	if err != nil {
		t.Fatalf("failed to get wd: %v", err)
	}
	tempWD := t.TempDir()
	if err := os.Chdir(tempWD); err != nil {
		t.Fatalf("failed to chdir temp dir: %v", err)
	}
	defer func() { _ = os.Chdir(origWD) }()

	store := NewStore(&failingEmbedder{})

	// Save should still persist memory even if embedding generation fails.
	if err := store.Save("test", "u1", "Loves carbonara pasta", "food"); err != nil {
		t.Fatalf("save failed: %v", err)
	}
	if err := store.Save("test", "u1", "Uses Go for backend services", "dev"); err != nil {
		t.Fatalf("save failed: %v", err)
	}

	got, err := store.Search("test", "u1", "carbonara", 5)
	if err != nil {
		t.Fatalf("search failed: %v", err)
	}
	if len(got) == 0 {
		t.Fatalf("expected keyword fallback results when embedding is unavailable")
	}
	if got[0].Content != "Loves carbonara pasta" {
		t.Fatalf("expected carbonara memory first, got %#v", got[0])
	}
}
