package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"strings"
	"time"

	"custom-agent/agent/planning"
	"custom-agent/tools"

	"github.com/sashabaranov/go-openai"
)

// toolAddition is one LLM-suggested batch of tools for a step.
type toolAddition struct {
	StepID string   `json:"step_id"`
	Tools  []string `json:"tools"`
}

type orchestrationPlanLintResponse struct {
	Additions []toolAddition `json:"additions"`
}

type orchestrationPlanLintPayload struct {
	Goal  string                      `json:"goal"`
	Steps []orchestrationPlanLintStep `json:"steps"`
}

type orchestrationPlanLintStep struct {
	ID           string   `json:"id"`
	Description  string   `json:"description"`
	AllowedTools []string `json:"allowed_tools"`
	DependsOn    []string `json:"depends_on"`
	Risk         string   `json:"risk"`
}

func orchestrationPlanForLintJSON(plan *planning.OrchestrationPlan) ([]byte, error) {
	if plan == nil {
		return nil, fmt.Errorf("plan is nil")
	}
	payload := orchestrationPlanLintPayload{
		Goal:  plan.Goal,
		Steps: make([]orchestrationPlanLintStep, 0, len(plan.Steps)),
	}
	for _, s := range plan.Steps {
		payload.Steps = append(payload.Steps, orchestrationPlanLintStep{
			ID:           s.ID,
			Description:  s.Description,
			AllowedTools: append([]string(nil), s.AllowedTools...),
			DependsOn:    append([]string(nil), s.DependsOn...),
			Risk:         string(s.Risk),
		})
	}
	return json.Marshal(payload)
}

func cloneOrchestrationPlan(p *planning.OrchestrationPlan) *planning.OrchestrationPlan {
	if p == nil {
		return nil
	}
	out := &planning.OrchestrationPlan{
		Goal:  p.Goal,
		Steps: make([]planning.OrchestrationStep, len(p.Steps)),
	}
	for i, s := range p.Steps {
		out.Steps[i] = planning.OrchestrationStep{
			ID:           s.ID,
			Description:  s.Description,
			Risk:         s.Risk,
			AllowedTools: append([]string(nil), s.AllowedTools...),
			DependsOn:    append([]string(nil), s.DependsOn...),
		}
	}
	return out
}

// mergeOrchestrationToolAdditions appends registered, non-duplicate tools from additions onto
// matching steps. Unknown tools, spawn_subagents, and wallet_* tools when walletConfigured is false are skipped.
func mergeOrchestrationToolAdditions(plan *planning.OrchestrationPlan, additions []toolAddition, walletConfigured bool) (*planning.OrchestrationPlan, error) {
	if plan == nil {
		return nil, fmt.Errorf("plan is nil")
	}
	stepIdx := make(map[string]int, len(plan.Steps))
	for i, s := range plan.Steps {
		stepIdx[s.ID] = i
	}
	for _, add := range additions {
		idx, ok := stepIdx[add.StepID]
		if !ok {
			continue
		}
		seen := make(map[string]bool, len(plan.Steps[idx].AllowedTools))
		for _, existing := range plan.Steps[idx].AllowedTools {
			seen[strings.TrimSpace(existing)] = true
		}
		for _, raw := range add.Tools {
			name := strings.TrimSpace(raw)
			if name == "" || name == "spawn_subagents" {
				continue
			}
			if !tools.IsRegisteredTool(name) {
				continue
			}
			if strings.HasPrefix(name, "wallet_") && !walletConfigured {
				continue
			}
			if seen[name] {
				continue
			}
			plan.Steps[idx].AllowedTools = append(plan.Steps[idx].AllowedTools, name)
			seen[name] = true
		}
	}
	return plan, nil
}

// lintOrchestrationPlanAllowedTools asks a small model for missing allowed_tools implied by each step,
// merges suggestions when validation passes, and returns the same plan pointer (unchanged on error or if nothing valid).
func (a *Agent) lintOrchestrationPlanAllowedTools(ctx context.Context, plan *planning.OrchestrationPlan, userText string, contextPrefix []openai.ChatCompletionMessage) *planning.OrchestrationPlan {
	if plan == nil {
		return plan
	}
	mode := strings.ToLower(strings.TrimSpace(a.orchestration.Mode))
	if mode == "" || mode == planning.OrchestrationModeOff {
		return plan
	}
	if a.tools == nil {
		return plan
	}

	sys := `You review an orchestration plan. Output ONLY valid JSON, no markdown.
Shape: {"additions":[{"step_id":"s1","tools":["tool_name",...]}]}

Rules:
- Suggest ONLY tools that are MISSING from that step's allowed_tools but are clearly implied by the step description/goal for that step.
- Every tool name MUST be a valid registered tool name for this assistant. Never invent names.
- Never include "spawn_subagents".
- If a suggested tool is already in allowed_tools for that step, omit it.
- If nothing is missing, return {"additions":[]}.`

	planJSON, err := orchestrationPlanForLintJSON(plan)
	if err != nil {
		log.Printf("[orchestration] plan lint: marshal plan: %v", err)
		return plan
	}
	userBody := "User request:\n" + userText + "\n\nPlan JSON:\n" + string(planJSON)

	msgs := make([]openai.ChatCompletionMessage, 0, 2+len(contextPrefix))
	msgs = append(msgs, openai.ChatCompletionMessage{Role: openai.ChatMessageRoleSystem, Content: sys})
	msgs = append(msgs, contextPrefix...)
	msgs = append(msgs, openai.ChatCompletionMessage{Role: openai.ChatMessageRoleUser, Content: userBody})

	req := openai.ChatCompletionRequest{
		Model:    subagentModelForIndex(0),
		Messages: msgs,
	}
	t0 := time.Now()
	resp, err := a.client.CreateChatCompletion(ctx, req)
	a.recordLLMRound(ctx, "orchestration_plan_lint", 0, req, resp, err, t0)
	if err != nil {
		log.Printf("[orchestration] plan lint LLM: %v", err)
		return plan
	}
	if len(resp.Choices) == 0 {
		return plan
	}
	raw := extractJSONObject(strings.TrimSpace(resp.Choices[0].Message.Content))
	var parsed orchestrationPlanLintResponse
	if err := json.Unmarshal([]byte(raw), &parsed); err != nil {
		log.Printf("[orchestration] plan lint parse: %v", err)
		return plan
	}

	trial := cloneOrchestrationPlan(plan)
	walletOK := a.tools.Wallet != nil
	if _, err := mergeOrchestrationToolAdditions(trial, parsed.Additions, walletOK); err != nil {
		log.Printf("[orchestration] plan lint merge: %v", err)
		return plan
	}
	if err := planning.ValidateOrchestrationPlan(trial, tools.IsRegisteredTool); err != nil {
		log.Printf("[orchestration] plan lint validate plan: %v", err)
		return plan
	}
	if err := planning.ValidateOrchestrationPlanPolicy(trial, a.tools); err != nil {
		log.Printf("[orchestration] plan lint validate policy: %v", err)
		return plan
	}

	for i := range plan.Steps {
		plan.Steps[i].AllowedTools = append([]string(nil), trial.Steps[i].AllowedTools...)
	}
	return plan
}
