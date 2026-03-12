package x402client

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestNew_EmptyKey(t *testing.T) {
	c, err := New("")
	if err != nil {
		t.Fatalf("New(\"\"): unexpected error: %v", err)
	}
	if c != nil {
		t.Error("New(\"\"): expected nil client")
	}
}

func TestNew_InvalidKey(t *testing.T) {
	_, err := New("not-hex")
	if err == nil {
		t.Error("New(\"not-hex\"): expected error")
	}
}

func TestNew_ValidKey(t *testing.T) {
	// Use a valid hex private key (32 bytes) - test key only
	key := "0xac0974bec39a17e36ba4a6b4d238ff944bacb478cbed5efcae784d7bf4f2ff80"
	c, err := New(key)
	if err != nil {
		t.Fatalf("New(valid): %v", err)
	}
	if c == nil || c.Client == nil {
		t.Fatal("expected non-nil client")
	}
}

func TestClient_Do_FreeAPI(t *testing.T) {
	key := "0xac0974bec39a17e36ba4a6b4d238ff944bacb478cbed5efcae784d7bf4f2ff80"
	c, err := New(key)
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"ok":true}`))
	}))
	defer server.Close()

	req, _ := http.NewRequest("GET", server.URL, nil)
	resp, err := c.Do(req)
	if err != nil {
		t.Fatalf("Do: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("status: got %d, want 200", resp.StatusCode)
	}
}
