package planning

import (
	"context"
	"fmt"
)

// Executor runs a validated plan step-by-step.
type Executor interface {
	Execute(ctx context.Context, p *Plan) (*ExecutionResult, error)
}

// ExecutionResult aggregates step outcomes.
type ExecutionResult struct {
	PlanID       string
	FinalReply   string
	StepResults  []StepResult
	StoppedEarly bool
}

// StepResult records one step completion.
type StepResult struct {
	StepID  string
	Status  StepStatus
	Output  string
	ErrText string
}

// MarkStepFailed returns a standard failed StepResult.
func MarkStepFailed(stepID, errText string) StepResult {
	return StepResult{StepID: stepID, Status: StepFailed, ErrText: errText}
}

// MarkStepOK returns a succeeded StepResult.
func MarkStepOK(stepID, output string) StepResult {
	return StepResult{StepID: stepID, Status: StepSucceeded, Output: output}
}

// ErrAbort is returned when execution must stop (e.g. approval pending).
var ErrAbort = fmt.Errorf("plan execution aborted")
