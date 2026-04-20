package telemetry

import (
	"sync"
	"time"
)

type MetricSnapshot struct {
	Counters map[string]int64   `json:"counters"`
	TimersMs map[string][]int64 `json:"timers_ms"`
}

type Metrics struct {
	mu       sync.Mutex
	counters map[string]int64
	timers   map[string][]int64
}

func NewMetrics() *Metrics {
	return &Metrics{
		counters: map[string]int64{},
		timers:   map[string][]int64{},
	}
}

func (m *Metrics) IncCounter(name string, delta int64) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.counters[name] += delta
}

func (m *Metrics) ObserveDuration(name string, d time.Duration) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.timers[name] = append(m.timers[name], d.Milliseconds())
}

func (m *Metrics) Snapshot() MetricSnapshot {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := MetricSnapshot{
		Counters: make(map[string]int64, len(m.counters)),
		TimersMs: make(map[string][]int64, len(m.timers)),
	}
	for k, v := range m.counters {
		out.Counters[k] = v
	}
	for k, v := range m.timers {
		cp := make([]int64, len(v))
		copy(cp, v)
		out.TimersMs[k] = cp
	}
	return out
}
