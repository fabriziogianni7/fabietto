package tools

import (
	"fmt"
	"strings"

	openai "github.com/sashabaranov/go-openai"
)

// RegisteredToolNames returns a copy of all tool names exposed to the model.
func RegisteredToolNames() []string {
	out := make([]string, len(toolNames))
	copy(out, toolNames)
	return out
}

// IsRegisteredTool returns true if name is a known tool.
func IsRegisteredTool(name string) bool {
	name = strings.TrimSpace(name)
	for _, n := range toolNames {
		if n == name {
			return true
		}
	}
	return false
}

// DefinitionsSubset returns only tools whose names appear in allow (order preserved by allow).
func DefinitionsSubset(allow []string) ([]openai.Tool, error) {
	if len(allow) == 0 {
		return nil, fmt.Errorf("empty tool allowlist")
	}
	all := Definitions()
	byName := make(map[string]openai.Tool, len(all))
	for _, t := range all {
		if t.Function != nil {
			byName[t.Function.Name] = t
		}
	}
	out := make([]openai.Tool, 0, len(allow))
	seen := make(map[string]bool)
	for _, name := range allow {
		name = strings.TrimSpace(name)
		if name == "" {
			continue
		}
		if seen[name] {
			continue
		}
		seen[name] = true
		t, ok := byName[name]
		if !ok {
			return nil, fmt.Errorf("unknown tool: %s", name)
		}
		out = append(out, t)
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("no valid tools in allowlist")
	}
	return out, nil
}
