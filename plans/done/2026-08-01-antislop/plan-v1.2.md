# Plan v1.2 — rival antislop

**Status:** executed — all stages done; branch Fable review found 2 MEDIUM (over-wide pipe placeholder check, unescapable plan token), both fixed with tests; dogfood smoke rated this plan Leanness 9/10 — 2026-08-01
**Spec:** spec.md (approved) · **Prev:** plan-v1.1.md
**Branch:** feat/antislop in worktree worktrees/antislop

Changes from v1.1, per Fable review: `buildDiffPreamble` returns
`(preamble, files)` so `resolveGitScope` keeps its ReviewScope assignment
without a second git fork; plan-mode grammar fully specified (rest-of-input
path, bare `plan` = usage error); Stage 3 reworded to defense-in-depth with
a test that bites; fable-low effort fallback kept and stated + test row.

## Stage 1 — Prompt templates (internal/config/config.go)

- `AntislopCodePrompt` (`{SCOPE}`) and `AntislopPlanPrompt` (`{FILE}`),
  charters per spec.
- The two antislop prompts share ONE JSON-contract block (a private const
  assembled with the rating gloss "leanness" and the antislop category enum
  `reuse|simplify|efficiency|altitude|compat|reinvention|slop|yagni`).
  `PlanReviewPrompt` stays untouched.
- Both schema examples use the EXACT existing example-summary string from
  `PlanReviewPrompt` ("1-3 sentence overall assessment of the plan") so
  `ParsePlanOutput`'s echo-skip works unchanged with zero parser edits.
- Test: templates contain their placeholder, the JSON keys, the shared
  example summary, and the category enum.

## Stage 2 — Generalize the doc-review runner (internal/review/planrun.go)

- `RunDocReview(ctx context.Context, prompt, target, effort, workdir,
  groupID string, noQueue bool, clis []string) (*PlanRunResult, error)` —
  current `runPlanReview` body minus prompt construction; `target` feeds
  `session.NewQueued`'s reviewScope (v1.0 dropped it — session metadata
  regression). Antislop runs keep session mode/queue class `"plan"`
  (explicit decision: same queue semantics, no new mode).
- `RunPlanReview` = `RunDocReview(ctx, buildPlanPrompt(absPath), absPath, …)`.
- Existing planrun tests stay green untouched.

## Stage 3 — Placeholder-finding hardening (internal/review/parse.go)

Defense-in-depth, not a live bug: `isPlaceholderFinding` (parse.go:94-98)
ORs over file/severity/category literals, and a fully-echoed antislop schema
finding is already caught by its severity literal. The gap is the
partial-echo case — a model that copies the example `category` while
inventing a real severity. Fix at the mechanism level: treat any `category`
containing `|` as a placeholder (covers every future enum), keeping the
existing literal checks. Test: an echoed finding with a REAL severity
(`"high"`) + example category (the antislop pipe enum) is stripped — this
fails against unmodified code, proving the change.

## Stage 4 — Formatter (internal/review/plan.go)

- `formatPlanBody` gains two string params (`ratingLabel`, `emptyLine`);
  existing callers pass `"Rating"` / `"No bugs or gaps found."` — public
  plan functions keep signatures and byte-identical output.
- New `FormatAntislopResult(result *PlanRunResult, target string, planMode
  bool) string`: copies `FormatPlanResult`'s small dispatch; writes header
  `═══ RIVAL ANTISLOP REVIEW ═══` (+ model labels in multi mode), target
  line `File:` (plan mode) / `Scope:` (code mode), body via
  `formatPlanBody("Leanness", "No slop found.")`.
- Tests: antislop single + multi rendering; existing plan format tests
  unchanged.

## Stage 5 — Command (cmd/command_antislop.go)

- Cobra cmd `antislop` under `commandCmd`; flags `--workdir`, `--no-queue`,
  `--model/-m` (default `[sol]`), `--effort`; global `--detach` applies.
- Stdin parsing: `parser.ParseReviewArgs` (already implements `-re`/`-m`
  in any order, `=`-inline values, comma model lists, `--` escape, help,
  empty→AutoScope). No new parser. Mode selection: first token of the
  parsed scope == `plan` → plan mode, and EVERYTHING after that token is
  the path (paths with spaces preserved, matching `parsePlanInput`
  semantics; validated via `resolvePlanPath`); bare `plan` with no path is
  an explicit usage error. Anything else is code-mode scope. `./plan`
  escape documented in usage text.
- Model resolution: map parsed model names through the sol/fable validator
  (reuse `parsePlanModels`); stdin models + `--model` flag both set →
  error via inline `cmd.Flags().Changed("model")` check (no helper).
  Effort conflict: reuse `mergePlanEffort`. The inherited single-fable
  effort fallback (`runPlanReview` drops to "low" when the only model is
  fable, planrun.go:167-170) is KEPT for antislop — same rationale as plan
  reviews — and covered by a test row.
- Code mode prompt: new shared helper `buildDiffPreamble(workdir)
  (preamble, files string)` in `gitscope_helper.go` — files=="" means no
  changes; returning the raw files list lets `resolveGitScope` keep its
  `parsed.ReviewScope = files` assignment and logging without a second
  `gitscope.Resolve` fork. `resolveGitScope` refactored onto it
  (byte-identical output); megareview's inline copy refactored onto it too
  if byte-identical with its own suffix, else left with a TODO. Antislop:
  preamble + `{SCOPE}` = "the changed files listed above", explicit scope
  verbatim, no changes → "the entire project".
- Plan mode prompt: `AntislopPlanPrompt` with `{FILE}`.
- Run `review.RunDocReview`, print `review.FormatAntislopResult`.
- Tests: mode detection table (plan/scope/`--`/options, bare `plan` error,
  path with spaces), model default = sol, conflict errors, fable-low effort
  fallback, code-mode prompt assembly (covers what the v1.0 live code smoke
  would have).

## Stage 6 — Skills (internal/skills/)

- `rival-antislop/SKILL.md` (code) and `rival-antislop-plan/SKILL.md`
  (plan) — deliberate near-verbatim clones of the `rival-plan-sol`
  detach/watch/present pattern; the plan variant differs ONLY in
  name/description/usage and prepending `plan ` to the stdin input. No
  rewording of shared boilerplate (drift is the known failure mode).
- `embed.go`: two `//go:embed` lines + `Names` entries; `embed_test.go`
  additions.

## Stage 7 — Docs

- README antislop section; CHANGELOG entry.
- Post-release (outside repo): `~/.claude/docs/rival.md`; note that
  `/rival-antislop-plan` can serve the plan-v1.0 workflow gate.

## Stage 8 — Verify

- `go build ./...`, `go test ./...` in the worktree.
- ONE live smoke: `plan` mode on this plan file (dogfood), `--no-queue`.
- `/rival-fable` review of the branch; fix HIGH/CRITICAL.

## Stage 9 — Release

- Merge to master, `Skill(rival-release)` (bumps skill versions, commit,
  tag, push, CI publishes), reinstall locally, verify both skills appear.

## Deferred (noted, out of scope)

- `gitscope.ResolveWithStat` subprocess dedup (pre-existing, all callers).
- Sharing the JSON contract with `PlanReviewPrompt`/`reviewerJSONContract`.
- Unifying the megareview/review.go gitscope variants beyond the mechanical
  helper extraction.

## Review gates

- Antislop pass: DONE (4-angle fan-out, findings applied in v1.1).
- Plan review: DONE — Sol failed on quota (429); Fable fallback reviewed
  v1.1 at 8/10, 0 crit/high; all 4 med/low findings applied in this v1.2.
- One final whole-branch `/rival-fable` at Stage 8.
