package planning

import (
	"encoding/json"
	"fmt"
	"strings"

	"custom-agent/tools"
)

// StepRisk categorizes orchestration steps for validation policy.
type StepRisk string

const (
	StepRiskReadOnly StepRisk = "read_only"
	StepRiskMutating StepRisk = "mutating"
	StepRiskWallet   StepRisk = "wallet"
)

// OrchestrationStep is one step in a tool-agnostic plan.
type OrchestrationStep struct {
	ID           string   `json:"id"`
	Description  string   `json:"description"`
	AllowedTools []string `json:"allowed_tools"`
	DependsOn    []string `json:"depends_on,omitempty"`
	Risk         StepRisk `json:"risk,omitempty"`
}

// OrchestrationPlan is a multi-step plan using only allowed tools per step.
type OrchestrationPlan struct {
	Goal  string              `json:"goal"`
	Steps []OrchestrationStep `json:"steps"`
}

const (
	MaxOrchestrationSteps = 12
)

// ValidateOrchestrationPlan checks structure, depends_on DAG (acyclic), and tool names.
func ValidateOrchestrationPlan(p *OrchestrationPlan, knownTool func(string) bool) error {
	if p == nil {
		return fmt.Errorf("plan is nil")
	}
	if strings.TrimSpace(p.Goal) == "" {
		return fmt.Errorf("goal is empty")
	}
	if len(p.Steps) == 0 {
		return fmt.Errorf("no steps")
	}
	if len(p.Steps) > MaxOrchestrationSteps {
		return fmt.Errorf("too many steps (max %d)", MaxOrchestrationSteps)
	}
	ids := make(map[string]bool)
	for _, s := range p.Steps {
		if strings.TrimSpace(s.ID) == "" {
			return fmt.Errorf("step id is empty")
		}
		if ids[s.ID] {
			return fmt.Errorf("duplicate step id: %s", s.ID)
		}
		ids[s.ID] = true
		if len(s.AllowedTools) == 0 {
			return fmt.Errorf("step %s has no allowed_tools", s.ID)
		}
		for _, t := range s.AllowedTools {
			t = strings.TrimSpace(t)
			if t == "" {
				return fmt.Errorf("step %s has empty tool name", s.ID)
			}
			if knownTool != nil && !knownTool(t) {
				return fmt.Errorf("step %s: unknown tool %q", s.ID, t)
			}
		}
		for _, d := range s.DependsOn {
			if strings.TrimSpace(d) == "" {
				return fmt.Errorf("step %s: empty depends_on", s.ID)
			}
		}
	}
	if _, err := OrchestrationExecutionOrder(p.Steps); err != nil {
		return err
	}
	return nil
}

// OrchestrationExecutionOrder returns the same steps in a valid topological order (dependencies
// before dependents). When several steps are ready (in-degree 0), the one with the lower
// original index in the input slice is chosen first. Unknown step ids in depends_on, duplicate
// step ids, empty ids, or a cycle produce an error.
func OrchestrationExecutionOrder(steps []OrchestrationStep) ([]OrchestrationStep, error) {
	if len(steps) == 0 {
		return nil, fmt.Errorf("no steps")
	}
	idToIndex := make(map[string]int, len(steps))
	for i, s := range steps {
		if strings.TrimSpace(s.ID) == "" {
			return nil, fmt.Errorf("step id is empty")
		}
		if _, dup := idToIndex[s.ID]; dup {
			return nil, fmt.Errorf("duplicate step id: %s", s.ID)
		}
		idToIndex[s.ID] = i
	}

	inDegree := make(map[string]int, len(steps))
	adj := make(map[string][]string, len(steps))
	for _, s := range steps {
		inDegree[s.ID] = 0
	}
	for _, s := range steps {
		seenDep := make(map[string]bool)
		for _, d := range s.DependsOn {
			d = strings.TrimSpace(d)
			if d == "" {
				return nil, fmt.Errorf("step %s: empty depends_on", s.ID)
			}
			if _, ok := idToIndex[d]; !ok {
				return nil, fmt.Errorf("step %s depends on unknown id %q", s.ID, d)
			}
			if seenDep[d] {
				continue
			}
			seenDep[d] = true
			inDegree[s.ID]++
			adj[d] = append(adj[d], s.ID)
		}
	}

	n := len(steps)
	result := make([]OrchestrationStep, 0, n)
	placed := make(map[string]bool, n)
	for len(result) < n {
		bestIdx := -1
		for i, s := range steps {
			if placed[s.ID] {
				continue
			}
			if inDegree[s.ID] != 0 {
				continue
			}
			if bestIdx == -1 || i < bestIdx {
				bestIdx = i
			}
		}
		if bestIdx == -1 {
			return nil, fmt.Errorf("orchestration plan has a cyclic depends_on graph")
		}
		st := steps[bestIdx]
		placed[st.ID] = true
		result = append(result, st)
		for _, v := range adj[st.ID] {
			inDegree[v]--
		}
	}
	return result, nil
}

// OrchestrationPlanFromJSON parses JSON and validates.
func OrchestrationPlanFromJSON(data []byte, knownTool func(string) bool) (*OrchestrationPlan, error) {
	var p OrchestrationPlan
	if err := json.Unmarshal(data, &p); err != nil {
		return nil, fmt.Errorf("orchestration json: %w", err)
	}
	if err := ValidateOrchestrationPlan(&p, knownTool); err != nil {
		return nil, err
	}
	return &p, nil
}

// ValidateOrchestrationPlanPolicy enforces wallet availability and blocked tools.
func ValidateOrchestrationPlanPolicy(p *OrchestrationPlan, t *tools.Tools) error {
	if err := ValidateOrchestrationPlan(p, tools.IsRegisteredTool); err != nil {
		return err
	}
	for _, s := range p.Steps {
		for _, name := range s.AllowedTools {
			name = strings.TrimSpace(name)
			if name == "spawn_subagents" {
				return fmt.Errorf("spawn_subagents not allowed in orchestration")
			}
			if strings.HasPrefix(name, "wallet_") && (t == nil || t.Wallet == nil) {
				return fmt.Errorf("step %s references wallet tool %q but wallet is not configured", s.ID, name)
			}
		}
	}
	return nil
}
