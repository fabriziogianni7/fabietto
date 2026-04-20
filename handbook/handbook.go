// Package handbook loads the agent handbook from a directory using manifest.txt.
package handbook

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

const (
	manifestName = "manifest.txt"
	separator    = "\n\n---\n\n"
)

// Load reads manifest.txt under root, concatenates listed files in order with stable
// separators. Paths in the manifest are relative to root. If exclude is non-nil,
// paths present as keys with value true are skipped (e.g. omit wallet instructions
// when the wallet is disabled). Returns a clear error if the manifest or any
// included file is missing.
func Load(root string, exclude map[string]bool) (string, error) {
	root = filepath.Clean(root)
	manifestPath := filepath.Join(root, manifestName)
	data, err := os.ReadFile(manifestPath)
	if err != nil {
		if os.IsNotExist(err) {
			return "", fmt.Errorf("handbook: missing %s under %q", manifestName, root)
		}
		return "", fmt.Errorf("handbook: read %s: %w", manifestPath, err)
	}

	var relPaths []string
	sc := bufio.NewScanner(strings.NewReader(string(data)))
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		if exclude != nil && exclude[line] {
			continue
		}
		relPaths = append(relPaths, line)
	}
	if err := sc.Err(); err != nil {
		return "", fmt.Errorf("handbook: parse manifest: %w", err)
	}

	if len(relPaths) == 0 {
		return "", fmt.Errorf("handbook: manifest %q has no file entries", manifestPath)
	}

	absRoot, err := filepath.Abs(root)
	if err != nil {
		return "", fmt.Errorf("handbook: resolve root %q: %w", root, err)
	}

	var parts []string
	for _, rel := range relPaths {
		rel = filepath.ToSlash(rel)
		full := filepath.Join(root, filepath.FromSlash(rel))
		absFull, err := filepath.Abs(full)
		if err != nil {
			return "", fmt.Errorf("handbook: resolve %q: %w", rel, err)
		}
		relToRoot, err := filepath.Rel(absRoot, absFull)
		if err != nil || strings.HasPrefix(relToRoot, "..") {
			return "", fmt.Errorf("handbook: manifest path %q escapes handbook root", rel)
		}
		b, err := os.ReadFile(full)
		if err != nil {
			if os.IsNotExist(err) {
				return "", fmt.Errorf("handbook: listed file missing: %q (under %q)", rel, root)
			}
			return "", fmt.Errorf("handbook: read %q: %w", rel, err)
		}
		parts = append(parts, strings.TrimSpace(string(b)))
	}
	return strings.Join(parts, separator), nil
}
