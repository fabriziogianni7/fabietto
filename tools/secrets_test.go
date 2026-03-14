package tools

import (
	"context"
	"errors"
	"testing"
)

type mockSecretGetter struct {
	secrets map[string]string
	err    error
}

func (m *mockSecretGetter) Get(ctx context.Context, vaultID, path string) (string, error) {
	if m.err != nil {
		return "", m.err
	}
	if v, ok := m.secrets[path]; ok {
		return v, nil
	}
	return "", errors.New("secret not found")
}

func TestGetSecret_NotConfigured(t *testing.T) {
	tools := NewTools("", nil)
	out, err := tools.getSecret(map[string]string{"path": "agent/api_key"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if out != "Error: get_secret not configured. Set 1CLAW_SECRET_PATH_PREFIX with 1claw to enable." {
		t.Errorf("got %q", out)
	}
}

func TestGetSecret_PathOutsidePrefix(t *testing.T) {
	getter := &mockSecretGetter{secrets: map[string]string{"agent/api_key": "sk-123"}}
	tools := NewTools("", nil)
	tools.SetSecrets(getter, "vault-1", "agent/")

	out, err := tools.getSecret(map[string]string{"path": "other/api_key"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if out != "Error: path must start with agent/" {
		t.Errorf("got %q", out)
	}
}

func TestGetSecret_PathAllowed(t *testing.T) {
	getter := &mockSecretGetter{secrets: map[string]string{"agent/api_key": "sk-123"}}
	tools := NewTools("", nil)
	tools.SetSecrets(getter, "vault-1", "agent/")

	out, err := tools.getSecret(map[string]string{"path": "agent/api_key"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if out != "sk-123" {
		t.Errorf("got %q", out)
	}
}

func TestGetSecret_VaultError(t *testing.T) {
	getter := &mockSecretGetter{secrets: map[string]string{}, err: errors.New("access denied")}
	tools := NewTools("", nil)
	tools.SetSecrets(getter, "vault-1", "agent/")

	out, err := tools.getSecret(map[string]string{"path": "agent/missing"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if out != "Error: secret not found or access denied." {
		t.Errorf("got %q", out)
	}
}

func TestGetSecret_EmptyPath(t *testing.T) {
	getter := &mockSecretGetter{secrets: map[string]string{}}
	tools := NewTools("", nil)
	tools.SetSecrets(getter, "vault-1", "agent/")

	out, err := tools.getSecret(map[string]string{"path": ""})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if out != "Error: path is required." {
		t.Errorf("got %q", out)
	}
}

func TestToolDefinitions_SecretsEnabled(t *testing.T) {
	getter := &mockSecretGetter{secrets: map[string]string{}}
	tools := NewTools("", nil)
	tools.SetSecrets(getter, "vault-1", "agent/")

	defs := tools.ToolDefinitions()
	var found bool
	for _, d := range defs {
		if d.Function != nil && d.Function.Name == "get_secret" {
			found = true
			break
		}
	}
	if !found {
		t.Error("get_secret not in ToolDefinitions when secrets enabled")
	}
}

func TestToolDefinitions_SecretsDisabled(t *testing.T) {
	tools := NewTools("", nil)

	defs := tools.ToolDefinitions()
	for _, d := range defs {
		if d.Function != nil && d.Function.Name == "get_secret" {
			t.Error("get_secret should not be in ToolDefinitions when secrets disabled")
		}
	}
}

func TestExecuteTool_GetSecret(t *testing.T) {
	getter := &mockSecretGetter{secrets: map[string]string{"agent/token": "secret-value"}}
	tools := NewTools("", nil)
	tools.SetSecrets(getter, "vault-1", "agent/")

	out, err := tools.ExecuteTool("get_secret", `{"path":"agent/token"}`)
	if err != nil {
		t.Fatalf("ExecuteTool: %v", err)
	}
	if out != "secret-value" {
		t.Errorf("got %q", out)
	}
}
