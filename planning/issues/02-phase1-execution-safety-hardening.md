# Phase 1: Execution Safety Hardening

## Summary
Harden tool execution boundaries so prompt injection or model mistakes cannot cause broad filesystem or shell side effects.

## Problem
Current safety controls are primarily policy/denylist based. The harness needs stronger enforcement boundaries for file access and command execution.

## Scope
- Add workspace path jail for file tools.
- Replace broad shell execution with allowlist-based command policy.
- Expand approval checks and bypass-resistance tests.

## Deliverables
- Path normalization + workspace-root guard for read/write tools.
- Command execution adapter that validates binary + args before execution.
- Security regression tests covering blocked patterns and obfuscation attempts.
- Updated docs describing allowed command surface and approval flow.

## Out of scope
- Full container or VM sandbox runtime (tracked in later phase if needed).

## Tasks
- [ ] Define `allowedRoots` policy (workspace default, optional explicit extensions).
- [ ] Refactor file tool path resolution to reject out-of-root traversal.
- [ ] Introduce command parser/validator and allowlist (e.g., `ls`, `pwd`, `go test`, etc. configurable).
- [ ] Ensure command approvals are tied to normalized command identity.
- [ ] Add tests for traversal, shell metacharacter tricks, chained commands, and approval replay edge cases.
- [ ] Document policy configuration in README.

## Acceptance criteria
- Any path outside allowed roots is rejected with a clear error.
- Any non-allowlisted command is denied or requires explicit approval based on policy.
- Security test suite passes in CI.

## Dependencies
- Phase 0 CI gate must exist first.

## Risks
- Over-restrictive policies may block legitimate workflows.
- Mitigation: configurable allowlist + explicit approval override with audit trail.

## Suggested labels
`harness`, `security`, `tools`, `phase-1`
