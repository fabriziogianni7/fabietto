# Custom Agent

A Go-based AI agent that responds to messages via multiple gateways (Telegram, Discord, HTTP, Signal). Uses Groq's Llama model with tools for file access, web search, memory, reminders, sub-agents, and optional EVM wallet operations. Step one of building an OpenClaw-like agent.

---

## Index

- [Run locally](#run-locally)
- [Setup](#setup)
- [Tools](#tools)
- [Personality](#personality)
- [Gateways](#gateways)
- [Context compaction](#context-compaction)
- [Long-term memory & embeddings](#long-term-memory--embeddings)
- [Observability & run artifacts](#observability--run-artifacts)
- [Evaluation harness](#evaluation-harness)
- [Benchmarking & release governance](#benchmarking--release-governance)
- [Wallet](#wallet)
- [Skills](#skills)
- [Contributing](#contributing)
- [Project structure](#project-structure)

---

## Run locally

### Prerequisites

- Go 1.21+
- API keys: Groq, Brave Search, and at least one gateway (e.g. Telegram)

### Quick start

```bash
# 1. Clone and enter the project
cd custom-agent

# 2. Install dependencies
go mod tidy

# 3. Configure environment
cp .env.example .env
# Edit .env with your TELEGRAM_BOT_TOKEN, GROQ_API_KEY, BRAVE_SEARCH_API_KEY

# 4. Run
go run .
```

Config is loaded from `.env` (if present) and validated at startup. At least one gateway must be configured.

### Test

Message your bot on Telegram (or your configured gateway). It will respond using Groq's Llama 3.1 8B model. Conversation history is stored per user in `sessions/` (JSONL files), so the bot remembers context—e.g. "what did I say earlier?" works.

**Commands:** Send `/new` to clear your session. Send `newSkill` to add a new skill interactively.

---

## Setup

### API keys

1. **Telegram**: Message [@BotFather](https://t.me/BotFather), send `/newbot`, copy the token
2. **Groq**: Sign up at [console.groq.com](https://console.groq.com), create an API key
3. **Brave Search**: Get a key at [brave.com/search/api](https://brave.com/search/api) (free tier available)

### Configure

Copy the example env file and fill in your tokens:

```bash
cp .env.example .env
# Edit .env with TELEGRAM_BOT_TOKEN, GROQ_API_KEY, BRAVE_SEARCH_API_KEY
```

Or export: `export TELEGRAM_BOT_TOKEN=...`, `export GROQ_API_KEY=...`, `export BRAVE_SEARCH_API_KEY=...`

Then follow [Run locally](#run-locally) to start the bot.

---

## Tools

The bot can use tools when the LLM decides they're helpful:

| Tool | Description |
|------|-------------|
| `run_command` | Run a command under strict execution policy. Shell features and metacharacters (`;`, `|`, `&&`, redirects, subshells, globs, etc.) are rejected. Executables must be allowlisted. Read-only commands (`ls`, `pwd`, `whoami`, `date`, `id`, `head`, `tail`, `wc`, `file`, `rg`) run immediately; selected developer commands (`go`, `git`, `make`) require prior approval (`approve: <command>` or `/approve <command>`). Approvals are normalized and persisted in `exec-approvals.json`. |
| `read_file` | Read a file from the filesystem, restricted to allowed workspace roots (defaults to repository root) |
| `write_file` | Write content to a file, restricted to allowed workspace roots (defaults to repository root) |
| `web_search` | Search the web (Brave Search API) |
| `save_memory` | Save a fact or preference to long-term memory (survives `/new`) |
| `read_memory` | Search long-term memory (semantic search when Ollama is available, keyword fallback otherwise) |
| `create_scheduled_reminder` | Schedule a reminder (cron expression). Messages are sent via the configured gateway. |
| `list_reminders` | List scheduled reminders |
| `delete_reminder` | Delete a reminder by ID |
| `spawn_subagents` | Run parallel stateless sub-agents for research or multi-step tasks (read-only tools) |
| `http_request` | Make HTTP requests to URLs. When wallet is enabled, automatically pays for x402-protected APIs (402 Payment Required). |
| `wallet_get_balance` | Get native token balance (when wallet enabled) |
| `wallet_execute_transfer` | Send native token (when wallet enabled) |
| `wallet_execute_contract_call` | Call a smart contract (when wallet enabled) |
| `wallet_list_transactions` | List recent agent-initiated transactions (when wallet enabled) |
| `list_skills` | List available skills (name + description) |
| `read_skill` | Read full skill content by name |
| `read_skill_script` | Read a script file within a skill |
| `write_skill` | Persist a new skill (after security/feasibility checks) |

The agent loop runs until the LLM returns a final text response or hits the tool limit (10 rounds). Add or modify tools in `tools/tools.go`.

### Tool safety boundaries

- **Workspace jail for file tools**: `read_file` and `write_file` only operate within configured workspace roots. By default this is the current Git repository root. Paths outside the jail (including traversal attempts like `../..`) are rejected.
- **Optional roots override**: set `TOOL_WORKSPACE_ROOTS` to a colon-separated list of absolute/relative roots to allow (example: `TOOL_WORKSPACE_ROOTS=.:/tmp/scratch`).
- **Command execution policy**: `run_command` does **not** run through `sh -c`. Commands are tokenized and executed directly after policy checks.
- **No shell chaining/expansion**: multiline/chaining and shell metacharacters are denied to prevent policy bypass via formatting tricks.
- **Normalized approvals**: approvals are matched against a normalized command identity (trimmed/lowercased executable + normalized whitespace args), so replay attempts with spacing/casing variants do not bypass controls.

---

## Personality

Edit `PERSONALITY.md` to define the bot's persona. Its contents are injected as the system prompt at startup. Change the tone, style, or add rules—the bot will adopt whatever you write.

---

## Gateways

The bot supports multiple platforms. Enable any combination:

| Gateway | Env vars | Description |
|---------|----------|-------------|
| Telegram | `TELEGRAM_BOT_TOKEN` | Telegram bot |
| Discord | `DISCORD_BOT_TOKEN` | Discord bot |
| HTTP | `HTTP_PORT` | REST API at `POST /chat` with `{"user_id":"x","message":"y"}` |
| Signal | `SIGNAL_CLI_URL`, `SIGNAL_NUMBER` | Signal via [signal-cli-rest-api](https://github.com/bbernhard/signal-cli-rest-api) (run separately, e.g. Docker) |

At least one gateway must be configured.

### Signal setup

Signal requires [signal-cli-rest-api](https://github.com/bbernhard/signal-cli-rest-api) running separately:

```bash
# Run signal-cli-rest-api (Docker)
docker run -p 8080:8080 -v $(pwd)/signal-cli-config:/home/.local/share/signal-cli bbernhard/signal-cli-rest-api

# Register your number (one-time)
curl -X POST "http://localhost:8080/v2/register/+1234567890"

# Verify with code sent via SMS
curl -X POST "http://localhost:8080/v2/register/+1234567890/verify/CODE"
```

Then set `SIGNAL_CLI_URL=http://localhost:8080` and `SIGNAL_NUMBER=+1234567890` in `.env`.

---

## Context Compaction

When conversation history exceeds ~4000 tokens, the agent uses **structured summarization** to compact old context. A JSON summary is produced with sections: `session_intent`, `key_decisions`, `key_facts`, `file_modifications`, `pending_actions`, `artifacts`, `momentum`, `tool_results_summary`. Only recent messages stay in full; older context is replaced by this structured block.

Set `CONTEXT_COMPACTION_THRESHOLD` (default 4000) to tune when compaction triggers.

---

## Long-term Memory & Embeddings

The bot has **persistent memory** that survives session resets. Use `save_memory` and `read_memory` tools.

**Semantic search** (optional): With [Ollama](https://ollama.ai) running, embeddings enable semantic search—e.g. "favorite pasta" matches "User loves carbonara". Without Ollama, keyword search is used.

```bash
# Install Ollama, then: ollama pull nomic-embed-text
# Add to .env: OLLAMA_URL=http://localhost:11434
```

Embeddings are **lazy** (only used when needed) and **cached** (stored with memories). If Ollama is unavailable, the bot falls back to keyword search.

---

## Observability & run artifacts

Phase 3 telemetry is enabled by default and writes:

- **Structured events** (JSON in logs) for:
  - `agent_turn_start` / `agent_turn_end`
  - `tool_call_start` / `tool_call_end`
  - tool error categories (`unknown_tool`, `invalid_arguments`, `permission_denied`, `timeout`, `not_configured`, `execution_error`)
  - `compaction_decision`
  - `memory_retrieval_decision`
- **In-process metrics** (machine-readable JSON artifact) with counters/timers:
  - turn count + turn latency
  - tool calls, per-tool latency, per-tool error counts
  - fallback parser usage
- **Run artifacts** per turn/run:
  - `metadata.json` (session/gateway/model/timestamps)
  - `transcript.json` (normalized transcript with event kinds)
  - `tool_summary.json` (tool call outcome/duration/error category)
  - `metrics.json` (counter/timer snapshot)

Artifacts are written to `run-artifacts/<run_id>/` by default.

### Telemetry env flags

| Env var | Default | Description |
|---------|---------|-------------|
| `TELEMETRY_ENABLED` | `true` | Master telemetry on/off switch |
| `TELEMETRY_VERBOSITY` | `basic` | `basic` or `debug` (debug includes truncated tool args in events) |
| `TELEMETRY_METRICS_ENABLED` | `true` | Enable in-process metric collection |
| `RUN_ARTIFACTS_ENABLED` | `true` | Enable writing run artifacts |
| `RUN_ARTIFACTS_DIR` | `run-artifacts` | Artifact output directory |
| `RUN_ARTIFACT_RETENTION_DAYS` | `7` | Retention window; older run directories are pruned |

### Retention policy

On each artifact write, the runtime prunes run directories older than `RUN_ARTIFACT_RETENTION_DAYS` using directory mtime. Set a larger value for longer forensic retention, or disable artifacts entirely with `RUN_ARTIFACTS_ENABLED=false`.

---

## Evaluation harness

This repo includes a deterministic evaluation harness for regression checks.

- **Run locally**: `make eval`
- **Output JSON report**: `eval/results/report.json`
- **Case corpus**: `eval/cases/*.json`
- **Schema docs**: `docs/eval-schema.md`

You can also run directly:

```bash
go run ./cmd/eval -cases eval/cases -out-dir eval/results -backend fake
```

The runner exits non-zero if any case fails, and writes:
- summary (total/passed/failed + per-category stats)
- per-case facts/assertions/results

`make ci` now includes eval execution as part of local parity with CI.

---

## Benchmarking & release governance

Phase 5 adds benchmark trend reporting and a release quality gate:

- **Run benchmark suite**: `make benchmark`
- **Run release gate**: `make release-gate`
- **Benchmark artifacts**:
  - `benchmarks/results/latest.json` (current run metrics; machine-readable)
  - `benchmarks/results/trend.json` (rolling trend history; machine-readable)
  - `benchmarks/results/summary.md` (human-readable summary)
- **Gate thresholds config**: `benchmarks/config.json`
- **Release process docs/checklist/template**: `docs/release-governance.md`

Benchmarks reuse the deterministic eval corpus and compute:
- task success rate
- tool correctness rate
- safety violation rate
- latency p50/p95 (from eval case timings)

CI runs benchmark + release gate and uploads benchmark artifacts.

---

## Wallet

Optional EVM wallet support. When `EVM_RPC_URL` and `WALLET_PRIVATE_KEY` (or signer backend) are set, wallet tools are enabled.

| Env var | Description |
|---------|-------------|
| `EVM_RPC_URL` | RPC endpoint (e.g. Alchemy, Infura) |
| `WALLET_PRIVATE_KEY` | 0x-prefixed private key (or use `WALLET_SIGNER_BACKEND` for KMS/HSM) |
| `CHAIN_ID` | Default chain (e.g. 1 for Ethereum) |
| `WALLET_NATIVE_SPEND_LIMIT` | Wei string; transactions above this require user approval |
| `WALLET_CHAINS` | JSON array for multichain: `[{"chain_id":1,"rpc_url":"...","explorer":"...","name":"Ethereum"}]` |

See `WALLET.md` for tool usage. Transactions above the spend limit trigger a notification; the user must reply `approve: tx_<id>` to execute.

**x402 buyer:** When the wallet is enabled (env backend), the `http_request` tool can automatically pay for APIs that return 402 Payment Required. The agent uses the same wallet to sign x402 payment payloads.

### Planner (plan-and-execute)

Pipeline for **on-chain contract interactions** (not plain native ETH sends). When active, the agent runs: structured `to` / `data` / `value_wei` from an LLM → validate → policy preview → **eth_call** simulation → broadcast → optional receipt check.

| Env var | Description |
|---------|-------------|
| `PLANNER_MODE` | `off` \| `wallet` \| `auto` \| `always_wallet`. **If unset:** with wallet configured defaults to **`auto`**; otherwise `off`. Legacy `PLANNER_ENABLED=true` maps to `wallet`; `false` maps to `off`. |
| `PLANNER_CAPABILITIES` | Comma-separated; use `wallet`. If omitted while planner is on, defaults to `wallet`. |

- **`auto`**: broad heuristics (DeFi keywords, `0x` addresses, etc.) trigger the planner; short or ambiguous messages may use a small router LLM turn.
- **`always_wallet`**: try the wallet planner on every message (except the simple native-send path).
- **`wallet`**: original narrow keywords only (swap / contract-call phrasing).

Telemetry emits `plan_*` events when telemetry is on. Native ETH sends (`wallet_execute_transfer` path) still use the reactive tool loop.

### Orchestration (multi-tool planner)

Optional **general** plan-and-execute layer that runs **before** the normal tool loop when enabled. It asks the model for a JSON plan (`goal` + ordered `steps` with per-step `allowed_tools`), validates tool names and dependencies, then runs **one micro-agent per step** (capped tool rounds) with only that step’s tools. Step outputs feed later steps. A final synthesis summarizes for the user. If planning or execution fails, the agent falls back to the usual reactive loop.

`wallet_execute_contract_call` inside orchestration uses the same preview → simulate → send pipeline as the wallet planner when possible.

| Env var | Description |
|---------|-------------|
| `ORCHESTRATION_MODE` | `off` (default) \| `auto` \| `always`. **`auto`**: multi-step cues (e.g. “then”, “first…then”), long messages, or a small router LLM. **`always`**: try orchestration on every non-trivial message. |

| Mode | Behavior |
|------|----------|
| `off` | Only the wallet planner (if any) + reactive loop. |
| `auto` | Orchestration when heuristics or router suggest multi-step work. |
| `always` | Orchestration first; fallback to reactive on failure. |

Wallet-only `tryWalletPlanner` is **skipped** when orchestration is not `off`, to avoid double-planning.

---

## Skills

User-installed skills extend the agent with new capabilities. Each skill is a directory with `SKILL.md` (YAML frontmatter + Markdown instructions) and optional Python or shell scripts.

| Env var | Description |
|---------|-------------|
| `SKILLS_DIR` | Skills root directory (default `./skills-data`). Separate from the `skills/` package source. |

**Adding skills:**
1. **Manually**: Create `skills-data/<name>/SKILL.md` (or `$SKILLS_DIR/<name>/SKILL.md`) with frontmatter and body.
2. **Via chat**: Send `newSkill` or `/newSkill`, then paste your SKILL.md when prompted. The agent runs security and feasibility checks before saving.

**Tools:** `list_skills`, `read_skill`, `read_skill_script` let the agent discover and use skills. Only short descriptions are injected into the system prompt; full content is fetched on demand.

See `skills/README.md` for format and script language policy.

---

## Contributing

- **Run CI checks locally**: `make ci`
  - `make fmt-check` verifies formatting (`gofmt -l .`)
  - `make vet` runs static checks (`go vet ./...`)
  - `make test` runs unit tests (`go test ./...`)
  - `make benchmark` writes benchmark trend artifacts
  - `make release-gate` enforces release thresholds
  - Reliability/regression-depth suite only: `go test ./agent ./tools ./memory ./compaction ./sessionqueue`
- **Add tools**: Define and implement in `tools/tools.go`; register in the tool set passed to the agent
- **Add gateways**: Implement the `gateway.Gateway` interface in `gateway/` and wire it in `main.go`
- **Code style**: Standard Go formatting (`gofmt`). Keep packages focused; wallet, reminders, and compaction are modular

---

## Project Structure

```
custom-agent/
├── cmd/
│   ├── benchmark/
│   │   └── main.go        # benchmark runner + trend report generator
│   ├── eval/
│   │   └── main.go        # eval runner entrypoint
│   └── releasegate/
│       └── main.go        # release threshold gate checker
├── benchmarks/
│   ├── config.json        # benchmark dimensions + release thresholds
│   └── results/           # benchmark outputs (latest.json/trend.json/summary.md)
├── eval/
│   ├── runner.go          # deterministic eval execution + assertions
│   ├── schema.go          # eval case/result schema
│   ├── cases/             # deterministic eval corpus (JSON fixtures)
│   └── results/           # local eval outputs (report.json)
├── docs/
│   ├── eval-schema.md     # eval case schema and examples
│   └── release-governance.md # release checklist + Agent Quality Impact template
├── agent/
│   ├── agent.go           # core LLM + tools logic
│   ├── subagents.go       # parallel sub-agent spawning
│   └── wallet_guard_test.go
├── compaction/
│   ├── summary.go         # CompactedContext struct, structured JSON format
│   └── compaction.go      # threshold-based compaction, summarization
├── config/
│   └── config.go
├── gateway/
│   ├── types.go           # IncomingMessage, Gateway interface
│   ├── sender.go          # SenderRegistry for reminders/wallet notifications
│   ├── telegram.go
│   ├── discord.go
│   ├── http.go
│   └── signal.go
├── tools/
│   ├── tools.go           # tool definitions + executeTool
│   └── approvals.go       # exec approval persistence
├── skills/
│   ├── manager.go         # skill discovery, parse, read, write
│   ├── security.go       # LLM-based security check for new skills
│   ├── feasibility.go    # LLM-based feasibility/clarity check
│   └── README.md         # skill format and script policy
├── memory/
│   └── memory.go          # long-term memory (save/read with embeddings)
├── embedding/
│   └── embedding.go       # Ollama embedding client
├── conversation/
│   └── store.go           # conversation embeddings for retrieval
├── reminders/
│   ├── store.go           # reminder persistence
│   └── cron.go            # scheduled reminder runner
├── session/
│   └── session.go         # session management
├── sessionqueue/
│   └── queue.go           # per-session request queue
├── sessionlock/
│   └── sessionlock.go     # session locking
├── x402client/            # x402 buyer HTTP client (payment handling)
├── wallet/
│   ├── service.go         # wallet service (balance, transfer, contract call)
│   ├── notifier.go        # approval notifications via SenderRegistry
│   ├── account/           # EOA and smart account types
│   ├── chains/            # chain registry (multichain)
│   ├── signer/            # env/KMS/HSM signer backends
│   ├── policy/            # spend limit policy
│   ├── approval/          # approval store
│   ├── history/           # transaction history
│   ├── abi/               # ABI parsing
│   ├── provider/          # RPC provider
│   └── redact/            # sensitive data redaction
├── skills-data/           # user-installed skills (SKILL.md folders, default; separate from skills/ package)
├── sessions/              # per-user conversation history (*.jsonl)
├── memories/              # per-user long-term memories (*.jsonl)
├── reminders/             # package + reminders.jsonl
├── wallet-approvals/      # pending wallet approvals
├── wallet-history/        # transaction history
├── .env
├── .env.example
├── go.mod
├── main.go
├── PERSONALITY.md         # bot persona (system prompt)
├── WALLET.md              # wallet tool instructions (injected when wallet enabled)
└── README.md
```
