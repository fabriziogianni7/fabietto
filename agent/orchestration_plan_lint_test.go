package agent

import (
	"testing"

	"custom-agent/agent/planning"
	"custom-agent/tools"
)

func TestMergeOrchestrationToolAdditions(t *testing.T) {
	basePlan := func() *planning.OrchestrationPlan {
		return &planning.OrchestrationPlan{
			Goal: "g",
			Steps: []planning.OrchestrationStep{
				{
					ID:           "s1",
					Description:  "search",
					AllowedTools: []string{"web_search"},
					Risk:         planning.StepRiskReadOnly,
				},
				{
					ID:           "s2",
					Description:  "read",
					AllowedTools: []string{"read_file"},
					Risk:         planning.StepRiskReadOnly,
				},
			},
		}
	}

	t.Run("duplicates ignored", func(t *testing.T) {
		p := basePlan()
		got, err := mergeOrchestrationToolAdditions(p, []toolAddition{
			{StepID: "s1", Tools: []string{"web_search", " web_search "}},
		}, false)
		if err != nil {
			t.Fatal(err)
		}
		if len(got.Steps[0].AllowedTools) != 1 || got.Steps[0].AllowedTools[0] != "web_search" {
			t.Fatalf("s1 tools: %v", got.Steps[0].AllowedTools)
		}
	})

	t.Run("unknown tool skipped", func(t *testing.T) {
		p := basePlan()
		got, err := mergeOrchestrationToolAdditions(p, []toolAddition{
			{StepID: "s1", Tools: []string{"not_a_registered_tool_xyz", "read_memory"}},
		}, false)
		if err != nil {
			t.Fatal(err)
		}
		want := []string{"web_search", "read_memory"}
		if len(got.Steps[0].AllowedTools) != len(want) {
			t.Fatalf("got %v want %v", got.Steps[0].AllowedTools, want)
		}
		for i, w := range want {
			if got.Steps[0].AllowedTools[i] != w {
				t.Fatalf("got %v want %v", got.Steps[0].AllowedTools, want)
			}
		}
	})

	t.Run("wallet tool skipped without wallet", func(t *testing.T) {
		p := basePlan()
		got, err := mergeOrchestrationToolAdditions(p, []toolAddition{
			{StepID: "s1", Tools: []string{"wallet_get_balance", "read_memory"}},
		}, false)
		if err != nil {
			t.Fatal(err)
		}
		want := []string{"web_search", "read_memory"}
		if len(got.Steps[0].AllowedTools) != len(want) {
			t.Fatalf("got %v want %v", got.Steps[0].AllowedTools, want)
		}
		for i, w := range want {
			if got.Steps[0].AllowedTools[i] != w {
				t.Fatalf("got %v want %v", got.Steps[0].AllowedTools, want)
			}
		}
	})

	t.Run("successful merge validates", func(t *testing.T) {
		p := basePlan()
		got, err := mergeOrchestrationToolAdditions(p, []toolAddition{
			{StepID: "s2", Tools: []string{"read_memory", "list_skills"}},
		}, false)
		if err != nil {
			t.Fatal(err)
		}
		if err := planning.ValidateOrchestrationPlan(got, tools.IsRegisteredTool); err != nil {
			t.Fatal(err)
		}
		toolsSet := map[string]bool{}
		for _, n := range got.Steps[1].AllowedTools {
			toolsSet[n] = true
		}
		if !toolsSet["read_file"] || !toolsSet["read_memory"] || !toolsSet["list_skills"] {
			t.Fatalf("s2 tools: %v", got.Steps[1].AllowedTools)
		}
	})

	t.Run("spawn_subagents skipped", func(t *testing.T) {
		p := basePlan()
		got, err := mergeOrchestrationToolAdditions(p, []toolAddition{
			{StepID: "s1", Tools: []string{"spawn_subagents", "read_memory"}},
		}, false)
		if err != nil {
			t.Fatal(err)
		}
		if len(got.Steps[0].AllowedTools) != 2 {
			t.Fatalf("got %v", got.Steps[0].AllowedTools)
		}
	})

	t.Run("nil plan errors", func(t *testing.T) {
		if _, err := mergeOrchestrationToolAdditions(nil, nil, true); err == nil {
			t.Fatal("expected error")
		}
	})
}
