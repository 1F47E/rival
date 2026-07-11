package skills

import "embed"

//go:embed all:rival-sol
//go:embed all:rival-antigravity-only
//go:embed all:rival-review
//go:embed all:rival-plan-sol
//go:embed all:rival-plan-fable
//go:embed all:rival-fable
var Files embed.FS

// Names lists all embedded skill directory names.
var Names = []string{"rival-sol", "rival-antigravity-only", "rival-review", "rival-plan-sol", "rival-plan-fable", "rival-fable"}

// Deprecated lists legacy or superseded skills that should be removed on
// install. Re-enable a skill by adding it back to Names and the //go:embed list.
var Deprecated = []string{
	"rival-gemini-only",
	"rival-claude-only",
	"rival-fable-only",
	"rival-plan",
	"rival-codex-only",
	"rival-plan-codex",
	"rival-gpt-5-6-sol",
	"rival-claude-fable",
}
