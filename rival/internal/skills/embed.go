package skills

import "embed"

//go:embed all:rival-codex-only
//go:embed all:rival-antigravity-only
//go:embed all:rival-review
//go:embed all:rival-plan-codex
//go:embed all:rival-plan-fable
var Files embed.FS

// Names lists all embedded skill directory names.
var Names = []string{"rival-codex-only", "rival-antigravity-only", "rival-review", "rival-plan-codex", "rival-plan-fable"}

// Deprecated lists skills that should be removed on install (superseded or
// disabled). The dual `rival-plan` skill was dropped in favour of the
// single-engine `rival-plan-codex` / `rival-plan-fable`; `rival command plan`
// (incl. its dual `--cli codex,fable`) stays in the binary and is still runnable
// from the CLI. rival-fable-only is disabled — the `rival command fable` executor
// stays in the binary, but no skill ships. Re-enable a skill by adding it back to
// Names and the //go:embed list above.
var Deprecated = []string{"rival-gemini-only", "rival-claude-only", "rival-fable-only", "rival-plan"}
