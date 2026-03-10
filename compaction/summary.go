package compaction

import (
	"encoding/json"
	"strings"
)

// CompactedContext is the structured format for compacted conversation context.
// Use 4-6 core sections; add domain-specific sections as needed.
type CompactedContext struct {
	// SessionIntent: what the user is trying to accomplish (1-2 sentences)
	SessionIntent string `json:"session_intent,omitempty"`

	// KeyDecisions: important choices made during the conversation
	KeyDecisions []string `json:"key_decisions,omitempty"`

	// KeyFacts: facts the user shared (name, preferences, constraints)
	KeyFacts map[string]string `json:"key_facts,omitempty"`

	// FileModifications: for coding agents—files changed and what was done
	FileModifications map[string]string `json:"file_modifications,omitempty"`

	// PendingActions: things still to do or follow up on
	PendingActions []string `json:"pending_actions,omitempty"`

	// Artifacts: important outputs (errors, tool results, excerpts)
	Artifacts map[string]string `json:"artifacts,omitempty"`

	// Momentum: for iterative tasks—current direction or learning trajectory
	Momentum string `json:"momentum,omitempty"`

	// ToolResultsSummary: condensed summary of tool outputs (run_command, read_file, etc.)
	ToolResultsSummary map[string]string `json:"tool_results_summary,omitempty"`
}

// ToJSON returns the compacted context as indented JSON.
func (c *CompactedContext) ToJSON() (string, error) {
	b, err := json.MarshalIndent(c, "", "  ")
	if err != nil {
		return "", err
	}
	return string(b), nil
}

// ToPromptBlock returns the compacted context formatted for injection into an LLM prompt.
// Uses a clear delimiter so the model knows this is prior context.
func (c *CompactedContext) ToPromptBlock() (string, error) {
	jsonStr, err := c.ToJSON()
	if err != nil {
		return "", err
	}
	return "--- Prior context (compacted) ---\n" + jsonStr + "\n--- End prior context ---", nil
}

// Merge merges updates from another CompactedContext into this one.
// For iterative updates: only new content is merged, minimizing drift.
func (c *CompactedContext) Merge(other *CompactedContext) {
	if other.SessionIntent != "" {
		c.SessionIntent = other.SessionIntent
	}
	c.KeyDecisions = appendUnique(c.KeyDecisions, other.KeyDecisions...)
	c.KeyFacts = mergeMaps(c.KeyFacts, other.KeyFacts)
	c.FileModifications = mergeMaps(c.FileModifications, other.FileModifications)
	c.PendingActions = appendUnique(c.PendingActions, other.PendingActions...)
	c.Artifacts = mergeMaps(c.Artifacts, other.Artifacts)
	if other.Momentum != "" {
		c.Momentum = other.Momentum
	}
	c.ToolResultsSummary = mergeMaps(c.ToolResultsSummary, other.ToolResultsSummary)
}

func appendUnique(slice []string, items ...string) []string {
	seen := make(map[string]bool)
	for _, s := range slice {
		seen[s] = true
	}
	for _, s := range items {
		if s != "" && !seen[s] {
			seen[s] = true
			slice = append(slice, s)
		}
	}
	return slice
}

func mergeMaps(a, b map[string]string) map[string]string {
	if a == nil {
		a = make(map[string]string)
	}
	for k, v := range b {
		if v != "" {
			a[k] = v
		}
	}
	return a
}

// SummarizationPrompt returns the system prompt for the LLM to produce a CompactedContext from messages.
func SummarizationPrompt() string {
	return `You are a context compaction assistant. Given a conversation, produce a structured JSON summary.

Output ONLY valid JSON matching this schema (omit empty fields):
{
  "session_intent": "1-2 sentences: what the user is trying to accomplish",
  "key_decisions": ["decision 1", "decision 2"],
  "key_facts": {"fact_name": "value"},
  "file_modifications": {"filename": "brief description of changes"},
  "pending_actions": ["action 1", "action 2"],
  "artifacts": {"error_logs": "excerpt", "other": "relevant excerpt"},
  "momentum": "current direction or next step (for iterative tasks)",
  "tool_results_summary": {"tool_name": "condensed result"}
}

Rules:
- Preserve relationships (e.g. error → cause → fix)
- Be concise but retain critical details
- No commentary, only the JSON object`
}

// ParseCompactedContext parses JSON into a CompactedContext.
func ParseCompactedContext(jsonStr string) (*CompactedContext, error) {
	var c CompactedContext
	// Trim markdown code blocks if present
	s := strings.TrimSpace(jsonStr)
	if strings.HasPrefix(s, "```") {
		lines := strings.SplitN(s, "\n", 2)
		if len(lines) > 1 {
			s = strings.TrimPrefix(lines[1], "json")
			s = strings.TrimSuffix(s, "```")
		}
	}
	if err := json.Unmarshal([]byte(s), &c); err != nil {
		return nil, err
	}
	return &c, nil
}
