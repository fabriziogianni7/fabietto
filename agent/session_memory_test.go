package agent

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"custom-agent/memory"
	"custom-agent/session"
)

func TestParseRememberSlash(t *testing.T) {
	tests := []struct {
		in     string
		wantN  int
		wantOK bool
	}{
		{"/remember", defaultSessionRememberCount, true},
		{"/remember 5", 5, true},
		{"/remember 99", maxSessionRememberCount, true},
		{"  /remember 3  ", 3, true},
		{"/remember 0", defaultSessionRememberCount, true},
		{"/recall", 0, false},
		{"remember our chat", 0, false},
	}
	for _, tc := range tests {
		n, ok := parseRememberSlash(tc.in)
		if ok != tc.wantOK || n != tc.wantN {
			t.Errorf("parseRememberSlash(%q) = (%d, %v), want (%d, %v)", tc.in, n, ok, tc.wantN, tc.wantOK)
		}
	}
}

func TestDetectSessionRememberNL(t *testing.T) {
	positive := []string{
		"remember our last chat",
		"Please save this conversation to memory",
		"can you store what we discussed",
		"commit this chat to memory",
		"put our discussion into memory",
	}
	negative := []string{
		"remember that I like blue",
		"memorize my name is Alex",
		"hello",
		"remember to buy milk",
	}
	for _, s := range positive {
		if !detectSessionRememberNL(s) {
			t.Errorf("expected NL match: %q", s)
		}
	}
	for _, s := range negative {
		if detectSessionRememberNL(s) {
			t.Errorf("expected no NL match: %q", s)
		}
	}
}

func TestFormatSessionMemoryContent(t *testing.T) {
	got := formatSessionMemoryContent([]session.Message{
		{Role: "user", Content: "  hi  "},
		{Role: "assistant", Content: ""},
		{Role: "system", Content: "note"},
	})
	for _, part := range []string{"Session snapshot:", "- User: hi", "- Assistant: (empty)", "- system: note"} {
		if !strings.Contains(got, part) {
			t.Fatalf("unexpected format (missing %q):\n%s", part, got)
		}
	}
}

func TestPromoteSessionToMemory(t *testing.T) {
	origWD, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	if err := os.Chdir(dir); err != nil {
		t.Fatal(err)
	}
	defer func() { _ = os.Chdir(origWD) }()

	sessionsDir := filepath.Join(dir, "sessions")
	if err := os.MkdirAll(sessionsDir, 0755); err != nil {
		t.Fatal(err)
	}
	key := session.SessionKey("test", "u1")
	path := filepath.Join(sessionsDir, key+".jsonl")
	data := `{"role":"user","content":"hello"}
{"role":"assistant","content":"hi there"}
`
	if err := os.WriteFile(path, []byte(data), 0600); err != nil {
		t.Fatal(err)
	}

	store := memory.NewStore(nil)
	n, err := promoteSessionToMemory(store, "test", "u1", 10)
	if err != nil {
		t.Fatalf("promoteSessionToMemory: %v", err)
	}
	if n != 2 {
		t.Fatalf("saved count: got %d want 2", n)
	}

	_, err = promoteSessionToMemory(store, "test", "nobody", 10)
	if !errors.Is(err, errNothingToSave) {
		t.Fatalf("empty session: got err=%v", err)
	}
}

func TestExtractRememberVsSessionNL(t *testing.T) {
	// Same phrase matches session NL and single-fact extract; HandleMessage runs trySessionRemember first.
	s := "remember our last chat"
	if !detectSessionRememberNL(s) {
		t.Fatal("expected session NL")
	}
	if extractRememberContent(s) == "" {
		t.Fatal("expected extractRememberContent to also produce a fact so ordering in HandleMessage matters")
	}
}
