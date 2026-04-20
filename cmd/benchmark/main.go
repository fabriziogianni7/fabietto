package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"custom-agent/eval"
)

type Config struct {
	SchemaVersion       string          `json:"schema_version"`
	MaxTrendHistoryRuns int             `json:"max_trend_history_runs"`
	Dimensions          DimensionConfig `json:"dimensions"`
	Thresholds          ThresholdConfig `json:"thresholds"`
}

type DimensionConfig struct {
	TaskSuccessCategory string `json:"task_success_category"`
	ToolCategory        string `json:"tool_correctness_category"`
	SafetyCategory      string `json:"safety_category"`
}

type ThresholdConfig struct {
	MinimumTaskSuccessRate     float64 `json:"minimum_task_success_rate"`
	MinimumToolCorrectnessRate float64 `json:"minimum_tool_correctness_rate"`
	MaximumSafetyViolationRate float64 `json:"maximum_safety_violation_rate"`
	MaximumLatencyP50Ms        float64 `json:"maximum_latency_p50_ms"`
	MaximumLatencyP95Ms        float64 `json:"maximum_latency_p95_ms"`
}

type BenchmarkReport struct {
	SchemaVersion string            `json:"schema_version"`
	GeneratedAt   string            `json:"generated_at"`
	Source        BenchmarkSource   `json:"source"`
	Summary       BenchmarkSummary  `json:"summary"`
	Dimensions    DimensionSnapshot `json:"dimensions"`
	CategoryRates map[string]Rate   `json:"category_rates"`
	LatencyMs     LatencySummary    `json:"latency_ms"`
	Thresholds    ThresholdSnapshot `json:"thresholds"`
}

type BenchmarkSource struct {
	CasesDir string `json:"cases_dir"`
	Backend  string `json:"backend"`
}

type BenchmarkSummary struct {
	TotalCases int `json:"total_cases"`
	Passed     int `json:"passed"`
	Failed     int `json:"failed"`
}

type Rate struct {
	Passed int     `json:"passed"`
	Total  int     `json:"total"`
	Rate   float64 `json:"rate"`
}

type LatencySummary struct {
	Samples int     `json:"samples"`
	P50     float64 `json:"p50"`
	P95     float64 `json:"p95"`
}

type ThresholdSnapshot struct {
	TaskSuccessRate     float64 `json:"task_success_rate"`
	ToolCorrectnessRate float64 `json:"tool_correctness_rate"`
	SafetyViolationRate float64 `json:"safety_violation_rate"`
	LatencyP50Ms        float64 `json:"latency_p50_ms"`
	LatencyP95Ms        float64 `json:"latency_p95_ms"`
}

type DimensionSnapshot struct {
	TaskSuccessCategory string `json:"task_success_category"`
	ToolCategory        string `json:"tool_correctness_category"`
	SafetyCategory      string `json:"safety_category"`
}

type TrendArtifact struct {
	SchemaVersion string       `json:"schema_version"`
	UpdatedAt     string       `json:"updated_at"`
	Runs          []TrendEntry `json:"runs"`
}

type TrendEntry struct {
	GeneratedAt         string  `json:"generated_at"`
	TaskSuccessRate     float64 `json:"task_success_rate"`
	ToolCorrectnessRate float64 `json:"tool_correctness_rate"`
	SafetyViolationRate float64 `json:"safety_violation_rate"`
	LatencyP50Ms        float64 `json:"latency_p50_ms"`
	LatencyP95Ms        float64 `json:"latency_p95_ms"`
	TotalCases          int     `json:"total_cases"`
	FailedCases         int     `json:"failed_cases"`
}

func main() {
	var (
		casesDir    = flag.String("cases", "eval/cases", "Directory containing eval case JSON files")
		backend     = flag.String("backend", "fake", "Eval backend mode (default: fake)")
		configPath  = flag.String("config", "benchmarks/config.json", "Benchmark config path")
		outDir      = flag.String("out-dir", "benchmarks/results", "Directory for benchmark artifacts")
		reportFile  = flag.String("report-file", "latest.json", "Benchmark report JSON filename")
		trendFile   = flag.String("trend-file", "trend.json", "Benchmark trend JSON filename")
		summaryFile = flag.String("summary-file", "summary.md", "Benchmark summary Markdown filename")
	)
	flag.Parse()

	cfg, err := loadConfig(*configPath)
	if err != nil {
		exitErr(fmt.Errorf("failed to load benchmark config: %w", err))
	}
	cases, err := eval.LoadCases(*casesDir)
	if err != nil {
		exitErr(fmt.Errorf("failed to load eval cases: %w", err))
	}
	if len(cases) == 0 {
		exitErr(fmt.Errorf("no eval cases found in %s", *casesDir))
	}

	report := eval.NewRunner(*backend).RunCases(cases)
	benchmark := buildBenchmarkReport(report, cfg, *casesDir, *backend)

	if err := os.MkdirAll(*outDir, 0o755); err != nil {
		exitErr(fmt.Errorf("failed to create output directory: %w", err))
	}
	reportPath := filepath.Join(*outDir, *reportFile)
	trendPath := filepath.Join(*outDir, *trendFile)
	summaryPath := filepath.Join(*outDir, *summaryFile)

	if err := writeJSON(reportPath, benchmark); err != nil {
		exitErr(fmt.Errorf("failed to write benchmark report: %w", err))
	}

	trend, err := updateTrend(trendPath, benchmark, cfg.MaxTrendHistoryRuns)
	if err != nil {
		exitErr(fmt.Errorf("failed to update trend report: %w", err))
	}
	if err := writeJSON(trendPath, trend); err != nil {
		exitErr(fmt.Errorf("failed to write trend report: %w", err))
	}
	if err := writeSummary(summaryPath, benchmark, trend); err != nil {
		exitErr(fmt.Errorf("failed to write markdown summary: %w", err))
	}

	fmt.Printf("Benchmark report written: %s\n", reportPath)
	fmt.Printf("Trend report written: %s\n", trendPath)
	fmt.Printf("Summary written: %s\n", summaryPath)
}

func loadConfig(path string) (Config, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return Config{}, err
	}
	var cfg Config
	if err := json.Unmarshal(raw, &cfg); err != nil {
		return Config{}, err
	}
	return cfg, nil
}

func buildBenchmarkReport(report eval.Report, cfg Config, casesDir, backend string) BenchmarkReport {
	catRates := map[string]Rate{}
	for _, c := range report.Summary.CategoryStats {
		catRates[c.Category] = Rate{
			Passed: c.Passed,
			Total:  c.Total,
			Rate:   ratio(c.Passed, c.Total),
		}
	}

	latencies := make([]float64, 0, len(report.Results))
	for _, r := range report.Results {
		latencies = append(latencies, r.DurationMs)
	}
	sort.Float64s(latencies)

	taskRate := ratio(report.Summary.CasesPassed, report.Summary.CasesTotal)
	toolRate := rateForCategory(catRates, cfg.Dimensions.ToolCategory)
	safetyPass := rateForCategory(catRates, cfg.Dimensions.SafetyCategory)

	return BenchmarkReport{
		SchemaVersion: "v1",
		GeneratedAt:   time.Now().UTC().Format(time.RFC3339),
		Source: BenchmarkSource{
			CasesDir: casesDir,
			Backend:  backend,
		},
		Summary: BenchmarkSummary{
			TotalCases: report.Summary.CasesTotal,
			Passed:     report.Summary.CasesPassed,
			Failed:     report.Summary.CasesFailed,
		},
		Dimensions: DimensionSnapshot{
			TaskSuccessCategory: cfg.Dimensions.TaskSuccessCategory,
			ToolCategory:        cfg.Dimensions.ToolCategory,
			SafetyCategory:      cfg.Dimensions.SafetyCategory,
		},
		CategoryRates: catRates,
		LatencyMs: LatencySummary{
			Samples: len(latencies),
			P50:     percentile(latencies, 0.50),
			P95:     percentile(latencies, 0.95),
		},
		Thresholds: ThresholdSnapshot{
			TaskSuccessRate:     taskRate,
			ToolCorrectnessRate: toolRate,
			SafetyViolationRate: 1.0 - safetyPass,
			LatencyP50Ms:        percentile(latencies, 0.50),
			LatencyP95Ms:        percentile(latencies, 0.95),
		},
	}
}

func updateTrend(path string, report BenchmarkReport, maxRuns int) (TrendArtifact, error) {
	trend := TrendArtifact{
		SchemaVersion: "v1",
		Runs:          []TrendEntry{},
	}
	if raw, err := os.ReadFile(path); err == nil {
		_ = json.Unmarshal(raw, &trend)
	}
	trend.Runs = append(trend.Runs, TrendEntry{
		GeneratedAt:         report.GeneratedAt,
		TaskSuccessRate:     report.Thresholds.TaskSuccessRate,
		ToolCorrectnessRate: report.Thresholds.ToolCorrectnessRate,
		SafetyViolationRate: report.Thresholds.SafetyViolationRate,
		LatencyP50Ms:        report.Thresholds.LatencyP50Ms,
		LatencyP95Ms:        report.Thresholds.LatencyP95Ms,
		TotalCases:          report.Summary.TotalCases,
		FailedCases:         report.Summary.Failed,
	})
	if maxRuns > 0 && len(trend.Runs) > maxRuns {
		trend.Runs = trend.Runs[len(trend.Runs)-maxRuns:]
	}
	trend.UpdatedAt = time.Now().UTC().Format(time.RFC3339)
	return trend, nil
}

func writeSummary(path string, report BenchmarkReport, trend TrendArtifact) error {
	var b strings.Builder
	b.WriteString("# Benchmark Summary\n\n")
	b.WriteString(fmt.Sprintf("- Generated at: `%s`\n", report.GeneratedAt))
	b.WriteString(fmt.Sprintf("- Cases: `%d passed / %d total`\n", report.Summary.Passed, report.Summary.TotalCases))
	b.WriteString(fmt.Sprintf("- Task success rate: `%.2f%%`\n", report.Thresholds.TaskSuccessRate*100))
	b.WriteString(fmt.Sprintf("- Tool correctness rate: `%.2f%%`\n", report.Thresholds.ToolCorrectnessRate*100))
	b.WriteString(fmt.Sprintf("- Safety violation rate: `%.2f%%`\n", report.Thresholds.SafetyViolationRate*100))
	b.WriteString(fmt.Sprintf("- Latency p50/p95: `%.2fms / %.2fms`\n\n", report.Thresholds.LatencyP50Ms, report.Thresholds.LatencyP95Ms))

	if len(trend.Runs) > 1 {
		prev := trend.Runs[len(trend.Runs)-2]
		b.WriteString("## Trend vs previous run\n\n")
		b.WriteString(fmt.Sprintf("- Task success delta: `%+.2fpp`\n", (report.Thresholds.TaskSuccessRate-prev.TaskSuccessRate)*100))
		b.WriteString(fmt.Sprintf("- Tool correctness delta: `%+.2fpp`\n", (report.Thresholds.ToolCorrectnessRate-prev.ToolCorrectnessRate)*100))
		b.WriteString(fmt.Sprintf("- Safety violation delta: `%+.2fpp`\n", (report.Thresholds.SafetyViolationRate-prev.SafetyViolationRate)*100))
		b.WriteString(fmt.Sprintf("- Latency p50 delta: `%+.2fms`\n", report.Thresholds.LatencyP50Ms-prev.LatencyP50Ms))
		b.WriteString(fmt.Sprintf("- Latency p95 delta: `%+.2fms`\n\n", report.Thresholds.LatencyP95Ms-prev.LatencyP95Ms))
	}

	b.WriteString("## Recent runs\n\n")
	b.WriteString("| Generated | Task Success | Tool Correctness | Safety Violation | p50 (ms) | p95 (ms) |\n")
	b.WriteString("|---|---:|---:|---:|---:|---:|\n")
	for _, run := range trend.Runs {
		b.WriteString(fmt.Sprintf(
			"| %s | %.2f%% | %.2f%% | %.2f%% | %.2f | %.2f |\n",
			run.GeneratedAt,
			run.TaskSuccessRate*100,
			run.ToolCorrectnessRate*100,
			run.SafetyViolationRate*100,
			run.LatencyP50Ms,
			run.LatencyP95Ms,
		))
	}

	return os.WriteFile(path, []byte(b.String()), 0o644)
}

func writeJSON(path string, data interface{}) error {
	raw, err := json.MarshalIndent(data, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, raw, 0o644)
}

func rateForCategory(rates map[string]Rate, category string) float64 {
	r, ok := rates[category]
	if !ok {
		return 0
	}
	return r.Rate
}

func percentile(vals []float64, p float64) float64 {
	if len(vals) == 0 {
		return 0
	}
	if p <= 0 {
		return vals[0]
	}
	if p >= 1 {
		return vals[len(vals)-1]
	}
	index := int(math.Ceil(float64(len(vals))*p)) - 1
	if index < 0 {
		index = 0
	}
	if index >= len(vals) {
		index = len(vals) - 1
	}
	return vals[index]
}

func ratio(num, den int) float64 {
	if den <= 0 {
		return 0
	}
	return float64(num) / float64(den)
}

func exitErr(err error) {
	fmt.Fprintln(os.Stderr, err)
	os.Exit(1)
}
