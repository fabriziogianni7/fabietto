# Skills

User-installed skills extend the agent with new capabilities. Each skill is a directory containing `SKILL.md` and optional scripts.

## Skill Format

User skills live in `SKILLS_DIR` (default `./skills-data`), separate from this package source:

```
skills-data/          # or $SKILLS_DIR
└── my-skill/
    ├── SKILL.md       # Required: YAML frontmatter + Markdown instructions
    └── scripts/       # Optional: Python or shell scripts
        ├── process.py
        └── helper.sh
```

### SKILL.md

```markdown
---
name: my-skill
description: Brief description of what this skill does and when to use it.
---

# My Skill

## Instructions

Clear, step-by-step guidance for the agent.
```

### Supported Script Languages

- **Python** (`.py`): Preferred for non-trivial logic, data processing, API calls.
- **Bash** (`.sh`): For simple glue scripts only.

Other languages are not supported. Scripts are executed via `run_command` when the agent follows the skill.

## Adding Skills

1. **Manually**: Create a directory under `SKILLS_DIR` (default `./skills-data`) with `SKILL.md`.
2. **Via chat**: Send `newSkill` and paste your SKILL.md content when prompted. The agent runs security and feasibility checks before saving.

## Security

New skills are evaluated for:
- Prompt injection
- Dangerous shell patterns
- Exfiltration of secrets
- Unclear or infeasible instructions

Rejected skills are not saved.
