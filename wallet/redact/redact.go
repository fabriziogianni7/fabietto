package redact

import (
	"regexp"
	"strings"
)

// Patterns that indicate secrets. Substring match, case-insensitive.
var secretPatterns = []string{
	"private_key", "privatekey", "privkey", "secret_key", "secretkey",
	"mnemonic", "seed_phrase", "seedphrase", "recovery_phrase",
	"0x[0-9a-fA-F]{64}", // 32-byte hex (private key length)
}

// BlockedPatternsForPrompts prevents prompt-injection attempts to exfiltrate secrets.
// Used in blockedPatterns for run_command and similar.
var BlockedPatternsForPrompts = []string{
	"PRIVATE_KEY", "private_key", "PRIVKEY", "privkey",
	"MNEMONIC", "mnemonic", "SEED_PHRASE", "seed_phrase",
	"SECRET_KEY", "secret_key", "echo $", "echo \"$",
	"printenv", "env | grep", "cat .env", "cat .env.",
}

var (
	hex64Re = regexp.MustCompile(`0x[0-9a-fA-F]{64}`)
	// Public contexts where 64-char hex is a tx hash, not a private key
	txHashContextRe = regexp.MustCompile(`(?i)(Hash:\s*|/tx/|Explorer:.*|transaction hash[^0-9a-fA-F]*|tx hash[^0-9a-fA-F]*)0x([0-9a-fA-F]{64})`)
	txHashPlaceholder = "«TXHASH»"
)

// Redact replaces secret-like substrings with a placeholder.
// Transaction hashes (64-char hex in "Hash:", "/tx/", "Explorer:" context) are preserved.
func Redact(s string) string {
	if s == "" {
		return s
	}
	out := s
	// Protect tx hashes: temporarily replace so they won't be redacted
	out = txHashContextRe.ReplaceAllString(out, "${1}"+txHashPlaceholder+"${2}")
	// Redact 64-char hex (private key length) - remaining ones are likely secrets
	out = hex64Re.ReplaceAllString(out, "[REDACTED]")
	// Restore protected tx hashes
	out = regexp.MustCompile(regexp.QuoteMeta(txHashPlaceholder)+`([0-9a-fA-F]{64})`).ReplaceAllString(out, "0x${1}")
	// Redact common secret variable names followed by = value
	for _, p := range secretPatterns {
		pat := regexp.MustCompile(`(?i)`+regexp.QuoteMeta(p)+`\s*[=:]\s*[^\s]+`)
		out = pat.ReplaceAllString(out, p+"=[REDACTED]")
	}
	return out
}

// ContainsSecret returns true if the string likely contains a secret.
func ContainsSecret(s string) bool {
	lower := strings.ToLower(s)
	for _, p := range secretPatterns {
		if strings.Contains(lower, strings.ToLower(p)) {
			return true
		}
	}
	return hex64Re.MatchString(s)
}
