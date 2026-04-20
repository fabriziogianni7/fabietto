# [Phase 4] Reliability and regression depth

## Summary
Expand test depth with integration and failure-injection coverage for the most critical agent workflows.

## Why this matters
- Unit tests are not enough to validate emergent behavior in multi-round tool loops.
- Queueing, memory fallback, and gateway interactions can fail in ways that only appear in end-to-end execution.

## Scope
- Add integration tests for representative user flows:
  - Multi-round tool use with final response
  - Approval-required command denial/approval/execute cycle
  - Wallet approval path (if wallet enabled)
- Add failure-injection tests:
  - Embedding service unavailable
  - Compaction returns empty/invalid payload
  - Queue worker panic and recovery behavior
- Add deterministic fixtures for session/memory/reminder stores.

## Out of scope
- Distributed load testing infrastructure.
- Large-scale benchmark trend analysis (Phase 5).

## Deliverables
- Integration test suite executable via `go test` and CI.
- Failure-mode regression tests for known fragile paths.
- Test documentation for local and CI execution.

## Acceptance criteria
- Critical workflows have integration coverage with deterministic assertions.
- Known failure paths produce expected fallback behavior and logs.
- Test flake rate is low and documented if non-zero.

## Dependencies
- Requires Phase 1 safety controls and Phase 3 instrumentation to maximize diagnostic value.

## Suggested labels
- `type:testing`
- `area:agent-loop`
- `priority:P1`

## Suggested milestone
- `Agent Harness v1`

## Task checklist
- [ ] Create integration test harness utilities and shared fixtures.
- [ ] Implement high-value scenario tests for tool loop and approvals.
- [ ] Implement failure-injection tests for memory/compaction/queue.
- [ ] Add CI job partitioning if runtime is too long.
- [ ] Document expected runtime and stability targets.
