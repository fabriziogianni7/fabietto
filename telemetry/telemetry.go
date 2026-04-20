package telemetry

import (
	"context"
	"encoding/json"
	"log"
	"strings"
	"sync"
	"time"
)

const schemaVersion = "v1"

type Config struct {
	Enabled               bool
	Verbosity             string
	ArtifactsEnabled      bool
	ArtifactsDir          string
	ArtifactRetentionDays int
	MetricsEnabled        bool
}

type Event struct {
	SchemaVersion string                 `json:"schema_version"`
	EventType     string                 `json:"event_type"`
	Timestamp     time.Time              `json:"timestamp"`
	RunID         string                 `json:"run_id,omitempty"`
	SessionID     string                 `json:"session_id,omitempty"`
	Platform      string                 `json:"platform,omitempty"`
	ToolName      string                 `json:"tool_name,omitempty"`
	ErrorCategory string                 `json:"error_category,omitempty"`
	DurationMs    int64                  `json:"duration_ms,omitempty"`
	Data          map[string]interface{} `json:"data,omitempty"`
}

type Runtime struct {
	cfg     Config
	metrics *Metrics
	store   *ArtifactStore
}

type turnContextKey struct{}

type TurnRecorder struct {
	RunID      string
	SessionID  string
	Platform   string
	Model      string
	Gateway    string
	StartedAt  time.Time
	EndedAt    time.Time
	transcript []TranscriptEntry
	tools      []ToolCallSummary
	mu         sync.Mutex
}

func NewRuntime(cfg Config) *Runtime {
	if cfg.Verbosity == "" {
		cfg.Verbosity = "basic"
	}
	if cfg.ArtifactsDir == "" {
		cfg.ArtifactsDir = "run-artifacts"
	}
	if cfg.ArtifactRetentionDays <= 0 {
		cfg.ArtifactRetentionDays = 7
	}
	rt := &Runtime{cfg: cfg}
	if cfg.MetricsEnabled {
		rt.metrics = NewMetrics()
	}
	if cfg.ArtifactsEnabled {
		rt.store = NewArtifactStore(cfg.ArtifactsDir, cfg.ArtifactRetentionDays)
	}
	return rt
}

func (r *Runtime) Enabled() bool { return r != nil && r.cfg.Enabled }

func (r *Runtime) WithTurn(ctx context.Context, tr *TurnRecorder) context.Context {
	if ctx == nil {
		ctx = context.Background()
	}
	return context.WithValue(ctx, turnContextKey{}, tr)
}

func (r *Runtime) turnFromContext(ctx context.Context) *TurnRecorder {
	if ctx == nil {
		return nil
	}
	tr, _ := ctx.Value(turnContextKey{}).(*TurnRecorder)
	return tr
}

func (r *Runtime) StartTurn(platform, sessionID, model, gateway string) *TurnRecorder {
	if !r.Enabled() {
		return nil
	}
	now := time.Now().UTC()
	runID := now.Format("20060102T150405.000000000Z07:00")
	runID = strings.ReplaceAll(runID, ":", "")
	tr := &TurnRecorder{
		RunID:     runID,
		SessionID: sessionID,
		Platform:  platform,
		Model:     model,
		Gateway:   gateway,
		StartedAt: now,
	}
	r.emit(Event{
		SchemaVersion: schemaVersion,
		EventType:     "agent_turn_start",
		Timestamp:     now,
		RunID:         tr.RunID,
		SessionID:     tr.SessionID,
		Platform:      tr.Platform,
		Data: map[string]interface{}{
			"model":   model,
			"gateway": gateway,
		},
	})
	return tr
}

func (r *Runtime) FinishTurn(tr *TurnRecorder, finalReply string) {
	if !r.Enabled() || tr == nil {
		return
	}
	tr.EndedAt = time.Now().UTC()
	tr.AddTranscript(TranscriptEntry{Role: "assistant", Content: finalReply, Kind: "final_response"})
	duration := tr.EndedAt.Sub(tr.StartedAt)
	if r.metrics != nil {
		r.metrics.IncCounter("agent_turn_total", 1)
		r.metrics.ObserveDuration("agent_turn_latency_ms", duration)
	}
	r.emit(Event{
		SchemaVersion: schemaVersion,
		EventType:     "agent_turn_end",
		Timestamp:     tr.EndedAt,
		RunID:         tr.RunID,
		SessionID:     tr.SessionID,
		Platform:      tr.Platform,
		DurationMs:    duration.Milliseconds(),
		Data: map[string]interface{}{
			"tool_calls": len(tr.ToolCalls()),
		},
	})
	if r.cfg.ArtifactsEnabled && r.store != nil {
		snapshot := MetricSnapshot{Counters: map[string]int64{}, TimersMs: map[string][]int64{}}
		if r.metrics != nil {
			snapshot = r.metrics.Snapshot()
		}
		_ = r.store.WriteRunArtifacts(tr, snapshot)
	}
}

func (r *Runtime) RecordCompactionDecision(ctx context.Context, used bool, totalTokens, threshold int) {
	if !r.Enabled() {
		return
	}
	tr := r.turnFromContext(ctx)
	r.emit(Event{
		SchemaVersion: schemaVersion,
		EventType:     "compaction_decision",
		Timestamp:     time.Now().UTC(),
		RunID:         turnID(tr),
		SessionID:     turnSession(tr),
		Data: map[string]interface{}{
			"used":         used,
			"total_tokens": totalTokens,
			"threshold":    threshold,
		},
	})
}

func (r *Runtime) RecordMemoryDecision(ctx context.Context, source, query string, resultCount int, mode string) {
	if !r.Enabled() {
		return
	}
	tr := r.turnFromContext(ctx)
	r.emit(Event{
		SchemaVersion: schemaVersion,
		EventType:     "memory_retrieval_decision",
		Timestamp:     time.Now().UTC(),
		RunID:         turnID(tr),
		SessionID:     turnSession(tr),
		Data: map[string]interface{}{
			"source":       source,
			"query":        truncate(query, 160),
			"result_count": resultCount,
			"mode":         mode,
		},
	})
}

func (r *Runtime) RecordFallbackParser(ctx context.Context, used bool) {
	if !r.Enabled() {
		return
	}
	if used && r.metrics != nil {
		r.metrics.IncCounter("fallback_parser_usage_total", 1)
	}
}

// RecordPlanEvent emits a plan lifecycle event (planner validation, step execution, etc.).
// eventType is a short suffix; orchestration uses prefixes like "orchestration_plan_start".
func (r *Runtime) RecordPlanEvent(ctx context.Context, eventType string, data map[string]interface{}) {
	if !r.Enabled() {
		return
	}
	tr := r.turnFromContext(ctx)
	if data == nil {
		data = map[string]interface{}{}
	}
	prefix := "plan_"
	if strings.HasPrefix(eventType, "orchestration_") {
		prefix = ""
	}
	r.emit(Event{
		SchemaVersion: schemaVersion,
		EventType:     prefix + eventType,
		Timestamp:     time.Now().UTC(),
		RunID:         turnID(tr),
		SessionID:     turnSession(tr),
		Data:          data,
	})
}

func (r *Runtime) OnToolCallStart(ctx context.Context, name, argsJSON string) {
	if !r.Enabled() {
		return
	}
	tr := r.turnFromContext(ctx)
	if tr != nil {
		tr.AddTranscript(TranscriptEntry{Role: "assistant", Content: name, Kind: "tool_call_start"})
	}
	data := map[string]interface{}{}
	if r.cfg.Verbosity == "debug" {
		data["args_json"] = truncate(argsJSON, 400)
	}
	r.emit(Event{
		SchemaVersion: schemaVersion,
		EventType:     "tool_call_start",
		Timestamp:     time.Now().UTC(),
		RunID:         turnID(tr),
		SessionID:     turnSession(tr),
		ToolName:      name,
		Data:          data,
	})
}

func (r *Runtime) OnToolCallEnd(ctx context.Context, name string, duration time.Duration, err error, errorCategory, result string) {
	if !r.Enabled() {
		return
	}
	tr := r.turnFromContext(ctx)
	if tr != nil {
		tr.AddToolSummary(ToolCallSummary{
			ToolName:      name,
			DurationMs:    duration.Milliseconds(),
			Success:       err == nil && errorCategory == "",
			ErrorCategory: errorCategory,
		})
		tr.AddTranscript(TranscriptEntry{Role: "tool", Content: truncate(result, 500), Kind: "tool_call_end"})
	}
	if r.metrics != nil {
		r.metrics.IncCounter("tool_call_total."+name, 1)
		r.metrics.ObserveDuration("tool_latency_ms."+name, duration)
		if errorCategory != "" {
			r.metrics.IncCounter("tool_error_total."+name+"."+errorCategory, 1)
		}
	}
	r.emit(Event{
		SchemaVersion: schemaVersion,
		EventType:     "tool_call_end",
		Timestamp:     time.Now().UTC(),
		RunID:         turnID(tr),
		SessionID:     turnSession(tr),
		ToolName:      name,
		ErrorCategory: errorCategory,
		DurationMs:    duration.Milliseconds(),
	})
}

func (r *Runtime) emit(evt Event) {
	if !r.Enabled() {
		return
	}
	b, err := json.Marshal(evt)
	if err != nil {
		log.Printf("[telemetry] marshal event failed: %v", err)
		return
	}
	log.Printf("[telemetry] %s", string(b))
}

func turnID(tr *TurnRecorder) string {
	if tr == nil {
		return ""
	}
	return tr.RunID
}

func turnSession(tr *TurnRecorder) string {
	if tr == nil {
		return ""
	}
	return tr.SessionID
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "...(truncated)"
}
