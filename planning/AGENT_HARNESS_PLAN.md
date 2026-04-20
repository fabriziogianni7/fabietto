# Agent Harness Improvement Plan

## Goal
Turn the current agent runtime into a measurable, safe, and scalable harness for improving agent quality over time.

## Success criteria
- Every change is validated by CI and a repeatable eval suite.
- Tool execution is constrained by explicit security boundaries.
- Regressions are detectable through tests, telemetry, and benchmark reports.
- Operations teams can debug failures from structured run artifacts.

## Phases

### Phase 0: Baseline and guardrails
**Objective:** Establish immediate engineering safety nets before deeper changes.

**Deliverables**
- CI workflow for format, vet, and tests.
- Baseline quality metrics snapshot (test pass rate, median tool latency where available).
- Contributor guidance for running checks locally.

**Exit criteria**
- Pull requests are blocked on CI failures.
- Local `make`/script workflow exists for the same checks as CI.

---

### Phase 1: Execution safety hardening
**Objective:** Reduce blast radius of tool misuse and prompt-injection-driven side effects.

**Deliverables**
- Workspace path jail for file tools.
- Allowlist-based command execution policy.
- Test suite for approval/deny paths and policy bypass attempts.

**Exit criteria**
- Disallowed command and path access attempts are consistently rejected.
- Security-related behavior has regression tests.

---

### Phase 2: Evaluation harness v1
**Objective:** Measure agent quality with deterministic, repeatable scenarios.

**Deliverables**
- Eval case schema (prompt, context, expected outcomes).
- Initial eval corpus focused on tool-calling, safety, and failure handling.
- CI eval job with machine-readable output.

**Exit criteria**
- A quality score is produced on every PR.
- Regressions are visible in CI output and easy to diff.

---

### Phase 3: Observability and run artifacts
**Objective:** Make behavior explainable and debuggable at turn/tool granularity.

**Deliverables**
- Structured JSON events for turns, tool calls, and failures.
- Metrics for latency/error rates by tool and gateway.
- Persisted run artifacts (transcript + tool summary + outcome metadata).

**Exit criteria**
- A failing session can be diagnosed from logs/artifacts without reproducing manually.
- Baseline dashboards or report scripts are available.

---

### Phase 4: Reliability and regression depth
**Objective:** Expand confidence through integration and chaos-style failure tests.

**Deliverables**
- End-to-end harness tests for multi-round workflows.
- Tests for memory/compaction failure and fallback behavior.
- Soak/retry/failure-injection tests for queueing and gateway paths.

**Exit criteria**
- Critical flows are covered by integration tests.
- Known flaky paths have deterministic repro tests.

---

### Phase 5: Benchmarking and release governance
**Objective:** Institutionalize improvement loops and release quality gates.

**Deliverables**
- Benchmark suite and periodic trend reports.
- Release checklist with eval/latency/safety thresholds.
- Change log format documenting quality impact per release.

**Exit criteria**
- Releases are gated on explicit quality thresholds.
- Performance and quality trends are tracked over time.

## Dependency order
1. Phase 0 is mandatory before all other phases.
2. Phase 1 and Phase 2 can run in parallel after Phase 0.
3. Phase 3 depends on initial schema decisions from Phase 2.
4. Phase 4 depends on Phase 1 and Phase 3 instrumentation.
5. Phase 5 depends on prior phases producing stable signals.

## Risks and mitigations
- **Risk:** Eval flakiness due to model nondeterminism.  
  **Mitigation:** Assert behavioral properties, not exact text, and capture seeds/metadata where possible.
- **Risk:** Safety controls block valid developer workflows.  
  **Mitigation:** Add explicit override mechanisms tied to approvals and audit logs.
- **Risk:** Instrumentation overhead harms latency.  
  **Mitigation:** Make high-cardinality diagnostics configurable and sampled.
