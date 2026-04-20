# Phase 3: Observability and run artifacts

## Summary
Instrument the harness so behavior is inspectable at turn and tool granularity, and preserve machine-readable artifacts for debugging and analysis.

## Problem
Current logging is mostly unstructured. Regressions and production failures are difficult to investigate without rerunning scenarios manually.

## Scope
- Introduce structured event schema for:
  - agent turn start/end
  - tool call start/end
  - tool error categories
  - compaction/memory retrieval decisions
- Add metrics counters/histograms for:
  - turn latency
  - tool latency by tool name
  - tool failure rate
  - fallback parser usage
- Persist run artifacts:
  - normalized transcript
  - tool execution summary
  - run metadata (session, gateway, model, timestamps)
- Add docs for artifact format and retention policy.

## Deliverables
- `telemetry/` package (or equivalent) with event + metric helpers.
- Structured logs emitted from core loop and tool executor.
- Artifact writer module with configurable output path.
- Developer docs for local troubleshooting using artifacts.

## Acceptance criteria
- A failed run can be diagnosed from artifacts/logs without reproducing interactively.
- Telemetry includes stable machine-readable keys suitable for dashboards.
- Feature flag/config controls verbosity and artifact retention.

## Dependencies
- Depends on Phase 2 schema decisions for eval/run metadata compatibility.

## Risks
- Logging volume and overhead increase.
- Inconsistent field names without schema governance.

## Suggested labels
- `type:enhancement`
- `area:observability`
- `area:telemetry`
- `priority:p1`

## Milestone
Agent Harness v1

