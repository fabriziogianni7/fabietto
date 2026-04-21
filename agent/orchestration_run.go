package agent

import (
	"context"
	"fmt"
	"log"
	"regexp"
	"strings"
	"time"

	"custom-agent/agent/planning"
	"custom-agent/gateway"
	"custom-agent/session"
	"custom-agent/tools"

	"github.com/sashabaranov/go-openai"
)

const maxOrchestrationStepRounds = 14

// HTTP status lines from http_request look like "Status: 404 404 Not Found".
var orchestrationHTTPRetryStatus = regexp.MustCompile(`(?i)Status:\s+(4\d\d|5\d\d)\s`)

// tryGeneralOrchestration builds a plan via LLM, validates, executes steps, optional synthesis.
// contextPrefix matches the reactive loop before the current user message (system prompt, skills,
// wallet nudge, memory/conversation blocks, compaction summary, recent turns). Returns ("", false) to use the reactive loop.
func (a *Agent) tryGeneralOrchestration(ctx context.Context, msg gateway.IncomingMessage, userText string, contextPrefix []openai.ChatCompletionMessage) (string, bool) {
	if a.orchestration.Mode == planning.OrchestrationModeOff {
		return "", false
	}
	if a.tools == nil {
		return "", false
	}
	if a.shouldSkipOrchestrationTrivial(userText) {
		return "", false
	}

	sys := `You are a task planner for an assistant with tools. Output ONLY valid JSON, no markdown.
Shape:
{"goal":"one line","steps":[{"id":"s1","description":"what to do in this step","allowed_tools":["tool_name",...],"depends_on":[],"risk":"read_only|mutating|wallet"}]}

CRITICAL — allowed_tools completeness:
- Each step exposes ONLY the tools listed in that step's "allowed_tools" to the executor. If you omit a tool from "allowed_tools", the executor cannot call it in that step—there is no separate "permission" toggle.
- Every tool the step description implies MUST appear in "allowed_tools" for that step. Example: if the step checks native gas balance, include "wallet_get_balance". If it checks ERC-20 balance, include "wallet_erc20_balance". If it broadcasts a standard ERC-20 transfer, include "wallet_execute_erc20_transfer". If it sets token allowance for a spender, include "wallet_execute_erc20_approve". If it needs swap or other raw calldata, include "wallet_execute_contract_call". If it sends native coin, include "wallet_execute_transfer".
- Do not plan a step that needs tool X but only list other tools in "allowed_tools".

Wallet tool choice (avoid wrong tools):
- Read-only native balance (gas, ETH on a chain): wallet_get_balance — NOT wallet_execute_contract_call.
- Read-only ERC-20 balance: wallet_erc20_balance — NOT wallet_execute_contract_call (no balanceOf via contract_call for reads).
- Native token transfer: wallet_execute_transfer.
- Standard ERC-20 transfer to an address: wallet_execute_erc20_transfer. Standard ERC-20 approve(spender, amount): wallet_execute_erc20_approve. Swap / arbitrary calldata: wallet_execute_contract_call.
- Past txs: wallet_list_transactions.

General rules:
- Do as many steps as you need. depends_on lists step ids that must finish before this step; you may reference any step id in the plan (not only steps listed earlier in the JSON array). The runtime executes steps in dependency order.
- Read-only research: web_search, read_file, read_memory, http_request, list_skills, read_skill, read_skill_script.
- Mutating filesystem: write_file, run_command (only if needed).
- risk: "wallet" for steps that sign and broadcast; "mutating" for writes/commands; "read_only" for searches, http GETs, wallet_erc20_balance, wallet_get_balance, wallet_list_transactions.
- Do not include spawn_subagents in plans.

If the task is a single trivial chat with no tools, output: {"goal":"","steps":[]}`

	plannerMsgs := make([]openai.ChatCompletionMessage, 0, 2+len(contextPrefix))
	plannerMsgs = append(plannerMsgs, openai.ChatCompletionMessage{Role: openai.ChatMessageRoleSystem, Content: sys})
	plannerMsgs = append(plannerMsgs, contextPrefix...)
	plannerMsgs = append(plannerMsgs, openai.ChatCompletionMessage{Role: openai.ChatMessageRoleUser, Content: userText})

	plannerReq := openai.ChatCompletionRequest{
		Model:    parentModel,
		Messages: plannerMsgs,
	}
	tPlanner := time.Now()
	resp, err := a.client.CreateChatCompletion(ctx, plannerReq)
	a.recordLLMRound(ctx, "orchestration_planner", 0, plannerReq, resp, err, tPlanner)
	if err != nil {
		log.Printf("[orchestration] planner LLM error: %v", err)
		return "", false
	}
	if len(resp.Choices) == 0 {
		return "", false
	}
	raw := extractJSONObject(strings.TrimSpace(resp.Choices[0].Message.Content))
	plan, err := planning.OrchestrationPlanFromJSON([]byte(raw), tools.IsRegisteredTool)
	if err != nil {
		log.Printf("[orchestration] plan parse/validate: %v", err)
		return "", false
	}
	if len(plan.Steps) == 0 {
		return "", false
	}
	if err := planning.ValidateOrchestrationPlanPolicy(plan, a.tools); err != nil {
		log.Printf("[orchestration] policy: %v", err)
		return "", false
	}

	for _, s := range plan.Steps {
		log.Printf("[orchestration] planner step %s allowed_tools=%v risk=%s", s.ID, s.AllowedTools, s.Risk)
	}

	planStepsMeta := make([]map[string]interface{}, 0, len(plan.Steps))
	for _, s := range plan.Steps {
		planStepsMeta = append(planStepsMeta, map[string]interface{}{
			"step_id":       s.ID,
			"allowed_tools": s.AllowedTools,
			"risk":          string(s.Risk),
		})
	}
	if a.telemetry != nil {
		a.telemetry.RecordPlanEvent(ctx, "orchestration_plan_start", map[string]interface{}{
			"goal":       plan.Goal,
			"step_count": len(plan.Steps),
			"steps":      planStepsMeta,
		})
	}

	final, err := a.runOrchestrationPlan(ctx, msg, userText, plan, contextPrefix)
	if err != nil {
		if a.telemetry != nil {
			a.telemetry.RecordPlanEvent(ctx, "orchestration_error", map[string]interface{}{"error": err.Error()})
		}
		return "", false
	}
	if a.telemetry != nil {
		a.telemetry.RecordPlanEvent(ctx, "orchestration_done", map[string]interface{}{"reply_len": len(final)})
	}

	_ = session.Append(msg.Platform, msg.UserID, userText, final)
	if a.convStore != nil {
		_ = a.convStore.Add(msg.Platform, msg.UserID, "user", userText)
		_ = a.convStore.Add(msg.Platform, msg.UserID, "assistant", final)
	}
	return final, true
}

func (a *Agent) shouldSkipOrchestrationTrivial(text string) bool {
	t := strings.TrimSpace(strings.ToLower(text))
	if len(t) <= 2 {
		return true
	}
	greetings := []string{"hi", "hello", "hey", "thanks", "thank you", "ok", "okay", "yes", "no"}
	for _, g := range greetings {
		if t == g || t == g+"." {
			return true
		}
	}
	return false
}

func (a *Agent) runOrchestrationPlan(ctx context.Context, msg gateway.IncomingMessage, userText string, plan *planning.OrchestrationPlan, contextPrefix []openai.ChatCompletionMessage) (string, error) {
	orderedSteps, err := planning.OrchestrationExecutionOrder(plan.Steps)
	if err != nil {
		return "", err
	}
	stepOut := make(map[string]string)
	llmRoundSeq := 0

	for stepIdx, st := range orderedSteps {
		var prior strings.Builder
		for _, dep := range st.DependsOn {
			if out, ok := stepOut[dep]; ok && out != "" {
				prior.WriteString("--- Output from step ")
				prior.WriteString(dep)
				prior.WriteString(" ---\n")
				prior.WriteString(truncateForOrchestration(out, 8000))
				prior.WriteString("\n")
			}
		}

		allowed := filterToolsForRuntime(st.AllowedTools, a.tools)
		allowed = augmentWalletReadToolsAllowlist(allowed, a.tools)
		log.Printf("[orchestration] step %s runtime_tools=%v (planner had %v)", st.ID, allowed, st.AllowedTools)
		if a.telemetry != nil {
			a.telemetry.RecordPlanEvent(ctx, "orchestration_step_start", map[string]interface{}{
				"step_id":               st.ID,
				"index":                 stepIdx,
				"allowed_tools_planner": st.AllowedTools,
				"allowed_tools_runtime": allowed,
			})
		}
		if len(allowed) == 0 {
			stepOut[st.ID] = "Error: no tools available for this step after filtering."
			continue
		}
		toolDefs, err := tools.DefinitionsSubset(allowed)
		if err != nil {
			stepOut[st.ID] = "Error: " + err.Error()
			continue
		}

		walletPipe := false
		for _, t := range allowed {
			if t == "wallet_execute_contract_call" || t == "wallet_execute_erc20_transfer" || t == "wallet_execute_erc20_approve" {
				walletPipe = a.tools.Wallet != nil
				break
			}
		}

		sys := "You execute ONE step of a multi-step plan. You may ONLY call tools from the tool list provided by the API for this request (the planner's allowed_tools for this step, plus wallet_get_balance and wallet_erc20_balance when any wallet tool is present).\n"
		sys += "Tools available in this step: " + strings.Join(allowed, ", ") + ".\n"
		sys += "Wallet hints: use wallet_get_balance for native coin balance; wallet_erc20_balance for token balance reads; wallet_execute_transfer for native sends; wallet_execute_erc20_transfer for standard ERC-20 sends; wallet_execute_erc20_approve for ERC-20 allowances (token, spender, amount in base units; 0 revokes); wallet_execute_contract_call only for contract writes that need raw calldata (e.g. swaps)—never use contract_call for read-only balance checks.\n"
		sys += "Overall user goal: " + plan.Goal + "\n"
		sys += "This step (" + st.ID + "): " + st.Description + "\n"
		if prior.Len() > 0 {
			sys += "Context from previous steps:\n" + prior.String()
		}
		sys += "\nRules: If a tool returns an error, invalid arguments, HTTP 4xx/5xx, or empty/wrong results, you MUST call tools again with corrected parameters (e.g. required fields like web_search \"query\"), a different URL, or another source—do not treat one failed call as the end of the step. When prior tool output already contains useful data (e.g. search hits or HTTP 200 bodies), your summary must reflect that success; do not claim the step failed entirely unless every attempt did."
		sys += "\nRespond with tool calls until this step is done, then a short text summary of what was learned or done for this step."

		msgs := make([]openai.ChatCompletionMessage, 0, 2+len(contextPrefix))
		msgs = append(msgs, openai.ChatCompletionMessage{Role: openai.ChatMessageRoleSystem, Content: sys})
		msgs = append(msgs, contextPrefix...)
		msgs = append(msgs, openai.ChatCompletionMessage{Role: openai.ChatMessageRoleUser, Content: "Original request:\n" + userText})

		var stepBuf strings.Builder
		var lastToolOut string
		for round := 0; round < maxOrchestrationStepRounds; round++ {
			llmRoundSeq++
			stepReq := openai.ChatCompletionRequest{
				Model:    parentModel,
				Messages: msgs,
				Tools:    toolDefs,
			}
			tStep := time.Now()
			resp, err := a.client.CreateChatCompletion(ctx, stepReq)
			a.recordLLMRound(ctx, fmt.Sprintf("orchestration_step:%s", st.ID), llmRoundSeq, stepReq, resp, err, tStep)
			if err != nil {
				stepBuf.WriteString("Error: " + err.Error())
				break
			}
			if len(resp.Choices) == 0 {
				break
			}
			mr := resp.Choices[0].Message
			if len(mr.ToolCalls) > 0 {
				msgs = append(msgs, mr)
				anyRetry := false
				for _, tc := range mr.ToolCalls {
					if strings.TrimSpace(tc.Function.Name) == "" {
						continue
					}
					if a.telemetry != nil {
						a.telemetry.RecordPlanEvent(ctx, "orchestration_step_tool", map[string]interface{}{
							"step_id":   st.ID,
							"tool_name": tc.Function.Name,
						})
					}
					res := a.executeToolForMessage(ctx, msg, tc.Function.Name, tc.Function.Arguments, walletPipe)
					lastToolOut = res
					if orchestrationToolOutputSuggestsRetry(res) {
						anyRetry = true
					}
					msgs = append(msgs, openai.ChatCompletionMessage{
						Role:       openai.ChatMessageRoleTool,
						Content:    res,
						ToolCallID: tc.ID,
					})
				}
				if anyRetry && round < maxOrchestrationStepRounds-1 {
					msgs = append(msgs, openai.ChatCompletionMessage{
						Role:    openai.ChatMessageRoleSystem,
						Content: orchestrationRetryAfterToolFailureNudge(),
					})
				}
				continue
			}
			if toolName, toolArgs, ok := tools.ParseToolCallFromContent(mr.Content); ok {
				msgs = append(msgs, openai.ChatCompletionMessage{Role: openai.ChatMessageRoleAssistant, Content: mr.Content})
				res := a.executeToolForMessage(ctx, msg, toolName, toolArgs, walletPipe)
				lastToolOut = res
				msgs = append(msgs, openai.ChatCompletionMessage{
					Role:       openai.ChatMessageRoleTool,
					Content:    res,
					ToolCallID: "fallback",
					Name:       toolName,
				})
				if orchestrationToolOutputSuggestsRetry(res) && round < maxOrchestrationStepRounds-1 {
					msgs = append(msgs, openai.ChatCompletionMessage{
						Role:    openai.ChatMessageRoleSystem,
						Content: orchestrationRetryAfterToolFailureNudge(),
					})
				}
				continue
			}
			txt := strings.TrimSpace(mr.Content)
			if txt != "" {
				if lastToolOut != "" && orchestrationToolOutputSuggestsRetry(lastToolOut) && round < maxOrchestrationStepRounds-1 {
					msgs = append(msgs, openai.ChatCompletionMessage{
						Role:    openai.ChatMessageRoleSystem,
						Content: orchestrationRetryBeforeStepSummaryNudge(),
					})
					continue
				}
				stepBuf.WriteString(txt)
			}
			break
		}
		out := strings.TrimSpace(stepBuf.String())
		if out == "" && lastToolOut != "" {
			out = truncateForOrchestration(lastToolOut, 12000)
		}
		if out == "" {
			out = "(step completed with no text summary)"
		}
		stepOut[st.ID] = out
		if a.telemetry != nil {
			a.telemetry.RecordPlanEvent(ctx, "orchestration_step_end", map[string]interface{}{
				"step_id": st.ID,
			})
		}
	}

	// Final synthesis
	var syn strings.Builder
	for _, st := range orderedSteps {
		if o, ok := stepOut[st.ID]; ok {
			syn.WriteString("### ")
			syn.WriteString(st.ID)
			syn.WriteString(": ")
			syn.WriteString(st.Description)
			syn.WriteString("\n")
			syn.WriteString(o)
			syn.WriteString("\n\n")
		}
	}
	summary := strings.TrimSpace(syn.String())
	if summary == "" {
		return "", fmt.Errorf("no step output to summarize")
	}

	synMsgs := make([]openai.ChatCompletionMessage, 0, 2+len(contextPrefix))
	synMsgs = append(synMsgs, openai.ChatCompletionMessage{Role: openai.ChatMessageRoleSystem, Content: orchestrationSynthesisSystemPrompt()})
	synMsgs = append(synMsgs, contextPrefix...)
	synMsgs = append(synMsgs, openai.ChatCompletionMessage{Role: openai.ChatMessageRoleUser, Content: "User request:\n" + userText + "\n\nStep results:\n" + summary})

	llmRoundSeq++
	synReq := openai.ChatCompletionRequest{
		Model:    parentModel,
		Messages: synMsgs,
	}
	tSyn := time.Now()
	synResp, err := a.client.CreateChatCompletion(ctx, synReq)
	a.recordLLMRound(ctx, "orchestration_synthesis", llmRoundSeq, synReq, synResp, err, tSyn)
	if err != nil || len(synResp.Choices) == 0 {
		return summary, nil
	}
	final := strings.TrimSpace(synResp.Choices[0].Message.Content)
	if final == "" {
		return summary, nil
	}
	return final, nil
}

func orchestrationToolOutputSuggestsRetry(s string) bool {
	s = strings.TrimSpace(s)
	if s == "" {
		return false
	}
	if strings.HasPrefix(s, "Error:") {
		return true
	}
	low := strings.ToLower(s)
	if strings.Contains(low, "invalid arguments") {
		return true
	}
	if strings.Contains(low, "search failed:") {
		return true
	}
	if orchestrationHTTPRetryStatus.MatchString(s) {
		return true
	}
	return false
}

func orchestrationRetryAfterToolFailureNudge() string {
	return "The last tool result indicates a failed or invalid request (missing args, HTTP error, or tool error). Do not end this step yet: call tools again with corrected arguments (e.g. web_search requires a non-empty \"query\" JSON field) or try a different URL/source. Only after you have a successful result or exhaust reasonable alternatives, write the short step summary."
}

func orchestrationRetryBeforeStepSummaryNudge() string {
	return "The last tool output shows an error or HTTP failure, but you responded with text only. Use another tool call with fixed parameters or another endpoint first; do not summarize this step as a total failure while a retry could succeed."
}

func orchestrationSynthesisSystemPrompt() string {
	return "Summarize the following step results into a clear, concise answer for the user.\n" +
		"Rules:\n" +
		"- Base your answer ONLY on the step results text. Do not invent facts, failures, or missing parameters that are not stated there.\n" +
		"- If step results include successful tool output (search hits, HTTP 200 response bodies, file contents), you MUST present that information. Do not claim the whole task failed if some steps clearly returned useful data.\n" +
		"- You may mention partial failures (e.g. one URL returned 404) alongside what succeeded.\n" +
		"- If step results are internally contradictory, prefer describing what the text actually shows rather than guessing.\n" +
		"- If a step failed because no tools were available, allowed_tools were wrong, or wallet tools were missing from the plan, explain plainly that the orchestration plan must list every required tool per step (or that the deployment may lack wallet configuration). Do NOT tell the user to enable vague \"platform permissions\" or to \"re-run with permissions\"—that is misleading here.\n" +
		"- For ERC-20 / token balance, the correct read tool is wallet_erc20_balance (not wallet_execute_contract_call). For sending ERC-20 tokens, wallet_execute_erc20_transfer; for approve/allowance, wallet_execute_erc20_approve. For native gas balance, wallet_get_balance.\n" +
		"- Keep a helpful tone: suggest retrying the same request (the system may fall back to a different path) or spell out which tools were needed for the task."
}

func truncateForOrchestration(s string, max int) string {
	if len(s) <= max {
		return s
	}
	return s[:max] + "\n...(truncated)"
}

func filterToolsForRuntime(allow []string, t *tools.Tools) []string {
	if t == nil {
		return nil
	}
	out := make([]string, 0, len(allow))
	for _, name := range allow {
		name = strings.TrimSpace(name)
		if name == "" {
			continue
		}
		if strings.HasPrefix(name, "wallet_") && t.Wallet == nil {
			continue
		}
		out = append(out, name)
	}
	return out
}

// augmentWalletReadToolsAllowlist adds wallet_get_balance and wallet_erc20_balance whenever
// any wallet_* tool is present for the step. Planners often omit these for "transfer" or
// mixed steps; without them the executor cannot check gas or ERC-20 balance before sending.
func augmentWalletReadToolsAllowlist(allowed []string, t *tools.Tools) []string {
	if t == nil || t.Wallet == nil || len(allowed) == 0 {
		return allowed
	}
	hasWalletTool := false
	for _, name := range allowed {
		if strings.HasPrefix(name, "wallet_") {
			hasWalletTool = true
			break
		}
	}
	if !hasWalletTool {
		return allowed
	}
	extra := []string{"wallet_get_balance", "wallet_erc20_balance"}
	seen := make(map[string]bool, len(allowed)+len(extra))
	for _, name := range allowed {
		seen[name] = true
	}
	out := make([]string, 0, len(allowed)+len(extra))
	out = append(out, allowed...)
	for _, name := range extra {
		if !seen[name] {
			out = append(out, name)
			seen[name] = true
		}
	}
	return out
}
