package planning

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	"custom-agent/tools"
)

// WalletExecutor runs wallet.tx plans using WalletService (deterministic steps).
type WalletExecutor struct {
	Wallet tools.WalletService
}

// Execute implements Executor.
func (e *WalletExecutor) Execute(ctx context.Context, p *Plan) (*ExecutionResult, error) {
	if e == nil || e.Wallet == nil {
		return nil, fmt.Errorf("wallet executor: no wallet")
	}
	if p.Capability != CapabilityWalletTx {
		return nil, fmt.Errorf("wallet executor: wrong capability %q", p.Capability)
	}
	if err := ValidateWalletConstraints(p.Constraints); err != nil {
		return nil, err
	}

	chainID := int64(0)
	if s := strings.TrimSpace(p.Constraints["chain_id"]); s != "" {
		n, err := strconv.ParseInt(s, 10, 64)
		if err != nil || n <= 0 {
			return nil, fmt.Errorf("invalid chain_id")
		}
		chainID = n
	}
	to := strings.TrimSpace(p.Constraints["to"])
	data := strings.TrimSpace(p.Constraints["data"])
	valueWei := strings.TrimSpace(p.Constraints["value_wei"])
	if valueWei == "" {
		valueWei = "0"
	}

	var results []StepResult
	platform := p.Constraints["_platform"]
	userID := p.Constraints["_user_id"]
	chatID := p.Constraints["_chat_id"]

	txAlreadySent := false

	for i := range p.Steps {
		s := &p.Steps[i]
		switch s.Type {
		case StepParseIntent:
			s.Status = StepSucceeded
			cid := chainID
			if cid <= 0 {
				cid = e.Wallet.DefaultChainID()
			}
			s.Output = fmt.Sprintf("to=%s chain_id=%d value_wei=%s calldata_chars=%d", to, cid, valueWei, len(data))
			results = append(results, MarkStepOK(s.ID, s.Output))

		case StepPolicyCheck:
			method, decision, gas, err := e.Wallet.WalletPreviewContract(ctx, chainID, to, data, valueWei)
			if err != nil {
				s.Status = StepFailed
				s.Error = err.Error()
				results = append(results, MarkStepFailed(s.ID, s.Error))
				return &ExecutionResult{PlanID: p.ID, StepResults: results, StoppedEarly: true, FinalReply: "Policy preview failed: " + s.Error}, nil
			}
			if decision == "deny" {
				s.Status = StepFailed
				s.Error = "transaction denied by policy"
				results = append(results, MarkStepFailed(s.ID, s.Error))
				return &ExecutionResult{PlanID: p.ID, StepResults: results, StoppedEarly: true, FinalReply: "Error: transaction denied by policy (method: " + method + ")."}, nil
			}
			s.Status = StepSucceeded
			s.Output = fmt.Sprintf("method=%s decision=%s gas_limit=%d", method, decision, gas)
			results = append(results, MarkStepOK(s.ID, s.Output))

		case StepQuoteRoute:
			s.Status = StepSkipped
			s.Output = "no external router in MVP"
			results = append(results, StepResult{StepID: s.ID, Status: StepSkipped, Output: s.Output})

		case StepBuildTransaction:
			s.Status = StepSucceeded
			s.Output = "calldata ready"
			results = append(results, MarkStepOK(s.ID, s.Output))

		case StepSimulate:
			if err := e.Wallet.WalletSimulateContract(ctx, chainID, to, data, valueWei); err != nil {
				s.Status = StepFailed
				s.Error = err.Error()
				results = append(results, MarkStepFailed(s.ID, s.Error))
				return &ExecutionResult{PlanID: p.ID, StepResults: results, StoppedEarly: true, FinalReply: "Simulation (eth_call) failed: " + s.Error}, nil
			}
			s.Status = StepSucceeded
			s.Output = "eth_call ok"
			results = append(results, MarkStepOK(s.ID, s.Output))

		case StepApprovalGate:
			out, err := e.Wallet.ExecuteContractCall(ctx, chainID, to, data, valueWei, platform, userID, chatID)
			if err != nil {
				s.Status = StepFailed
				s.Error = err.Error()
				results = append(results, MarkStepFailed(s.ID, s.Error))
				return &ExecutionResult{PlanID: p.ID, StepResults: results, StoppedEarly: true, FinalReply: out}, nil
			}
			s.Status = StepSucceeded
			s.Output = out
			results = append(results, MarkStepOK(s.ID, out))
			if strings.Contains(out, "Approval required") {
				return &ExecutionResult{PlanID: p.ID, StepResults: results, StoppedEarly: true, FinalReply: out}, nil
			}
			if strings.Contains(out, "Transaction sent") {
				txAlreadySent = true
			}

		case StepSignSend:
			if txAlreadySent {
				s.Status = StepSkipped
				s.Output = "already broadcast in prior step"
				results = append(results, StepResult{StepID: s.ID, Status: StepSkipped, Output: s.Output})
				break
			}
			out, err := e.Wallet.ExecuteContractCall(ctx, chainID, to, data, valueWei, platform, userID, chatID)
			if err != nil {
				s.Status = StepFailed
				s.Error = err.Error()
				results = append(results, MarkStepFailed(s.ID, s.Error))
				return &ExecutionResult{PlanID: p.ID, StepResults: results, StoppedEarly: true, FinalReply: out}, nil
			}
			s.Status = StepSucceeded
			s.Output = out
			results = append(results, MarkStepOK(s.ID, out))
			if strings.Contains(out, "Approval required") {
				return &ExecutionResult{PlanID: p.ID, StepResults: results, StoppedEarly: true, FinalReply: out}, nil
			}

		case StepVerifyReceipt:
			hash := extractTxHash(results)
			if hash == "" {
				s.Status = StepSkipped
				s.Output = "no tx hash in prior steps"
				results = append(results, StepResult{StepID: s.ID, Status: StepSkipped, Output: s.Output})
				continue
			}
			summary, err := e.Wallet.WalletReceiptSummary(ctx, chainID, hash)
			if err != nil {
				s.Status = StepSucceeded
				s.Output = "receipt pending or unavailable: " + err.Error()
				results = append(results, MarkStepOK(s.ID, s.Output))
				continue
			}
			s.Status = StepSucceeded
			s.Output = summary
			results = append(results, MarkStepOK(s.ID, summary))

		default:
			s.Status = StepSkipped
			results = append(results, StepResult{StepID: s.ID, Status: StepSkipped})
		}
	}

	final := ""
	for _, r := range results {
		if r.Status == StepSucceeded && r.Output != "" && strings.Contains(r.Output, "Transaction sent") {
			final = r.Output
			break
		}
	}
	if final == "" && len(results) > 0 {
		final = results[len(results)-1].Output
	}
	return &ExecutionResult{PlanID: p.ID, StepResults: results, FinalReply: final}, nil
}

func extractTxHash(results []StepResult) string {
	for i := len(results) - 1; i >= 0; i-- {
		if h := parseHashFromToolOutput(results[i].Output); h != "" {
			return h
		}
	}
	return ""
}

func parseHashFromToolOutput(s string) string {
	const prefix = "Hash: "
	for _, line := range strings.Split(s, "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, prefix) {
			return strings.TrimSpace(strings.TrimPrefix(line, prefix))
		}
	}
	return ""
}
