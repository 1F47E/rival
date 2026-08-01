# Plan v1.0 — rival antislop

**Status:** superseded by plan-v1.1.md (antislop pass applied) — 2026-08-01
**Spec:** spec.md (approved)
**Branch:** feat/antislop in a fresh worktree off master

## Stage 1 — Prompt templates (internal/config/config.go)

Add two consts next to `PlanReviewPrompt`:

- `AntislopCodePrompt` — `{SCOPE}` placeholder; charter per spec (role
  preamble, 4 /simplify-derived angles + compat-hoarding + reinvention +
  slop signatures, rules), ending in the same JSON contract as
  `PlanReviewPrompt` with `rating` documented as leanness 1-10 and
  categories `reuse|simplify|efficiency|altitude|compat|reinvention|slop|yagni`.
- `AntislopPlanPrompt` — `{FILE}` placeholder; plan-level charter per spec.

Both schema examples keep their own example-summary sentinel strings (see
Stage 3). Test: `config` test asserting each template contains its
placeholder, the JSON keys, and no `{FILE}`/`{SCOPE}` typos.

## Stage 2 — Generalize the doc-review runner (internal/review/planrun.go)

- New exported `RunDocReview(ctx, prompt, effort, workdir, groupID string,
  noQueue bool, clis []string) (*PlanRunResult, error)` — the current
  `runPlanReview` body minus prompt construction.
- `RunPlanReview` becomes `RunDocReview(ctx, buildPlanPrompt(absPath), …)`.
- No behavior change; existing planrun tests must stay green untouched.

## Stage 3 — Parser sentinel set (internal/review/plan.go)

`ParsePlanOutput` skips the echoed schema example by comparing against the
single `planExampleSummary` string. Replace with a small sentinel set
containing the plan example plus the two antislop example summaries. Test:
raw output echoing an antislop schema example followed by a real payload
parses to the real payload.

## Stage 4 — Formatter parametrization (internal/review/plan.go)

Introduce `renderOpts{header, targetLabel, ratingLabel, emptyLine string}`;
thread through `formatPlanBody` / `FormatPlanConsole` /
`FormatPlanMultiConsole` / `FormatPlanResult` internals. Public plan
functions keep their exact signatures and output (defaults: header
`RIVAL PLAN REVIEW`, `File:`, `Rating:`, `No bugs or gaps found.`).
New `FormatAntislopResult(result, target, planMode)` using header
`RIVAL ANTISLOP REVIEW`, `File:`/`Scope:` per mode, `Leanness:`,
`No slop found.`. Tests: golden-ish assertions for both modes; existing
plan format tests unchanged.

## Stage 5 — Command (cmd/command_antislop.go)

Modeled on `command_plan.go`:

- Cobra cmd `antislop` under `commandCmd`; flags `--workdir`, `--no-queue`,
  `--model/-m` (default `[sol]`), `--effort`; global `--detach` works as-is.
- Stdin grammar (skill-facing), reusing `popPlanToken`/`splitPlanOption`:
  leading options `-re/--effort <v>` and `-m/--model <v>` in any order
  (repeatable `-m` not needed — comma list), then either `plan <path>`
  (plan mode) or the remainder as scope (code mode; empty → gitscope).
  `--` escape before a path/scope starting with `-`. Stdin/flag effort
  conflict → error (reuse `mergePlanEffort`); same for model (new
  `mergeAntislopModels`, same rule).
- Plan mode: `resolvePlanPath` → prompt = `AntislopPlanPrompt` with `{FILE}`.
- Code mode: scope string or `gitscope.Resolve(workdir)`; when auto-detected,
  prepend `DiffReviewPreamble` (files + diffstat) exactly like
  `resolveGitScope` and substitute `{SCOPE}` with "the changed files listed
  above"; explicit scope substitutes verbatim; no changes → "the entire
  project".
- Run `review.RunDocReview`, print `review.FormatAntislopResult`.
- Usage text (`antislopUsage`) covering both skills + native command, incl.
  the `./plan` escape note.
- Tests (`command_antislop_test.go`): stdin grammar table (options, plan
  mode, scope mode, `--` escape, conflicts, invalid model/effort), model
  parsing default=sol.

## Stage 6 — Skills (internal/skills/)

- `rival-antislop/SKILL.md` — code mode; frontmatter name/version/
  description/argument-hint/allowed-tools per existing skills; body cloned
  from `rival-plan-sol` pattern with: usage block, Write-tool input file,
  `rival command antislop --detach --workdir "$(pwd)"` launch, `rival wait`
  background watcher, verbatim presentation rules.
- `rival-antislop-plan/SKILL.md` — plan mode; same pattern, input
  `plan <path>` prepended to the stdin file by the skill instructions
  (user passes just the path).
- `embed.go`: two `//go:embed` lines + `Names` entries; `embed_test.go`
  updated.

## Stage 7 — Docs

- README: antislop section (what it is, both modes, examples, default Sol).
- CHANGELOG entry.
- Post-release (outside repo): `~/.claude/docs/rival.md` + note that
  `/rival-antislop-plan` can serve the plan-v1.0 gate.

## Stage 8 — Verify

- `go build ./...`, `go test ./...` in the worktree.
- Smoke: `echo "plan <this plan>" | rival command antislop --no-queue`
  (dogfood — antislop reviews its own plan); code mode smoke on the branch
  diff.
- `/rival-fable` review of the branch; fix HIGH/CRITICAL.

## Stage 9 — Release

- Merge to master (after user-visible review gate passed), then
  `Skill(rival-release)`: version bumps, commit, tag, push, CI release;
  reinstall locally; verify `/rival-antislop` + `/rival-antislop-plan`
  appear in installed skills.

## Review gates

- Antislop pass on this plan (built-in /simplify) before Sol.
- Sol plan review (`/rival-plan-sol`) — WAIT; fallback Fable → self only on
  actual failure.
- Per-stage `/rival-fable` not needed (single-session exec, one final
  whole-branch review at Stage 8).
