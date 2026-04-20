package agent

import (
	"context"
	"encoding/json"
	"log"
	"regexp"
	"strings"

	"custom-agent/agent/planning"

	"github.com/sashabaranov/go-openai"
)

var orchestrationMultiStepHints = regexp.MustCompile(`(?i)\b(then|after that|first|second|next|finally|step\s*\d|search.*then|look up.*then|find.*and)\b`)

func (a *Agent) shouldTryGeneralOrchestration(ctx context.Context, text string) bool {
	mode := strings.ToLower(strings.TrimSpace(a.orchestration.Mode))
	if mode == "" || mode == planning.OrchestrationModeOff {
		return false
	}
	if a.tools == nil {
		return false
	}
	if a.shouldSkipOrchestrationTrivial(text) {
		return false
	}
	switch mode {
	case planning.OrchestrationModeAlways:
		return true
	case planning.OrchestrationModeAuto:
		if orchestrationMultiStepHints.MatchString(text) {
			return true
		}
		if len(strings.TrimSpace(text)) > 500 {
			return true
		}
		return a.orchestrationRouterQuick(ctx, text)
	default:
		return false
	}
}

func (a *Agent) orchestrationRouterQuick(ctx context.Context, userText string) bool {
	sys := `Reply with ONLY JSON: {"use_orchestration":true|false}
Set true if the user likely needs multiple tools or ordered steps (e.g. research then act, read files then write, web then wallet).
Set false for a single simple action (one web search, one balance check, one file read) or pure chat.`

	resp, err := a.client.CreateChatCompletion(ctx, openai.ChatCompletionRequest{
		Model: subagentModelForIndex(0),
		Messages: []openai.ChatCompletionMessage{
			{Role: openai.ChatMessageRoleSystem, Content: sys},
			{Role: openai.ChatMessageRoleUser, Content: userText},
		},
	})
	if err != nil {
		log.Printf("[orchestration] router: %v", err)
		return false
	}
	if len(resp.Choices) == 0 {
		return false
	}
	raw := extractJSONObject(strings.TrimSpace(resp.Choices[0].Message.Content))
	var out struct {
		UseOrchestration bool `json:"use_orchestration"`
	}
	if err := json.Unmarshal([]byte(raw), &out); err != nil {
		return false
	}
	return out.UseOrchestration
}
