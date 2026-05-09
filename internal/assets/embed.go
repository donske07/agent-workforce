package assets

import "embed"

// Files embeds the Agent Workforce frontend, bundled agents, and bundled skills.
//
//go:embed files/frontend/* files/forge/agents/*.md files/forge/skills/*/SKILL.md
var Files embed.FS
