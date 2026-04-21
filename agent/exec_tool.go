package agent

import (
	"context"
	"encoding/hex"
	"encoding/json"
	"math/big"
	"strconv"
	"strings"

	"custom-agent/agent/planning"
	"custom-agent/gateway"
	"custom-agent/tools"
	"custom-agent/wallet/abi"

	"github.com/ethereum/go-ethereum/common"
	"github.com/sashabaranov/go-openai"
)

// reactiveWalletContractPipeline is true when the main tool loop should route
// wallet_execute_contract_call through the preview → simulate → send pipeline (same as
// orchestration when contract-call tools are allowed), instead of a direct tool execution.
func (a *Agent) reactiveWalletContractPipeline() bool {
	if a == nil || a.tools == nil || a.tools.Wallet == nil {
		return false
	}
	return a.planner.Enabled && a.planner.CapabilityEnabled("wallet")
}

// executeToolForMessage runs a single tool with the same injection behavior as HandleMessage.
// If walletPipeline is true and the tool is wallet_execute_contract_call or
// wallet_execute_erc20_transfer or wallet_execute_erc20_approve, runs the wallet
// preview/simulate/send pipeline via planning.WalletExecutor instead of a raw tool call.
func (a *Agent) executeToolForMessage(ctx context.Context, msg gateway.IncomingMessage, name, argsJSON string, walletPipeline bool) string {
	if a == nil || a.tools == nil {
		return "Error: tools not configured."
	}
	name = strings.TrimSpace(name)
	if name == "spawn_subagents" {
		return a.handleSpawnSubagents(ctx, argsJSON, msg)
	}
	if walletPipeline && name == "wallet_execute_contract_call" && a.tools.Wallet != nil {
		if out, ok := a.tryWalletContractPipeline(ctx, msg, argsJSON); ok {
			return out
		}
	}
	if walletPipeline && name == "wallet_execute_erc20_transfer" && a.tools.Wallet != nil {
		if out, ok := a.tryWalletERC20TransferPipeline(ctx, msg, argsJSON); ok {
			return out
		}
	}
	if walletPipeline && name == "wallet_execute_erc20_approve" && a.tools.Wallet != nil {
		if out, ok := a.tryWalletERC20ApprovePipeline(ctx, msg, argsJSON); ok {
			return out
		}
	}
	args := argsJSON
	if name == "save_memory" || name == "read_memory" {
		if injected, err := tools.InjectMemoryArgs(args, msg.Platform, msg.UserID); err == nil {
			args = injected
		}
	}
	if name == "create_scheduled_reminder" || name == "list_reminders" || name == "delete_reminder" {
		if injected, err := tools.InjectReminderArgs(args, msg.Platform, msg.UserID, msg.ChatID); err == nil {
			args = injected
		}
	}
	if name == "wallet_execute_transfer" || name == "wallet_execute_erc20_transfer" || name == "wallet_execute_erc20_approve" || name == "wallet_execute_contract_call" {
		if injected, err := tools.InjectWalletArgs(args, msg.Platform, msg.UserID, msg.ChatID); err == nil {
			args = injected
		}
	}
	result, err := a.tools.ExecuteToolWithContext(ctx, name, args)
	if err != nil {
		return "Error: " + err.Error()
	}
	return result
}

type walletContractArgs struct {
	To       string      `json:"to"`
	Data     string      `json:"data"`
	ValueWei string      `json:"value_wei"`
	ChainID  interface{} `json:"chain_id"` // number or string
}

// tryWalletContractPipeline runs WalletExecutor for contract calls (preview → simulate → send).
func (a *Agent) tryWalletContractPipeline(ctx context.Context, msg gateway.IncomingMessage, argsJSON string) (string, bool) {
	var wa walletContractArgs
	if err := json.Unmarshal([]byte(argsJSON), &wa); err != nil {
		return "", false
	}
	if strings.TrimSpace(wa.To) == "" || strings.TrimSpace(wa.Data) == "" {
		return "", false
	}
	valueWei := wa.ValueWei
	if valueWei == "" {
		valueWei = "0"
	}
	chainID := ""
	switch v := wa.ChainID.(type) {
	case float64:
		if v > 0 {
			chainID = strconv.FormatInt(int64(v), 10)
		}
	case string:
		chainID = strings.TrimSpace(v)
	}

	constraints := map[string]string{
		"to":        strings.TrimSpace(wa.To),
		"data":      strings.TrimSpace(wa.Data),
		"value_wei": valueWei,
		"_platform": msg.Platform,
		"_user_id":  msg.UserID,
		"_chat_id":  msg.ChatID,
	}
	if chainID != "" {
		constraints["chain_id"] = chainID
	}
	if err := planning.ValidateWalletConstraints(constraints); err != nil {
		return "", false
	}
	p := planning.WalletTxTemplate("contract call", constraints)
	if err := planning.ValidatePlan(p); err != nil {
		return "", false
	}
	v := planning.NewValidator(planning.CapabilityWalletTx)
	if err := v.Validate(p); err != nil {
		return "", false
	}
	exec := &planning.WalletExecutor{Wallet: a.tools.Wallet}
	res, err := exec.Execute(ctx, p)
	if err != nil {
		return "Planner execution failed: " + err.Error(), true
	}
	out := strings.TrimSpace(res.FinalReply)
	if out == "" {
		out = "Plan completed."
	}
	return out, true
}

type walletERC20Args struct {
	Token   string      `json:"token"`
	To      string      `json:"to"`
	Amount  string      `json:"amount"`
	ChainID interface{} `json:"chain_id"`
}

// tryWalletERC20TransferPipeline builds ERC-20 transfer calldata in-process and runs the same
// WalletExecutor path as contract_call (preview → simulate → send).
func (a *Agent) tryWalletERC20TransferPipeline(ctx context.Context, msg gateway.IncomingMessage, argsJSON string) (string, bool) {
	var wa walletERC20Args
	if err := json.Unmarshal([]byte(argsJSON), &wa); err != nil {
		return "", false
	}
	token := strings.TrimSpace(wa.Token)
	toAddr := strings.TrimSpace(wa.To)
	amountStr := strings.TrimSpace(wa.Amount)
	if token == "" || toAddr == "" || amountStr == "" {
		return "", false
	}
	if !common.IsHexAddress(token) || !common.IsHexAddress(toAddr) {
		return "", false
	}
	amt := new(big.Int)
	if _, ok := amt.SetString(amountStr, 10); !ok || amt.Sign() <= 0 {
		return "", false
	}
	recipient := common.HexToAddress(toAddr)
	data, err := abi.EncodeERC20Transfer(recipient, amt)
	if err != nil {
		return "", false
	}
	dataHex := "0x" + hex.EncodeToString(data)
	chainID := ""
	switch v := wa.ChainID.(type) {
	case float64:
		if v > 0 {
			chainID = strconv.FormatInt(int64(v), 10)
		}
	case string:
		chainID = strings.TrimSpace(v)
	}

	constraints := map[string]string{
		"to":        token,
		"data":      dataHex,
		"value_wei": "0",
		"_platform": msg.Platform,
		"_user_id":  msg.UserID,
		"_chat_id":  msg.ChatID,
	}
	if chainID != "" {
		constraints["chain_id"] = chainID
	}
	if err := planning.ValidateWalletConstraints(constraints); err != nil {
		return "", false
	}
	p := planning.WalletTxTemplate("ERC-20 transfer", constraints)
	if err := planning.ValidatePlan(p); err != nil {
		return "", false
	}
	v := planning.NewValidator(planning.CapabilityWalletTx)
	if err := v.Validate(p); err != nil {
		return "", false
	}
	exec := &planning.WalletExecutor{Wallet: a.tools.Wallet}
	res, err := exec.Execute(ctx, p)
	if err != nil {
		return "Planner execution failed: " + err.Error(), true
	}
	out := strings.TrimSpace(res.FinalReply)
	if out == "" {
		out = "Plan completed."
	}
	return out, true
}

type walletERC20ApproveArgs struct {
	Token   string      `json:"token"`
	Spender string      `json:"spender"`
	Amount  string      `json:"amount"`
	ChainID interface{} `json:"chain_id"`
}

// tryWalletERC20ApprovePipeline builds ERC-20 approve calldata in-process and runs the same
// WalletExecutor path as contract_call (preview → simulate → send).
func (a *Agent) tryWalletERC20ApprovePipeline(ctx context.Context, msg gateway.IncomingMessage, argsJSON string) (string, bool) {
	var wa walletERC20ApproveArgs
	if err := json.Unmarshal([]byte(argsJSON), &wa); err != nil {
		return "", false
	}
	token := strings.TrimSpace(wa.Token)
	spenderAddr := strings.TrimSpace(wa.Spender)
	amountStr := strings.TrimSpace(wa.Amount)
	if token == "" || spenderAddr == "" || amountStr == "" {
		return "", false
	}
	if !common.IsHexAddress(token) || !common.IsHexAddress(spenderAddr) {
		return "", false
	}
	amt := new(big.Int)
	if _, ok := amt.SetString(amountStr, 10); !ok || amt.Sign() < 0 {
		return "", false
	}
	spender := common.HexToAddress(spenderAddr)
	data, err := abi.EncodeERC20Approve(spender, amt)
	if err != nil {
		return "", false
	}
	dataHex := "0x" + hex.EncodeToString(data)
	chainID := ""
	switch v := wa.ChainID.(type) {
	case float64:
		if v > 0 {
			chainID = strconv.FormatInt(int64(v), 10)
		}
	case string:
		chainID = strings.TrimSpace(v)
	}

	constraints := map[string]string{
		"to":        token,
		"data":      dataHex,
		"value_wei": "0",
		"_platform": msg.Platform,
		"_user_id":  msg.UserID,
		"_chat_id":  msg.ChatID,
	}
	if chainID != "" {
		constraints["chain_id"] = chainID
	}
	if err := planning.ValidateWalletConstraints(constraints); err != nil {
		return "", false
	}
	p := planning.WalletTxTemplate("ERC-20 approve", constraints)
	if err := planning.ValidatePlan(p); err != nil {
		return "", false
	}
	v := planning.NewValidator(planning.CapabilityWalletTx)
	if err := v.Validate(p); err != nil {
		return "", false
	}
	exec := &planning.WalletExecutor{Wallet: a.tools.Wallet}
	res, err := exec.Execute(ctx, p)
	if err != nil {
		return "Planner execution failed: " + err.Error(), true
	}
	out := strings.TrimSpace(res.FinalReply)
	if out == "" {
		out = "Plan completed."
	}
	return out, true
}

// executeToolCallsFromMessage handles structured tool_calls from ChatCompletionMessage.
func (a *Agent) executeToolCallsFromMessage(ctx context.Context, msg gateway.IncomingMessage, toolCalls []openai.ToolCall, walletPipeline bool) []openai.ChatCompletionMessage {
	out := make([]openai.ChatCompletionMessage, 0, len(toolCalls))
	for _, tc := range toolCalls {
		if strings.TrimSpace(tc.Function.Name) == "" {
			continue
		}
		result := a.executeToolForMessage(ctx, msg, tc.Function.Name, tc.Function.Arguments, walletPipeline)
		out = append(out, openai.ChatCompletionMessage{
			Role:       openai.ChatMessageRoleTool,
			Content:    result,
			ToolCallID: tc.ID,
		})
	}
	return out
}
