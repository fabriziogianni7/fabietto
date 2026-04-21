package telemetry

import (
	"context"
	"log"
	"time"
)

// LLMRoundRecord is one chat completion request/response pair for debugging (reactive loop,
// orchestration, etc.). Enable via TELEMETRY_LLM_ROUNDS=true.
type LLMRoundRecord struct {
	Phase      string            `json:"phase"`
	Round      int               `json:"round"` // index within that phase (e.g. tool loop iteration)
	Model      string            `json:"model"`
	Request    LLMRoundRequest   `json:"request"`
	Response   *LLMRoundResponse `json:"response,omitempty"`
	Error      string            `json:"error,omitempty"`
	DurationMs int64             `json:"duration_ms"`
	// NoteReasoning: API/library may not expose chain-of-thought; extend when provider returns it.
	Note string `json:"note,omitempty"`
}

type LLMRoundRequest struct {
	MessageCount int              `json:"message_count"`
	Messages     []LLMMessageSnap `json:"messages"`
	ToolNames    []string         `json:"tool_names,omitempty"`
}

// LLMMessageSnap is a truncated, log-safe view of a chat message.
type LLMMessageSnap struct {
	Role             string   `json:"role"`
	Content          string   `json:"content,omitempty"`
	Name             string   `json:"name,omitempty"`
	ToolCallID       string   `json:"tool_call_id,omitempty"`
	ToolCallsSummary []string `json:"tool_calls_summary,omitempty"`
}

type LLMToolCallSnap struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	ArgsPreview string `json:"args_preview,omitempty"`
}

type LLMRoundResponse struct {
	Content               string            `json:"content,omitempty"`
	FinishReason          string            `json:"finish_reason,omitempty"`
	UsagePromptTokens     int               `json:"usage_prompt_tokens,omitempty"`
	UsageCompletionTokens int               `json:"usage_completion_tokens,omitempty"`
	UsageTotalTokens      int               `json:"usage_total_tokens,omitempty"`
	ToolCalls             []LLMToolCallSnap `json:"tool_calls,omitempty"`
}

// AddLLMRound appends a round to the current turn (used when run artifacts are enabled).
func (tr *TurnRecorder) AddLLMRound(rec LLMRoundRecord) {
	if tr == nil {
		return
	}
	tr.mu.Lock()
	defer tr.mu.Unlock()
	tr.llmRounds = append(tr.llmRounds, rec)
}

// LLMRounds returns a copy of recorded LLM rounds for this turn.
func (tr *TurnRecorder) LLMRounds() []LLMRoundRecord {
	if tr == nil {
		return nil
	}
	tr.mu.Lock()
	defer tr.mu.Unlock()
	out := make([]LLMRoundRecord, len(tr.llmRounds))
	copy(out, tr.llmRounds)
	return out
}

// LLMRoundsEnabled reports whether per-request LLM capture is on (TELEMETRY_LLM_ROUNDS).
func (r *Runtime) LLMRoundsEnabled() bool {
	return r != nil && r.cfg.LLMRoundsEnabled
}

// RecordLLMRound stores a round on the turn from ctx and emits a short log line.
func (r *Runtime) RecordLLMRound(ctx context.Context, rec LLMRoundRecord) {
	if r == nil || !r.cfg.LLMRoundsEnabled {
		return
	}
	tr := r.turnFromContext(ctx)
	if tr == nil {
		return
	}
	tr.AddLLMRound(rec)
	log.Printf("[telemetry] llm_round phase=%s round=%d model=%s duration_ms=%d req_msgs=%d err=%q",
		rec.Phase, rec.Round, rec.Model, rec.DurationMs, rec.Request.MessageCount, rec.Error)
	r.emit(Event{
		SchemaVersion: schemaVersion,
		EventType:     "llm_round",
		Timestamp:     time.Now().UTC(),
		RunID:         turnID(tr),
		SessionID:     turnSession(tr),
		Platform:      tr.Platform,
		DurationMs:    rec.DurationMs,
		Data: map[string]interface{}{
			"phase":       rec.Phase,
			"round":       rec.Round,
			"model":       rec.Model,
			"message_cnt": rec.Request.MessageCount,
			"error":       rec.Error,
		},
	})
}
