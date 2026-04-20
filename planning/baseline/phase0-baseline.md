# Phase 0 Baseline Metrics (Initial Snapshot)

This report captures the initial quality baseline requested in Phase 0 and is intended to be updated with real values as measurements are collected.

## Metadata

- Date captured: `TBD`
- Commit SHA: `TBD`
- Captured by: `TBD`
- Environment notes: `TBD` (for example, local machine or CI runner details)

## Commands Used

Run from repository root:

```bash
make ci
go test ./...
go test -json ./... > planning/baseline/test-report.json
go test -json ./... | go run ./cmd/testdurations  # Optional helper if one is added later
```

If no duration helper exists, package-level durations can be derived from `go test -json` output with a small parser script in a follow-up task.

## Snapshot

### Test pass/fail

- Status: `TBD` (pass/fail)
- Notes: `TBD`

### Package-level test durations

| Package | Duration (s) | Notes |
| --- | ---: | --- |
| `TBD` | `TBD` | `TBD` |

### Existing latency metrics availability

- Availability: `TBD` (available/not available/partial)
- Source(s): `TBD` (for example benchmark files, runtime logs, tracing)
- Notes: `TBD`

## Follow-ups

- Add automation for extracting package-level durations from `go test -json`.
- Define where latency metrics should live (benchmarks, tracing, or runtime telemetry).
