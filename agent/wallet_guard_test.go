package agent

import "testing"

func TestClaimsWalletWasSent(t *testing.T) {
	tests := []struct {
		text string
		want bool
	}{
		{"Done! I've sent 0.001 ETH to that address.", true},
		{"Transaction Hash: 0x1234567890abcdef1234567890abcdef1234567890abcdef1234567890abcdef", true},
		{"Explorer: https://monadscan.com/tx/0x1234567890abcdef1234567890abcdef1234567890abcdef1234567890abcdef", true},
		{"I need the destination address before I can send.", false},
		{"What chain should I use?", false},
	}

	for _, tt := range tests {
		if got := claimsWalletWasSent(tt.text); got != tt.want {
			t.Fatalf("claimsWalletWasSent(%q) = %v, want %v", tt.text, got, tt.want)
		}
	}
}

func TestWantsWalletSend(t *testing.T) {
	tests := []struct {
		text string
		want bool
	}{
		{"send 0.001 to 0xC837F5B7E9997a80C829913e51B3609f763ab828 on monad chain", true},
		{"transfer 1 wei to 0xC837F5B7E9997a80C829913e51B3609f763ab828", true},
		{"what is my balance", false},
	}

	for _, tt := range tests {
		if got := wantsWalletSend(tt.text); got != tt.want {
			t.Fatalf("wantsWalletSend(%q) = %v, want %v", tt.text, got, tt.want)
		}
	}
}
