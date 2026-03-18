package wallet

import (
	"context"
	"fmt"
	"log"
	"math/big"
	"strings"
	"sync"

	"github.com/ethereum/go-ethereum/common"

	"custom-agent/wallet/account"
	"custom-agent/wallet/approval"
	"custom-agent/wallet/chains"
	"custom-agent/wallet/history"
	"custom-agent/wallet/policy"
	"custom-agent/wallet/provider"
	"custom-agent/wallet/redact"
	"custom-agent/wallet/signer"
)

// ApprovalNotifier sends approval requests to the guardian. Optional.
type ApprovalNotifier interface {
	Notify(ctx context.Context, platform, userID, chatID, message string) error
}

// Service is the policy-gated wallet service used by tools.
type Service struct {
	chainRegistry *chains.Registry
	signer        signer.Signer
	accounts      map[int64]account.Account
	accountsMu    sync.RWMutex
	policy        *policy.Engine
	approval      *approval.Store
	notifier      ApprovalNotifier
	history       *history.Store
}

// NewService creates a WalletService. If approvalStore is nil, approval flow is skipped (deny instead).
// notifier is optional; when set, guardian is notified when approval is required.
func NewService(chainRegistry *chains.Registry, sgn signer.Signer, policyEngine *policy.Engine, approvalStore *approval.Store, notifier ApprovalNotifier, historyStore *history.Store) *Service {
	if historyStore == nil {
		historyStore = history.NewStore("")
	}
	return &Service{
		chainRegistry: chainRegistry,
		signer:        sgn,
		accounts:      make(map[int64]account.Account),
		policy:        policyEngine,
		approval:      approvalStore,
		notifier:      notifier,
		history:       historyStore,
	}
}

func (s *Service) getAccount(chainID int64) (account.Account, *provider.Provider, error) {
	prov, err := s.chainRegistry.GetProvider(chainID)
	if err != nil {
		return nil, nil, err
	}
	s.accountsMu.RLock()
	acc, ok := s.accounts[chainID]
	s.accountsMu.RUnlock()
	if ok {
		return acc, prov, nil
	}
	s.accountsMu.Lock()
	defer s.accountsMu.Unlock()
	if acc, ok = s.accounts[chainID]; ok {
		return acc, prov, nil
	}
	acc = account.NewEOAAccount(s.signer, prov, s.chainRegistry.BigInt(chainID))
	s.accounts[chainID] = acc
	return acc, prov, nil
}

func (s *Service) resolveChainID(chainID int64) (int64, error) {
	return s.chainRegistry.ResolveChainID(chainID)
}

// Address returns the wallet address.
func (s *Service) Address() common.Address {
	return s.signer.Address()
}

// WalletAddress returns the wallet address as hex string. Implements tools.WalletService.
func (s *Service) WalletAddress() string {
	return s.signer.Address().Hex()
}

// DefaultChainID returns the default chain ID.
func (s *Service) DefaultChainID() int64 {
	return s.chainRegistry.DefaultChainID()
}

// ChainIDs returns all configured chain IDs. Implements tools.WalletService.
func (s *Service) ChainIDs() []int64 {
	return s.chainRegistry.ChainIDs()
}

// GetBalance returns the native token balance at the given block (nil = latest).
// chainID 0 = default chain.
func (s *Service) GetBalance(ctx context.Context, chainID int64, block *big.Int) (*big.Int, error) {
	cid, err := s.resolveChainID(chainID)
	if err != nil {
		return nil, err
	}
	acc, prov, err := s.getAccount(cid)
	if err != nil {
		return nil, err
	}
	return prov.BalanceAt(ctx, acc.Address(), block)
}

// GetBalanceString returns balance as wei string for tool output.
// chainID 0 = default chain. Implements tools.WalletService; block can be nil or *big.Int.
func (s *Service) GetBalanceString(ctx context.Context, chainID int64, block interface{}) (string, error) {
	var b *big.Int
	if block != nil {
		if bi, ok := block.(*big.Int); ok {
			b = bi
		}
	}
	bal, err := s.GetBalance(ctx, chainID, b)
	if err != nil {
		return "", err
	}
	return bal.String(), nil
}

// ExecuteTransfer sends native token to an address. valueWei is wei as string.
// chainID 0 = default chain.
func (s *Service) ExecuteTransfer(ctx context.Context, chainID int64, toAddr string, valueWei string, platform, userID, chatID string) (string, error) {
	to := common.HexToAddress(toAddr)
	val := new(big.Int)
	if _, ok := val.SetString(valueWei, 10); !ok {
		return "Error: invalid value (must be wei as decimal string)", nil
	}
	action := &account.Action{
		Type:     "transfer",
		To:       to,
		Value:    val,
		Data:     nil,
		GasLimit: 21000,
	}
	return s.ExecuteAction(ctx, chainID, action, platform, userID, chatID)
}

// ExecuteContractCall executes a contract call. data is hex-encoded calldata; valueWei can be "0".
// chainID 0 = default chain.
func (s *Service) ExecuteContractCall(ctx context.Context, chainID int64, toAddr, dataHex, valueWei string, platform, userID, chatID string) (string, error) {
	cid, err := s.resolveChainID(chainID)
	if err != nil {
		return "Error: " + err.Error(), nil
	}
	acc, _, err := s.getAccount(cid)
	if err != nil {
		return "Error: " + err.Error(), nil
	}
	to := common.HexToAddress(toAddr)
	val := new(big.Int)
	if valueWei != "" {
		val.SetString(valueWei, 10)
	}
	data := common.FromHex(dataHex)
	gas, err := acc.Estimate(ctx, &account.Action{To: to, Value: val, Data: data, GasLimit: 0})
	if err != nil {
		gas = 300000 // fallback
	}
	action := &account.Action{
		Type:     "contract_call",
		To:       to,
		Value:    val,
		Data:     data,
		GasLimit: gas + gas/10, // add 10% buffer
	}
	return s.ExecuteAction(ctx, chainID, action, platform, userID, chatID)
}

// EstimateAction returns estimated gas for the action on the given chain.
func (s *Service) EstimateAction(ctx context.Context, chainID int64, action *account.Action) (uint64, error) {
	cid, err := s.resolveChainID(chainID)
	if err != nil {
		return 0, err
	}
	acc, _, err := s.getAccount(cid)
	if err != nil {
		return 0, err
	}
	return acc.Estimate(ctx, action)
}

// ExecuteAction runs policy check, optionally requests approval, then signs and broadcasts.
// Returns redacted result suitable for tool output (no secrets).
// chainID 0 = default chain.
func (s *Service) ExecuteAction(ctx context.Context, chainID int64, action *account.Action, platform, userID, chatID string) (result string, err error) {
	cid, err := s.resolveChainID(chainID)
	if err != nil {
		return "Error: " + err.Error(), nil
	}
	acc, prov, err := s.getAccount(cid)
	if err != nil {
		return "Error: " + err.Error(), nil
	}

	dec := s.policy.Evaluate(action)
	switch dec {
	case policy.Deny:
		return "Error: transaction denied by policy.", nil
	case policy.RequireApproval:
		if s.approval == nil {
			return "Error: approval required but approval store not configured.", nil
		}
		p := &approval.Pending{
			Action:   action,
			ChainID:  cid,
			Summary:  actionSummary(action),
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

	raw, err := acc.PrepareExecution(ctx, action)
	if err != nil {
		return "Error: " + redact.Redact(err.Error()), nil
	}
	txHash, err := prov.SendRawTransaction(ctx, raw)
	if err != nil {
		return "Error: " + redact.Redact(err.Error()), nil
	}
	hashStr := txHash.Hex()
	chainName := s.chainRegistry.GetChainName(cid)
	explorerURL := s.chainRegistry.GetExplorerURL(cid, hashStr)

	// Record to history
	if err := s.history.Add(&history.Entry{
		ChainID:       cid,
		ChainName:     chainName,
		WalletAddress: s.WalletAddress(),
		TxHash:        hashStr,
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

	return formatTxResult(hashStr, chainName, explorerURL), nil
}

// ExecuteApproved runs a previously approved action by ID. Used when user replies "approve: tx_123".
// Bypasses policy check since user explicitly approved.
func (s *Service) ExecuteApproved(ctx context.Context, approvalID, platform, userID, chatID string) (result string, err error) {
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
	acc, prov, err := s.getAccount(chainID)
	if err != nil {
		return "Error: " + err.Error(), nil
	}

	raw, err := acc.PrepareExecution(ctx, p.Action)
	if err != nil {
		return "Error: " + redact.Redact(err.Error()), nil
	}
	txHash, err := prov.SendRawTransaction(ctx, raw)
	if err != nil {
		return "Error: " + redact.Redact(err.Error()), nil
	}
	hashStr := txHash.Hex()
	chainName := s.chainRegistry.GetChainName(chainID)
	explorerURL := s.chainRegistry.GetExplorerURL(chainID, hashStr)

	// Record to history
	if err := s.history.Add(&history.Entry{
		ChainID:       chainID,
		ChainName:     chainName,
		WalletAddress: s.WalletAddress(),
		TxHash:        hashStr,
		ExplorerURL:   explorerURL,
		Status:        "submitted",
		ActionType:    p.Action.Type,
		ToAddress:     p.Action.To.Hex(),
		ValueWei:      p.Action.Value.String(),
		Platform:      platform,
		UserID:        userID,
		ChatID:        chatID,
		ApprovalID:    approvalID,
	}); err != nil {
		log.Printf("[wallet] history add failed: %v", err)
	}

	return formatTxResult(hashStr, chainName, explorerURL), nil
}

// ListTransactions returns formatted transaction history for tool output.
// chainID 0 = all chains. limit 0 = no limit (default 20).
func (s *Service) ListTransactions(chainID int64, limit int) (string, error) {
	if limit <= 0 {
		limit = 20
	}
	entries, err := s.history.List(chainID, limit)
	if err != nil {
		return "", err
	}
	if len(entries) == 0 {
		return "No transactions in history.", nil
	}
	var b strings.Builder
	for i, e := range entries {
		if i > 0 {
			b.WriteString("\n\n")
		}
		b.WriteString(formatHistoryEntry(e))
	}
	return b.String(), nil
}

func actionSummary(a *account.Action) string {
	if a.Type == "transfer" {
		return "Transfer " + a.Value.String() + " wei to " + a.To.Hex()
	}
	if a.Method != "" {
		return "Contract call " + a.Method + " on " + a.To.Hex()
	}
	return "Contract call to " + a.To.Hex()
}

func formatHistoryEntry(e history.Entry) string {
	ts := e.Timestamp.Format("2006-01-02 15:04")
	chainLabel := e.ChainName
	if e.ChainID > 0 {
		if chainLabel != "" {
			chainLabel = fmt.Sprintf("%s (chain %d)", chainLabel, e.ChainID)
		} else {
			chainLabel = fmt.Sprintf("Chain %d", e.ChainID)
		}
	}
	line := fmt.Sprintf("[%s] %s | %s | %s → %s", ts, chainLabel, e.ActionType, e.TxHash, e.ToAddress)
	if e.ExplorerURL != "" {
		line += "\n  Explorer: " + e.ExplorerURL
	}
	return line
}

func formatTxResult(hashStr, chainName, explorerURL string) string {
	out := "Transaction sent.\nHash: " + hashStr
	if chainName != "" {
		out += "\nChain: " + chainName
	}
	if explorerURL != "" {
		out += "\nExplorer: " + explorerURL
	}
	return out
}
