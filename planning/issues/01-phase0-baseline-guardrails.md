# [Phase 0] Baseline and engineering guardrails

## Summary
Create immediate safety nets so future harness work is validated automatically and consistently.

## Why
The project currently lacks CI workflow enforcement. This makes regressions likely and slows iteration quality.

## Scope
- Add GitHub Actions workflow for:
  - `gofmt -l`
  - `go vet ./...`
  - `go test ./...`
- Add a local helper (`Makefile` targets or script) that mirrors CI checks.
- Document contribution workflow in README.
- Capture a baseline quality snapshot:
  - Test pass/fail
  - Package-level test durations
  - Any existing latency metrics availability

## Out of scope
- Full eval harness
- Benchmark dashboards
- Security sandbox implementation

## Deliverables
- `.github/workflows/ci.yml`
- Local check command (`make ci` or equivalent)
- README update for contributor checks
- Baseline metrics artifact or markdown report

## Acceptance criteria
- CI runs on push and PR.
- CI is required for merge (branch protection setup documented if not automated).
- Running the local command produces equivalent results to CI.

## Dependencies
- None

## Risks
- CI runtime may be noisy initially due to flaky tests.
- Mitigation: mark flaky tests and open follow-up issues immediately.

## Suggested labels
`phase:0`, `type:infrastructure`, `area:ci`, `priority:P0`

## Milestone
`Agent Harness Hardening`
