package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"strings"
)

type Config struct {
	Thresholds ThresholdConfig `json:"thresholds"`
}

type ThresholdConfig struct {
	MinimumTaskSuccessRate     float64 `json:"minimum_task_success_rate"`
	MinimumToolCorrectnessRate float64 `json:"minimum_tool_correctness_rate"`
	MaximumSafetyViolationRate float64 `json:"maximum_safety_violation_rate"`
	MaximumLatencyP50Ms        float64 `json:"maximum_latency_p50_ms"`
	MaximumLatencyP95Ms        float64 `json:"maximum_latency_p95_ms"`
}

type BenchmarkReport struct {
	Thresholds ThresholdSnapshot `json:"thresholds"`
}

type ThresholdSnapshot struct {
	TaskSuccessRate     float64 `json:"task_success_rate"`
	ToolCorrectnessRate float64 `json:"tool_correctness_rate"`
	SafetyViolationRate float64 `json:"safety_violation_rate"`
	LatencyP50Ms        float64 `json:"latency_p50_ms"`
	LatencyP95Ms        float64 `json:"latency_p95_ms"`
}

func main() {
	var (
		configPath = flag.String("config", "benchmarks/config.json", "Benchmark config path")
		reportPath = flag.String("report", "benchmarks/results/latest.json", "Benchmark report path")
	)
	flag.Parse()

	cfg, err := loadConfig(*configPath)
	if err != nil {
		exitErr(fmt.Errorf("failed to load config: %w", err))
	}
	report, err := loadReport(*reportPath)
	if err != nil {
		exitErr(fmt.Errorf("failed to load benchmark report: %w", err))
	}

	var failures []string
	if report.Thresholds.TaskSuccessRate < cfg.Thresholds.MinimumTaskSuccessRate {
		failures = append(failures, fmt.Sprintf(
			"task success rate %.2f%% is below threshold %.2f%%",
			report.Thresholds.TaskSuccessRate*100,
			cfg.Thresholds.MinimumTaskSuccessRate*100,
		))
	}
	if report.Thresholds.ToolCorrectnessRate < cfg.Thresholds.MinimumToolCorrectnessRate {
		failures = append(failures, fmt.Sprintf(
			"tool correctness rate %.2f%% is below threshold %.2f%%",
			report.Thresholds.ToolCorrectnessRate*100,
			cfg.Thresholds.MinimumToolCorrectnessRate*100,
		))
	}
	if report.Thresholds.SafetyViolationRate > cfg.Thresholds.MaximumSafetyViolationRate {
		failures = append(failures, fmt.Sprintf(
			"safety violation rate %.2f%% exceeds threshold %.2f%%",
			report.Thresholds.SafetyViolationRate*100,
			cfg.Thresholds.MaximumSafetyViolationRate*100,
		))
	}
	if report.Thresholds.LatencyP50Ms > cfg.Thresholds.MaximumLatencyP50Ms {
		failures = append(failures, fmt.Sprintf(
			"latency p50 %.2fms exceeds threshold %.2fms",
			report.Thresholds.LatencyP50Ms,
			cfg.Thresholds.MaximumLatencyP50Ms,
		))
	}
	if report.Thresholds.LatencyP95Ms > cfg.Thresholds.MaximumLatencyP95Ms {
		failures = append(failures, fmt.Sprintf(
			"latency p95 %.2fms exceeds threshold %.2fms",
			report.Thresholds.LatencyP95Ms,
			cfg.Thresholds.MaximumLatencyP95Ms,
		))
	}

	if len(failures) > 0 {
		fmt.Println("Release gate failed:")
		for _, f := range failures {
			fmt.Printf("- %s\n", f)
		}
		os.Exit(1)
	}

	fmt.Println("Release gate passed.")
	fmt.Printf("- Task success rate: %.2f%%\n", report.Thresholds.TaskSuccessRate*100)
	fmt.Printf("- Tool correctness rate: %.2f%%\n", report.Thresholds.ToolCorrectnessRate*100)
	fmt.Printf("- Safety violation rate: %.2f%%\n", report.Thresholds.SafetyViolationRate*100)
	fmt.Printf("- Latency p50/p95: %.2fms / %.2fms\n", report.Thresholds.LatencyP50Ms, report.Thresholds.LatencyP95Ms)
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

func loadReport(path string) (BenchmarkReport, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return BenchmarkReport{}, err
	}
	var report BenchmarkReport
	if err := json.Unmarshal(raw, &report); err != nil {
		return BenchmarkReport{}, err
	}
	return report, nil
}

func exitErr(err error) {
	fmt.Fprintln(os.Stderr, strings.TrimSpace(err.Error()))
	os.Exit(1)
}
