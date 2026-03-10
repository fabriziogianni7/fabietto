# Custom Agent

A basic Go Telegram bot that responds to messages. Step one of building an OpenClaw-like agent.

## Setup

### 1. Create a Telegram Bot

1. Open Telegram and message [@BotFather](https://t.me/BotFather)
2. Send `/newbot` and follow the prompts
3. Copy the token you receive

### 2. Get a Groq API Key

1. Sign up at [console.groq.com](https://console.groq.com)
2. Create an API key

### 3. Configure

Copy the example env file and fill in your tokens:

```bash
cp .env.example .env
# Edit .env with your TELEGRAM_BOT_TOKEN, GROQ_API_KEY, and BRAVE_SEARCH_API_KEY
```

Get a Brave Search API key at [brave.com/search/api](https://brave.com/search/api) (free tier available).

Or export them: `export TELEGRAM_BOT_TOKEN=...`, `export GROQ_API_KEY=...`, `export BRAVE_SEARCH_API_KEY=...`

### 4. Run the Bot

```bash
go mod tidy
go run .
```

Config is loaded from `.env` (if present) and validated at startup.

### 5. Test

Message your bot on Telegram. It will respond using Groq's Llama 3.1 8B model. Conversation history is stored per user in `sessions/` (JSONL files), so the bot remembers context—e.g. "what did I say earlier?" works.

## Tools

The bot can use tools when the LLM decides they're helpful:

| Tool | Description |
|------|-------------|
| `run_command` | Run a shell command (uses current working directory). Blocked commands (e.g. `rm -rf /`) are denied. Safe commands (`ls`, `pwd`, `cat`, etc.) run immediately. Others require approval: say `approve: <command>` or `/approve <command>`. Approvals persist in `exec-approvals.json`. |
| `read_file` | Read a file from the filesystem |
| `write_file` | Write content to a file |
| `web_search` | Search the web (Brave Search API) |

The agent loop runs until the LLM returns a final text response or hits the tool limit (10 rounds). Add or modify tools in `tools/tools.go`.

## Personality

Edit `PERSONALITY.md` to define the bot's persona. Its contents are injected as the system prompt at startup. Change the tone, style, or add rules—the bot will adopt whatever you write.

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

## Context Compaction

When conversation history exceeds ~4000 tokens, the agent uses **structured summarization** to compact old context. A JSON summary is produced with sections: `session_intent`, `key_decisions`, `key_facts`, `file_modifications`, `pending_actions`, `artifacts`, `momentum`, `tool_results_summary`. Only recent messages stay in full; older context is replaced by this structured block.

Set `CONTEXT_COMPACTION_THRESHOLD` (default 4000) to tune when compaction triggers.

## Project Structure

```
custom-agent/
├── agent/
│   └── agent.go       # core LLM + tools logic
├── compaction/
│   ├── summary.go     # CompactedContext struct, structured JSON format
│   └── compaction.go  # threshold-based compaction, summarization
├── config/
│   └── config.go
├── gateway/
│   ├── types.go       # IncomingMessage, Gateway interface
│   ├── telegram.go
│   ├── discord.go
│   ├── http.go
│   └── signal.go
├── tools/
│   └── tools.go       # tool definitions + executeTool
├── sessions/
│   └── {userID}.jsonl   # per-user conversation history
├── .env
├── .env.example
├── go.mod
├── main.go
├── PERSONALITY.md      # bot persona (system prompt)
└── README.md
```
