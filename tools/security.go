package tools

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

type parsedCommand struct {
	Executable string
	Args       []string
	Identity   string
}

var (
	autoAllowedCommands = map[string]bool{
		"ls":     true,
		"pwd":    true,
		"whoami": true,
		"date":   true,
		"id":     true,
		"head":   true,
		"tail":   true,
		"wc":     true,
		"file":   true,
		"rg":     true,
	}
	approvalAllowedCommands = map[string]bool{
		"go":   true,
		"git":  true,
		"make": true,
	}
)

func parseCommand(command string) (parsedCommand, error) {
	command = strings.TrimSpace(command)
	if command == "" {
		return parsedCommand{}, fmt.Errorf("empty command")
	}
	if strings.ContainsAny(command, "\r\n") {
		return parsedCommand{}, fmt.Errorf("command chaining or multiline input is not allowed")
	}
	if strings.ContainsAny(command, `;&|><`+"`"+`$(){}[]*?!~\\`) {
		return parsedCommand{}, fmt.Errorf("shell metacharacters are not allowed")
	}

	fields := strings.Fields(command)
	if len(fields) == 0 {
		return parsedCommand{}, fmt.Errorf("empty command")
	}
	execName := strings.ToLower(fields[0])
	identity := execName
	if len(fields) > 1 {
		identity += " " + strings.Join(fields[1:], " ")
	}
	return parsedCommand{
		Executable: execName,
		Args:       fields[1:],
		Identity:   identity,
	}, nil
}

func isAllowedExecutable(execName string) bool {
	return autoAllowedCommands[execName] || approvalAllowedCommands[execName]
}

func needsApproval(execName string) bool {
	return approvalAllowedCommands[execName]
}

func getWorkspaceRoots() []string {
	if roots := os.Getenv("TOOL_WORKSPACE_ROOTS"); strings.TrimSpace(roots) != "" {
		parts := strings.Split(roots, ":")
		out := make([]string, 0, len(parts))
		for _, p := range parts {
			p = strings.TrimSpace(p)
			if p == "" {
				continue
			}
			abs, err := filepath.Abs(p)
			if err != nil {
				continue
			}
			out = append(out, filepath.Clean(abs))
		}
		if len(out) > 0 {
			return out
		}
	}
	return []string{getRepoRoot()}
}

func getRepoRoot() string {
	wd := getWorkDir()
	cur := wd
	for {
		gitPath := filepath.Join(cur, ".git")
		if _, err := os.Stat(gitPath); err == nil {
			return cur
		}
		next := filepath.Dir(cur)
		if next == cur {
			return wd
		}
		cur = next
	}
}

func resolvePathWithinWorkspace(path string, forWrite bool) (string, error) {
	if strings.TrimSpace(path) == "" {
		return "", fmt.Errorf("path is required")
	}
	if strings.Contains(path, "\x00") {
		return "", fmt.Errorf("invalid path")
	}

	candidate := path
	if !filepath.IsAbs(candidate) {
		candidate = filepath.Join(getWorkDir(), candidate)
	}
	candidate = filepath.Clean(candidate)
	abs, err := filepath.Abs(candidate)
	if err != nil {
		return "", fmt.Errorf("invalid path: %w", err)
	}

	checked := abs
	if forWrite {
		parent := filepath.Dir(abs)
		if resolvedParent, err := filepath.EvalSymlinks(parent); err == nil {
			checked = filepath.Join(resolvedParent, filepath.Base(abs))
		}
		if resolvedTarget, err := filepath.EvalSymlinks(abs); err == nil {
			checked = resolvedTarget
		}
	} else {
		resolved, err := filepath.EvalSymlinks(abs)
		if err != nil {
			return "", fmt.Errorf("path resolution failed: %w", err)
		}
		checked = resolved
	}

	for _, root := range getWorkspaceRoots() {
		rootAbs, err := filepath.Abs(root)
		if err != nil {
			continue
		}
		rootAbs = filepath.Clean(rootAbs)
		if checked == rootAbs || strings.HasPrefix(checked, rootAbs+string(filepath.Separator)) {
			return abs, nil
		}
	}

	return "", fmt.Errorf("path %q is outside allowed workspace roots", path)
}
