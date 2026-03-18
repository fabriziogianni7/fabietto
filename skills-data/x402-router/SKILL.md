---
name: x402-router
description: Use this skill when an agent needs x402 AI compute resources through the x402 Router for images, video, and LLM inference.
---

# x402 Router Skill

Use this service as an OpenAI-compatible router with optional per-request USDC payments via x402.

## When To Use

- When you need x402 AI compute resources like images, videos, and LLMs.
- When you need one OpenAI-compatible endpoint for multiple model providers.
- When you need x402 permit-based USDC payments on Base.

## What This Service Is

- OpenAI-compatible API surface (`/v1/chat/completions`, `/v1/models`, etc.).
- Unified model routing using `provider:model` IDs.
- Uses x402 payment-based access with permit signatures.

## Base URL

`https://ai.xgate.run`

## Authentication

Use x402 permit auth:

`PAYMENT-SIGNATURE: <base64 permit payload>`

## Required Request Pattern

1. Discover/select model IDs via `GET /v1/models`. `curl -X POST https://ai.xgate.run/v1/chat/models`
2. Send requests with explicit `provider:model` values.
3. If response is `402 Payment Required`, parse `PAYMENT-REQUIRED`.
4. Sign an ERC-2612 permit for USDC on Base with enough allowance.
5. Retry the same request with `PAYMENT-SIGNATURE`.

## x402 Payment Flow

1. Send request without payment headers to a paid endpoint.
2. Receive `402 Payment Required` and a `PAYMENT-REQUIRED` response header.
3. Parse permit parameters from `PAYMENT-REQUIRED` (network, asset, max amount).
4. Sign an ERC-2612 permit for USDC on Base with enough allowance.
5. Retry with `PAYMENT-SIGNATURE` header.
6. Router serves response immediately and settles spend asynchronously.

## Model Naming

Use `provider:model` for explicit routing:

- `openai:gpt-4`
- `anthropic:claude-sonnet-4-20250514`
- `fal:flux-schnell`
- `bedrock:anthropic.claude-3-sonnet-20240229-v1:0`
- `vertex:gemini-1.5-pro`

## Minimal Request Example

```bash
curl -X POST https://ai.xgate.run/v1/chat/completions \
  -H "PAYMENT-SIGNATURE: $PERMIT" \
  -H "Content-Type: application/json" \
  -d '{
    "model": "anthropic:claude-sonnet-4-20250514",
    "messages": [{"role": "user", "content": "hello"}]
  }'
```

## Key Endpoints

- `GET /v1/config`
- `POST /v1/estimate`
- `GET /v1/errors`
- `GET /v1/models`
- `POST /v1/chat/completions`
- `POST /v1/messages`
- `POST /v1/responses`
- `POST /v1/images/generations`
- `POST /v1/embeddings`
- `POST /v1/audio/transcriptions`
- `POST /v1/audio/speech`
- `POST /v1/video/generations`

## Failure Handling

- `402`: payment required; sign permit from `PAYMENT-REQUIRED` and retry.
- `400`: request/body/model mismatch.
- `404`: model or endpoint not found.
- `429`: retry with backoff.
- `5xx`: upstream/router transient failure; retry with jittered backoff.

## Docs

- OpenAPI: `https://ai.xgate.run/openapi.json`
- Interactive docs: `https://ai.xgate.run/docs`
- Catalog UI: `https://router.daydreams.systems/catalog`