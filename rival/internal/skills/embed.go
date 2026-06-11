package skills

import "embed"

//go:embed all:rival-codex-only
//go:embed all:rival-antigravity-only
//go:embed all:rival-review
//go:embed all:rival-plan
//go:embed all:rival-fable-only
var Files embed.FS

// Names lists all embedded skill directory names.
var Names = []string{"rival-codex-only", "rival-antigravity-only", "rival-review", "rival-plan", "rival-fable-only"}

// Deprecated lists skills that should be removed on install (superseded or disabled).
var Deprecated = []string{"rival-gemini-only", "rival-claude-only"}
