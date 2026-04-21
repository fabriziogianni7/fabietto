package planning

import (
	"encoding/json"
	"fmt"
	"strings"
)

// Capability identifies a high-level planning domain (e.g. wallet contract interactions).
type Capability string

const (
	CapabilityWalletTx Capability = "wallet.tx"
)

// StepStatus is the lifecycle of a single plan step.
type StepStatus string

const (
	StepPending   StepStatus = "pending"
	StepRunning   StepStatus = "running"
	StepSucceeded StepStatus = "succeeded"
	StepFailed    StepStatus = "failed"
	StepSkipped   StepStatus = "skipped"
)

// StepType is a semantic label for wallet and future plugins.
type StepType string

const (
	StepParseIntent      StepType = "parse_intent"
	StepPolicyCheck      StepType = "policy_check"
	StepQuoteRoute       StepType = "quote_route"
	StepBuildTransaction StepType = "build_transaction"
	StepSimulate         StepType = "simulate"
	StepApprovalGate     StepType = "approval_gate"
	StepSignSend         StepType = "sign_send"
	StepVerifyReceipt    StepType = "verify_receipt"
)

// Plan is an ordered execution graph for one user turn.
type Plan struct {
	ID          string            `json:"id"`
	Goal        string            `json:"goal"`
	Capability  Capability        `json:"capability"`
	Constraints map[string]string `json:"constraints,omitempty"`
	Steps       []Step            `json:"steps"`
}

// Step is one atomic unit of work with optional tool allowlist.
type Step struct {
	ID           string     `json:"id"`
	Type         StepType   `json:"type"`
	Description  string     `json:"description,omitempty"`
	AllowedTools []string   `json:"allowed_tools,omitempty"`
	Status       StepStatus `json:"status"`
	Output       string     `json:"output,omitempty"`
	Error        string     `json:"error,omitempty"`
}

// Planner mode (wallet contract pipeline). See PLANNER_MODE env.
const (
	PlannerModeOff          = "off"
	PlannerModeWallet       = "wallet"        // swap/contract-call keywords only (legacy)
	PlannerModeAuto         = "auto"          // heuristics + optional router for ambiguity
	PlannerModeAlwaysWallet = "always_wallet" // try planner on every message (except native send path)
)

// Orchestration modes (ORCHESTRATION_MODE). General multi-tool planner.
const (
	OrchestrationModeOff    = "off"
	OrchestrationModeAuto   = "auto"   // router/heuristic — use tryGeneralOrchestration when likely multi-step
	OrchestrationModeAlways = "always" // always try orchestration first (fallback to reactive)
)

// OrchestrationRuntime configures the general orchestration layer.
type OrchestrationRuntime struct {
	Mode string // OrchestrationMode* or ""
}

// RuntimeConfig is planner enablement passed from main config.
type RuntimeConfig struct {
	Enabled       bool
	Mode          string          // PlannerMode* or ""
	Capabilities  map[string]bool // e.g. "wallet" -> true
	Orchestration OrchestrationRuntime
}

// ParseCapabilities parses a comma-separated list (e.g. "wallet,files").
func ParseCapabilities(s string) map[string]bool {
	out := make(map[string]bool)
	for _, p := range strings.Split(s, ",") {
		p = strings.TrimSpace(strings.ToLower(p))
		if p != "" {
			out[p] = true
		}
	}
	return out
}

// CapabilityEnabled returns true if name is listed (e.g. "wallet").
func (c RuntimeConfig) CapabilityEnabled(name string) bool {
	if !c.Enabled {
		return false
	}
	name = strings.TrimSpace(strings.ToLower(name))
	return c.Capabilities[name]
}

// ValidatePlan checks structural invariants.
func ValidatePlan(p *Plan) error {
	if p == nil {
		return fmt.Errorf("plan is nil")
	}
	if strings.TrimSpace(p.Goal) == "" {
		return fmt.Errorf("plan goal is empty")
	}
	if p.Capability == "" {
		return fmt.Errorf("plan capability is empty")
	}
	if len(p.Steps) == 0 {
		return fmt.Errorf("plan has no steps")
	}
	if len(p.Steps) > maxPlanSteps {
		return fmt.Errorf("plan exceeds max steps (%d)", maxPlanSteps)
	}
	seen := make(map[string]bool)
	for _, s := range p.Steps {
		if strings.TrimSpace(s.ID) == "" {
			return fmt.Errorf("step id is empty")
		}
		if seen[s.ID] {
			return fmt.Errorf("duplicate step id: %s", s.ID)
		}
		seen[s.ID] = true
	}
	return nil
}

const maxPlanSteps = 32

// PlanFromJSON parses and validates a plan JSON payload.
func PlanFromJSON(data []byte) (*Plan, error) {
	var p Plan
	if err := json.Unmarshal(data, &p); err != nil {
		return nil, fmt.Errorf("plan json: %w", err)
	}
	if err := ValidatePlan(&p); err != nil {
		return nil, err
	}
	return &p, nil
}
