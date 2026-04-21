package planning

import (
	"fmt"
	"strings"
)

// Validator performs deterministic checks on a plan before execution.
type Validator struct {
	allowedCapabilities map[Capability]bool
}

// NewValidator creates a validator that only accepts listed capabilities.
func NewValidator(allowed ...Capability) *Validator {
	m := make(map[Capability]bool)
	for _, c := range allowed {
		m[c] = true
	}
	return &Validator{allowedCapabilities: m}
}

// Validate checks capability allowlist and structural rules.
func (v *Validator) Validate(p *Plan) error {
	if err := ValidatePlan(p); err != nil {
		return err
	}
	if v != nil && len(v.allowedCapabilities) > 0 {
		if !v.allowedCapabilities[p.Capability] {
			return fmt.Errorf("capability %q not allowed for this validator", p.Capability)
		}
	}
	for _, s := range p.Steps {
		if err := validateStepTools(s); err != nil {
			return err
		}
	}
	return nil
}

func validateStepTools(s Step) error {
	for _, t := range s.AllowedTools {
		t = strings.TrimSpace(t)
		if t == "" {
			return fmt.Errorf("step %s has empty allowed tool entry", s.ID)
		}
	}
	return nil
}
