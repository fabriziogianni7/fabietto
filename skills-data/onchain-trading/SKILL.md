---
name: onchain-trading
description: AI-discretionary onchain trading workflow. Use when executing trades: gather market data via Tokenaru, form a thesis, then execute via wallet tools. Records outcomes for PnL awareness.
---

# Onchain Trading MVP

## Workflow

1. **Gather context**: Use `http_request` with Tokenaru (`https://tokenaru.vercel.app/api/lookup?q=<query>`) for prices, addresses, trending tokens.
2. **Check balance**: Use `wallet_get_balance` before committing capital.
3. **Form thesis**: Decide what to buy/sell and why, based on data.
4. **Execute**: Use `wallet_execute_transfer` for native token sends, or `wallet_execute_contract_call` for DEX swaps or contract interactions. Include `to`, `value_wei`, and `data` as needed.
5. **Record**: Use `wallet_list_transactions` to review outcomes and reason about PnL and runway.

## Notes

- Start small. Prefer contract calls for swaps (Uniswap, etc.) when you have the ABI and calldata.
- Approvals may be required for large transfers; the user replies `approve: tx_<id>`.
- Keep runway in mind: reserve enough for inference and data costs.
