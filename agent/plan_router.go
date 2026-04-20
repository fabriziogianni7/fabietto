package agent

import (
	"context"
	"encoding/json"
	"log"
	"regexp"
	"strings"

	"github.com/sashabaranov/go-openai"
)

var (
	autoWalletKeywords = regexp.MustCompile(`(?i)\b(swap|trade|stake|unstake|bridge|mint|burn|deposit|withdraw|claim|harvest|farm|pool|liquidity|yield|defi|aggregator|router|approve|allowance|permit|erc-?20|nft|uniswap|sushiswap|aave|compound|lido|curve|1inch|paraswap|cowswap|dex|amm|open\s*sea|usdc|usdt|dai|weth|matic|arb|op\s*eth|base\s*eth)\b`)
	hexAddressInText     = regexp.MustCompile(`(?i)0x[0-9a-f]{40}\b`)
)

func autoWalletHeuristicQuick(text string) bool {
	if walletPlannerIntent.MatchString(text) {
		return true
	}
	if autoWalletKeywords.MatchString(text) {
		return true
	}
	if hexAddressInText.MatchString(text) {
		return true
	}
	return false
}

func autoWalletNeedsRouter(text string) bool {
	if strings.Contains(text, "?") {
		return true
	}
	t := strings.TrimSpace(text)
	if len(t) > 0 && len(t) < 100 {
		return true
	}
	return false
}

// autoWalletTryRouter asks a tiny LLM whether to run the wallet contract planner.
func (a *Agent) autoWalletTryRouter(ctx context.Context, userText string) bool {
	sys := `You route user messages for an assistant that has an EVM wallet.
Output ONLY JSON: {"use_planner":true|false}
Set use_planner to true if the user may need executing or preparing an on-chain contract interaction (swap, approve, stake, bridge, NFT mint, protocol deposit, etc.) — even if details are incomplete.
Set use_planner to false for: greetings, general chat, coding, file tasks, web search, scheduling reminders, checking balance only, listing history only, or a plain native ETH transfer to an address without contract calldata.
When unsure, prefer true.`

	resp, err := a.client.CreateChatCompletion(ctx, openai.ChatCompletionRequest{
		Model: subagentModelForIndex(0),
		Messages: []openai.ChatCompletionMessage{
			{Role: openai.ChatMessageRoleSystem, Content: sys},
			{Role: openai.ChatMessageRoleUser, Content: userText},
		},
	})
	if err != nil {
		log.Printf("[planner] router LLM error: %v", err)
		return false
	}
	if len(resp.Choices) == 0 {
		return false
	}
	raw := extractJSONObject(strings.TrimSpace(resp.Choices[0].Message.Content))
	var out struct {
		UsePlanner bool `json:"use_planner"`
	}
	if err := json.Unmarshal([]byte(raw), &out); err != nil {
		log.Printf("[planner] router json: %v", err)
		return false
	}
	return out.UsePlanner
}
