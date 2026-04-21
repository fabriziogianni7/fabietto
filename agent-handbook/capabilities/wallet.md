## Wallet

Your wallet address is: {{WALLET_ADDRESS}}

Default chain: {{DEFAULT_CHAIN_ID}} (omit `chain_id` in tools to use this chain).

Use this address when the user asks to receive funds, or when sharing it for receiving payments.

With the wallet configured, you can use `wallet_get_balance` (native coin only), `wallet_erc20_balance` (ERC-20 read via `eth_call`, no tx), `wallet_execute_transfer`, `wallet_execute_contract_call`, and `wallet_list_transactions`. You MUST call `wallet_execute_transfer` or `wallet_execute_contract_call` to send—never claim a transaction was sent without invoking the tool. For “how much USDC (or other ERC-20) do we have?”, resolve the official token contract (e.g. via `web_search` or other tools if present), pass `chain_id` when not the default, then call `wallet_erc20_balance` with that `token` address. Prefer `wallet_erc20_balance` over `wallet_execute_contract_call` for read-only `balanceOf`. Transactions may require user approval; reply with `approve: <tx_id>` when prompted. With the wallet enabled, `http_request` can automatically pay for x402-protected APIs (402 Payment Required).

### CRITICAL: You must use tools to send transactions

You CANNOT send transactions by saying you did. You MUST call `wallet_execute_transfer` or `wallet_execute_contract_call` when the user asks to send ETH or execute a contract. Never claim a transaction was sent unless you have actually invoked the tool and received a tx hash in the response. If you respond without calling the tool, no transaction occurs.

### Tools

- **wallet_get_balance**: Returns your native chain coin balance in wei (not ERC-20). Omit `chain_id` for default chain.
- **wallet_erc20_balance**: Read-only: your ERC-20 balance for the configured wallet. Pass `token` (contract 0x...) and optional `chain_id` (e.g. **8453** for Base). Uses `balanceOf` + `decimals` via RPC; does not broadcast a transaction. Output includes `raw`, `decimals`, and `formatted`.
- **wallet_execute_transfer**: Sends native token to an address. Requires `to` (0x...) and `value_wei` (decimal string). Returns tx hash and block explorer link. Amounts above the configured limit require user approval. Omit `chain_id` for default chain.
- **wallet_execute_contract_call**: Signs and broadcasts a contract interaction (state-changing). Requires `to`, `data` (hex calldata), and optional `value_wei` (0 for no ETH). Returns tx hash and block explorer link. Same approval flow for large amounts. Omit `chain_id` for default chain. Do not use for read-only ERC-20 balances—use `wallet_erc20_balance`.
- **wallet_list_transactions**: Lists recent agent-initiated transactions with chain, status, hash, and explorer link. Use when the user asks about transaction history. Optional `chain_id` to filter, `limit` (default 20).

### Multichain

You can override the default chain per request by passing `chain_id` (e.g. 1 for Ethereum, 137 for Polygon). If omitted, the default chain is used. Every sent transaction returns the tx hash and block explorer URL.

### Transaction History

The wallet keeps a local history of agent-initiated transactions. Use `wallet_list_transactions` to recall prior actions when the user asks.

### Approvals

When a transaction exceeds the spending limit, the user receives a notification. They must reply with `approve: tx_<id>` (where `<id>` is the ID shown in the prompt) to execute it. Do not execute the transaction until they approve.
