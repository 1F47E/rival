# Spec — rival antislop

**Status:** shipped — reconciled with implementation 2026-08-01
**Artifacts:** idea.md → spec.md (this) → plan-v1.0/1.1/1.2.md

Post-exec reconciliation (drift from the original spec):
- Stdin grammar is `parser.ParseReviewArgs`, not a new parser; `--` now sets
  a new `ParseResult.Escaped` flag and antislop treats an escaped scope as
  code mode verbatim (so `-- plan handling in the parser` is reviewable).
- Echo protection: all three prompts share the exact example-summary string
  (zero parser change) and `isPlaceholderFinding` matches the three contract
  category enums exactly — not any pipe-containing category, which would
  have dropped real dual-category findings like "bug|security".
- Runner: `RunDocReview(ctx, prompt, target, effort, workdir, groupID,
  noQueue, clis)`; antislop sessions keep mode/queue class "plan".
- Formatter: two label params on formatPlanBody + shared multi renderer,
  no opts struct.
- Shared `buildDiffPreamble(workdir) (preamble, files)` in cmd; both
  resolveGitScope and megareview refactored onto it.

## What

A quality-only, report-only review mode in the rival binary: an independent
model (Sol by default, Fable optional) hunts slop and over-engineering — never
correctness bugs — in either **changed code** or a **plan/spec document**, and
returns structured findings plus a leanness rating. The main Claude session
applies the cuts.

Derived from Claude Code's built-in `/simplify` skill (prompt text extracted
from the 2.1.220 binary), with three modifications:

1. **Report-only.** `/simplify`'s Phase 2 (apply fixes) is dropped; rival
   reviewers are read-only by architecture.
2. **Single-pass angles.** `/simplify` fans out 4 subagents; rival's model
   works the angles itself in sequence (it has repo read access via its own
   tools), like `/simplify`'s no-subagent fallback variant.
3. **Wider charter.** Over-engineering, backward-compat hoarding, library
   reinvention, DRY, and the five 8+/10 slop signatures are added.

## Surfaces

### CLI

One new command: `rival command antislop` (skill-facing, stdin args, queued,
detachable — same conventions as `rival command plan`).

- **Plan mode:** first stdin token `plan`, second the path:
  `plan path/to/plan.md`. Path resolution identical to `resolvePlanPath`
  (abs/`~`/workdir-relative, control-char rejection, regular-file check).
- **Code mode:** everything else is a review scope string. Empty scope →
  gitscope auto-detect of changed files (same `resolveGitScope` flow as
  `/rival-fable review`), falling back to "the entire project".
  A directory literally named `plan` must be passed as `./plan` (documented
  in usage text).
- Options — stdin (skill-facing): `-re <effort>`, `-m <model[,model]>`;
  native flags: `--effort`, `--model`, `--workdir`, `--no-queue`, plus the
  global `--detach`. Effort conflict between flag and stdin is an error
  (same rule as `mergePlanEffort`).
- **Default model: Sol only.** `-m fable` or `-m sol,fable` to switch/stack.
  Unavailable model = skipped, not fatal (same as plan review).
- Effort default: resolved per model from config, same as plan review
  (`config.DefaultPlanEffort` chain).

### Skills (embedded, installed by `rival install`)

- `/rival-antislop [scope]` — code mode. Usage:
  `/rival-antislop`, `/rival-antislop src/api/`, `/rival-antislop -m fable`,
  `/rival-antislop -re high src/`.
- `/rival-antislop-plan <path.md>` — plan mode. Usage:
  `/rival-antislop-plan plans/x/plan-v1.0.md`, optional `-m` / `-re`.

Both follow the `rival-plan-sol` SKILL.md pattern exactly: Write-tool input
file → launch `--detach` → background `rival wait` watcher → present stats
summary + full output verbatim as final message text. Registered in
`internal/skills/embed.go` (`//go:embed` + `Names`).

## Prompts

Two new consts in `internal/config/config.go`. Both end with the same JSON
contract as `PlanReviewPrompt` (summary + rating + findings) so
`review.ParsePlanOutput` parses both unchanged. `rating` is reinterpreted:
**leanness 1-10** — 10 = nothing left to cut, 1 = mostly slop.

Finding categories: `reuse|simplify|efficiency|altitude|compat|reinvention|slop|yagni`.

### AntislopCodePrompt

Placeholders: `{SCOPE}` (same as `ReviewPrompt`; the gitscope preamble with
the changed-file list is prepended in code mode exactly as for code reviews).

Charter (base text adapted from /simplify, additions marked):

- Role preamble: *"You are improving the quality of the changed code, not
  hunting for bugs. Do not report correctness bugs, security issues, or
  missing features — other reviews cover those."* Work all angles in
  sequence, single pass, using repo read tools.
- **Reuse & DRY** (base: /simplify "Reuse" + DRY addition): new code that
  re-implements something the codebase already has — grep shared/utility
  modules and files adjacent to the change, name the existing helper to call
  instead. *Addition:* duplicated logic across files/functions is a finding
  even when each copy is individually fine; name the single home for it.
- **Simplification** (base: /simplify): redundant or derivable state,
  copy-paste with slight variation, deep nesting, dead code left behind.
  Name the simpler form that does the same job.
- **Efficiency** (base: /simplify, near-verbatim): redundant computation or
  repeated I/O, independent operations run sequentially, blocking work added
  to startup or hot paths, long-lived objects built from closures that keep
  the enclosing scope alive. Name the cheaper alternative.
- **Altitude** (base: /simplify, near-verbatim): change implemented at the
  right depth, not as a fragile bandaid; special cases layered on shared
  infrastructure mean the fix isn't deep enough — prefer generalizing the
  underlying mechanism.
- **Backward-compat hoarding** (new): compat shims, legacy fallbacks,
  deprecated-but-kept paths, versioned duplicates (`doThingV2`), re-export
  layers kept "just in case". Default stance: delete the old path and
  migrate callers. Spare compat code only when a named external consumer
  (published API, on-disk format, wire protocol) depends on it — the finding
  must name that consumer or recommend the cut.
- **Library reinvention** (new): hand-rolled implementations of what a
  well-established library already does (parsers, retry/backoff, date math,
  semver, glob…). Prefer the language stdlib and the project's existing
  dependencies before naming a new dependency. The finding names the exact
  replacement package/function.
- **Slop signatures** (new, the four 8+ code items):
  - *Comment slop* — comments narrating the obvious, docstrings restating
    the signature, section banners, comments justifying the change to a
    reviewer. Keep only comments stating a constraint the code cannot show.
  - *Silent-fallback slop* — unrequested graceful degradation: quiet
    defaults on missing config, empty catch returning a zero value. Findings
    ask "where was this fallback specified?" — flag as unspecified behavior
    masking failures, don't assert the intended behavior.
  - *Wrapper/pass-through slop* — functions whose body is one call,
    interfaces with a single implementation, getters over public fields,
    grab-bag utils/helpers layers.
  - *Speculative-generality slop* — options nobody passes, parameters always
    called with the same value, generics used at one type, both directions
    implemented when only one is needed. Verify via call-site search before
    reporting.
- Rules: only report what you verified in the code (cite file:line); every
  finding names the concrete cut/replacement; if the diff is already lean,
  say so and return few or zero findings — do not invent problems.

### AntislopPlanPrompt

Placeholder: `{FILE}` (absolute path, like `PlanReviewPrompt`).

Charter — same stances at plan altitude. The reviewer reads the plan/spec
document and produces a cut list:

- **Scope-creep / YAGNI** — features, phases, and abstractions the stated
  goal doesn't need; work that can be deferred without loss. Each finding is
  a concrete cut/merge/defer with justification.
- **Gold-plating & premature optimization** — caching layers, perf work,
  configurability, and generality scheduled before anything demands them.
- **Compat hoarding (plan-level)** — migration shims/compat layers planned
  for interfaces with no external consumers.
- **Reinvention (plan-level)** — steps that spec building something a
  well-established library or an existing module in the same codebase
  already provides.
- **DRY (plan-level)** — the same mechanism designed twice in different
  sections of the plan.
- **Ceremony padding** (the 9/10 slop item) — boilerplate sections that
  exist because plans "should" have them: risk matrices, rollback plans,
  observability/monitoring phases, "Future Considerations" — for features
  whose size doesn't warrant them.
- Rules: judge cuts against the plan's own stated goal; a lean plan gets a
  high rating and zero findings; never pad the review itself.

## Runner & output plumbing (internal/review)

- Generalize the plan-review runner: `RunPlanReview` currently hardcodes
  `config.PlanReviewPrompt` (planrun.go). Extract a variant that accepts the
  final prompt string + the review target (feeds session reviewScope), with
  `RunPlanReview` becoming a thin wrapper. Queue, sessions, quota-skip,
  multi-model fan-out, and `PlanRunResult` are reused untouched; antislop
  sessions keep mode/queue class `plan`.
- Code mode builds its prompt as: gitscope preamble (changed files +
  diffstat, when auto-detected) + `AntislopCodePrompt` with `{SCOPE}`
  substituted — mirroring `resolveGitScope`.
- Parsing: `ParsePlanOutput` unchanged (the antislop JSON carries the same
  three keys; the schema-example summary sentinel gets its own antislop
  example string to skip echoes).
- Formatting: reuse `formatPlanBody` and the multi-model renderer with two
  parametrized strings: header `RIVAL ANTISLOP REVIEW` and the rating line
  labeled `Leanness: N/10`; empty-findings line `No slop found.`. Target
  line: `File: <path>` in plan mode, `Scope: <scope>` in code mode.

## Out of scope

- Applying fixes (report-only, decided).
- Consilium judge across models.
- Per-model skill variants (`-m` covers it).
- The dropped slop signatures rated <8: defensive slop, leftover slop, test
  slop, restating-the-obvious plan steps.
- CLAUDE.md-conventions angle from the binary (belongs to the session model,
  which owns CLAUDE.md context, not an external reviewer).

## Acceptance

1. `echo "plan plans/x/plan.md" | rival command antislop` returns a leanness
   rating + findings from Sol; `--model sol,fable` returns two blocks.
2. `echo "" | rival command antislop` (with uncommitted changes) auto-detects
   gitscope and reviews the changed files quality-only — output contains no
   bug/security findings by charter.
3. `/rival-antislop` and `/rival-antislop-plan` installed by `rival install`,
   launch detached, watcher presents verbatim output.
4. `go build ./... && go test ./...` green.
5. Existing plan review (`rival command plan`) behavior unchanged.
