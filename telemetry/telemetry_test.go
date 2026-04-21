package telemetry

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestArtifactStoreWriteRunArtifacts(t *testing.T) {
	tmp := t.TempDir()
	store := NewArtifactStore(tmp, 7)
	tr := &TurnRecorder{
		RunID:     "run-1",
		SessionID: "sess-1",
		Platform:  "http",
		Gateway:   "http",
		Model:     "test-model",
		StartedAt: time.Now().Add(-1 * time.Second).UTC(),
		EndedAt:   time.Now().UTC(),
	}
	tr.AddTranscript(TranscriptEntry{Role: "user", Content: "hi", Kind: "input"})
	tr.AddToolSummary(ToolCallSummary{ToolName: "read_file", DurationMs: 12, Success: true})

	if err := store.WriteRunArtifacts(tr, MetricSnapshot{
		Counters: map[string]int64{"x": 1},
		TimersMs: map[string][]int64{},
	}); err != nil {
		t.Fatalf("write artifacts: %v", err)
	}

	runDir := filepath.Join(tmp, "run-1")
	for _, name := range []string{"metadata.json", "transcript.json", "tool_summary.json", "metrics.json"} {
		if _, err := os.Stat(filepath.Join(runDir, name)); err != nil {
			t.Fatalf("expected file %s: %v", name, err)
		}
	}
}

func TestMetricsSnapshot(t *testing.T) {
	m := NewMetrics()
	m.IncCounter("tool_call_total.read_file", 1)
	m.ObserveDuration("tool_latency_ms.read_file", 25*time.Millisecond)
	snap := m.Snapshot()
	if snap.Counters["tool_call_total.read_file"] != 1 {
		t.Fatalf("unexpected counter: %#v", snap.Counters)
	}
	if len(snap.TimersMs["tool_latency_ms.read_file"]) != 1 {
		t.Fatalf("unexpected timers: %#v", snap.TimersMs)
	}
}

func TestFinishTurnWritesArtifacts(t *testing.T) {
	tmp := t.TempDir()
	rt := NewRuntime(Config{
		Enabled:               true,
		ArtifactsEnabled:      true,
		ArtifactsDir:          tmp,
		MetricsEnabled:        true,
		ArtifactRetentionDays: 1,
	})
	tr := rt.StartTurn("http", "sess-2", "model-x", "http")
	if tr == nil {
		t.Fatalf("expected turn recorder")
	}
	ctx := rt.WithTurn(context.Background(), tr)
	rt.OnToolCallStart(ctx, "read_file", `{"path":"README.md"}`)
	rt.OnToolCallEnd(ctx, "read_file", 10*time.Millisecond, nil, "", "ok", `{"path":"README.md"}`)
	rt.FinishTurn(tr, "done")

	runDir := filepath.Join(tmp, tr.RunID)
	b, err := os.ReadFile(filepath.Join(runDir, "metadata.json"))
	if err != nil {
		t.Fatalf("read metadata: %v", err)
	}
	var meta RunMetadata
	if err := json.Unmarshal(b, &meta); err != nil {
		t.Fatalf("unmarshal metadata: %v", err)
	}
	if meta.RunID != tr.RunID {
		t.Fatalf("run id mismatch: got %s want %s", meta.RunID, tr.RunID)
	}
}
