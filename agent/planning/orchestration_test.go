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
	// s1 depends on s2 (listed later in JSON) — allowed; graph is s2 -> s1
	p2 := &OrchestrationPlan{
		Goal: "x",
		Steps: []OrchestrationStep{
			{ID: "s1", Description: "a", AllowedTools: []string{"web_search"}, DependsOn: []string{"s2"}},
			{ID: "s2", Description: "b", AllowedTools: []string{"read_file"}},
		},
	}
	if err := ValidateOrchestrationPlan(p2, tools.IsRegisteredTool); err != nil {
		t.Fatal(err)
	}
}

func TestOrchestrationExecutionOrder_ReverseListDeps(t *testing.T) {
	steps := []OrchestrationStep{
		{ID: "s2", Description: "b", AllowedTools: []string{"read_file"}, DependsOn: []string{"s1"}},
		{ID: "s1", Description: "a", AllowedTools: []string{"web_search"}},
	}
	if err := ValidateOrchestrationPlan(&OrchestrationPlan{Goal: "x", Steps: steps}, tools.IsRegisteredTool); err != nil {
		t.Fatal(err)
	}
	ordered, err := OrchestrationExecutionOrder(steps)
	if err != nil {
		t.Fatal(err)
	}
	if len(ordered) != 2 || ordered[0].ID != "s1" || ordered[1].ID != "s2" {
		t.Fatalf("got %v, want s1 then s2", ordered)
	}
}

func TestValidateOrchestrationPlan_CycleDetected(t *testing.T) {
	p := &OrchestrationPlan{
		Goal: "x",
		Steps: []OrchestrationStep{
			{ID: "s1", Description: "a", AllowedTools: []string{"web_search"}, DependsOn: []string{"s2"}},
			{ID: "s2", Description: "b", AllowedTools: []string{"read_file"}, DependsOn: []string{"s1"}},
		},
	}
	if err := ValidateOrchestrationPlan(p, tools.IsRegisteredTool); err == nil {
		t.Fatal("expected cycle error from validation")
	}
	_, err := OrchestrationExecutionOrder(p.Steps)
	if err == nil {
		t.Fatal("expected cycle error from OrchestrationExecutionOrder")
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
