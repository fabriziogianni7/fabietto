package config

import (
	"fmt"
	"os"
	"strconv"
	"strings"

	"github.com/joho/godotenv"
)

func parseInt(s string, defaultVal int) int {
	s = strings.TrimSpace(s)
	if s == "" {
		return defaultVal
	}
	n, err := strconv.Atoi(s)
	if err != nil {
		return defaultVal
	}
	return n
}

// Config holds validated application configuration.
type Config struct {
	TelegramBotToken   string
	GroqAPIKey        string
	BraveSearchAPIKey string
	DiscordToken      string // optional
	HTTPPort          string // optional, e.g. "5000"
	SignalCliURL      string // optional, signal-cli-rest-api URL
	SignalNumber      string // optional, bot's Signal number
	CompactionThreshold int   // optional, token count to trigger compaction (default 4000)
}

// Load reads environment variables from .env (if present) and validates required values.
// Returns an error if any required variable is missing or invalid.
func Load() (*Config, error) {
	_ = godotenv.Load() // ignore error if .env doesn't exist

	cfg := &Config{
		TelegramBotToken:   strings.TrimSpace(os.Getenv("TELEGRAM_BOT_TOKEN")),
		GroqAPIKey:         strings.TrimSpace(os.Getenv("GROQ_API_KEY")),
		BraveSearchAPIKey:  strings.TrimSpace(os.Getenv("BRAVE_SEARCH_API_KEY")),
		DiscordToken:       strings.TrimSpace(os.Getenv("DISCORD_BOT_TOKEN")),
		HTTPPort:           strings.TrimSpace(os.Getenv("HTTP_PORT")),
		SignalCliURL:       strings.TrimSpace(os.Getenv("SIGNAL_CLI_URL")),
		SignalNumber:       strings.TrimSpace(os.Getenv("SIGNAL_NUMBER")),
		CompactionThreshold: parseInt(os.Getenv("CONTEXT_COMPACTION_THRESHOLD"), 4000),
	}

	if err := cfg.validate(); err != nil {
		return nil, err
	}

	return cfg, nil
}

func (c *Config) validate() error {
	var missing []string

	if c.GroqAPIKey == "" {
		missing = append(missing, "GROQ_API_KEY")
	}
	if c.BraveSearchAPIKey == "" {
		missing = append(missing, "BRAVE_SEARCH_API_KEY")
	}

	// At least one gateway must be enabled
	if c.TelegramBotToken == "" && c.DiscordToken == "" && c.HTTPPort == "" &&
		!(c.SignalCliURL != "" && c.SignalNumber != "") {
		missing = append(missing, "TELEGRAM_BOT_TOKEN, DISCORD_BOT_TOKEN, HTTP_PORT, or SIGNAL_CLI_URL+SIGNAL_NUMBER (at least one)")
	}

	// Signal requires both URL and number
	if (c.SignalCliURL != "" && c.SignalNumber == "") || (c.SignalCliURL == "" && c.SignalNumber != "") {
		missing = append(missing, "SIGNAL_CLI_URL and SIGNAL_NUMBER must be set together")
	}

	if len(missing) > 0 {
		return fmt.Errorf("missing required environment variables: %s (set them in .env or export)", strings.Join(missing, ", "))
	}

	return nil
}
