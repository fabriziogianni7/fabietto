package compaction

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"custom-agent/session"

	"github.com/sashabaranov/go-openai"
)

func longHistoryForCompaction() []session.Message {
	msg := strings.Repeat("x", 400) // ~100 tokens each with charsPerToken=4
	return []session.Message{
		{Role: "user", Content: msg + "1"},
		{Role: "assistant", Content: msg + "2"},
		{Role: "user", Content: msg + "3"},
		{Role: "assistant", Content: msg + "4"},
		{Role: "user", Content: msg + "5"},
		{Role: "assistant", Content: msg + "6"},
	}
}

func TestCompactIfNeeded_SummarizeFailureFallsBackToRecent(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "upstream error", http.StatusInternalServerError)
	}))
	defer server.Close()

	cfg := openai.DefaultConfig("test-key")
	cfg.BaseURL = server.URL + "/v1"
	client := openai.NewClientWithConfig(cfg)

	comp := NewCompactor(client, "fake", 300)
	history := longHistoryForCompaction()
	summary, recent, err := comp.CompactIfNeeded(context.Background(), history, "system")
	if err != nil {
		t.Fatalf("expected graceful fallback, got err: %v", err)
	}
	if summary != "" {
		t.Fatalf("expected empty summary on summarize failure, got %q", summary)
	}
	if len(recent) == 0 || len(recent) > len(history) {
		t.Fatalf("expected recent fallback slice, got %d", len(recent))
	}
}

func TestCompactIfNeeded_InvalidSummaryJSONFallsBackToRecent(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
			"id":"bad-json",
			"choices":[{"message":{"role":"assistant","content":"{not-json"}}]
		}`))
	}))
	defer server.Close()

	cfg := openai.DefaultConfig("test-key")
	cfg.BaseURL = server.URL + "/v1"
	client := openai.NewClientWithConfig(cfg)

	comp := NewCompactor(client, "fake", 300)
	history := longHistoryForCompaction()
	summary, recent, err := comp.CompactIfNeeded(context.Background(), history, "system")
	if err != nil {
		t.Fatalf("expected graceful fallback, got err: %v", err)
	}
	if summary != "" {
		t.Fatalf("expected empty summary when parser fails, got %q", summary)
	}
	if len(recent) == 0 {
		t.Fatalf("expected recent history fallback, got none")
	}
}
