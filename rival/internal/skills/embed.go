package skills

import "embed"

//go:embed all:rival-codex-only
//go:embed all:rival-gemini-only
//go:embed all:rival-claude-only
//go:embed all:rival-review
var Files embed.FS

// Names lists all embedded skill directory names.
var Names = []string{"rival-codex-only", "rival-gemini-only", "rival-claude-only", "rival-review"}
