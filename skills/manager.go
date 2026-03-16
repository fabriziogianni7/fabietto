package skills

import (
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"
)

const (
	skillFileName = "SKILL.md"
	scriptsDir    = "scripts"
)

// SkillSummary holds name and description for listing.
type SkillSummary struct {
	Name        string `json:"name"`
	Description string `json:"description"`
}

// ScriptMeta describes a script file in a skill.
type ScriptMeta struct {
	Path     string `json:"path"`
	Language string `json:"language"`
}

// Skill holds full skill content.
type Skill struct {
	Name        string       `json:"name"`
	Description string       `json:"description"`
	Body        string       `json:"body"`
	Scripts     []ScriptMeta `json:"scripts"`
}

// Manager discovers and reads skills from a directory.
type Manager struct {
	dir string
}

// NewManager creates a Manager for the given skills root directory.
func NewManager(dir string) *Manager {
	return &Manager{dir: dir}
}

// List returns all skill summaries (name + description). Returns empty slice if dir is empty or missing.
func (m *Manager) List() ([]SkillSummary, error) {
	if m.dir == "" {
		return nil, nil
	}
	entries, err := os.ReadDir(m.dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	var out []SkillSummary
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		path := filepath.Join(m.dir, e.Name(), skillFileName)
		b, err := os.ReadFile(path)
		if err != nil {
			continue
		}
		name, desc, _ := parseSkillMD(string(b), e.Name())
		if name == "" {
			name = e.Name()
		}
		out = append(out, SkillSummary{Name: name, Description: desc})
	}
	return out, nil
}

// Get returns the full skill by name. Name can be directory name or frontmatter name.
func (m *Manager) Get(name string) (Skill, error) {
	if m.dir == "" {
		return Skill{}, os.ErrNotExist
	}
	// Try direct directory match first
	skillPath := filepath.Join(m.dir, name, skillFileName)
	b, err := os.ReadFile(skillPath)
	if err != nil {
		// Try to find by scanning
		entries, err2 := os.ReadDir(m.dir)
		if err2 != nil {
			return Skill{}, err
		}
		for _, e := range entries {
			if !e.IsDir() {
				continue
			}
			p := filepath.Join(m.dir, e.Name(), skillFileName)
			data, err3 := os.ReadFile(p)
			if err3 != nil {
				continue
			}
			fmName, _, _ := parseSkillMD(string(data), e.Name())
			if fmName == name || e.Name() == name {
				return m.loadSkill(e.Name(), string(data))
			}
		}
		return Skill{}, err
	}
	return m.loadSkill(name, string(b))
}

// loadSkill parses SKILL.md and loads scripts metadata.
func (m *Manager) loadSkill(dirName, content string) (Skill, error) {
	name, desc, body := parseSkillMD(content, dirName)
	if name == "" {
		name = dirName
	}
	scriptsDirPath := filepath.Join(m.dir, dirName, scriptsDir)
	scripts, _ := listScripts(scriptsDirPath)
	return Skill{
		Name:        name,
		Description: desc,
		Body:        body,
		Scripts:     scripts,
	}, nil
}

// ReadScript returns the contents of a script file within a skill.
func (m *Manager) ReadScript(skillName, relPath string) (string, error) {
	if m.dir == "" {
		return "", os.ErrNotExist
	}
	// Sanitize: no path traversal
	relPath = filepath.Clean(relPath)
	if strings.HasPrefix(relPath, "..") || filepath.IsAbs(relPath) {
		return "", os.ErrInvalid
	}
	fullPath := filepath.Join(m.dir, skillName, relPath)
	data, err := os.ReadFile(fullPath)
	if err != nil {
		return "", err
	}
	return string(data), nil
}

// Dir returns the skills root directory.
func (m *Manager) Dir() string {
	return m.dir
}

// WriteSkill persists a skill to disk. Creates skills/<name>/SKILL.md and optional scripts.
func (m *Manager) WriteSkill(name string, skillMd string, scripts map[string]string) error {
	if m.dir == "" {
		return os.ErrInvalid
	}
	name = strings.TrimSpace(name)
	if name == "" {
		return os.ErrInvalid
	}
	// Sanitize name: only alphanumeric, hyphen, underscore
	for _, c := range name {
		if (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') || (c >= '0' && c <= '9') || c == '-' || c == '_' {
			continue
		}
		return os.ErrInvalid
	}
	skillDir := filepath.Join(m.dir, name)
	if err := os.MkdirAll(skillDir, 0755); err != nil {
		return err
	}
	skillPath := filepath.Join(skillDir, skillFileName)
	if err := os.WriteFile(skillPath, []byte(skillMd), 0644); err != nil {
		return err
	}
	scriptsDirPath := filepath.Join(skillDir, scriptsDir)
	for relPath, content := range scripts {
		relPath = filepath.Clean(relPath)
		if strings.HasPrefix(relPath, "..") || filepath.IsAbs(relPath) {
			continue
		}
		ext := strings.ToLower(filepath.Ext(relPath))
		if !AllowedScriptExtensions[ext] {
			continue // skip disallowed script types
		}
		fullPath := filepath.Join(scriptsDirPath, relPath)
		if err := os.MkdirAll(filepath.Dir(fullPath), 0755); err != nil {
			return err
		}
		if err := os.WriteFile(fullPath, []byte(content), 0644); err != nil {
			return err
		}
	}
	return nil
}

// parseSkillMD extracts YAML frontmatter (name, description) and body from SKILL.md content.
func parseSkillMD(content, dirName string) (name, description, body string) {
	content = strings.TrimSpace(content)
	if !strings.HasPrefix(content, "---") {
		return dirName, "", content
	}
	rest := content[3:]
	idx := strings.Index(rest, "---")
	if idx < 0 {
		return dirName, "", content
	}
	front := strings.TrimSpace(rest[:idx])
	body = strings.TrimSpace(rest[idx+3:])
	var fm struct {
		Name        string `yaml:"name"`
		Description string `yaml:"description"`
	}
	_ = yaml.Unmarshal([]byte(front), &fm)
	name = strings.TrimSpace(fm.Name)
	description = strings.TrimSpace(fm.Description)
	if name == "" {
		name = dirName
	}
	return name, description, body
}

// listScripts enumerates scripts in scripts/ and returns metadata with language guess.
func listScripts(dir string) ([]ScriptMeta, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}
	var out []ScriptMeta
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		path := filepath.Join(scriptsDir, e.Name())
		lang := guessLanguage(e.Name(), "")
		out = append(out, ScriptMeta{Path: path, Language: lang})
	}
	return out, nil
}

// AllowedScriptExtensions are the only script extensions permitted when writing skills.
// Python (.py) primary; Bash (.sh) for simple glue.
var AllowedScriptExtensions = map[string]bool{
	".py": true,
	".sh": true,
}

// guessLanguage returns a best-effort language tag from filename and optional first line.
func guessLanguage(filename, firstLine string) string {
	ext := strings.ToLower(filepath.Ext(filename))
	switch ext {
	case ".py":
		return "python"
	case ".sh", ".bash":
		return "bash"
	case ".js":
		return "javascript"
	default:
		if strings.HasPrefix(firstLine, "#!/usr/bin/env python") || strings.HasPrefix(firstLine, "#!/usr/bin/python") {
			return "python"
		}
		if strings.HasPrefix(firstLine, "#!/bin/bash") || strings.HasPrefix(firstLine, "#!/bin/sh") {
			return "bash"
		}
		return "unknown"
	}
}
