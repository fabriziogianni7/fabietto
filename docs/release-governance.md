# Release Governance and Quality Gates

Phase 5 introduces benchmark-backed release checks with explicit threshold gates.

## Release checklist

Use this checklist for every release candidate:

- [ ] `make ci` passes (format, vet, test, eval)
- [ ] `make benchmark` generates updated benchmark artifacts
- [ ] `make release-gate` passes against configured thresholds
- [ ] Any gate failure has an explicit waive decision recorded in release notes
- [ ] Changelog includes **Agent Quality Impact** section

## Fail / waive policy

- A failed release gate is a blocking failure by default.
- A waive is allowed only when:
  - a known regression has an approved mitigation plan, and
  - owner + due date for follow-up are documented.
- Every waive must be documented in release notes under **Agent Quality Impact**.

## Gate dimensions

Thresholds are configured in `benchmarks/config.json`:

- Task success rate
- Tool correctness rate
- Safety violation rate
- Latency p50
- Latency p95

These thresholds are evaluated by `scripts/release-gate.sh` (or `make release-gate`).

## Changelog template: Agent Quality Impact

Use this section in release notes:

```markdown
### Agent Quality Impact

- Benchmark timestamp: `<RFC3339 timestamp>`
- Task success rate: `<value>%` (threshold: `>= <value>%`)
- Tool correctness rate: `<value>%` (threshold: `>= <value>%`)
- Safety violation rate: `<value>%` (threshold: `<= <value>%`)
- Latency p50/p95: `<value>ms / <value>ms` (thresholds: `<= <p50>ms`, `<= <p95>ms`)
- Gate status: `pass|waived`
- If waived: `<reason>`, owner `<name>`, remediation due `<date>`
```
