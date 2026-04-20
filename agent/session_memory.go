package agent

import (
	"errors"
	"fmt"
	"regexp"
	"strconv"
	"strings"

	"custom-agent/memory"
	"custom-agent/session"
)

const (
	defaultSessionRememberCount = 10
	maxSessionRememberCount     = 20 // aligns with session.Recent window
	sessionMemoryTag            = "from-session"
)

var (
	rememberSlashRe = regexp.MustCompile(`^/remember(?:\s+(\d+))?\s*$`)

	// sessionRememberNL matches requests to save the conversation thread (not a single fact).
	// Checked before extractRememberContent so phrases like "remember our last chat" promote the session.
	sessionRememberNL = regexp.MustCompile(`(?i)(` +
		`(save|store|preserve|record|keep|memorize)\s+.{0,80}?(conversation|chat|discussion|thread|session|talk)` +
		`|(conversation|chat|discussion|thread|session)\s+.{0,40}?(save|store|preserve|record|keep|memory)` +
		`|(remember|memorize)\s+.{0,40}?(our|the|this|that|last|previous)\s+.{0,40}?(chat|conversation|discussion|talk)` +
		`|(remember|memorize)\s+.{0,40}?(what|that)\s+we\s+(said|talked|discussed)` +
		`|(save|store|preserve|record|keep)\s+.{0,80}?(what|that)\s+we\s+(said|talked|discussed)` +
		`|(commit|add|put)\s+.{0,40}?(conversation|chat|discussion|this)\s+.{0,40}?(to memory|into memory)` +
		`)`)

	errNothingToSave = errors.New("nothing to save")
)

// parseRememberSlash parses "/remember" or "/remember N". Returns ok false if not a remember command.
func parseRememberSlash(text string) (n int, ok bool) {
	text = strings.TrimSpace(text)
	m := rememberSlashRe.FindStringSubmatch(text)
	if m == nil {
		return 0, false
	}
	if m[1] == "" {
		return defaultSessionRememberCount, true
	}
	v, err := strconv.Atoi(m[1])
	if err != nil || v < 1 {
		return defaultSessionRememberCount, true
	}
	if v > maxSessionRememberCount {
		v = maxSessionRememberCount
	}
	return v, true
}

// detectSessionRememberNL returns true when natural language asks to save the chat/session.
func detectSessionRememberNL(text string) bool {
	text = strings.TrimSpace(text)
	if len(text) < 12 {
		return false
	}
	return sessionRememberNL.MatchString(text)
}

func formatRoleLabel(role string) string {
	switch strings.ToLower(strings.TrimSpace(role)) {
	case "user":
		return "User"
	case "assistant":
		return "Assistant"
	default:
		if role == "" {
			return "?"
		}
		return role
	}
}

// formatSessionMemoryContent turns recent turns into a compact markdown-ish blob for storage.
func formatSessionMemoryContent(msgs []session.Message) string {
	var b strings.Builder
	b.WriteString("Session snapshot:\n")
	for _, m := range msgs {
		line := strings.TrimSpace(m.Content)
		if line == "" {
			line = "(empty)"
		}
		fmt.Fprintf(&b, "- %s: %s\n", formatRoleLabel(m.Role), line)
	}
	return strings.TrimSpace(b.String())
}

// promoteSessionToMemory loads session history, takes the last n messages, formats, and saves (memory.Store redacts).
func promoteSessionToMemory(store *memory.Store, platform, userID string, n int) (savedCount int, err error) {
	if store == nil {
		return 0, fmt.Errorf("memory store unavailable")
	}
	if n < 1 {
		n = defaultSessionRememberCount
	}
	if n > maxSessionRememberCount {
		n = maxSessionRememberCount
	}

	history, err := session.Load(platform, userID)
	if err != nil {
		return 0, err
	}
	if len(history) == 0 {
		return 0, errNothingToSave
	}

	tail := history
	if len(tail) > n {
		tail = tail[len(tail)-n:]
	}
	content := formatSessionMemoryContent(tail)
	if strings.TrimSpace(content) == "" || content == "Session snapshot:" {
		return 0, errNothingToSave
	}
	if err := store.Save(platform, userID, content, sessionMemoryTag); err != nil {
		return 0, err
	}
	return len(tail), nil
}

// trySessionRemember handles /remember, /remember N, and NL session-save phrases.
// Returns ok=true when this path handled the message (caller should return reply).
func (a *Agent) trySessionRemember(text, platform, userID string) (reply string, ok bool) {
	n, slash := parseRememberSlash(text)
	switch {
	case slash:
		// n already clamped
	case detectSessionRememberNL(text):
		n = defaultSessionRememberCount
	default:
		return "", false
	}

	if a.memoryStore == nil {
		return "I can't save that to long-term memory right now (memory isn't configured).", true
	}

	saved, err := promoteSessionToMemory(a.memoryStore, platform, userID, n)
	if errors.Is(err, errNothingToSave) {
		return "There's nothing from this chat to save yet—have a short back-and-forth first, then try again.", true
	}
	if err != nil {
		return "I couldn't save that to memory: " + err.Error(), true
	}
	return fmt.Sprintf("Saved %d message(s) from this session to memory.", saved), true
}
