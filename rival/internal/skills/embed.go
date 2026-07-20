package skills

import "embed"

//go:embed all:rival-sol
//go:embed all:rival-review
//go:embed all:rival-plan
//go:embed all:rival-plan-sol
//go:embed all:rival-plan-fable
//go:embed all:rival-fable
//go:embed all:rival-k3
var Files embed.FS

// Names lists all embedded skill directory names.
var Names = []string{"rival-sol", "rival-review", "rival-plan", "rival-plan-sol", "rival-plan-fable", "rival-fable", "rival-k3"}

// Deprecated lists legacy or superseded skills that should be removed on
// install. Re-enable a skill by adding it back to Names and the //go:embed list.
var Deprecated = []string{
	"rival-gemini-only",
	"rival-antigravity-only",
	"rival-claude-only",
	"rival-fable-only",
	"rival-codex-only",
	"rival-plan-codex",
	"rival-gpt-5-6-sol",
	"rival-claude-fable",
	"rival-kimi", // renamed to rival-k3 before release
}
