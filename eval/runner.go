package eval

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"custom-agent/compaction"
	"custom-agent/embedding"
	"custom-agent/memory"
	"custom-agent/session"
	"custom-agent/tools"
)

const (
	defaultPlatform = "eval"
	defaultUserID   = "runner"
)

type Runner struct {
	backend string
}

func NewRunner(backend string) *Runner {
	if strings.TrimSpace(backend) == "" {
		backend = "fake"
	}
	return &Runner{backend: backend}
}

func (r *Runner) RunCases(cases []EvalCase) Report {
	results := make([]CaseResult, 0, len(cases))
	for _, c := range cases {
		results = append(results, r.runCase(c))
	}
	return buildReport(r.backend, results)
}

func (r *Runner) runCase(c EvalCase) CaseResult {
	res := CaseResult{
		ID:          c.ID,
		Category:    c.Category,
		Description: c.Description,
		Passed:      true,
		Facts:       map[string]interface{}{},
	}
	if c.SchemaVersion != SchemaVersion {
		res.Passed = false
		res.Error = fmt.Sprintf("schema_version must be %q", SchemaVersion)
		return res
	}
	if strings.TrimSpace(c.ID) == "" || strings.TrimSpace(c.Category) == "" {
		res.Passed = false
		res.Error = "id and category are required"
		return res
	}

	var err error
	switch c.Operation.Type {
	case "tool_execute":
		res.Facts, err = r.runToolExecute(c.Operation)
	case "tool_parse":
		res.Facts, err = r.runToolParse(c.Operation)
	case "memory_search":
		res.Facts, err = r.runMemorySearch(c.Operation)
	case "compaction_estimate":
		res.Facts, err = r.runCompactionEstimate(c.Operation)
	case "compaction_prompt_block":
		res.Facts, err = r.runCompactionPromptBlock(c.Operation)
	case "compaction_merge":
		res.Facts, err = r.runCompactionMerge(c.Operation)
	case "session_recent":
		res.Facts, err = r.runSessionRecent(c.Operation)
	default:
		err = fmt.Errorf("unsupported operation type %q", c.Operation.Type)
	}
	if err != nil {
		res.Passed = false
		res.Error = err.Error()
		return res
	}

	for _, a := range c.Assertions {
		ar := evaluateAssertion(res.Facts, a)
		if !ar.Passed {
			res.Passed = false
		}
		res.Assertions = append(res.Assertions, ar)
	}
	return res
}

func (r *Runner) runToolExecute(op Operation) (map[string]interface{}, error) {
	tmpRoot := ""
	var err error
	if op.IsolateFS {
		tmpRoot, err = os.MkdirTemp("", "eval-tool-*")
		if err != nil {
			return nil, err
		}
		defer os.RemoveAll(tmpRoot)
		oldWd, wdErr := os.Getwd()
		if wdErr != nil {
			return nil, wdErr
		}
		if chErr := os.Chdir(tmpRoot); chErr != nil {
			return nil, chErr
		}
		defer os.Chdir(oldWd)
		if setErr := os.Setenv("TOOL_WORKSPACE_ROOTS", tmpRoot); setErr != nil {
			return nil, setErr
		}
		defer os.Unsetenv("TOOL_WORKSPACE_ROOTS")
	}

	t := tools.NewTools("", nil)
	argsJSON, err := json.Marshal(op.ToolArgs)
	if err != nil {
		return nil, err
	}
	output, execErr := t.ExecuteTool(op.ToolName, string(argsJSON))
	facts := map[string]interface{}{
		"tool_name": op.ToolName,
		"output":    output,
	}
	if execErr != nil {
		facts["error"] = execErr.Error()
	}
	return facts, nil
}

func (r *Runner) runToolParse(op Operation) (map[string]interface{}, error) {
	name, args, ok := tools.ParseToolCallFromContent(op.Content)
	return map[string]interface{}{
		"ok":        ok,
		"tool_name": name,
		"args_json": args,
	}, nil
}

func (r *Runner) runMemorySearch(op Operation) (map[string]interface{}, error) {
	var embedder embedding.Embedder
	switch strings.ToLower(strings.TrimSpace(op.EmbedderMode)) {
	case "", "disabled":
		embedder = nil
	case "deterministic":
		embedder = deterministicEmbedder{}
	case "fail":
		embedder = failingEmbedder{}
	default:
		return nil, fmt.Errorf("unsupported embedder_mode %q", op.EmbedderMode)
	}

	tmpRoot, err := os.MkdirTemp("", "eval-memory-*")
	if err != nil {
		return nil, err
	}
	defer os.RemoveAll(tmpRoot)
	oldWd, err := os.Getwd()
	if err != nil {
		return nil, err
	}
	if err := os.Chdir(tmpRoot); err != nil {
		return nil, err
	}
	defer os.Chdir(oldWd)

	store := memory.NewStore(embedder)
	for _, m := range op.Memories {
		if err := store.Save(defaultPlatform, defaultUserID, m.Content, m.Tags); err != nil {
			return nil, err
		}
	}
	limit := op.Limit
	if limit <= 0 {
		limit = 5
	}
	matches, err := store.Search(defaultPlatform, defaultUserID, op.Query, limit)
	if err != nil {
		return nil, err
	}
	content := make([]string, 0, len(matches))
	tags := make([]string, 0, len(matches))
	for _, m := range matches {
		content = append(content, m.Content)
		tags = append(tags, m.Tags)
	}
	return map[string]interface{}{
		"result_count":    len(matches),
		"result_contents": content,
		"result_tags":     tags,
	}, nil
}

func (r *Runner) runCompactionEstimate(op Operation) (map[string]interface{}, error) {
	msgs := make([]session.Message, 0, len(op.Messages))
	for _, m := range op.Messages {
		msgs = append(msgs, session.Message{Role: m.Role, Content: m.Content})
	}
	return map[string]interface{}{
		"estimated_tokens": compaction.EstimateTokens(msgs),
	}, nil
}

func (r *Runner) runCompactionPromptBlock(op Operation) (map[string]interface{}, error) {
	if op.Context == nil {
		return nil, fmt.Errorf("context is required")
	}
	block, err := op.Context.ToPromptBlock()
	if err != nil {
		return nil, err
	}
	return map[string]interface{}{
		"block": block,
	}, nil
}

func (r *Runner) runCompactionMerge(op Operation) (map[string]interface{}, error) {
	if op.Context == nil || op.Merge == nil {
		return nil, fmt.Errorf("context and merge are required")
	}
	base := *op.Context
	base.Merge(op.Merge)
	b, err := json.Marshal(base)
	if err != nil {
		return nil, err
	}
	var asMap map[string]interface{}
	if err := json.Unmarshal(b, &asMap); err != nil {
		return nil, err
	}
	return asMap, nil
}

func (r *Runner) runSessionRecent(op Operation) (map[string]interface{}, error) {
	msgs := make([]session.Message, 0, len(op.Messages))
	for _, m := range op.Messages {
		msgs = append(msgs, session.Message{Role: m.Role, Content: m.Content})
	}
	recent := session.Recent(msgs)
	return map[string]interface{}{
		"input_count":  len(msgs),
		"recent_count": len(recent),
		"first_recent": pickContent(recent, 0),
		"last_recent":  pickContent(recent, len(recent)-1),
	}, nil
}

func pickContent(msgs []session.Message, idx int) string {
	if len(msgs) == 0 || idx < 0 || idx >= len(msgs) {
		return ""
	}
	return msgs[idx].Content
}

func buildReport(backend string, results []CaseResult) Report {
	stats := map[string]*CategoryStat{}
	passed := 0
	for _, r := range results {
		if r.Passed {
			passed++
		}
		if _, ok := stats[r.Category]; !ok {
			stats[r.Category] = &CategoryStat{Category: r.Category}
		}
		stats[r.Category].Total++
		if r.Passed {
			stats[r.Category].Passed++
		} else {
			stats[r.Category].Failed++
		}
	}
	categories := make([]CategoryStat, 0, len(stats))
	for _, s := range stats {
		categories = append(categories, *s)
	}
	sort.Slice(categories, func(i, j int) bool {
		return categories[i].Category < categories[j].Category
	})

	return Report{
		SchemaVersion: SchemaVersion,
		Summary: Summary{
			Backend:       backend,
			CasesTotal:    len(results),
			CasesPassed:   passed,
			CasesFailed:   len(results) - passed,
			CategoryStats: categories,
		},
		Results: results,
	}
}

func evaluateAssertion(facts map[string]interface{}, a Assertion) AssertionResult {
	actual := lookupFact(facts, a.Key)
	ar := AssertionResult{
		Type:     a.Type,
		Key:      a.Key,
		Expected: a.Value,
		Actual:   actual,
	}
	switch a.Type {
	case "equals":
		ar.Passed = fmt.Sprintf("%v", actual) == fmt.Sprintf("%v", a.Value)
	case "contains":
		ar.Passed = strings.Contains(strings.ToLower(fmt.Sprintf("%v", actual)), strings.ToLower(fmt.Sprintf("%v", a.Value)))
	case "not_contains":
		ar.Passed = !strings.Contains(strings.ToLower(fmt.Sprintf("%v", actual)), strings.ToLower(fmt.Sprintf("%v", a.Value)))
	case "gte":
		ar.Passed = asFloat(actual) >= asFloat(a.Value)
	case "lte":
		ar.Passed = asFloat(actual) <= asFloat(a.Value)
	default:
		ar.Message = "unknown assertion type"
		ar.Passed = false
	}
	if !ar.Passed && ar.Message == "" {
		ar.Message = "assertion failed"
	}
	return ar
}

func lookupFact(facts map[string]interface{}, key string) interface{} {
	if !strings.Contains(key, ".") {
		return facts[key]
	}
	parts := strings.Split(key, ".")
	var cur interface{} = facts
	for _, p := range parts {
		switch c := cur.(type) {
		case map[string]interface{}:
			cur = c[p]
		default:
			return nil
		}
	}
	return cur
}

func asFloat(v interface{}) float64 {
	switch n := v.(type) {
	case float64:
		return n
	case float32:
		return float64(n)
	case int:
		return float64(n)
	case int64:
		return float64(n)
	case json.Number:
		f, _ := n.Float64()
		return f
	case string:
		f, _ := strconv.ParseFloat(strings.TrimSpace(n), 64)
		return f
	default:
		f, _ := strconv.ParseFloat(strings.TrimSpace(fmt.Sprintf("%v", n)), 64)
		return f
	}
}

type deterministicEmbedder struct{}

func (deterministicEmbedder) Embed(text string) ([]float32, error) {
	text = strings.ToLower(strings.TrimSpace(text))
	words := strings.Fields(text)
	var vec [4]float32
	for _, w := range words {
		switch {
		case strings.Contains(w, "pizza"), strings.Contains(w, "pasta"), strings.Contains(w, "food"), strings.Contains(w, "dinner"):
			vec[0] += 1
		case strings.Contains(w, "bike"), strings.Contains(w, "hiking"), strings.Contains(w, "fitness"), strings.Contains(w, "run"):
			vec[1] += 1
		case strings.Contains(w, "router"), strings.Contains(w, "network"), strings.Contains(w, "ops"), strings.Contains(w, "infra"):
			vec[2] += 1
		default:
			vec[3] += float32(len(w)%3 + 1)
		}
	}
	return []float32{vec[0], vec[1], vec[2], vec[3]}, nil
}

type failingEmbedder struct{}

func (failingEmbedder) Embed(_ string) ([]float32, error) {
	return nil, fmt.Errorf("forced embedding failure")
}

func LoadCases(dir string) ([]EvalCase, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}
	var cases []EvalCase
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".json") {
			continue
		}
		path := filepath.Join(dir, e.Name())
		data, err := os.ReadFile(path)
		if err != nil {
			return nil, err
		}
		var c EvalCase
		if err := json.Unmarshal(data, &c); err != nil {
			return nil, fmt.Errorf("%s: %w", e.Name(), err)
		}
		cases = append(cases, c)
	}
	sort.Slice(cases, func(i, j int) bool { return cases[i].ID < cases[j].ID })
	return cases, nil
}
