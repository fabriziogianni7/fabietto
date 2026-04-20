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
	parsed, err := parseCommand(cmd)
	if err != nil {
		return false
	}
	return autoAllowedCommands[parsed.Executable]
}

// IsApproved returns true if the command is in the approved list (exact match).
func IsApproved(cmd string, approved []string) bool {
	parsed, err := parseCommand(cmd)
	if err != nil {
		return false
	}
	cmd = parsed.Identity
	for _, a := range approved {
		parsedApproved, err := parseCommand(a)
		if err != nil {
			continue
		}
		if parsedApproved.Identity == cmd {
			return true
		}
	}
	return false
}

// ApproveCommand adds a command to the approvals file if not already present.
func ApproveCommand(cmd string) error {
	parsed, err := parseCommand(cmd)
	if err != nil {
		return err
	}
	if !isAllowedExecutable(parsed.Executable) || !needsApproval(parsed.Executable) {
		return fmt.Errorf("cannot approve command outside approval allowlist")
	}
	if IsBlocked(parsed.Identity) {
		return fmt.Errorf("cannot approve blocked command")
	}
	approved, err := LoadApprovals()
	if err != nil {
		return err
	}
	if IsApproved(parsed.Identity, approved) {
		return nil // already approved
	}
	approved = append(approved, parsed.Identity)
	return SaveApprovals(approved)
}

func normalizeCommand(cmd string) string {
	parsed, err := parseCommand(cmd)
	if err != nil {
		return strings.TrimSpace(strings.Join(strings.Fields(cmd), " "))
	}
	return parsed.Identity
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
