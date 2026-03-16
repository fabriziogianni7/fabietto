package agent

import (
	"context"
	"strings"
	"sync"

	"custom-agent/skills"
)

// newSkillState tracks users awaiting skill content paste.
var newSkillStateMu sync.Mutex
var newSkillAwaiting = make(map[string]bool)

func newSkillKey(platform, userID string) string {
	return platform + ":" + userID
}

func isNewSkillTrigger(text string) bool {
	t := strings.TrimSpace(strings.ToLower(text))
	return t == "newskill" || t == "/newskill" || strings.HasPrefix(t, "newskill ") || strings.HasPrefix(t, "/newskill ")
}

func isNewSkillCancel(text string) bool {
	t := strings.TrimSpace(strings.ToLower(text))
	return t == "cancel" || t == "/cancel"
}

// handleNewSkill processes the newSkill command flow.
// When user first says newSkill: ask for content and set awaiting state.
// When user sends content while awaiting: run security/feasibility checks and write.
func (a *Agent) handleNewSkill(ctx context.Context, text string, platform, userID string) (handled bool, result string) {
	key := newSkillKey(platform, userID)
	newSkillStateMu.Lock()
	awaiting := newSkillAwaiting[key]
	newSkillStateMu.Unlock()

	if awaiting {
		// User is pasting skill content
		newSkillStateMu.Lock()
		delete(newSkillAwaiting, key)
		newSkillStateMu.Unlock()

		// Extract skill from pasted content (may include --- YAML --- and body)
		skillMd := strings.TrimSpace(text)
		if len(skillMd) < 20 {
			return true, "Skill content too short. Please paste the full SKILL.md including YAML frontmatter (name, description) and body."
		}
		return true, a.processNewSkillContent(ctx, skillMd, platform, userID)
	}

	// First message: trigger
	if isNewSkillCancel(text) {
		return true, "Cancelled."
	}
	if !isNewSkillTrigger(text) {
		return false, ""
	}

	newSkillStateMu.Lock()
	newSkillAwaiting[key] = true
	newSkillStateMu.Unlock()

	return true, "I'll help you add a new skill. Please paste your SKILL.md content in one message. Include:\n\n1. YAML frontmatter with `name` and `description`\n2. The instruction body in Markdown\n\nExample:\n```\n---\nname: my-skill\ndescription: Does something useful.\n---\n\n# My Skill\n\nWhen the user asks for X, do Y.\n```\n\nIf you have scripts, mention them and we can add them next. Reply with `cancel` to abort."
}

// processNewSkillContent runs security/feasibility checks and persists the skill.
func (a *Agent) processNewSkillContent(ctx context.Context, skillMd string, platform, userID string) string {
	if a.skillsMgr == nil {
		return "Skills are not configured. Set SKILLS_DIR to enable."
	}

	// Extract and sanitize name from frontmatter
	name, ok := extractSkillNameFromMD(skillMd)
	if !ok || name == "" {
		return "Skill must include YAML frontmatter with a 'name' field. Example:\n---\nname: my-skill\ndescription: ...\n---"
	}
	name = sanitizeSkillName(name)
	if name == "" {
		return "Invalid skill name. Use only letters, numbers, hyphens, and underscores."
	}

	// Run security check
	secResult, err := skills.RunSecurityCheck(ctx, a.client, skillMd, nil)
	if err != nil {
		return "Security check failed: " + err.Error()
	}
	if !secResult.Safe || secResult.Severity == "high" {
		return "Skill rejected for security reasons:\n" + strings.Join(secResult.Issues, "\n") + "\n\nPlease revise and try again."
	}
	if len(secResult.Issues) > 0 {
		// Medium/low: warn but allow
		// Could show warnings to user - for now we proceed
	}

	// Run feasibility check
	feasResult, err := skills.RunFeasibilityCheck(ctx, a.client, skillMd, nil)
	if err != nil {
		return "Feasibility check failed: " + err.Error()
	}
	if !feasResult.Clear {
		return "Skill has clarity or feasibility issues:\n" + strings.Join(feasResult.Issues, "\n") + "\n\nPlease revise and try again."
	}

	// Persist
	if err := a.skillsMgr.WriteSkill(name, skillMd, nil); err != nil {
		return "Failed to save skill: " + err.Error()
	}
	return "Skill \"" + name + "\" added successfully. You can use it with read_skill or list_skills."
}

func sanitizeSkillName(s string) string {
	var b strings.Builder
	for _, c := range s {
		if (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') || (c >= '0' && c <= '9') || c == '-' || c == '_' {
			b.WriteRune(c)
		} else if c == ' ' || c == '.' {
			b.WriteRune('-')
		}
	}
	return strings.Trim(b.String(), "- ")
}

func extractSkillNameFromMD(content string) (string, bool) {
	content = strings.TrimSpace(content)
	if !strings.HasPrefix(content, "---") {
		return "", false
	}
	rest := content[3:]
	idx := strings.Index(rest, "---")
	if idx < 0 {
		return "", false
	}
	front := rest[:idx]
	lines := strings.Split(front, "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "name:") {
			return strings.TrimSpace(strings.TrimPrefix(line, "name:")), true
		}
	}
	return "", false
}
