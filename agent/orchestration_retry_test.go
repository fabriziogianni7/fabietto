package agent

import "testing"

func TestOrchestrationToolOutputSuggestsRetry(t *testing.T) {
	tests := []struct {
		out  string
		want bool
	}{
		{"", false},
		{"Balance: 0 wei", false},
		{"Status: 200 200 OK\nBody:\n{}", false},
		{"Error: invalid arguments: foo", true},
		{"Error: search failed: HTTP 429", true},
		{"search failed: HTTP 429", true},
		{"Status: 404 404 Not Found\nBody:\n{}", true},
		{"Status: 502 502 Bad Gateway\n", true},
		{"invalid arguments", true},
	}
	for _, tt := range tests {
		if got := orchestrationToolOutputSuggestsRetry(tt.out); got != tt.want {
			t.Errorf("orchestrationToolOutputSuggestsRetry(%q) = %v, want %v", tt.out, got, tt.want)
		}
	}
}
