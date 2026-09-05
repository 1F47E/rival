package skills

import "embed"

//go:embed all:rival-sol
//go:embed all:rival-astra
//go:embed all:rival-review
//go:embed all:rival-plan
//go:embed all:rival-plan-sol
//go:embed all:rival-plan-fable
//go:embed all:rival-fable
//go:embed all:rival-k3
//go:embed all:rival-grok
//go:embed all:rival-antislop
//go:embed all:rival-security
//go:embed codex.md.tmpl
var Files embed.FS

// Names lists all embedded skill directory names.
var Names = []string{"rival-sol", "rival-astra", "rival-review", "rival-plan", "rival-plan-sol", "rival-plan-fable", "rival-fable", "rival-k3", "rival-grok", "rival-antislop", "rival-security"}

// Deprecated lists legacy or superseded skills that should be removed on
// install. Re-enable a skill by adding it back to Names and the //go:embed list.
var Deprecated = []string{
	"rival-claude-only",
	"rival-fable-only",
	"rival-codex-only",
	"rival-plan-codex",
	"rival-gpt-5-6-sol",
	"rival-claude-fable",
	"rival-kimi",          // renamed to rival-k3 before release
	"rival-antislop-plan", // plan mode dropped on 2026-08-20
}
