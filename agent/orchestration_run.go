package agent

import (
	"context"
	"fmt"
	"log"
	"strings"

	"custom-agent/agent/planning"
	"custom-agent/gateway"
	"custom-agent/session"
	"custom-agent/tools"

	"github.com/sashabaranov/go-openai"
)

const maxOrchestrationStepRounds = 5

// tryGeneralOrchestration builds a plan via LLM, validates, executes steps, optional synthesis.
// Returns ("", false) to use the reactive loop.
func (a *Agent) tryGeneralOrchestration(ctx context.Context, msg gateway.IncomingMessage, userText string) (string, bool) {
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
Rules:
- 1 to 12 steps. Each step must list ONLY tools needed for that step from the allowed set (subset of real tool names).
- Order steps so dependencies make sense. depends_on lists step ids that must complete before this step (earlier ids only).
- For read-only research use: web_search, read_file, read_memory, http_request, list_skills, read_skill, read_skill_script.
- For mutating filesystem: write_file, run_command (only if needed).
- For wallet: wallet_get_balance, wallet_list_transactions, wallet_execute_transfer, wallet_execute_contract_call — only if user needs on-chain action.
- risk: use "wallet" for steps that send transactions; "mutating" for writes/commands; "read_only" otherwise.
- Do not include spawn_subagents in plans.
If the task is a single trivial chat with no tools, output: {"goal":"","steps":[]}`

	resp, err := a.client.CreateChatCompletion(ctx, openai.ChatCompletionRequest{
		Model: parentModel,
		Messages: []openai.ChatCompletionMessage{
			{Role: openai.ChatMessageRoleSystem, Content: sys},
			{Role: openai.ChatMessageRoleUser, Content: userText},
		},
	})
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

	if a.telemetry != nil {
		a.telemetry.RecordPlanEvent(ctx, "orchestration_plan_start", map[string]interface{}{
			"goal":       plan.Goal,
			"step_count": len(plan.Steps),
		})
	}

	final, err := a.runOrchestrationPlan(ctx, msg, userText, plan)
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

func (a *Agent) runOrchestrationPlan(ctx context.Context, msg gateway.IncomingMessage, userText string, plan *planning.OrchestrationPlan) (string, error) {
	stepOut := make(map[string]string)

	for i, st := range plan.Steps {
		if a.telemetry != nil {
			a.telemetry.RecordPlanEvent(ctx, "orchestration_step_start", map[string]interface{}{
				"step_id": st.ID,
				"index":   i,
			})
		}
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
			if t == "wallet_execute_contract_call" {
				walletPipe = a.tools.Wallet != nil
				break
			}
		}

		sys := "You execute ONE step of a multi-step plan. Use only the provided tools. Call tools as needed.\n"
		sys += "Overall user goal: " + plan.Goal + "\n"
		sys += "This step (" + st.ID + "): " + st.Description + "\n"
		if prior.Len() > 0 {
			sys += "Context from previous steps:\n" + prior.String()
		}
		sys += "\nRespond with tool calls until this step is done, then a short text summary of what was learned or done for this step."

		msgs := []openai.ChatCompletionMessage{
			{Role: openai.ChatMessageRoleSystem, Content: sys},
			{Role: openai.ChatMessageRoleUser, Content: "Original request:\n" + userText},
		}

		var stepBuf strings.Builder
		var lastToolOut string
		for round := 0; round < maxOrchestrationStepRounds; round++ {
			resp, err := a.client.CreateChatCompletion(ctx, openai.ChatCompletionRequest{
				Model:    parentModel,
				Messages: msgs,
				Tools:    toolDefs,
			})
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
					msgs = append(msgs, openai.ChatCompletionMessage{
						Role:       openai.ChatMessageRoleTool,
						Content:    res,
						ToolCallID: tc.ID,
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
				continue
			}
			txt := strings.TrimSpace(mr.Content)
			if txt != "" {
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
	for _, st := range plan.Steps {
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

	synResp, err := a.client.CreateChatCompletion(ctx, openai.ChatCompletionRequest{
		Model: parentModel,
		Messages: []openai.ChatCompletionMessage{
			{Role: openai.ChatMessageRoleSystem, Content: "Summarize the following step results into a clear, concise answer for the user. Do not invent facts."},
			{Role: openai.ChatMessageRoleUser, Content: "User request:\n" + userText + "\n\nStep results:\n" + summary},
		},
	})
	if err != nil || len(synResp.Choices) == 0 {
		return summary, nil
	}
	final := strings.TrimSpace(synResp.Choices[0].Message.Content)
	if final == "" {
		return summary, nil
	}
	return final, nil
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
