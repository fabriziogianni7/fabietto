package agent

import (
	"testing"

	"custom-agent/agent/planning"
	"custom-agent/tools"
)

func TestReactiveWalletContractPipeline(t *testing.T) {
	ts := tools.NewTools("", nil)
	a := &Agent{tools: ts}
	if a.reactiveWalletContractPipeline() {
		t.Fatal("no wallet: want false")
	}

	ts.SetWallet(&mockWalletService{})
	if a.reactiveWalletContractPipeline() {
		t.Fatal("planner off: want false")
	}

	a.SetPlanner(planning.RuntimeConfig{
		Enabled:      true,
		Mode:         planning.PlannerModeAuto,
		Capabilities: map[string]bool{"wallet": true},
	})
	if !a.reactiveWalletContractPipeline() {
		t.Fatal("planner on + wallet cap: want true")
	}

	a.SetPlanner(planning.RuntimeConfig{
		Enabled:      true,
		Mode:         planning.PlannerModeAuto,
		Capabilities: map[string]bool{"files": true},
	})
	if a.reactiveWalletContractPipeline() {
		t.Fatal("wallet capability missing: want false")
	}
}
