## Title
Epic: Agent Harness Hardening Program (Phases 0-5)

## Summary
Coordinate the full initiative to evolve this repository from a working agent runtime into a robust agent harness with CI gates, safety boundaries, evals, observability, reliability testing, and release governance.

## Why
Current architecture has strong core runtime pieces, but lacks the quality system required to improve agent performance safely and repeatedly.

## Scope
- Drive delivery and sequencing for phases 0 through 5.
- Track cross-phase dependencies and risks.
- Standardize definitions of done and quality thresholds.

## Out of Scope
- Individual implementation details for each phase ticket (covered by child issues).

## Milestones
- M1: Phase 0 complete
- M2: Phases 1 and 2 complete
- M3: Phases 3 and 4 complete
- M4: Phase 5 complete

## Child Issues
- [ ] Phase 0: Baseline and guardrails
- [ ] Phase 1: Execution safety hardening
- [ ] Phase 2: Evaluation harness v1
- [ ] Phase 3: Observability and run artifacts
- [ ] Phase 4: Reliability and regression depth
- [ ] Phase 5: Benchmarking and release governance

## Program-level Acceptance Criteria
- All phase issues are delivered and closed.
- Every PR is gated by CI with deterministic checks.
- Agent quality can be measured and compared release-over-release.
- Safety and execution boundaries are validated by automated tests.

## Suggested Labels
- `epic`
- `agent-harness`
- `program`
