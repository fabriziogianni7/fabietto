package tools

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"

	"custom-agent/memory"
	"custom-agent/reminders"

	openai "github.com/sashabaranov/go-openai"
	"github.com/sashabaranov/go-openai/jsonschema"
)

// Tool names for fallback parsing
var toolNames = []string{"run_command", "read_file", "write_file", "web_search", "save_memory", "read_memory", "create_scheduled_reminder", "list_reminders", "delete_reminder", "spawn_subagents", "wallet_get_balance", "wallet_execute_transfer", "wallet_execute_contract_call", "wallet_list_transactions"}

// ReadOnlyToolNames are tools allowed for stateless sub-agents (no session/memory/reminder writes).
var ReadOnlyToolNames = map[string]bool{
	"read_file":   true,
	"web_search":  true,
	"read_memory": true,
}

// WalletService is the interface for policy-gated wallet operations. Optional.
// Implemented by *wallet.Service when wallet is enabled.
// chainID 0 = default chain.
type WalletService interface {
	WalletAddress() string
	DefaultChainID() int64
	GetBalanceString(ctx context.Context, chainID int64, block interface{}) (string, error)
	ExecuteTransfer(ctx context.Context, chainID int64, to, valueWei, platform, userID, chatID string) (string, error)
	ExecuteContractCall(ctx context.Context, chainID int64, to, dataHex, valueWei, platform, userID, chatID string) (string, error)
	ExecuteApproved(ctx context.Context, approvalID, platform, userID, chatID string) (string, error)
	ListTransactions(chainID int64, limit int) (string, error)
}

// Tools holds tool execution state (e.g. API keys). Create with NewTools.
type Tools struct {
	BraveSearchAPIKey string
	MemoryStore       *memory.Store
	ReminderStore     *reminders.Store
	Wallet            WalletService // optional; when set, wallet tools are available
}

// NewTools creates a Tools instance with the given Brave Search API key and optional memory store.
func NewTools(braveSearchAPIKey string, memoryStore *memory.Store) *Tools {
	return NewToolsWithReminderStore(braveSearchAPIKey, memoryStore, reminders.NewStore())
}

// NewToolsWithReminderStore creates a Tools instance with the given reminder store (for sharing with cron).
func NewToolsWithReminderStore(braveSearchAPIKey string, memoryStore *memory.Store, reminderStore *reminders.Store) *Tools {
	return &Tools{
		BraveSearchAPIKey: braveSearchAPIKey,
		MemoryStore:       memoryStore,
		ReminderStore:     reminderStore,
	}
}

// SetWallet sets the optional wallet service. Call after construction when wallet is enabled.
func (t *Tools) SetWallet(w WalletService) {
	t.Wallet = w
}

// ParseToolCallFromContent extracts a tool call from model output when the model
// returns tool format as text. Supports:
//   - <write_file>{"path":"x","content":"y"}</function>
//   - <function=web_search>{"query":"..."}</function>
//   - (function=web_search>{"query":"..."}</function>  (model typo: ( instead of <)
// Returns name, args, and true if found.
func ParseToolCallFromContent(content string) (name string, args string, ok bool) {
	patterns := []string{
		`<` + `%s` + `>\s*(\{)`,           // <toolname>{
		`<function=` + `%s` + `>\s*(\{)`,  // <function=toolname>{
		`\(function=` + `%s` + `>\s*(\{)`, // (function=toolname>{ (typo)
	}
	for _, n := range toolNames {
		for _, pat := range patterns {
			re := regexp.MustCompile(fmt.Sprintf(pat, regexp.QuoteMeta(n)))
			loc := re.FindStringIndex(content)
			if loc == nil {
				continue
			}
			jsonStart := loc[1] - 1 // position of {
			argsStr, _ := extractJSON(content, jsonStart)
			if argsStr != "" {
				return n, argsStr, true
			}
		}
	}
	return "", "", false
}

// extractJSON extracts a complete JSON object starting at start, handling nested braces and strings.
func extractJSON(s string, start int) (string, int) {
	if start >= len(s) || s[start] != '{' {
		return "", -1
	}
	depth := 1
	inString := false
	quote := byte(0)
	escape := false
	for i := start + 1; i < len(s); i++ {
		c := s[i]
		if escape {
			escape = false
			continue
		}
		if c == '\\' && inString {
			escape = true
			continue
		}
		if (c == '"' || c == '\'') && !escape {
			if !inString {
				inString = true
				quote = c
			} else if c == quote {
				inString = false
			}
			continue
		}
		if inString {
			continue
		}
		if c == '{' {
			depth++
		} else if c == '}' {
			depth--
			if depth == 0 {
				return s[start : i+1], i + 1
			}
		}
	}
	return "", -1
}

// Definitions returns the tool definitions for the LLM.
func Definitions() []openai.Tool {
	return []openai.Tool{
		{
			Type: openai.ToolTypeFunction,
			Function: &openai.FunctionDefinition{
				Name:        "run_command",
				Description: "Run a shell command on the user's computer",
				Parameters: jsonschema.Definition{
					Type: jsonschema.Object,
					Properties: map[string]jsonschema.Definition{
						"command": {Type: jsonschema.String, Description: "The command to run"},
					},
					Required: []string{"command"},
				},
			},
		},
		{
			Type: openai.ToolTypeFunction,
			Function: &openai.FunctionDefinition{
				Name:        "read_file",
				Description: "Read a file from the filesystem",
				Parameters: jsonschema.Definition{
					Type: jsonschema.Object,
					Properties: map[string]jsonschema.Definition{
						"path": {Type: jsonschema.String, Description: "Path to the file"},
					},
					Required: []string{"path"},
				},
			},
		},
		{
			Type: openai.ToolTypeFunction,
			Function: &openai.FunctionDefinition{
				Name:        "write_file",
				Description: "Write content to a file",
				Parameters: jsonschema.Definition{
					Type: jsonschema.Object,
					Properties: map[string]jsonschema.Definition{
						"path":    {Type: jsonschema.String, Description: "Path to the file"},
						"content": {Type: jsonschema.String, Description: "Content to write"},
					},
					Required: []string{"path", "content"},
				},
			},
		},
		{
			Type: openai.ToolTypeFunction,
			Function: &openai.FunctionDefinition{
				Name:        "web_search",
				Description: "Search the web for information",
				Parameters: jsonschema.Definition{
					Type: jsonschema.Object,
					Properties: map[string]jsonschema.Definition{
						"query": {Type: jsonschema.String, Description: "Search query"},
					},
					Required: []string{"query"},
				},
			},
		},
		{
			Type: openai.ToolTypeFunction,
			Function: &openai.FunctionDefinition{
				Name:        "save_memory",
				Description: "Save a fact, preference, or important information to long-term memory. Use when the user shares something worth remembering (name, preferences, decisions). Survives session resets.",
				Parameters: jsonschema.Definition{
					Type: jsonschema.Object,
					Properties: map[string]jsonschema.Definition{
						"content": {Type: jsonschema.String, Description: "The memory to store (concise, factual)"},
						"tags":    {Type: jsonschema.String, Description: "Optional comma-separated tags for organization"},
					},
					Required: []string{"content"},
				},
			},
		},
		{
			Type: openai.ToolTypeFunction,
			Function: &openai.FunctionDefinition{
				Name:        "read_memory",
				Description: "Search long-term memory for relevant facts. Use before answering when the question might relate to past context (preferences, prior conversations, facts the user shared).",
				Parameters: jsonschema.Definition{
					Type: jsonschema.Object,
					Properties: map[string]jsonschema.Definition{
						"query": {Type: jsonschema.String, Description: "What to search for (semantic search supported)"},
						"limit": {Type: jsonschema.Integer, Description: "Max memories to return (default 5)"},
					},
					Required: []string{"query"},
				},
			},
		},
		{
			Type: openai.ToolTypeFunction,
			Function: &openai.FunctionDefinition{
				Name:        "create_scheduled_reminder",
				Description: "Schedule a recurring reminder to send the user a message at a specific time. Use when the user asks to be reminded (e.g. 'remind me every day at 9am', 'check in with me every Monday'). Schedule format: cron expression (min hour day month weekday). Examples: '0 9 * * *' = daily 9am, '0 10 * * 1' = Mondays 10am, '30 8 * * 1-5' = weekdays 8:30am.",
				Parameters: jsonschema.Definition{
					Type: jsonschema.Object,
					Properties: map[string]jsonschema.Definition{
						"schedule": {Type: jsonschema.String, Description: "Cron expression. 5-field: min hour day month weekday (e.g. 0 9 * * * daily 9am, */10 * * * * every 10 min). 6-field: sec min hour day month weekday (e.g. */10 * * * * * every 10 sec)."},
						"message":  {Type: jsonschema.String, Description: "The message to send when the reminder triggers"},
					},
					Required: []string{"schedule", "message"},
				},
			},
		},
		{
			Type: openai.ToolTypeFunction,
			Function: &openai.FunctionDefinition{
				Name:        "list_reminders",
				Description: "List the user's scheduled reminders. Use when the user asks what reminders they have or to show their schedule.",
				Parameters: jsonschema.Definition{
					Type: jsonschema.Object,
					Properties: map[string]jsonschema.Definition{},
				},
			},
		},
		{
			Type: openai.ToolTypeFunction,
			Function: &openai.FunctionDefinition{
				Name:        "delete_reminder",
				Description: "Delete a scheduled reminder by ID. Use when the user wants to cancel or remove a reminder. Get the ID from list_reminders.",
				Parameters: jsonschema.Definition{
					Type: jsonschema.Object,
					Properties: map[string]jsonschema.Definition{
						"id": {Type: jsonschema.String, Description: "The reminder ID to delete"},
					},
					Required: []string{"id"},
				},
			},
		},
		{
			Type: openai.ToolTypeFunction,
			Function: &openai.FunctionDefinition{
				Name:        "spawn_subagents",
				Description: "Delegate independent subtasks to concurrent sub-agents. Use when a request can be parallelized (e.g. research multiple topics, compare several options, gather info from different angles). Each subtask runs in parallel. Sub-agents can use read_file, web_search, read_memory. Pass 2-5 focused tasks for best results.",
				Parameters: jsonschema.Definition{
					Type: jsonschema.Object,
					Properties: map[string]jsonschema.Definition{
						"tasks": {Type: jsonschema.Array, Description: "List of independent subtasks to run in parallel", Items: &jsonschema.Definition{Type: jsonschema.String}},
						"role":  {Type: jsonschema.String, Description: "Optional role for sub-agents (e.g. 'research specialist')"},
					},
					Required: []string{"tasks"},
				},
			},
		},
		{
			Type: openai.ToolTypeFunction,
			Function: &openai.FunctionDefinition{
				Name:        "wallet_get_balance",
				Description: "Get the native token (ETH) balance of the wallet in wei. Use when the user asks about balance or funds. Omit chain_id for default chain.",
				Parameters: jsonschema.Definition{
					Type: jsonschema.Object,
					Properties: map[string]jsonschema.Definition{
						"chain_id": {Type: jsonschema.Integer, Description: "Optional chain ID. Omit to use default chain."},
					},
				},
			},
		},
		{
			Type: openai.ToolTypeFunction,
			Function: &openai.FunctionDefinition{
				Name:        "wallet_execute_transfer",
				Description: "REQUIRED to send ETH: call this tool when the user asks to send or transfer. You cannot send without invoking this tool. Params: to (0x...), value_wei (decimal string). Returns tx hash and explorer link. Requires approval if above policy limits.",
				Parameters: jsonschema.Definition{
					Type: jsonschema.Object,
					Properties: map[string]jsonschema.Definition{
						"to":         {Type: jsonschema.String, Description: "Recipient address (0x...)"},
						"value_wei": {Type: jsonschema.String, Description: "Amount in wei as decimal string"},
						"chain_id":   {Type: jsonschema.Integer, Description: "Optional chain ID. Omit to use default chain."},
					},
					Required: []string{"to", "value_wei"},
				},
			},
		},
		{
			Type: openai.ToolTypeFunction,
			Function: &openai.FunctionDefinition{
				Name:        "wallet_execute_contract_call",
				Description: "REQUIRED to execute contract calls: call this tool when the user asks to call a contract or send to a contract. You cannot execute without invoking this tool. Params: to, data (hex), value_wei (0 for none). Returns tx hash and explorer link.",
				Parameters: jsonschema.Definition{
					Type: jsonschema.Object,
					Properties: map[string]jsonschema.Definition{
						"to":         {Type: jsonschema.String, Description: "Contract address (0x...)"},
						"data":       {Type: jsonschema.String, Description: "Hex-encoded calldata (0x...)"},
						"value_wei": {Type: jsonschema.String, Description: "ETH to send in wei (0 for none)"},
						"chain_id":   {Type: jsonschema.Integer, Description: "Optional chain ID. Omit to use default chain."},
					},
					Required: []string{"to", "data"},
				},
			},
		},
		{
			Type: openai.ToolTypeFunction,
			Function: &openai.FunctionDefinition{
				Name:        "wallet_list_transactions",
				Description: "List recent agent-initiated wallet transactions with chain, status, hash, and explorer link. Use when the user asks about transaction history.",
				Parameters: jsonschema.Definition{
					Type: jsonschema.Object,
					Properties: map[string]jsonschema.Definition{
						"chain_id": {Type: jsonschema.Integer, Description: "Optional chain ID to filter. Omit for all chains."},
						"limit":    {Type: jsonschema.Integer, Description: "Max transactions to return (default 20)."},
					},
				},
			},
		},
	}
}

// IsAllowedForSubagent returns true if the tool can be used by stateless sub-agents.
func IsAllowedForSubagent(name string) bool {
	return ReadOnlyToolNames[name]
}

// DefinitionsForSubagent returns only read-only tools (read_file, web_search, read_memory)
// for stateless sub-agents that must not write session, memory, or reminders.
func DefinitionsForSubagent() []openai.Tool {
	all := Definitions()
	out := make([]openai.Tool, 0, len(ReadOnlyToolNames))
	for _, t := range all {
		if t.Function != nil && ReadOnlyToolNames[t.Function.Name] {
			out = append(out, t)
		}
	}
	return out
}

// ExecuteTool runs the named tool with the given JSON arguments and returns the result.
// For save_memory and read_memory, platform and userID must be injected by the caller via InjectMemoryArgs.
func (t *Tools) ExecuteTool(name, argsJSON string) (string, error) {
	var args map[string]interface{}
	if err := json.Unmarshal([]byte(argsJSON), &args); err != nil {
		return "", fmt.Errorf("invalid arguments: %w", err)
	}
	strArgs := toStringMap(args)

	switch name {
	case "run_command":
		return runCommand(strArgs["command"])
	case "read_file":
		return readFile(strArgs["path"])
	case "write_file":
		return writeFile(strArgs["path"], strArgs["content"])
	case "web_search":
		return t.webSearch(strArgs["query"])
	case "save_memory":
		return t.saveMemory(strArgs)
	case "read_memory":
		return t.readMemory(strArgs, args)
	case "create_scheduled_reminder":
		return t.createScheduledReminder(strArgs)
	case "list_reminders":
		return t.listReminders(strArgs)
	case "delete_reminder":
		return t.deleteReminder(strArgs)
	case "wallet_get_balance":
		return t.walletGetBalance(strArgs, args)
	case "wallet_execute_transfer":
		return t.walletExecuteTransfer(strArgs, args)
	case "wallet_execute_contract_call":
		return t.walletExecuteContractCall(strArgs, args)
	case "wallet_list_transactions":
		return t.walletListTransactions(strArgs, args)
	default:
		return "", fmt.Errorf("unknown tool: %s", name)
	}
}

func parseChainID(args map[string]string, rawArgs map[string]interface{}) int64 {
	if rawArgs != nil {
		if v := rawArgs["chain_id"]; v != nil {
			switch n := v.(type) {
			case float64:
				if n > 0 {
					return int64(n)
				}
			}
		}
	}
	if s := args["chain_id"]; s != "" {
		if n, err := strconv.ParseInt(s, 10, 64); err == nil && n > 0 {
			return n
		}
	}
	return 0
}

func (t *Tools) walletGetBalance(args map[string]string, rawArgs map[string]interface{}) (string, error) {
	if t.Wallet == nil {
		return "Wallet not configured. Set EVM_RPC_URL and WALLET_PRIVATE_KEY to enable.", nil
	}
	chainID := parseChainID(args, rawArgs)
	bal, err := t.Wallet.GetBalanceString(context.Background(), chainID, nil)
	if err != nil {
		return "Error: " + err.Error(), nil
	}
	return "Balance: " + bal + " wei", nil
}

func (t *Tools) walletExecuteTransfer(args map[string]string, rawArgs map[string]interface{}) (string, error) {
	if t.Wallet == nil {
		return "Wallet not configured. Set EVM_RPC_URL and WALLET_PRIVATE_KEY to enable.", nil
	}
	platform := args["platform"]
	userID := args["user_id"]
	chatID := args["chat_id"]
	if platform == "" || userID == "" {
		return "Error: missing user context (platform, user_id).", nil
	}
	if chatID == "" {
		chatID = userID
	}
	chainID := parseChainID(args, rawArgs)
	return t.Wallet.ExecuteTransfer(context.Background(), chainID, args["to"], args["value_wei"], platform, userID, chatID)
}

func (t *Tools) walletExecuteContractCall(args map[string]string, rawArgs map[string]interface{}) (string, error) {
	if t.Wallet == nil {
		return "Wallet not configured. Set EVM_RPC_URL and WALLET_PRIVATE_KEY to enable.", nil
	}
	platform := args["platform"]
	userID := args["user_id"]
	chatID := args["chat_id"]
	if platform == "" || userID == "" {
		return "Error: missing user context (platform, user_id).", nil
	}
	if chatID == "" {
		chatID = userID
	}
	valueWei := args["value_wei"]
	if valueWei == "" {
		valueWei = "0"
	}
	chainID := parseChainID(args, rawArgs)
	return t.Wallet.ExecuteContractCall(context.Background(), chainID, args["to"], args["data"], valueWei, platform, userID, chatID)
}

func (t *Tools) walletListTransactions(args map[string]string, rawArgs map[string]interface{}) (string, error) {
	if t.Wallet == nil {
		return "Wallet not configured. Set EVM_RPC_URL and WALLET_PRIVATE_KEY to enable.", nil
	}
	chainID := parseChainID(args, rawArgs)
	limit := 20
	if rawArgs != nil {
		if v := rawArgs["limit"]; v != nil {
			switch n := v.(type) {
			case float64:
				if n > 0 && n <= 100 {
					limit = int(n)
				}
			}
		}
	}
	if s := args["limit"]; s != "" {
		if n, err := strconv.Atoi(s); err == nil && n > 0 && n <= 100 {
			limit = n
		}
	}
	return t.Wallet.ListTransactions(chainID, limit)
}

// InjectWalletArgs adds platform, user_id, chat_id to args for wallet tools.
func InjectWalletArgs(argsJSON, platform, userID, chatID string) (string, error) {
	var args map[string]interface{}
	if err := json.Unmarshal([]byte(argsJSON), &args); err != nil {
		return "", err
	}
	args["platform"] = platform
	args["user_id"] = userID
	args["chat_id"] = chatID
	b, err := json.Marshal(args)
	if err != nil {
		return "", err
	}
	return string(b), nil
}

// TryWalletApproval attempts to execute a wallet approval by ID. Returns (true, result) if handled.
func (t *Tools) TryWalletApproval(ctx context.Context, cmd, platform, userID, chatID string) (bool, string) {
	if t.Wallet == nil {
		return false, ""
	}
	if !strings.HasPrefix(strings.TrimSpace(cmd), "tx_") {
		return false, ""
	}
	result, err := t.Wallet.ExecuteApproved(ctx, strings.TrimSpace(cmd), platform, userID, chatID)
	if err != nil {
		return true, "Approval failed: " + err.Error()
	}
	return true, result
}

func toStringMap(m map[string]interface{}) map[string]string {
	out := make(map[string]string, len(m))
	for k, v := range m {
		if v == nil {
			continue
		}
		switch val := v.(type) {
		case string:
			out[k] = val
		case float64:
			out[k] = strconv.FormatFloat(val, 'f', -1, 64)
		case int:
			out[k] = strconv.Itoa(val)
		}
	}
	return out
}

// InjectMemoryArgs adds platform and user_id to args for memory tools. Call before ExecuteTool.
func InjectMemoryArgs(argsJSON, platform, userID string) (string, error) {
	var args map[string]interface{}
	if err := json.Unmarshal([]byte(argsJSON), &args); err != nil {
		return "", err
	}
	args["platform"] = platform
	args["user_id"] = userID
	b, err := json.Marshal(args)
	if err != nil {
		return "", err
	}
	return string(b), nil
}

// InjectReminderArgs adds platform, user_id, and chat_id to args for reminder tools.
func InjectReminderArgs(argsJSON, platform, userID, chatID string) (string, error) {
	var args map[string]interface{}
	if err := json.Unmarshal([]byte(argsJSON), &args); err != nil {
		return "", err
	}
	args["platform"] = platform
	args["user_id"] = userID
	args["chat_id"] = chatID
	b, err := json.Marshal(args)
	if err != nil {
		return "", err
	}
	return string(b), nil
}

func (t *Tools) saveMemory(args map[string]string) (string, error) {
	if t.MemoryStore == nil {
		return "Memory not configured.", nil
	}
	content := args["content"]
	platform := args["platform"]
	userID := args["user_id"]
	if platform == "" || userID == "" {
		return "Error: missing user context.", nil
	}
	if strings.TrimSpace(content) == "" {
		return "Error: content cannot be empty.", nil
	}
	if err := t.MemoryStore.Save(platform, userID, content, args["tags"]); err != nil {
		return "Error: " + err.Error(), nil
	}
	return "Memory saved.", nil
}

func (t *Tools) readMemory(args map[string]string, rawArgs map[string]interface{}) (string, error) {
	if t.MemoryStore == nil {
		return "Memory not configured.", nil
	}
	query := args["query"]
	platform := args["platform"]
	userID := args["user_id"]
	if platform == "" || userID == "" {
		return "Error: missing user context.", nil
	}
	limit := 5
	if v := rawArgs["limit"]; v != nil {
		switch n := v.(type) {
		case float64:
			if n > 0 && n <= 20 {
				limit = int(n)
			}
		case string:
			if k, err := strconv.Atoi(n); err == nil && k > 0 && k <= 20 {
				limit = k
			}
		}
	} else if s := args["limit"]; s != "" {
		if n, err := strconv.Atoi(s); err == nil && n > 0 && n <= 20 {
			limit = n
		}
	}
	memories, err := t.MemoryStore.Search(platform, userID, query, limit)
	if err != nil {
		return "Error: " + err.Error(), nil
	}
	if len(memories) == 0 {
		return "No relevant memories found.", nil
	}
	var b strings.Builder
	for i, m := range memories {
		if i > 0 {
			b.WriteString("\n\n")
		}
		b.WriteString(fmt.Sprintf("- %s", m.Content))
		if m.Tags != "" {
			b.WriteString(fmt.Sprintf(" [%s]", m.Tags))
		}
	}
	return b.String(), nil
}

func (t *Tools) createScheduledReminder(args map[string]string) (string, error) {
	if t.ReminderStore == nil {
		return "Reminders not configured.", nil
	}
	schedule := args["schedule"]
	message := args["message"]
	platform := args["platform"]
	userID := args["user_id"]
	chatID := args["chat_id"]
	if platform == "" || userID == "" {
		return "Error: missing user context.", nil
	}
	if strings.TrimSpace(schedule) == "" || strings.TrimSpace(message) == "" {
		return "Error: schedule and message are required.", nil
	}
	if chatID == "" {
		chatID = userID
	}
	id, err := t.ReminderStore.Add(platform, userID, chatID, schedule, message)
	if err != nil {
		return "Error: " + err.Error(), nil
	}
	return fmt.Sprintf("Reminder created (ID: %s). You'll receive \"%s\" on schedule: %s", id, message, schedule), nil
}

func (t *Tools) listReminders(args map[string]string) (string, error) {
	if t.ReminderStore == nil {
		return "Reminders not configured.", nil
	}
	platform := args["platform"]
	userID := args["user_id"]
	if platform == "" || userID == "" {
		return "Error: missing user context.", nil
	}
	list, err := t.ReminderStore.ListForPlatform(platform, userID)
	if err != nil {
		return "Error: " + err.Error(), nil
	}
	if len(list) == 0 {
		return "No reminders scheduled.", nil
	}
	var b strings.Builder
	for i, r := range list {
		if i > 0 {
			b.WriteString("\n")
		}
		b.WriteString(fmt.Sprintf("- [%s] %s → \"%s\" (schedule: %s)", r.ID, r.CreatedAt.Format("2006-01-02"), r.Message, r.Schedule))
	}
	return b.String(), nil
}

func (t *Tools) deleteReminder(args map[string]string) (string, error) {
	if t.ReminderStore == nil {
		return "Reminders not configured.", nil
	}
	id := args["id"]
	if strings.TrimSpace(id) == "" {
		return "Error: reminder ID is required.", nil
	}
	if err := t.ReminderStore.Delete(id, args["platform"], args["user_id"]); err != nil {
		return "Error: " + err.Error(), nil
	}
	return "Reminder deleted.", nil
}

func runCommand(command string) (string, error) {
	command = strings.TrimSpace(command)
	if command == "" {
		return "", fmt.Errorf("empty command")
	}

	if IsBlocked(command) {
		return "Permission denied: command is blocked for safety.", nil
	}

	if IsSafe(command) {
		return executeCommand(command)
	}

	approved, err := LoadApprovals()
	if err != nil {
		return "", fmt.Errorf("failed to load approvals: %w", err)
	}
	if IsApproved(command, approved) {
		return executeCommand(command)
	}

	return "Permission denied. User can approve by saying 'approve: " + command + "' in chat.", nil
}

func executeCommand(command string) (string, error) {
	cmd := exec.Command("sh", "-c", command)
	cmd.Dir = getWorkDir()
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	err := cmd.Run()
	out := strings.TrimSpace(stdout.String())
	if stderr.Len() > 0 {
		out += "\nstderr: " + strings.TrimSpace(stderr.String())
	}
	if err != nil {
		return out, fmt.Errorf("command failed: %w", err)
	}
	if out == "" {
		return "(no output)", nil
	}
	return out, nil
}

func readFile(path string) (string, error) {
	path = resolvePath(path)
	data, err := os.ReadFile(path)
	if err != nil {
		return "", fmt.Errorf("read failed: %w", err)
	}
	return string(data), nil
}

func writeFile(path, content string) (string, error) {
	path = resolvePath(path)
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return "", fmt.Errorf("mkdir failed: %w", err)
	}
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		return "", fmt.Errorf("write failed: %w", err)
	}
	return fmt.Sprintf("Wrote %d bytes to %s", len(content), path), nil
}

func (t *Tools) webSearch(query string) (string, error) {
	if t.BraveSearchAPIKey == "" {
		return "", fmt.Errorf("BRAVE_SEARCH_API_KEY not configured")
	}

	u := "https://api.search.brave.com/res/v1/web/search?q=" + url.QueryEscape(query)
	req, err := http.NewRequest("GET", u, nil)
	if err != nil {
		return "", fmt.Errorf("search failed: %w", err)
	}
	req.Header.Set("X-Subscription-Token", t.BraveSearchAPIKey)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("search failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("search failed: HTTP %d", resp.StatusCode)
	}

	var result struct {
		Web struct {
			Results []struct {
				Title       string `json:"title"`
				URL         string `json:"url"`
				Description string `json:"description"`
			} `json:"results"`
		} `json:"web"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return "", fmt.Errorf("parse failed: %w", err)
	}

	if len(result.Web.Results) == 0 {
		return "No results found. Try a different query.", nil
	}

	var out strings.Builder
	for i, r := range result.Web.Results {
		if i >= 5 {
			break
		}
		if i > 0 {
			out.WriteString("\n\n")
		}
		out.WriteString(fmt.Sprintf("%d. %s\n   %s\n   %s", i+1, r.Title, r.URL, r.Description))
	}
	return out.String(), nil
}

func getWorkDir() string {
	wd, err := os.Getwd()
	if err != nil {
		return "."
	}
	return wd
}

func resolvePath(path string) string {
	if filepath.IsAbs(path) {
		return path
	}
	return filepath.Join(getWorkDir(), path)
}
