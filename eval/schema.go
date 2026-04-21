package eval

import "custom-agent/compaction"

const SchemaVersion = "v1"

type EvalCase struct {
	SchemaVersion string      `json:"schema_version"`
	ID            string      `json:"id"`
	Category      string      `json:"category"`
	Description   string      `json:"description"`
	Backend       string      `json:"backend,omitempty"`
	Operation     Operation   `json:"operation"`
	Assertions    []Assertion `json:"assertions"`
}

// EvalToolWallet injects a stub WalletService for tool_execute cases.
type EvalToolWallet struct {
	ERC20BalanceOutput string `json:"erc20_balance_output,omitempty"`
}

type Operation struct {
	Type string `json:"type"`

	// Generic tool op
	ToolName   string                 `json:"tool_name,omitempty"`
	ToolArgs   map[string]interface{} `json:"tool_args,omitempty"`
	ToolWallet *EvalToolWallet        `json:"tool_wallet,omitempty"`

	// Parse-tool-call op
	Content string `json:"content,omitempty"`

	// Memory-search op
	Memories     []MemorySeed `json:"memories,omitempty"`
	Query        string       `json:"query,omitempty"`
	Limit        int          `json:"limit,omitempty"`
	EmbedderMode string       `json:"embedder_mode,omitempty"` // disabled|deterministic|fail

	// Compaction/session ops
	Messages []MessageSeed                `json:"messages,omitempty"`
	Context  *compaction.CompactedContext `json:"context,omitempty"`
	Merge    *compaction.CompactedContext `json:"merge,omitempty"`

	IsolateFS bool `json:"isolate_fs,omitempty"`
}

type MemorySeed struct {
	Content string `json:"content"`
	Tags    string `json:"tags,omitempty"`
}

type MessageSeed struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type Assertion struct {
	Type  string      `json:"type"` // equals|contains|not_contains|gte|lte
	Key   string      `json:"key"`
	Value interface{} `json:"value"`
}

type CaseResult struct {
	ID          string                 `json:"id"`
	Category    string                 `json:"category"`
	Description string                 `json:"description"`
	Passed      bool                   `json:"passed"`
	DurationMs  float64                `json:"duration_ms"`
	Assertions  []AssertionResult      `json:"assertions"`
	Facts       map[string]interface{} `json:"facts"`
	Error       string                 `json:"error,omitempty"`
}

type AssertionResult struct {
	Type     string      `json:"type"`
	Key      string      `json:"key"`
	Expected interface{} `json:"expected"`
	Actual   interface{} `json:"actual"`
	Passed   bool        `json:"passed"`
	Message  string      `json:"message,omitempty"`
}

type Summary struct {
	Backend       string         `json:"backend"`
	CasesTotal    int            `json:"cases_total"`
	CasesPassed   int            `json:"cases_passed"`
	CasesFailed   int            `json:"cases_failed"`
	CategoryStats []CategoryStat `json:"category_stats"`
}

type CategoryStat struct {
	Category string `json:"category"`
	Total    int    `json:"total"`
	Passed   int    `json:"passed"`
	Failed   int    `json:"failed"`
}

type Report struct {
	SchemaVersion string       `json:"schema_version"`
	Summary       Summary      `json:"summary"`
	Results       []CaseResult `json:"results"`
}
