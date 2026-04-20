package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"

	"custom-agent/eval"
)

func main() {
	var (
		casesDir = flag.String("cases", "eval/cases", "Directory containing eval case JSON files")
		outDir   = flag.String("out-dir", "eval/results", "Directory where eval report JSON will be written")
		backend  = flag.String("backend", "fake", "Eval backend mode (default: fake)")
		outFile  = flag.String("out-file", "report.json", "Eval report filename")
	)
	flag.Parse()

	cases, err := eval.LoadCases(*casesDir)
	if err != nil {
		exitErr(fmt.Errorf("failed to load cases: %w", err))
	}
	if len(cases) == 0 {
		exitErr(fmt.Errorf("no eval cases found in %s", *casesDir))
	}

	runner := eval.NewRunner(*backend)
	report := runner.RunCases(cases)

	if err := os.MkdirAll(*outDir, 0755); err != nil {
		exitErr(fmt.Errorf("failed to create output directory: %w", err))
	}
	outputPath := filepath.Join(*outDir, *outFile)
	data, err := json.MarshalIndent(report, "", "  ")
	if err != nil {
		exitErr(fmt.Errorf("failed to encode report: %w", err))
	}
	if err := os.WriteFile(outputPath, data, 0644); err != nil {
		exitErr(fmt.Errorf("failed to write report: %w", err))
	}

	fmt.Printf("Eval report written: %s\n", outputPath)
	fmt.Printf("Summary: %d total, %d passed, %d failed\n",
		report.Summary.CasesTotal, report.Summary.CasesPassed, report.Summary.CasesFailed)
	for _, c := range report.Summary.CategoryStats {
		fmt.Printf("- %s: %d/%d passed\n", c.Category, c.Passed, c.Total)
	}

	if report.Summary.CasesFailed > 0 {
		os.Exit(1)
	}
}

func exitErr(err error) {
	fmt.Fprintln(os.Stderr, err)
	os.Exit(1)
}
