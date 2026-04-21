package agent

import (
	"context"
	"fmt"
	"strings"
	"time"

	"custom-agent/telemetry"

	"github.com/sashabaranov/go-openai"
)

const llmRoundContentLimit = 16000 // per message field in llm_rounds.json

func truncateLLMString(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "...(truncated)"
}

// recordLLMRound captures request/response for debugging when TELEMETRY_LLM_ROUNDS is enabled.
func (a *Agent) recordLLMRound(ctx context.Context, phase string, round int, req openai.ChatCompletionRequest, resp openai.ChatCompletionResponse, err error, start time.Time) {
	if a == nil || a.telemetry == nil || !a.telemetry.LLMRoundsEnabled() {
		return
	}
	model := req.Model
	if model == "" {
		model = parentModel
	}
	rec := telemetry.LLMRoundRecord{
		Phase:      phase,
		Round:      round,
		Model:      model,
		DurationMs: time.Since(start).Milliseconds(),
		Note:       "Chain-of-thought / reasoning is not exposed on ChatCompletionMessage in go-openai v1.20; capture via provider-specific fields or proxy logs if needed.",
	}
	rec.Request.MessageCount = len(req.Messages)
	rec.Request.Messages = make([]telemetry.LLMMessageSnap, 0, len(req.Messages))
	for _, m := range req.Messages {
		rec.Request.Messages = append(rec.Request.Messages, snapshotOpenAIMessage(m))
	}
	for _, t := range req.Tools {
		if t.Type == openai.ToolTypeFunction && t.Function != nil && strings.TrimSpace(t.Function.Name) != "" {
			rec.Request.ToolNames = append(rec.Request.ToolNames, t.Function.Name)
		}
	}
	if err != nil {
		rec.Error = err.Error()
		a.telemetry.RecordLLMRound(ctx, rec)
		return
	}
	if len(resp.Choices) > 0 {
		ch := resp.Choices[0]
		out := telemetry.LLMRoundResponse{
			Content:      truncateLLMString(ch.Message.Content, llmRoundContentLimit),
			FinishReason: string(ch.FinishReason),
		}
		if resp.Usage.PromptTokens != 0 {
			out.UsagePromptTokens = resp.Usage.PromptTokens
		}
		if resp.Usage.CompletionTokens != 0 {
			out.UsageCompletionTokens = resp.Usage.CompletionTokens
		}
		if resp.Usage.TotalTokens != 0 {
			out.UsageTotalTokens = resp.Usage.TotalTokens
		}
		for _, tc := range ch.Message.ToolCalls {
			out.ToolCalls = append(out.ToolCalls, telemetry.LLMToolCallSnap{
				ID:          tc.ID,
				Name:        tc.Function.Name,
				ArgsPreview: truncateLLMString(tc.Function.Arguments, 4000),
			})
		}
		rec.Response = &out
	}
	a.telemetry.RecordLLMRound(ctx, rec)
}

func snapshotOpenAIMessage(m openai.ChatCompletionMessage) telemetry.LLMMessageSnap {
	s := telemetry.LLMMessageSnap{
		Role:       m.Role,
		Content:    truncateLLMString(m.Content, llmRoundContentLimit),
		Name:       m.Name,
		ToolCallID: m.ToolCallID,
	}
	if len(m.MultiContent) > 0 {
		if s.Content != "" {
			s.Content += " "
		}
		s.Content += fmt.Sprintf("[multi_content: %d parts]", len(m.MultiContent))
	}
	for _, tc := range m.ToolCalls {
		args := truncateLLMString(tc.Function.Arguments, 400)
		s.ToolCallsSummary = append(s.ToolCallsSummary, fmt.Sprintf("%s(%s)", tc.Function.Name, args))
	}
	return s
}
