package redact

import (
	"strings"
	"testing"
)

func TestRedact_PreservesTxHash(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want string
	}{
		{
			"Hash: prefix",
			"Transaction sent.\nHash: 0x1234567890abcdef1234567890abcdef1234567890abcdef1234567890abcdef\nExplorer: https://etherscan.io/tx/0x1234567890abcdef1234567890abcdef1234567890abcdef1234567890abcdef",
			"Transaction sent.\nHash: 0x1234567890abcdef1234567890abcdef1234567890abcdef1234567890abcdef\nExplorer: https://etherscan.io/tx/0x1234567890abcdef1234567890abcdef1234567890abcdef1234567890abcdef",
		},
		{
			"Explorer URL",
			"Explorer: https://monadscan.com/tx/0xabcdef1234567890abcdef1234567890abcdef1234567890abcdef1234567890",
			"Explorer: https://monadscan.com/tx/0xabcdef1234567890abcdef1234567890abcdef1234567890abcdef1234567890",
		},
		{
			"transaction hash is",
			"The transaction hash is: 0x1234567890abcdef1234567890abcdef1234567890abcdef1234567890abcdef",
			"The transaction hash is: 0x1234567890abcdef1234567890abcdef1234567890abcdef1234567890abcdef",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := Redact(tt.in)
			if got != tt.want {
				t.Errorf("Redact() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestRedact_RedactsPrivateKey(t *testing.T) {
	in := "private_key=0x1234567890abcdef1234567890abcdef1234567890abcdef1234567890abcdef"
	got := Redact(in)
	if got == in {
		t.Errorf("Redact() should redact private key, got %q", got)
	}
	if strings.Contains(got, "0x1234567890abcdef1234567890abcdef1234567890abcdef1234567890abcdef") {
		t.Errorf("Redact() should not contain raw hex, got %q", got)
	}
}

func TestRedact_RedactsBare64Hex(t *testing.T) {
	in := "some secret value 0x1234567890abcdef1234567890abcdef1234567890abcdef1234567890abcdef"
	got := Redact(in)
	if strings.Contains(got, "0x1234567890abcdef1234567890abcdef1234567890abcdef1234567890abcdef") {
		t.Errorf("Redact() should redact bare 64-char hex, got %q", got)
	}
}
