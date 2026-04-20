## Title
Phase 2: Build evaluation harness v1 (deterministic scenarios + CI scoring)

## Summary
Introduce a first-class evaluation harness so quality is measurable and regressions are detectable. This phase defines the eval case format, adds an initial corpus of scenarios, and runs eval scoring in CI on every PR.

## Problem
Current development lacks a repeatable quality measurement system. Improvements are difficult to compare objectively, and regressions can ship unnoticed.

## Scope
- In scope:
  - Eval case schema (JSON or YAML) capturing:
    - input prompt
    - optional setup context
    - expected behavioral assertions (tool usage, safety constraints, output properties)
  - Eval runner command (e.g. `go run ./cmd/eval` or `go test` package-based harness)
  - Initial corpus (at least 20 scenarios) across:
    - tool-calling correctness
    - command safety boundaries
    - memory retrieval and fallback behavior
    - compaction-sensitive conversation flow
  - Machine-readable output (JSON summary + per-case result)
  - CI integration to run eval suite and surface failures in PR checks
- Out of scope:
  - Full benchmark dashboards (Phase 5)
  - Large-scale production traffic replay

## Deliverables
- Eval schema docs and examples.
- Eval runner with deterministic execution hooks where possible.
- Initial eval dataset committed to repo.
- CI workflow/job for eval scoring.
- Developer docs for running eval locally.

## Acceptance criteria
- Running a single command locally executes all evals and produces a summary report.
- CI fails when any required eval assertion fails.
- Results include enough detail to understand which behavior regressed.
- Eval corpus covers key safety and tool-routing behaviors.

## Dependencies
- Requires Phase 0 CI baseline.
- Coordinates with Phase 1 for safety-specific assertions.

## Suggested labels
- `type:enhancement`
- `area:evals`
- `priority:P0`

## Suggested milestone
Agent Harness v1

## Implementation checklist
- [ ] Define eval case schema and fixtures directory structure.
- [ ] Implement eval runner and assertion engine.
- [ ] Add at least 20 initial scenarios with expected assertions.
- [ ] Add CI job and artifact upload for eval results.
- [ ] Document workflow in README or docs.

## Risks / notes
- Model nondeterminism can cause flaky assertions. Favor behavioral/property assertions over exact string matching.
- Keep seed and metadata where feasible to aid reproducibility.
