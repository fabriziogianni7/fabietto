package session

import (
	"bufio"
	"encoding/json"
	"os"
	"path/filepath"
	"regexp"

	"custom-agent/wallet/redact"
)

const (
	sessionDir  = "sessions"
	maxMessages = 20 // keep last N messages to stay within context limits
)

var safeKeyRe = regexp.MustCompile(`[^a-zA-Z0-9_-]`)

// Message represents a single message in the conversation.
type Message struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

// SessionKey returns a filesystem-safe key for platform+userID.
func SessionKey(platform, userID string) string {
	return safeKeyRe.ReplaceAllString(platform+"_"+userID, "_")
}

// Load reads the conversation history for a user from sessions/{key}.jsonl.
// Use SessionKey(platform, userID) for the key.
// For telegram, falls back to legacy {userID}.jsonl if new file doesn't exist.
// Returns nil if the file doesn't exist or is empty.
func Load(platform, userID string) ([]Message, error) {
	key := SessionKey(platform, userID)
	if key == "" || key == "_" {
		return nil, nil
	}
	path := filepath.Join(sessionDir, key+".jsonl")
	f, err := os.Open(path)
	if err != nil && os.IsNotExist(err) && platform == "telegram" {
		// Legacy: try sessions/{userID}.jsonl for Telegram
		path = filepath.Join(sessionDir, userID+".jsonl")
		f, err = os.Open(path)
	}
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	defer f.Close()

	var messages []Message
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		var m Message
		if err := json.Unmarshal(scanner.Bytes(), &m); err != nil {
			continue // skip malformed lines
		}
		messages = append(messages, m)
	}
	if err := scanner.Err(); err != nil {
		return nil, err
	}

	return messages, nil
}

// Append adds a user message and assistant response to the session.
func Append(platform, userID string, userContent, assistantContent string) error {
	if err := ensureSessionDir(); err != nil {
		return err
	}

	key := SessionKey(platform, userID)
	if key == "" || key == "_" {
		return nil
	}
	path := filepath.Join(sessionDir, key+".jsonl")
	// Migrate legacy telegram sessions: copy {userID}.jsonl -> telegram_{userID}.jsonl
	if platform == "telegram" {
		if _, err := os.Stat(path); os.IsNotExist(err) {
			legacy := filepath.Join(sessionDir, userID+".jsonl")
			if data, err := os.ReadFile(legacy); err == nil {
				_ = os.WriteFile(path, data, 0600)
				_ = os.Remove(legacy)
			}
		}
	}
	f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0600)
	if err != nil {
		return err
	}
	defer f.Close()

	encoder := json.NewEncoder(f)
	if err := encoder.Encode(Message{Role: "user", Content: redact.Redact(userContent)}); err != nil {
		return err
	}
	if err := encoder.Encode(Message{Role: "assistant", Content: redact.Redact(assistantContent)}); err != nil {
		return err
	}

	return nil
}

// Clear removes the session file for a user, starting a fresh conversation on next message.
// Returns nil if the file didn't exist (already fresh).
func Clear(platform, userID string) error {
	key := SessionKey(platform, userID)
	if key == "" || key == "_" {
		return nil
	}
	path := filepath.Join(sessionDir, key+".jsonl")
	err := os.Remove(path)
	if err != nil && os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return err
	}
	if platform == "telegram" {
		legacy := filepath.Join(sessionDir, userID+".jsonl")
		_ = os.Remove(legacy)
	}
	return nil
}

// Recent returns the last maxMessages from the history for LLM context.
func Recent(messages []Message) []Message {
	if len(messages) <= maxMessages {
		return messages
	}
	return messages[len(messages)-maxMessages:]
}

func ensureSessionDir() error {
	return os.MkdirAll(sessionDir, 0755)
}
