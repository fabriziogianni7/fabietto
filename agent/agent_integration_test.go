package agent

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"custom-agent/gateway"
	"custom-agent/tools"

	"github.com/sashabaranov/go-openai"
)

type mockWalletService struct {
	executeApprovedFn func(ctx context.Context, approvalID, platform, userID, chatID string) (string, error)
}

func (m *mockWalletService) WalletAddress() string { return "0x123" }
func (m *mockWalletService) DefaultChainID() int64 { return 1 }
func (m *mockWalletService) GetBalanceString(ctx context.Context, chainID int64, block interface{}) (string, error) {
	return "0", nil
}
func (m *mockWalletService) ERC20Balance(ctx context.Context, chainID int64, token string) (string, error) {
	return "mock erc20", nil
}
func (m *mockWalletService) ExecuteTransfer(ctx context.Context, chainID int64, to, valueWei, platform, userID, chatID string) (string, error) {
	return "tx-sent", nil
}
func (m *mockWalletService) ExecuteContractCall(ctx context.Context, chainID int64, to, dataHex, valueWei, platform, userID, chatID string) (string, error) {
	return "contract-sent", nil
}
func (m *mockWalletService) ExecuteApproved(ctx context.Context, approvalID, platform, userID, chatID string) (string, error) {
	if m.executeApprovedFn != nil {
		return m.executeApprovedFn(ctx, approvalID, platform, userID, chatID)
	}
	return "approved", nil
}
func (m *mockWalletService) ListTransactions(chainID int64, limit int) (string, error) {
	return "", nil
}
func (m *mockWalletService) WalletPreviewContract(ctx context.Context, chainID int64, to, dataHex, valueWei string) (method string, decision string, gasLimit uint64, err error) {
	return "swap", "allow", 210000, nil
}
func (m *mockWalletService) WalletSimulateContract(ctx context.Context, chainID int64, to, dataHex, valueWei string) error {
	return nil
}
func (m *mockWalletService) WalletReceiptSummary(ctx context.Context, chainID int64, txHash string) (string, error) {
	return "success block=1 gas_used=21000", nil
}

func TestHandleMessage_MultiRoundToolUseReturnsFinalResponse(t *testing.T) {
	var calls int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/chat/completions" {
			http.NotFound(w, r)
			return
		}
		call := atomic.AddInt32(&calls, 1)
		w.Header().Set("Content-Type", "application/json")
		switch call {
		case 1:
			_, _ = w.Write([]byte(`{
				"id":"c1",
				"choices":[{"message":{
					"role":"assistant",
					"tool_calls":[
						{
							"id":"tc_1",
							"type":"function",
							"function":{"name":"read_file","arguments":"{\"path\":\"README.md\"}"}
						}
					]
				}}]
			}`))
		case 2:
			_, _ = w.Write([]byte(`{
				"id":"c2",
				"choices":[{"message":{"role":"assistant","content":"Final answer after tool output."}}]
			}`))
		default:
			t.Fatalf("unexpected completion call %d", call)
		}
	}))
	defer server.Close()

	cfg := openai.DefaultConfig("test-key")
	cfg.BaseURL = server.URL + "/v1"
	client := openai.NewClientWithConfig(cfg)

	a := New(client, "system", 0, tools.NewTools("", nil), nil, "", nil)
	reply := a.HandleMessage(context.Background(), gateway.IncomingMessage{
		Platform: "test",
		UserID:   "multi-round-user",
		ChatID:   "chat-1",
		Text:     "Read the README and answer.",
	})

	if reply != "Final answer after tool output." {
		t.Fatalf("unexpected final reply: %q", reply)
	}
	if got := atomic.LoadInt32(&calls); got != 2 {
		t.Fatalf("expected 2 completion calls, got %d", got)
	}
}

func TestHandleMessage_WalletApprovalMessageRoutesToWallet(t *testing.T) {
	var received struct {
		id       string
		platform string
		userID   string
		chatID   string
	}
	toolSet := tools.NewTools("", nil)
	toolSet.SetWallet(&mockWalletService{
		executeApprovedFn: func(ctx context.Context, approvalID, platform, userID, chatID string) (string, error) {
			received.id = approvalID
			received.platform = platform
			received.userID = userID
			received.chatID = chatID
			return "Transaction sent.\nHash: 0xabc", nil
		},
	})

	// Wallet approval messages are handled before any LLM call, so a nil client is safe here.
	a := New(nil, "system", 0, toolSet, nil, "", nil)
	reply := a.HandleMessage(context.Background(), gateway.IncomingMessage{
		Platform: "telegram",
		UserID:   "user-42",
		ChatID:   "chat-42",
		Text:     "approve: tx_999",
	})

	if !strings.Contains(reply, "Transaction sent.") {
		t.Fatalf("expected wallet execution result, got %q", reply)
	}
	if received.id != "tx_999" || received.platform != "telegram" || received.userID != "user-42" || received.chatID != "chat-42" {
		got, _ := json.Marshal(received)
		t.Fatalf("wallet approval context mismatch: %s", string(got))
	}
}
