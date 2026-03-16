package skills

import (
	"context"
	"encoding/json"
	"strings"

	"github.com/sashabaranov/go-openai"
)

// FeasibilityResult holds the result of a feasibility/clarity check.
type FeasibilityResult struct {
	Clear       bool     `json:"clear"`
	Issues      []string `json:"issues"`
	Suggestions []string `json:"suggestions"`
}

// RunFeasibilityCheck calls the LLM to evaluate skill for clarity and feasibility.
func RunFeasibilityCheck(ctx context.Context, client *openai.Client, skillMd string, scripts map[string]string) (*FeasibilityResult, error) {
	var b strings.Builder
	b.WriteString("Evaluate this skill for clarity and feasibility. Reply with JSON only:\n")
	b.WriteString(`{"clear": true|false, "issues": ["issue1"], "suggestions": ["suggestion1"]}`)
	b.WriteString("\n\nSet clear=false if:\n")
	b.WriteString("- Instructions are ambiguous or contradictory\n")
	b.WriteString("- References tools that don't exist (e.g. fake_tool, unknown API)\n")
	b.WriteString("- Scripts assume missing binaries or unsupported OS\n")
	b.WriteString("- Inputs/outputs are unclear\n\n")
	b.WriteString("Agent has: run_command, read_file, write_file, web_search, save_memory, read_memory, create_scheduled_reminder, list_reminders, delete_reminder, spawn_subagents, http_request")
	b.WriteString(", list_skills, read_skill, read_skill_script. Wallet tools if configured. Environment: Linux/macOS, Python 3 typically available.\n\n")
	b.WriteString("--- Skill content ---\n")
	b.WriteString(skillMd)
	if len(scripts) > 0 {
		b.WriteString("\n\n--- Scripts ---\n")
		for path, content := range scripts {
			b.WriteString("\n[" + path + "]\n")
			b.WriteString(content)
			b.WriteString("\n")
		}
	}
	b.WriteString("\n--- End ---")

	resp, err := client.CreateChatCompletion(ctx, openai.ChatCompletionRequest{
		Model: "llama-3.1-8b-instant",
		Messages: []openai.ChatCompletionMessage{
			{Role: openai.ChatMessageRoleSystem, Content: "You are a skill reviewer. Output only valid JSON, no markdown or explanation."},
			{Role: openai.ChatMessageRoleUser, Content: b.String()},
		},
	})
	if err != nil {
		return nil, err
	}
	if len(resp.Choices) == 0 {
		return &FeasibilityResult{Clear: false, Issues: []string{"No response from feasibility check"}}, nil
	}
	content := strings.TrimSpace(resp.Choices[0].Message.Content)
	if strings.HasPrefix(content, "```") {
		content = strings.TrimPrefix(content, "```json")
		content = strings.TrimPrefix(content, "```")
		content = strings.TrimSuffix(content, "```")
		content = strings.TrimSpace(content)
	}
	var result FeasibilityResult
	if err := json.Unmarshal([]byte(content), &result); err != nil {
		return &FeasibilityResult{Clear: false, Issues: []string{"Invalid feasibility check response"}}, nil
	}
	return &result, nil
}
