# [Phase 5] Benchmarking and release governance

## Summary
Institutionalize the agent improvement loop with trend reporting and release gates based on quality, safety, and latency thresholds.

## Why this matters
- Improvement requires stable measurement and release discipline.
- Teams need explicit ship criteria tied to benchmark outcomes.

## Scope
### In scope
- Benchmark suite runner and reporting format.
- Release checklist with gating thresholds.
- Changelog template for quality-impact reporting.

### Out of scope
- Full product analytics platform.
- Replacing existing release tooling beyond harness gates.

## Deliverables
- `benchmarks/` runner and configuration.
- Trend report artifact (JSON + markdown summary).
- Release gate checklist and documentation.

## Implementation tasks
- [ ] Define benchmark dimensions:
  - Task success rate
  - Tool correctness rate
  - Safety violation rate
  - p50/p95 latency
- [ ] Build benchmark runner over eval corpus + integration tasks.
- [ ] Generate trend artifacts per run (machine + human readable).
- [ ] Add release gate script checking thresholds.
- [ ] Add release checklist to docs/README with fail/waive policy.
- [ ] Add changelog section template: "Agent Quality Impact".

## Acceptance criteria
- [ ] Benchmarks run on demand and in CI release pipeline.
- [ ] Threshold failures clearly block release unless explicitly waived.
- [ ] Trend reports are retained and comparable between runs.

## Dependencies
- Phase 2 eval outputs.
- Phase 3 telemetry outputs.
- Phase 4 reliability tests.

## Risks
- Thresholds may be unrealistic initially.
  - Mitigation: start with baseline-derived thresholds and tighten iteratively.

## Suggested labels
- `type:enhancement`
- `area:benchmarking`
- `area:release`
- `priority:p2`

## Suggested milestone
- `Harness Hardening v1`
