package tools

import (
	"encoding/json"
	"fmt"
	"os"
	"regexp"
	"strings"

	"custom-agent/wallet/redact"
)

const approvalsFile = "exec-approvals.json"

// Blocked patterns (substring match, case-insensitive)
var blockedPatterns = []string{
	"rm -rf /", "rm -rf /*", "rm -rf/",
	"mkfs", "dd if=", "> /dev/sd", "> /dev/nvme",
	"chmod -R 777", "chmod 777",
	":(){ :|:& };:", // fork bomb
	"sudo ",
	// Secret exfiltration
	"printenv", "env | grep",
	"echo $", "echo \"$", "echo '$",
}

func init() {
	blockedPatterns = append(blockedPatterns, redact.BlockedPatternsForPrompts...)
}

// Safe commands that run without approval (exact match of first word).
// echo is removed: it can exfiltrate env vars (e.g. echo $PRIVATE_KEY).
var safeCommands = []string{
	"ls", "pwd", "whoami", "date", "id",
	"head", "tail", "wc", "file",
}

// LoadApprovals reads approved commands from exec-approvals.json.
func LoadApprovals() ([]string, error) {
	data, err := os.ReadFile(approvalsFile)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	var approved []string
	if err := json.Unmarshal(data, &approved); err != nil {
		return nil, err
	}
	return approved, nil
}

// SaveApprovals writes approved commands to exec-approvals.json.
func SaveApprovals(commands []string) error {
	data, err := json.MarshalIndent(commands, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(approvalsFile, data, 0600)
}

// IsBlocked returns true if the command matches a dangerous pattern.
func IsBlocked(cmd string) bool {
	cmd = strings.TrimSpace(strings.ToLower(cmd))
	for _, p := range blockedPatterns {
		if strings.Contains(cmd, strings.ToLower(p)) {
			return true
		}
	}
	// Block rm -rf with path traversal
	if matched, _ := regexp.MatchString(`rm\s+-rf\s+[/\*]`, cmd); matched {
		return true
	}
	// Block cat .env
	if matched, _ := regexp.MatchString(`cat\s+\.env`, cmd); matched {
		return true
	}
	// Block commands that might contain secrets
	if redact.ContainsSecret(cmd) {
		return true
	}
	return false
}

// IsSafe returns true if the command is in the safe allowlist.
func IsSafe(cmd string) bool {
	cmd = strings.TrimSpace(cmd)
	first := strings.Fields(cmd)
	if len(first) == 0 {
		return false
	}
	base := strings.ToLower(first[0])
	for _, s := range safeCommands {
		if base == s {
			return true
		}
	}
	return false
}

// IsApproved returns true if the command is in the approved list (exact match).
func IsApproved(cmd string, approved []string) bool {
	cmd = normalizeCommand(cmd)
	for _, a := range approved {
		if normalizeCommand(a) == cmd {
			return true
		}
	}
	return false
}

// ApproveCommand adds a command to the approvals file if not already present.
func ApproveCommand(cmd string) error {
	cmd = strings.TrimSpace(cmd)
	if cmd == "" {
		return fmt.Errorf("empty command")
	}
	if IsBlocked(cmd) {
		return fmt.Errorf("cannot approve blocked command")
	}
	approved, err := LoadApprovals()
	if err != nil {
		return err
	}
	if IsApproved(cmd, approved) {
		return nil // already approved
	}
	approved = append(approved, cmd)
	return SaveApprovals(approved)
}

func normalizeCommand(cmd string) string {
	return strings.TrimSpace(strings.Join(strings.Fields(cmd), " "))
}

// ParseApprovalMessage extracts a command from "approve: <cmd>" or "/approve <cmd>".
// Returns the command and true if the message is an approval, otherwise "", false.
func ParseApprovalMessage(text string) (string, bool) {
	text = strings.TrimSpace(text)
	if text == "" {
		return "", false
	}
	lower := strings.ToLower(text)
	if strings.HasPrefix(lower, "approve:") {
		return strings.TrimSpace(text[8:]), true
	}
	if strings.HasPrefix(lower, "/approve ") {
		return strings.TrimSpace(text[9:]), true
	}
	return "", false
}
