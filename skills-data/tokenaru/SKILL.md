---
name: tokenaru
description: Fetch onchain data (prices, addresses, trending coins) via Tokenaru. Use when the user or agent needs token prices, contract addresses, trending tokens, or other onchain lookup data.
---

# Tokenaru Onchain Lookup

## When to use

Use `http_request` with Tokenaru when you need:
- Token prices
- Contract addresses
- Trending coins or tokens
- Other onchain lookup data

## Endpoint

```
https://tokenaru.vercel.app/api/lookup?q=<query>
```

Replace `<query>` with the search term (e.g. token symbol, address, or topic).

## Example

For "ETH price" or "ethereum address":
```
http_request with url: https://tokenaru.vercel.app/api/lookup?q=ETH%20price
```

The response is JSON. Parse and use the data in your reply.
