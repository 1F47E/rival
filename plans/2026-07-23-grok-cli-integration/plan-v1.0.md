# Grok CLI Integration Implementation Plan v1.0

**Date:** 2026-07-23
**Status:** draft
**Spec:** ./spec.md
**Goal:** Add the xAI Grok CLI (`grok-4.5`) as a rival provider: `rival command grok`, `rival run grok`, `/rival-grok` skill, and an opt-in megareview selector, with review mode kernel-sandboxed read-only.
**Architecture:** New standalone executor modeled on codex.go, except the prompt travels via a temp `--prompt-file` (grok ignores stdin) and full-auto is `--yolo`. All shared plumbing (queue, `--detach`, `RIVAL_RUN_TIMEOUT`, `rival wait`, session store, log teeing) is reused untouched. Config/session/dashboard/review each gain a `"grok"` case.
**Tech Stack:** Go stdlib + cobra (existing deps only). Grok CLI 0.2.111 at runtime.

> For agentic workers: use superpowers:subagent-driven-development to implement task-by-task. Checkbox syntax for tracking.

## File map

**Modify:**
- `rival/internal/config/config.go` (+ `config_test.go`)
- `rival/internal/parser/parser.go` (+ `parser_test.go`)
- `rival/internal/session/session.go` (+ `session_test.go`)
- `rival/internal/dashboard/session_list.go` (+ `session_list_test.go`)
- `rival/internal/server/templates/index.html` (+ `server_test.go`)
- `rival/internal/review/role.go` (+ `role_test.go`)
- `rival/internal/review/runner.go`
- `rival/cmd/review.go`, `rival/cmd/command_megareview.go` (+ `cmd/review_model_test.go`)
- `rival/internal/skills/embed.go` (+ `embed_test.go`)
- `rival/internal/skills/rival-review/SKILL.md`
- `scripts/bump-skill-versions.sh`
- `README.md`, `CHANGELOG.md`, `docs/runtime-reference.md`

**Create:**
- `rival/internal/executor/grok.go`, `rival/internal/executor/grok_test.go`
- `rival/cmd/command_grok.go`, `rival/cmd/run_grok.go`
- `rival/internal/skills/rival-grok/SKILL.md`

**Out of scope:** `rival/internal/executor/quota.go` (no grok signatures yet), `rival/cmd/command_plan.go` + plan skills, `rival/internal/executor/subprocess.go`, default roster in `DefaultReviewTargets`.

## Locked interfaces

```go
// internal/config/config.go
const GrokModel = "grok-4.5"
const GrokLabel = "grok"

// internal/executor/grok.go
func GrokPreflight() error
func RunGrok(ctx context.Context, sess *session.Session, prompt, effort, workdir string, review bool, mirror io.Writer) (*Result, error)
func grokRunArgs(model, promptFile, effort, workdir string, review bool) ([]string, error)
func grokEffort(effort string) (string, error) // minimal/low/medium/high/xhigh/max verbatim; "ultra"->"max"; else error

// internal/parser/parser.go
func ParseGrokArgs(raw string) (*ParseResult, error)
```

Session CLI adapter string is `"grok"` everywhere (session.NewQueued first arg, review switches, labels).

---

### Task 0 — Baseline sanity

- [ ] `cd rival && go build ./... && go test ./...` — all green on a fresh worktree off master (post-1c2c2ec). If red, STOP: base is broken, report.

### Task 1 — Config: grok model surface

**Files:** Modify `rival/internal/config/config.go`, `rival/internal/config/config_test.go`.

- [ ] Failing tests first in `config_test.go`: `ModelLabel(GrokModel)=="grok"`, `ModelLabel("grok")=="grok"`; `EngineLabel` maps both the exact model id and adapter `"grok"` to `GrokLabel`; effort resolution — default effort for grok is `xhigh`, configured `efforts: {grok: <level>}` accepted, invalid key error message updated; `replaceConcreteModelIDs` rewrites `grok-4.5`→`grok`. Run `go test ./internal/config/ -run 'Grok|ModelLabel|EngineLabel|Effort'` → FAIL.
- [ ] Implement: add `GrokModel`/`GrokLabel` consts; cases in `ModelLabel`, `EngineLabel`, `replaceConcreteModelIDs`, `publicReviewHeader`, `builtinModelEffort` (grok→xhigh), `knownEffortModel`, `validConfiguredModelEffort`, `ResolveEffort`, `DefaultEffortForModel`; extend the valid-model error strings that enumerate `sol/kimi-k3/fable` to include `grok`.
- [ ] `go test ./internal/config/` → PASS. `go build ./...` green.

### Task 2 — Executor: grok.go

**Files:** Create `rival/internal/executor/grok.go`, `rival/internal/executor/grok_test.go`.

- [ ] Failing tests first (pattern `codex_test.go`), table-driven:
  - `grokEffort`: each of minimal/low/medium/high/xhigh/max maps verbatim; `ultra`→`max`; `""`→error; `bogus`→error.
  - `grokRunArgs(model, promptFile, effort, workdir, review)`: always contains `--prompt-file <promptFile>`, `-m grok-4.5`, `--effort <mapped>`, `--output-format plain`, `--no-auto-update`, `--yolo`; contains `--cwd <workdir>` iff workdir non-empty; contains `--sandbox read-only` iff `review==true`; never contains a bare `-p`.
  - Prompt-file content helper: full prompt written = `config.SystemPrompt + "\n\n" + config.BuildWorkdirPreamble(workdir) + "\n" + prompt` (extract a small pure func, e.g. `grokFullPrompt(prompt, workdir string) string`, and test it).
- [ ] Implement `GrokPreflight` (LookPath `grok`; then `$GROK_HOME`-or-`~/.grok` `auth.json` exists OR `XAI_API_KEY` non-empty; actionable error messages naming `grok login` / `XAI_API_KEY`), `RunGrok` (temp file `os.CreateTemp("", "rival-grok-*.md")`, defer remove, `RunSubprocess` with empty stdin prompt `""`, env addition `GROK_DISABLE_AUTOUPDATER=1` — check `RunSubprocess`'s env hook; if it only supports dropEnv, set the var via `cmd` env in the same pattern the codebase already uses, or omit and rely on grok's non-TTY stderr auto-suppression, noting which in the commit).
- [ ] `go test ./internal/executor/` → PASS.

### Task 3 — Parser + session rank

**Files:** Modify `rival/internal/parser/parser.go` (+test), `rival/internal/session/session.go` (+test).

- [ ] Failing tests: `ParseGrokArgs` follows the `ParseGPT56SolArgs` pattern (standard effort ladder, same defaults); `groupModelRank` returns a stable slot for `config.GrokLabel` (after fable, exact rank per existing ordering — read the current func and slot grok last among named models, before the default case).
- [ ] Implement both. `go test ./internal/parser/ ./internal/session/` → PASS.

### Task 4 — `rival command grok`

**Files:** Create `rival/cmd/command_grok.go`.

- [ ] Mirror `cmd/command_codex.go` exactly: usage const, cobra registration under `commandCmd` in `init()`, flags `--workdir`/`--no-queue` (`--detach` inherited), stdin raw-args read, `parser.ParseGrokArgs`, `executor.GrokPreflight()`, `config.ResolveEffort(config.GrokModel, …)`, `session.NewQueued("grok", mode, config.GrokModel, effort, …)`, `waitForQueueSlot`, `config.WithRunTimeout(ctx, 1)`, `executor.RunGrok(…, review=false, mirror=nil)`, Complete/Fail + `config.PublicRuntimeLog`.
- [ ] `go build ./...` green; `echo "hello" | go run . command grok --no-queue` smoke (may be skipped if binary auth unavailable in CI — verify locally in Task 9).

### Task 5 — `rival run grok`

**Files:** Create `rival/cmd/run_grok.go`.

- [ ] Mirror `cmd/run_codex.go`: flags `--effort --workdir --prompt-stdin --review --no-queue`, mirror=`os.Stdout`, `--review` → review prompt build + `RunGrok(review=true)`, raw `--prompt-stdin` → `RunGrok(review=false)`.
- [ ] `go build ./...` green.

### Task 6 — Dashboard + web labels

**Files:** Modify `rival/internal/dashboard/session_list.go` (+test), `rival/internal/server/templates/index.html`, `rival/internal/server/server_test.go`.

- [ ] Failing tests: `cliLabel` for a session with CLI `"grok"` → icon+label per existing table style. Icon: `𝕏` primary; if the existing test helpers measure rune width / alignment or manual TUI check misaligns, use `✖` instead — the test locks whichever ships.
- [ ] Implement icon const + `cliLabel` case + `CLI_ICONS` entry in index.html (same glyph).
- [ ] `go test ./internal/dashboard/ ./internal/server/` → PASS.

### Task 7 — Megareview opt-in selector

**Files:** Modify `rival/internal/config/config.go` (+test), `rival/internal/review/role.go` (+test), `rival/internal/review/runner.go`, `rival/cmd/review.go`, `rival/cmd/command_megareview.go`, `rival/cmd/review_model_test.go`, `rival/internal/skills/rival-review/SKILL.md`.

- [ ] Failing tests: `ResolveReviewTargets(["grok"])` → `[{CLI:"grok", Model:GrokModel, Role:"bug_hunter"}]`; mixed `["sol","grok"]` order preserved + deduped; unknown-selector error text includes `grok` in the valid list; `RoleForCLI("grok")==RoleBugHunter`; `cmd/review_model_test.go` row for `--models grok`.
- [ ] Implement: selector case in `ResolveReviewTargets`; `role.go` case; `runner.go` — preflight switch (`executor.GrokPreflight()`), `runReviewer` exec switch (`executor.RunGrok(…, review=true, …)`), `runConsilium` judge switch (same call shape as other judges; grok stays LAST in any judge-preference ordering — do not alter Sol→K3 priority), `modelForCLIToo` (`modelForCLI` case → `config.GrokModel`); help/error text in `review.go` + `command_megareview.go`; add `grok` to rival-review SKILL.md argument-hint + usage lines.
- [ ] `go test ./internal/config/ ./internal/review/ ./cmd/` → PASS.

### Task 8 — `/rival-grok` skill + embedding

**Files:** Create `rival/internal/skills/rival-grok/SKILL.md`; modify `rival/internal/skills/embed.go`, `rival/internal/skills/embed_test.go`, `scripts/bump-skill-versions.sh`.

- [ ] Failing test: embed_test asserts `rival-grok` present in `Names`/embedded FS with a `version:` frontmatter.
- [ ] Write SKILL.md by copying `rival-k3/SKILL.md` structure verbatim, adapted: `name: rival-grok`, description "Run Grok (grok-4.5) through the rival binary, detached and watched in the background. Use only when the user explicitly invokes /rival-grok.", Step 1 `rival command grok --detach --workdir "$(pwd)"` via the Write-tool prompt-file pattern the k3 skill uses (no heredoc), Step 2 `rival wait --log <err> --timeout 75m` with `run_in_background: true`, Step 3 end turn, present OUT verbatim on completion. Same `version:` stamp as the other skills' current value.
- [ ] Add `//go:embed all:rival-grok` + `"rival-grok"` in `Names`; add the dir to `SKILL_DIRS` in `scripts/bump-skill-versions.sh`.
- [ ] `go test ./internal/skills/` → PASS.

### Task 9 — Live e2e + docs

**Files:** Modify `README.md`, `CHANGELOG.md`, `docs/runtime-reference.md`.

- [ ] `cd rival && make install`, then live checks (grok is authenticated on this machine):
  - `printf 'say the word pineapple and stop' | rival run grok --prompt-stdin --no-queue` → completes, session visible in `rival sessions`, TUI/web label `grok` renders aligned.
  - Effort menu probe: run once with `--effort xhigh` and once with a level grok might reject; if grok errors on a tier, clamp mapping in `grokEffort` to nearest supported and update its test (spec allows this).
  - Review sandbox proof: `rival run grok --review` (or a raw prompt with review=true path) asking grok to create `/tmp/rival-grok-sandbox-proof`; verify the file does NOT exist afterward and the transcript shows the write blocked.
  - Detach path: `rival command grok --detach` + `rival wait --log` exits 0.
- [ ] Docs: CHANGELOG `Unreleased` entry (grok provider, opt-in selector, macOS network-sandbox caveat); README provider roster + auth note (grok.com OAuth precedence, `XAI_API_KEY` fallback); runtime-reference invocation shape.
- [ ] Full gate: `cd rival && make test` green.

---

**Type-consistency check:** `GrokModel`/`GrokLabel` (Tasks 1,3,4,7), `RunGrok(ctx, sess, prompt, effort, workdir string, review bool, mirror io.Writer)` (Tasks 2,4,5,7), `ParseGrokArgs` (Tasks 3,4), CLI string `"grok"` (Tasks 4,6,7), skill dir `rival-grok` (Task 8) — all match the Locked interfaces block.

**Review gates:** per-task `/rival-fable` after each code-writing task per global workflow; one final whole-branch review before merge.
