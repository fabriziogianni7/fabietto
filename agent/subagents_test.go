package agent

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"custom-agent/conversation"
	"custom-agent/gateway"
	"custom-agent/memory"
	"custom-agent/tools"

	"github.com/sashabaranov/go-openai"
)

// mockCompletionResponse returns a minimal OpenAI-compatible completion (text only, no tools).
func mockCompletionResponse(content string) string {
	return `{"id":"mock","choices":[{"message":{"role":"assistant","content":"` + strings.ReplaceAll(content, `"`, `\"`) + `"}}],"usage":{"prompt_tokens":1,"completion_tokens":1}}`
}

func TestFormatSubagentResults(t *testing.T) {
	results := []SubtaskResult{
		{Task: "a", Output: "out1", Err: nil, Index: 0, DurationMs: 100},
		{Task: "b", Output: "", Err: context.Canceled, Index: 1, DurationMs: 50},
		{Task: "c", Output: "out3", Err: nil, Index: 2, DurationMs: 200},
	}
	got := FormatSubagentResults(results)
	if !strings.Contains(got, "out1") || !strings.Contains(got, "error: context canceled") || !strings.Contains(got, "out3") {
		t.Errorf("FormatSubagentResults missing expected content: %s", got)
	}
	if !strings.HasPrefix(got, "--- Sub-agent results ---") || !strings.HasSuffix(got, "--- End sub-agent results ---") {
		t.Errorf("FormatSubagentResults wrong format: %s", got)
	}
}

func TestRunSubagents_EmptySpecs(t *testing.T) {
	a := &Agent{}
	results, err := (&Agent{}).RunSubagents(context.Background(), nil, gateway.IncomingMessage{}, nil)
	if err != nil {
		t.Fatalf("RunSubagents(nil): %v", err)
	}
	if results != nil {
		t.Errorf("expected nil results, got %v", results)
	}
	_ = a
}

func TestRunSubagents_OrderPreserved(t *testing.T) {
	// Mock HTTP server that returns fixed completion
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		// Return content that includes a marker so we can identify which request
		body := make([]byte, 1024)
		n, _ := r.Body.Read(body)
		_ = n
		// Simple response - we'll use request order: first request gets "r1", etc.
		// We can't easily distinguish requests, so we return a generic response.
		w.Write([]byte(mockCompletionResponse("sub-result")))
	}))
	defer server.Close()

	cfg := openai.DefaultConfig("test-key")
	cfg.BaseURL = server.URL + "/v1"
	client := openai.NewClientWithConfig(cfg)

	toolSet := tools.NewTools("", memory.NewStore(nil))
	var convStore *conversation.Store
	a := New(client, "You are helpful.", 0, toolSet, convStore)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	specs := []SubtaskSpec{
		{Task: "task A", Index: 0},
		{Task: "task B", Index: 1},
		{Task: "task C", Index: 2},
	}
	msg := gateway.IncomingMessage{Platform: "test", UserID: "u1"}

	results, err := a.RunSubagents(ctx, specs, msg, &SubagentOpts{
		MaxConcurrency:  2,
		PerChildTimeout: 5 * time.Second,
		MaxChildCount:   10,
	})
	if err != nil {
		t.Fatalf("RunSubagents: %v", err)
	}
	if len(results) != 3 {
		t.Fatalf("expected 3 results, got %d", len(results))
	}
	// Results should be in same order as specs
	for i := range results {
		if results[i].Index != specs[i].Index {
			t.Errorf("result %d: index %d != spec index %d", i, results[i].Index, specs[i].Index)
		}
	}
}
