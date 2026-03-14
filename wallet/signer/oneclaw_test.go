package signer

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	sdk "github.com/1clawAI/1claw-go-sdk"
)

type mockOneClawGetter struct {
	secret string
	err   error
}

func (m *mockOneClawGetter) Get(ctx context.Context, vaultID, path string) (string, error) {
	if m.err != nil {
		return "", m.err
	}
	return m.secret, nil
}

func TestNewOneClawSignerFromGetter_ProducesValidAddress(t *testing.T) {
	// Use a valid secp256k1 private key (32 bytes hex)
	keyHex := "ac0974bec39a17e36ba4a6b4d238ff944bacb478cbed5efcae784d7bf4f2ff80"
	getter := &mockOneClawGetter{secret: keyHex}
	ctx := context.Background()

	sgn, err := NewOneClawSignerFromGetter(ctx, getter, "vault-1", "WALLET_PRIVATE_KEY")
	if err != nil {
		t.Fatalf("NewOneClawSignerFromGetter: %v", err)
	}
	addr := sgn.Address()
	if addr.Hex() == "" {
		t.Error("expected non-empty address")
	}
	if sgn.PublicIdentifier() == "" {
		t.Error("expected non-empty PublicIdentifier")
	}
}

func TestNewOneClawSignerFromGetter_With0xPrefix(t *testing.T) {
	keyHex := "0xac0974bec39a17e36ba4a6b4d238ff944bacb478cbed5efcae784d7bf4f2ff80"
	getter := &mockOneClawGetter{secret: keyHex}
	ctx := context.Background()

	sgn, err := NewOneClawSignerFromGetter(ctx, getter, "vault-1", "WALLET_PRIVATE_KEY")
	if err != nil {
		t.Fatalf("NewOneClawSignerFromGetter: %v", err)
	}
	if sgn.Address().Hex() == "" {
		t.Error("expected valid address from 0x-prefixed key")
	}
}

func TestNewOneClawSignerFromGetter_EmptySecret(t *testing.T) {
	getter := &mockOneClawGetter{secret: ""}
	ctx := context.Background()

	_, err := NewOneClawSignerFromGetter(ctx, getter, "vault-1", "WALLET_PRIVATE_KEY")
	if err == nil {
		t.Error("expected error for empty secret")
	}
}

func TestNewOneClawSignerFromGetter_InvalidKey(t *testing.T) {
	getter := &mockOneClawGetter{secret: "not-a-valid-hex-key"}
	ctx := context.Background()

	_, err := NewOneClawSignerFromGetter(ctx, getter, "vault-1", "WALLET_PRIVATE_KEY")
	if err == nil {
		t.Error("expected error for invalid key")
	}
}

func TestNewFromBackend_OneClaw_RequiresClient(t *testing.T) {
	_, err := NewFromBackend(BackendOneClaw, map[string]string{
		"vault_id": "vault-1",
		"key_path": "WALLET_PRIVATE_KEY",
	})
	if err == nil {
		t.Error("expected error when WithOneClawClient not provided")
	}
}

func TestNewFromBackend_OneClaw_ReturnsSignerWhenClientProvided(t *testing.T) {
	keyHex := "ac0974bec39a17e36ba4a6b4d238ff944bacb478cbed5efcae784d7bf4f2ff80"
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(map[string]interface{}{
			"id": "s1", "path": "WALLET_PRIVATE_KEY", "value": keyHex, "type": "generic",
			"version": 1, "created_at": "2024-01-01T00:00:00Z",
		})
	}))
	defer server.Close()

	client, err := sdk.New(sdk.WithBaseURL(server.URL), sdk.WithToken("eyJ.test"))
	if err != nil {
		t.Fatal(err)
	}

	sgn, err := NewFromBackend(BackendOneClaw, map[string]string{
		"vault_id": "vault-1",
		"key_path": "WALLET_PRIVATE_KEY",
	}, WithOneClawClient(client))
	if err != nil {
		t.Fatalf("NewFromBackend: %v", err)
	}
	if sgn == nil {
		t.Fatal("expected non-nil signer")
	}
	if sgn.Address().Hex() == "" {
		t.Error("expected valid address")
	}
}
