# Evaluation Harness Schema (v1)

Eval cases are deterministic JSON fixtures consumed by `go run ./cmd/eval`.

## Case format

```json
{
  "schema_version": "v1",
  "id": "tool.parse.basic",
  "category": "tool-calling-correctness",
  "description": "Parses explicit tool XML-like call content",
  "backend": "fake",
  "operation": {
    "type": "tool_parse",
    "content": "<function=read_file>{\"path\":\"README.md\"}</function>"
  },
  "assertions": [
    { "type": "equals", "key": "ok", "value": true },
    { "type": "equals", "key": "tool_name", "value": "read_file" }
  ]
}
```

## Operation types

- `tool_execute`: runs a tool via `tools.ExecuteTool`.
  - Fields: `tool_name`, `tool_args`, optional `isolate_fs`.
- `tool_parse`: validates fallback parser behavior.
  - Fields: `content`.
- `memory_search`: seeds memory store and performs deterministic search.
  - Fields: `memories`, `query`, `limit`, `embedder_mode` (`disabled|deterministic|fail`).
- `compaction_estimate`: evaluates token estimate behavior.
  - Fields: `messages`.
- `compaction_prompt_block`: renders compacted context prompt block.
  - Fields: `context`.
- `compaction_merge`: verifies merge semantics.
  - Fields: `context`, `merge`.
- `session_recent`: validates recent-window truncation.
  - Fields: `messages`.

## Assertion types

Assertions are property-based to reduce nondeterminism flakiness:

- `equals`: exact normalized value equality
- `contains`: case-insensitive substring check
- `not_contains`: negative substring check
- `gte`: numeric greater-than-or-equal
- `lte`: numeric less-than-or-equal

`key` supports dotted lookups for nested result objects.

## Runner output

`cmd/eval` writes machine-readable JSON:

- summary: backend, total/passed/failed, per-category stats
- results: per-case pass/fail, assertions, facts, error

Default output path: `eval/results/report.json`.
