package skills

import "embed"

//go:embed all:rival-gpt-5-6-sol
//go:embed all:rival-antigravity-only
//go:embed all:rival-review
//go:embed all:rival-plan-sol
//go:embed all:rival-plan-fable
//go:embed all:rival-claude-fable
var Files embed.FS

// Names lists all embedded skill directory names.
var Names = []string{"rival-gpt-5-6-sol", "rival-antigravity-only", "rival-review", "rival-plan-sol", "rival-plan-fable", "rival-claude-fable"}

// Deprecated lists skills that should be removed on install (superseded or
// disabled). The dual `rival-plan` skill was dropped in favour of the
// single-model plan skills. `rival-plan-codex` was replaced by
// `rival-plan-sol`, which names its model directly. rival-fable-only (the old
// general fable runner) is superseded by rival-claude-fable (code review at
// medium effort). Re-enable a skill by adding it back to Names and the
// //go:embed list above.
var Deprecated = []string{"rival-gemini-only", "rival-claude-only", "rival-fable-only", "rival-plan", "rival-codex-only", "rival-plan-codex"}
