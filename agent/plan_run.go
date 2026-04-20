package agent

import (
	"context"
	"encoding/json"
	"log"
	"regexp"
	"strings"

	"custom-agent/agent/planning"
	"custom-agent/gateway"
	"custom-agent/session"

	"github.com/sashabaranov/go-openai"
)

var walletPlannerIntent = regexp.MustCompile(`(?i)\b(swap|contract\s+call|execute\s+contract|call\s+contract|interact\s+with\s+(?:a\s+)?contract|smart\s+contract)\b`)

func (a *Agent) shouldUseWalletPlanner(ctx context.Context, text string) bool {
	if !a.planner.Enabled || !a.planner.CapabilityEnabled("wallet") {
		return false
	}
	if a.tools == nil || a.tools.Wallet == nil {
		return false
	}
	if wantsWalletSend(text) {
		return false
	}
	mode := strings.ToLower(strings.TrimSpace(a.planner.Mode))
	if mode == "" {
		mode = planning.PlannerModeOff
	}
	switch mode {
	case planning.PlannerModeOff:
		return false
	case planning.PlannerModeAlwaysWallet:
		return true
	case planning.PlannerModeWallet:
		return walletPlannerIntent.MatchString(text)
	case planning.PlannerModeAuto:
		if autoWalletHeuristicQuick(text) {
			return true
		}
		if autoWalletNeedsRouter(text) {
			return a.autoWalletTryRouter(ctx, text)
		}
		return false
	default:
		return walletPlannerIntent.MatchString(text)
	}
}

// tryWalletPlanner runs plan-and-execute for wallet contract flows. Returns ("", false) to fall back to reactive loop.
func (a *Agent) tryWalletPlanner(ctx context.Context, msg gateway.IncomingMessage, userText string) (string, bool) {
	type plannerOut struct {
		Goal         string            `json:"goal"`
		Constraints  map[string]string `json:"constraints"`
		Capability   string            `json:"capability"`
	}

	sys := `You are a planning assistant. The user wants an on-chain contract interaction (swap, contract call, etc.).
Output ONLY a single JSON object, no markdown, no other text. Shape:
{"goal":"short description","capability":"wallet.tx","constraints":{"to":"0x...","data":"0x...","value_wei":"0","chain_id":"optional number"}}
Rules:
- "to" must be the contract address (checksummed or lowercase 0x + 40 hex).
- "data" is hex calldata including the 4-byte selector (0x...).
- "value_wei" is a decimal string for ETH sent with the call; use "0" if none.
- "chain_id" only if the user specified a chain; omit if default chain should be used.
If you cannot derive to and data from the message, output: {"goal":"","capability":"wallet.tx","constraints":{}}
`

	resp, err := a.client.CreateChatCompletion(ctx, openai.ChatCompletionRequest{
		Model: parentModel,
		Messages: []openai.ChatCompletionMessage{
			{Role: openai.ChatMessageRoleSystem, Content: sys},
			{Role: openai.ChatMessageRoleUser, Content: userText},
		},
	})
	if err != nil {
		log.Printf("[planner] LLM error: %v", err)
		return "", false
	}
	if len(resp.Choices) == 0 {
		return "", false
	}
	raw := extractJSONObject(strings.TrimSpace(resp.Choices[0].Message.Content))
	var out plannerOut
	if err := json.Unmarshal([]byte(raw), &out); err != nil {
		log.Printf("[planner] json parse: %v", err)
		return "", false
	}
	if strings.TrimSpace(out.Goal) == "" || out.Capability != string(planning.CapabilityWalletTx) {
		return "", false
	}

	p := planning.WalletTxTemplate(out.Goal, out.Constraints)
	planning.MergePlannerConstraints(p.Constraints, map[string]string{
		"_platform": msg.Platform,
		"_user_id":  msg.UserID,
		"_chat_id":  msg.ChatID,
	})
	if err := planning.ValidatePlan(p); err != nil {
		log.Printf("[planner] validate plan: %v", err)
		return "", false
	}
	v := planning.NewValidator(planning.CapabilityWalletTx)
	if err := v.Validate(p); err != nil {
		log.Printf("[planner] validator: %v", err)
		return "", false
	}

	planID := ""
	if a.telemetry != nil {
		a.telemetry.RecordPlanEvent(ctx, "plan_validated", map[string]interface{}{
			"capability": string(p.Capability),
			"step_count": len(p.Steps),
		})
	}

	exec := &planning.WalletExecutor{Wallet: a.tools.Wallet}
	res, err := exec.Execute(ctx, p)
	if err != nil {
		if a.telemetry != nil {
			a.telemetry.RecordPlanEvent(ctx, "plan_execute_error", map[string]interface{}{"error": err.Error()})
		}
		return "Planner execution failed: " + err.Error(), true
	}
	if a.telemetry != nil {
		planID = "wallet_tx"
		a.telemetry.RecordPlanEvent(ctx, "plan_execute_done", map[string]interface{}{
			"plan_id":       planID,
			"stopped_early": res.StoppedEarly,
			"steps":         len(res.StepResults),
		})
		for _, sr := range res.StepResults {
			a.telemetry.RecordPlanEvent(ctx, "plan_step", map[string]interface{}{
				"plan_id": planID,
				"step_id": sr.StepID,
				"status":  string(sr.Status),
			})
		}
	}

	reply := strings.TrimSpace(res.FinalReply)
	if reply == "" {
		reply = "Plan completed with no final message."
	}
	_ = session.Append(msg.Platform, msg.UserID, userText, reply)
	if a.convStore != nil {
		_ = a.convStore.Add(msg.Platform, msg.UserID, "user", userText)
		_ = a.convStore.Add(msg.Platform, msg.UserID, "assistant", reply)
	}
	return reply, true
}

// extractJSONObject strips optional markdown fences and returns the first {...} block.
func extractJSONObject(s string) string {
	s = strings.TrimSpace(s)
	if strings.HasPrefix(s, "```") {
		if idx := strings.Index(s, "\n"); idx != -1 {
			s = strings.TrimSpace(s[idx+1:])
		}
		if end := strings.LastIndex(s, "```"); end != -1 {
			s = strings.TrimSpace(s[:end])
		}
	}
	start := strings.Index(s, "{")
	end := strings.LastIndex(s, "}")
	if start >= 0 && end > start {
		return s[start : end+1]
	}
	return s
}
