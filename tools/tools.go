package tools

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"

	openai "github.com/sashabaranov/go-openai"
	"github.com/sashabaranov/go-openai/jsonschema"
)

// Tool names for fallback parsing
var toolNames = []string{"run_command", "read_file", "write_file", "web_search"}

// Tools holds tool execution state (e.g. API keys). Create with NewTools.
type Tools struct {
	BraveSearchAPIKey string
}

// NewTools creates a Tools instance with the given Brave Search API key.
func NewTools(braveSearchAPIKey string) *Tools {
	return &Tools{BraveSearchAPIKey: braveSearchAPIKey}
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
	}
}

// ExecuteTool runs the named tool with the given JSON arguments and returns the result.
func (t *Tools) ExecuteTool(name, argsJSON string) (string, error) {
	var args map[string]string
	if err := json.Unmarshal([]byte(argsJSON), &args); err != nil {
		return "", fmt.Errorf("invalid arguments: %w", err)
	}

	switch name {
	case "run_command":
		return runCommand(args["command"])
	case "read_file":
		return readFile(args["path"])
	case "write_file":
		return writeFile(args["path"], args["content"])
	case "web_search":
		return t.webSearch(args["query"])
	default:
		return "", fmt.Errorf("unknown tool: %s", name)
	}
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
