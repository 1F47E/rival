# Plan v1.1 — rival antislop

**Status:** superseded by plan-v1.2.md (Fable review findings applied) — 2026-08-01
**Spec:** spec.md (approved) · **Prev:** plan-v1.0.md
**Branch:** feat/antislop in worktree worktrees/antislop

Changes from v1.0, per simplify findings: Stage 5 parser replaced with
`parser.ParseReviewArgs` reuse; gitscope preamble extracted to one shared
helper instead of a fourth copy; old Stage 3 (sentinel set) deleted — all
prompts share one example-summary string; new Stage 3 fixes the
`isPlaceholderFinding` category-enum gap (real bug); `renderOpts` dropped;
`mergeAntislopModels` dropped; `RunDocReview` keeps a `target` param; one
live smoke, not two. Two-skill split kept deliberately (user requirement).

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

## Stage 3 — Placeholder-finding fix (internal/review/parse.go)

`isPlaceholderFinding` (parse.go:94-98) recognizes echoed schema findings by
matching the literal code-review category enum only; an echoed antislop
schema finding would leak into output as a phantom finding. Fix at the
mechanism level: treat any `category` containing `|` as a placeholder
(covers plan, antislop, and every future enum), keeping the existing literal
checks. Test: echoed antislop schema example followed by a real payload
parses clean.

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
  parsed scope == `plan` → plan mode, second token is the path (validated
  via `resolvePlanPath`); anything else is code-mode scope. `./plan`
  escape documented in usage text.
- Model resolution: map parsed model names through the sol/fable validator
  (reuse `parsePlanModels`); stdin models + `--model` flag both set →
  error via inline `cmd.Flags().Changed("model")` check (no helper).
  Effort conflict: reuse `mergePlanEffort`.
- Code mode prompt: new shared helper `buildDiffPreamble(workdir) (string,
  bool)` in `gitscope_helper.go` (files + diffstat block, "" when no
  changes); `resolveGitScope` refactored onto it (byte-identical output);
  megareview's inline copy refactored onto it too if byte-identical with
  its own suffix, else left with a TODO. Antislop: preamble + `{SCOPE}` =
  "the changed files listed above", explicit scope verbatim, no changes →
  "the entire project".
- Plan mode prompt: `AntislopPlanPrompt` with `{FILE}`.
- Run `review.RunDocReview`, print `review.FormatAntislopResult`.
- Tests: mode detection table (plan/scope/`--`/options), model default =
  sol, conflict errors, code-mode prompt assembly (covers what the v1.0
  live code smoke would have).

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

- Antislop pass: DONE (4-angle fan-out, findings applied above).
- Sol plan review (`/rival-plan-sol` on this file) — WAIT; fallback
  Fable → self only on actual failure.
- One final whole-branch `/rival-fable` at Stage 8.
