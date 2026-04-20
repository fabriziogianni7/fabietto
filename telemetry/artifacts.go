package telemetry

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"
)

type TranscriptEntry struct {
	Role    string `json:"role"`
	Content string `json:"content"`
	Kind    string `json:"kind,omitempty"`
}

type ToolCallSummary struct {
	ToolName      string `json:"tool_name"`
	DurationMs    int64  `json:"duration_ms"`
	Success       bool   `json:"success"`
	ErrorCategory string `json:"error_category,omitempty"`
}

type RunMetadata struct {
	SchemaVersion string    `json:"schema_version"`
	RunID         string    `json:"run_id"`
	SessionID     string    `json:"session_id"`
	Platform      string    `json:"platform"`
	Gateway       string    `json:"gateway"`
	Model         string    `json:"model"`
	StartedAt     time.Time `json:"started_at"`
	EndedAt       time.Time `json:"ended_at"`
	DurationMs    int64     `json:"duration_ms"`
}

type ArtifactStore struct {
	baseDir       string
	retentionDays int
	mu            sync.Mutex
}

func NewArtifactStore(baseDir string, retentionDays int) *ArtifactStore {
	if strings.TrimSpace(baseDir) == "" {
		baseDir = "run-artifacts"
	}
	if retentionDays <= 0 {
		retentionDays = 7
	}
	return &ArtifactStore{
		baseDir:       filepath.Clean(baseDir),
		retentionDays: retentionDays,
	}
}

func (s *ArtifactStore) WriteRunArtifacts(tr *TurnRecorder, metrics MetricSnapshot) error {
	if tr == nil {
		return nil
	}
	s.mu.Lock()
	defer s.mu.Unlock()

	if err := os.MkdirAll(s.baseDir, 0755); err != nil {
		return err
	}
	runDir := filepath.Join(s.baseDir, tr.RunID)
	if err := os.MkdirAll(runDir, 0755); err != nil {
		return err
	}

	meta := RunMetadata{
		SchemaVersion: schemaVersion,
		RunID:         tr.RunID,
		SessionID:     tr.SessionID,
		Platform:      tr.Platform,
		Gateway:       tr.Gateway,
		Model:         tr.Model,
		StartedAt:     tr.StartedAt,
		EndedAt:       tr.EndedAt,
		DurationMs:    tr.EndedAt.Sub(tr.StartedAt).Milliseconds(),
	}
	if err := writeJSON(filepath.Join(runDir, "metadata.json"), meta); err != nil {
		return err
	}
	if err := writeJSON(filepath.Join(runDir, "transcript.json"), tr.Transcript()); err != nil {
		return err
	}
	if err := writeJSON(filepath.Join(runDir, "tool_summary.json"), tr.ToolCalls()); err != nil {
		return err
	}
	if err := writeJSON(filepath.Join(runDir, "metrics.json"), metrics); err != nil {
		return err
	}
	return s.prune()
}

func (s *ArtifactStore) prune() error {
	entries, err := os.ReadDir(s.baseDir)
	if err != nil {
		return nil
	}
	cutoff := time.Now().Add(-time.Duration(s.retentionDays) * 24 * time.Hour)
	var toDelete []string
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		info, err := e.Info()
		if err != nil {
			continue
		}
		if info.ModTime().Before(cutoff) {
			toDelete = append(toDelete, filepath.Join(s.baseDir, e.Name()))
		}
	}
	sort.Strings(toDelete)
	for _, p := range toDelete {
		_ = os.RemoveAll(p)
	}
	return nil
}

func writeJSON(path string, v interface{}) error {
	b, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return err
	}
	b = append(b, '\n')
	return os.WriteFile(path, b, 0644)
}

func (tr *TurnRecorder) AddTranscript(entry TranscriptEntry) {
	if tr == nil {
		return
	}
	tr.mu.Lock()
	defer tr.mu.Unlock()
	if strings.TrimSpace(entry.Content) == "" && entry.Kind != "tool_call_start" {
		return
	}
	tr.transcript = append(tr.transcript, entry)
}

func (tr *TurnRecorder) AddToolSummary(s ToolCallSummary) {
	if tr == nil {
		return
	}
	tr.mu.Lock()
	defer tr.mu.Unlock()
	tr.tools = append(tr.tools, s)
}

func (tr *TurnRecorder) Transcript() []TranscriptEntry {
	tr.mu.Lock()
	defer tr.mu.Unlock()
	out := make([]TranscriptEntry, len(tr.transcript))
	copy(out, tr.transcript)
	return out
}

func (tr *TurnRecorder) ToolCalls() []ToolCallSummary {
	tr.mu.Lock()
	defer tr.mu.Unlock()
	out := make([]ToolCallSummary, len(tr.tools))
	copy(out, tr.tools)
	return out
}
