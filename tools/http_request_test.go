package tools

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/sashabaranov/go-openai/jsonschema"
)

func TestHttpRequest_EmptyURL(t *testing.T) {
	tools := NewTools("", nil)
	out, err := tools.httpRequest(map[string]string{"url": ""}, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if out != "Error: url is required." {
		t.Errorf("got %q", out)
	}
}

func TestHttpRequest_InvalidURL(t *testing.T) {
	tools := NewTools("", nil)
	out, err := tools.httpRequest(map[string]string{"url": "://bad"}, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if out == "" || out == "Error: url is required." {
		t.Errorf("expected invalid url error, got %q", out)
	}
}

func TestHttpRequest_WithoutX402_FreeAPI(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"data":"hello"}`))
	}))
	defer server.Close()

	tools := NewTools("", nil)
	out, err := tools.httpRequest(map[string]string{"url": server.URL}, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if out == "" {
		t.Error("expected non-empty output")
	}
	if out != "" && out[:7] != "Status:" {
		t.Errorf("expected Status: prefix, got %q", out[:min(50, len(out))])
	}
}

func TestHttpRequest_HeaderRedaction(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Api-Key", "secret-key-123")
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{}`))
	}))
	defer server.Close()

	tools := NewTools("", nil)
	out, err := tools.httpRequest(map[string]string{"url": server.URL}, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if out != "" && len(out) >= 7 && out[:7] != "Status:" {
		t.Errorf("expected Status: prefix, got %q", out[:min(50, len(out))])
	}
	// X-Api-Key should be redacted (we redact x-api-key)
	if strings.Contains(out, "secret-key-123") {
		t.Errorf("sensitive header should be redacted in output")
	}
	if !strings.Contains(out, "[redacted]") {
		t.Errorf("expected [redacted] for sensitive header")
	}
}

func TestHttpRequest_402WithoutX402_Note(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusPaymentRequired)
		w.Write([]byte(`payment required`))
	}))
	defer server.Close()

	tools := NewTools("", nil)
	out, err := tools.httpRequest(map[string]string{"url": server.URL}, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(out, "402") {
		t.Errorf("expected 402 in output: %s", out)
	}
	if !strings.Contains(out, "x402") && !strings.Contains(out, "Configure wallet") {
		t.Errorf("expected x402 config note when 402 without client: %s", out)
	}
}

func TestHttpRequest_WithHeaders(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("X-Custom") != "value" {
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"ok":true}`))
	}))
	defer server.Close()

	headers := map[string]interface{}{"X-Custom": "value"}
	rawArgs := map[string]interface{}{"headers": headers}

	tools := NewTools("", nil)
	out, err := tools.httpRequest(map[string]string{"url": server.URL}, rawArgs)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(out, "200") {
		t.Errorf("expected 200 OK with custom header: %s", out)
	}
}

func TestHttpRequest_ResponseTruncation(t *testing.T) {
	largeBody := make([]byte, httpRequestMaxBody+1000)
	for i := range largeBody {
		largeBody[i] = 'x'
	}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write(largeBody)
	}))
	defer server.Close()

	tools := NewTools("", nil)
	out, err := tools.httpRequest(map[string]string{"url": server.URL}, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(out, "[truncated]") {
		t.Errorf("expected [truncated] for large response: %d chars", len(out))
	}
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func TestHttpRequest_Definitions(t *testing.T) {
	defs := Definitions()
	var found bool
	for _, d := range defs {
		if d.Function != nil && d.Function.Name == "http_request" {
			found = true
			if p, ok := d.Function.Parameters.(jsonschema.Definition); ok && p.Required != nil {
				reqJSON, _ := json.Marshal(p.Required)
				if !strings.Contains(string(reqJSON), "url") {
					t.Error("url should be required")
				}
			}
			break
		}
	}
	if !found {
		t.Error("http_request not found in Definitions()")
	}
}
