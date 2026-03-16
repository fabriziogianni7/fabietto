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
| `run_command` | Run a shell command (uses current working directory). Blocked commands (e.g. `rm -rf /`) are denied. Safe commands (`ls`, `pwd`, `cat`, etc.) run immediately. Others require approval: say `approve: <command>` or `/approve <command>`. Approvals persist in `exec-approvals.json`. |
| `read_file` | Read a file from the filesystem |
| `write_file` | Write content to a file |
| `web_search` | Search the web (Brave Search API) |
| `save_memory` | Save a fact or preference to long-term memory (survives `/new`) |
| `read_memory` | Search long-term memory (semantic search when Ollama is available) |
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

- **Run tests**: `go test ./...`
- **Add tools**: Define and implement in `tools/tools.go`; register in the tool set passed to the agent
- **Add gateways**: Implement the `gateway.Gateway` interface in `gateway/` and wire it in `main.go`
- **Code style**: Standard Go formatting (`gofmt`). Keep packages focused; wallet, reminders, and compaction are modular

---

## Project Structure

```
custom-agent/
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
