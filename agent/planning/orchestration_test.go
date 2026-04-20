package planning

import (
	"testing"

	"custom-agent/tools"
)

func TestValidateOrchestrationPlan_OK(t *testing.T) {
	p := &OrchestrationPlan{
		Goal: "test",
		Steps: []OrchestrationStep{
			{ID: "s1", Description: "search", AllowedTools: []string{"web_search"}, Risk: StepRiskReadOnly},
			{ID: "s2", Description: "read", AllowedTools: []string{"read_file"}, DependsOn: []string{"s1"}, Risk: StepRiskReadOnly},
		},
	}
	if err := ValidateOrchestrationPlan(p, tools.IsRegisteredTool); err != nil {
		t.Fatal(err)
	}
}

func TestValidateOrchestrationPlan_CycleOrder(t *testing.T) {
	p := &OrchestrationPlan{
		Goal: "x",
		Steps: []OrchestrationStep{
			{ID: "s1", Description: "a", AllowedTools: []string{"web_search"}},
			{ID: "s2", Description: "b", AllowedTools: []string{"read_file"}, DependsOn: []string{"s1"}},
		},
	}
	if err := ValidateOrchestrationPlan(p, tools.IsRegisteredTool); err != nil {
		t.Fatal(err)
	}
	// s1 depends on s2 (later step) — invalid
	p2 := &OrchestrationPlan{
		Goal: "x",
		Steps: []OrchestrationStep{
			{ID: "s1", Description: "a", AllowedTools: []string{"web_search"}, DependsOn: []string{"s2"}},
			{ID: "s2", Description: "b", AllowedTools: []string{"read_file"}},
		},
	}
	if err := ValidateOrchestrationPlan(p2, tools.IsRegisteredTool); err == nil {
		t.Fatal("expected error for forward depends_on")
	}
}

func TestValidateOrchestrationPlanPolicy_SpawnBlocked(t *testing.T) {
	p := &OrchestrationPlan{
		Goal: "x",
		Steps: []OrchestrationStep{
			{ID: "s1", Description: "x", AllowedTools: []string{"spawn_subagents"}},
		},
	}
	if err := ValidateOrchestrationPlanPolicy(p, nil); err == nil {
		t.Fatal("expected spawn blocked")
	}
}
