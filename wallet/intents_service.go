package wallet

import (
	"context"
	"fmt"
	"log"
	"math/big"
	"strconv"
	"strings"

	"github.com/ethereum/go-ethereum/common"

	sdk "github.com/1clawAI/1claw-go-sdk"

	"custom-agent/wallet/account"
	"custom-agent/wallet/approval"
	"custom-agent/wallet/chains"
	"custom-agent/wallet/history"
	"custom-agent/wallet/policy"
	"custom-agent/wallet/redact"
)

// IntentsService implements tools.WalletService using the 1claw Intents API.
// Keys stay in HSM; the agent submits intents and 1claw signs and broadcasts.
type IntentsService struct {
	client       *sdk.Client
	agentID      string
	walletAddr   string
	chainRegistry *chains.Registry
	policy       *policy.Engine
	approval     *approval.Store
	notifier     ApprovalNotifier
	history      *history.Store
}

// NewIntentsService creates an Intents-backed wallet service.
func NewIntentsService(
	client *sdk.Client,
	agentID, walletAddr string,
	chainRegistry *chains.Registry,
	policyEngine *policy.Engine,
	approvalStore *approval.Store,
	notifier ApprovalNotifier,
	historyStore *history.Store,
) *IntentsService {
	if historyStore == nil {
		historyStore = history.NewStore("")
	}
	return &IntentsService{
		client:        client,
		agentID:       agentID,
		walletAddr:    walletAddr,
		chainRegistry: chainRegistry,
		policy:        policyEngine,
		approval:      approvalStore,
		notifier:      notifier,
		history:       historyStore,
	}
}

func (s *IntentsService) resolveChainID(chainID int64) (int64, error) {
	return s.chainRegistry.ResolveChainID(chainID)
}

// WalletAddress implements tools.WalletService.
func (s *IntentsService) WalletAddress() string {
	return s.walletAddr
}

// DefaultChainID implements tools.WalletService.
func (s *IntentsService) DefaultChainID() int64 {
	return s.chainRegistry.DefaultChainID()
}

// GetBalanceString implements tools.WalletService. Uses RPC to fetch balance.
func (s *IntentsService) GetBalanceString(ctx context.Context, chainID int64, block interface{}) (string, error) {
	cid, err := s.resolveChainID(chainID)
	if err != nil {
		return "", err
	}
	prov, err := s.chainRegistry.GetProvider(cid)
	if err != nil {
		return "", err
	}
	addr := common.HexToAddress(s.walletAddr)
	var b *big.Int
	if block != nil {
		if bi, ok := block.(*big.Int); ok {
			b = bi
		}
	}
	bal, err := prov.BalanceAt(ctx, addr, b)
	if err != nil {
		return "", err
	}
	return bal.String(), nil
}

// weiToEth converts wei (decimal string) to ETH (decimal string).
func weiToEth(weiStr string) (string, error) {
	wei := new(big.Int)
	if _, ok := wei.SetString(weiStr, 10); !ok {
		return "", fmt.Errorf("invalid wei: %s", weiStr)
	}
	// 1 ETH = 10^18 wei
	eth := new(big.Float).SetInt(wei)
	eth.Quo(eth, big.NewFloat(1e18))
	return eth.Text('f', 18), nil
}

// ExecuteTransfer implements tools.WalletService.
func (s *IntentsService) ExecuteTransfer(ctx context.Context, chainID int64, to, valueWei, platform, userID, chatID string) (string, error) {
	val := new(big.Int)
	if _, ok := val.SetString(valueWei, 10); !ok {
		return "Error: invalid value (must be wei as decimal string)", nil
	}
	action := &accountAction{
		Type:  "transfer",
		To:    common.HexToAddress(to),
		Value: val,
		Data:  nil,
	}
	return s.executeAction(ctx, chainID, action, platform, userID, chatID)
}

// ExecuteContractCall implements tools.WalletService.
func (s *IntentsService) ExecuteContractCall(ctx context.Context, chainID int64, to, dataHex, valueWei, platform, userID, chatID string) (string, error) {
	val := new(big.Int)
	if valueWei != "" {
		val.SetString(valueWei, 10)
	}
	action := &accountAction{
		Type:  "contract_call",
		To:    common.HexToAddress(to),
		Value: val,
		Data:  common.FromHex(dataHex),
	}
	return s.executeAction(ctx, chainID, action, platform, userID, chatID)
}

type accountAction struct {
	Type  string
	To    common.Address
	Value *big.Int
	Data  []byte
}

func (s *IntentsService) executeAction(ctx context.Context, chainID int64, action *accountAction, platform, userID, chatID string) (string, error) {
	cid, err := s.resolveChainID(chainID)
	if err != nil {
		return "Error: " + err.Error(), nil
	}

	dec := s.policy.Evaluate(&account.Action{
		Type:     action.Type,
		To:       action.To,
		Value:    action.Value,
		Data:     action.Data,
		GasLimit: 21000,
	})
	switch dec {
	case policy.Deny:
		return "Error: transaction denied by policy.", nil
	case policy.RequireApproval:
		if s.approval == nil {
			return "Error: approval required but approval store not configured.", nil
		}
		p := &approval.Pending{
			Action:   toAccountAction(action),
			ChainID:  cid,
			Summary:  intentsActionSummary(action),
			Platform: platform,
			UserID:   userID,
			ChatID:   chatID,
		}
		id, addErr := s.approval.Add(p)
		if addErr != nil {
			return "Error: " + addErr.Error(), nil
		}
		msg := approval.FormatPendingForNotification(p) + "\n\nReply with: approve: " + id
		if s.notifier != nil {
			_ = s.notifier.Notify(ctx, platform, userID, chatID, msg)
		}
		return "Approval required. ID: " + id + ". Reply with: approve: " + id, nil
	case policy.Allow:
		// proceed
	}

	valueEth, err := weiToEth(action.Value.String())
	if err != nil {
		return "Error: " + err.Error(), nil
	}
	chainStr := strconv.FormatInt(cid, 10)
	chainName := s.chainRegistry.GetChainName(cid)
	if chainName != "" {
		chainStr = chainName
	}
	var data *string
	if len(action.Data) > 0 {
		hex := "0x" + common.Bytes2Hex(action.Data)
		data = &hex
	}
	resp, err := s.client.SubmitTransaction(ctx, s.agentID, action.To.Hex(), valueEth, chainStr, data)
	if err != nil {
		return "Error: " + redact.Redact(err.Error()), nil
	}
	txHash := ""
	if resp.TxHash != nil {
		txHash = *resp.TxHash
	}
	if txHash == "" && resp.SignedTx != nil {
		// Fallback: we might need to broadcast ourselves - for now just report
		txHash = "pending"
	}
	explorerURL := s.chainRegistry.GetExplorerURL(cid, txHash)
	if err := s.history.Add(&history.Entry{
		ChainID:       cid,
		ChainName:     chainName,
		WalletAddress: s.walletAddr,
		TxHash:        txHash,
		ExplorerURL:   explorerURL,
		Status:        "submitted",
		ActionType:    action.Type,
		ToAddress:     action.To.Hex(),
		ValueWei:      action.Value.String(),
		Platform:      platform,
		UserID:        userID,
		ChatID:        chatID,
	}); err != nil {
		log.Printf("[wallet] history add failed: %v", err)
	}
	return formatTxResult(txHash, chainName, explorerURL), nil
}

func toAccountAction(a *accountAction) *account.Action {
	gasLimit := uint64(21000)
	if len(a.Data) > 0 {
		gasLimit = 300000
	}
	return &account.Action{
		Type:     a.Type,
		To:       a.To,
		Value:    a.Value,
		Data:     a.Data,
		GasLimit: gasLimit,
	}
}

func intentsActionSummary(a *accountAction) string {
	if a.Type == "transfer" {
		return "Transfer " + a.Value.String() + " wei to " + a.To.Hex()
	}
	return "Contract call to " + a.To.Hex()
}

// ExecuteApproved implements tools.WalletService.
func (s *IntentsService) ExecuteApproved(ctx context.Context, approvalID, platform, userID, chatID string) (string, error) {
	if s.approval == nil {
		return "Error: approval store not configured.", nil
	}
	p, ok := s.approval.Get(approvalID)
	if !ok {
		return "Error: approval not found or expired.", nil
	}
	if p.Platform != platform || p.UserID != userID {
		return "Error: approval belongs to another user.", nil
	}
	s.approval.Remove(approvalID)
	chainID := p.ChainID
	if chainID <= 0 {
		chainID = s.chainRegistry.DefaultChainID()
	}
	action := &accountAction{
		Type:  p.Action.Type,
		To:    p.Action.To,
		Value: p.Action.Value,
		Data:  p.Action.Data,
	}
	return s.executeAction(ctx, chainID, action, platform, userID, chatID)
}

// ListTransactions implements tools.WalletService. Uses 1claw ListTransactions.
func (s *IntentsService) ListTransactions(chainID int64, limit int) (string, error) {
	if limit <= 0 {
		limit = 20
	}
	resp, err := s.client.ListTransactions(context.Background(), s.agentID)
	if err != nil {
		return "", err
	}
	if resp.Transactions == nil || len(resp.Transactions) == 0 {
		return "No transactions in history.", nil
	}
	var b strings.Builder
	for i, tx := range resp.Transactions {
		if i >= limit {
			break
		}
		if i > 0 {
			b.WriteString("\n\n")
		}
		txHash := ""
		if tx.TxHash != nil {
			txHash = *tx.TxHash
		}
		to := ""
		if tx.To != nil {
			to = *tx.To
		}
		chainName := ""
		if tx.Chain != nil {
			chainName = *tx.Chain
		}
		valueWei := ""
		if tx.ValueWei != nil {
			valueWei = *tx.ValueWei
		}
		line := fmt.Sprintf("[intents] %s | %s → %s (value: %s wei)", chainName, txHash, to, valueWei)
		b.WriteString(line)
	}
	return b.String(), nil
}
