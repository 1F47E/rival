# Grok CLI Integration Implementation Plan v1.1

**Date:** 2026-07-25
**Status:** done
**Spec:** ./spec.md
**Supersedes:** plan-v1.0.md (Sol review 3/10, 10 findings — all folded in; see "Review deltas" at bottom)
**Goal:** Add the xAI Grok CLI (`grok-4.5`) as a rival provider: `rival command grok`, `rival run grok`, `/rival-grok` skill, and an opt-in megareview selector, with review mode sandboxed read-only and the grok runtime shielded from repo-`.env` injection.
**Architecture:** New standalone executor modeled on codex.go, except the prompt travels via a temp `--prompt-file` (grok ignores stdin) and full-auto is `--yolo`. All shared plumbing (queue, `--detach`, `RIVAL_RUN_TIMEOUT`, `rival wait`, session store, log teeing) reused untouched. Config/session/dashboard/review each gain a `"grok"` case. `blockedEnvPrefixes` gains `GROK_`/`XAI_`.
**Tech Stack:** Go stdlib + cobra (existing deps only). Grok CLI 0.2.111 at runtime.

> For agentic workers: use superpowers:subagent-driven-development to implement task-by-task. Checkbox syntax for tracking.

## File map

**Modify:**
- `rival/internal/config/config.go` (+ `config_test.go`)
- `rival/internal/executor/subprocess.go` (+ `subprocess_test.go`)
- `rival/internal/parser/parser.go` (+ `parser_test.go`)
- `rival/internal/session/session.go` (+ `session_test.go`)
- `rival/internal/dashboard/session_list.go` (+ `session_list_test.go`)
- `rival/internal/server/templates/index.html` (+ `server_test.go`)
- `rival/internal/review/role.go` (+ `role_test.go`)
- `rival/internal/review/runner.go` (+ runner dispatch tests)
- `rival/cmd/review.go`, `rival/cmd/command_megareview.go` (+ `cmd/review_model_test.go`)
- `rival/internal/skills/embed.go` (+ `embed_test.go`)
- `rival/internal/skills/rival-review/SKILL.md`
- `scripts/bump-skill-versions.sh`
- `README.md`, `CHANGELOG.md`, `docs/runtime-reference.md`

**Create:**
- `rival/internal/executor/grok.go`, `rival/internal/executor/grok_test.go`
- `rival/cmd/command_grok.go`, `rival/cmd/run_grok.go`
- `rival/internal/skills/rival-grok/SKILL.md`

**Out of scope:** `quota.go` (no grok signatures yet), plan review surfaces, `RunSubprocess` signature (env blocklist is a var edit only), `DefaultReviewTargets`, judge-order special-casing.

## Locked interfaces

```go
// internal/config/config.go
const GrokModel = "grok-4.5"
const GrokLabel = "grok"
// builtinModelEffort(GrokLabel) == "high"  — equals DefaultReviewEffort, so the
// standard call-site fallback (config.DefaultReviewEffort) is CORRECT for grok;
// call sites mirror codex verbatim, no special fallback handling.

// internal/executor/grok.go
func GrokPreflight() error   // LookPath("grok") + ~/.grok/auth.json exists; errors name `grok login`
func RunGrok(ctx context.Context, sess *session.Session, prompt, effort, workdir string, review bool, mirror io.Writer) (*Result, error)
func grokRunArgs(model, promptFile, effort, workdir string, review bool) ([]string, error)
func grokEffort(effort string) (string, error)
// grokEffort table (grok-4.5 menu is exactly low/medium/high, default high —
// verified in ~/.grok/models_cache.json reasoning_efforts):
//   low|medium|high -> verbatim
//   xhigh|ultra|max -> "high" (clamp)
//   minimal|none    -> "low"  (clamp)
//   ""              -> error (caller resolves first)
//   anything else   -> error

// internal/parser/parser.go
func ParseGrokArgs(raw string) (*ParseResult, error)
```

Session CLI adapter string is `"grok"` everywhere. Review-mode boolean is ALWAYS derived from the parsed/derived session mode (`mode == "review"`), never hardcoded.

---

### Task 0 — Baseline sanity

- [ ] Fresh worktree off master (post-c6f68fc). `cd rival && go build ./... && go test ./...` all green. If red, STOP and report.

### Task 1 — Config: grok model surface

**Files:** Modify `rival/internal/config/config.go`, `rival/internal/config/config_test.go`.

- [ ] Failing tests first: `ModelLabel(GrokModel)=="grok"` and `ModelLabel("grok")=="grok"`; `EngineLabel` for exact id + adapter `"grok"`; `DefaultEffortForModel(GrokModel)=="high"`; `ResolveEffort(GrokModel, "", DefaultReviewEffort)=="high"`; configured `efforts: {grok: low}` honored; invalid-model error strings now enumerate `grok`; `replaceConcreteModelIDs` rewrites `grok-4.5`→`grok`. Run `go test ./internal/config/ -run 'Grok|ModelLabel|EngineLabel|Effort'` → FAIL.
- [ ] Implement: consts + cases in `ModelLabel`, `EngineLabel`, `replaceConcreteModelIDs`, `publicReviewHeader`, `builtinModelEffort` (grok→**high**), `knownEffortModel`, `validConfiguredModelEffort`, `ResolveEffort` (no k3-style pin; generic path), `DefaultEffortForModel`; extend valid-model error strings.
- [ ] `go test ./internal/config/` PASS; `go build ./...` green.

### Task 2 — Executor: grok.go + env hardening

**Files:** Create `rival/internal/executor/grok.go`, `grok_test.go`. Modify `rival/internal/executor/subprocess.go`, `subprocess_test.go`.

- [ ] Failing tests first, table-driven:
  - `grokEffort`: full table from Locked interfaces (incl. clamps and both error rows).
  - `grokRunArgs(model, promptFile, effort, workdir, review)`: always `--prompt-file <promptFile>`, `-m grok-4.5`, `--effort <mapped>`, `--output-format plain`, `--no-auto-update`, `--yolo`; `--cwd <workdir>` iff workdir non-empty; `--sandbox read-only` iff review; never a bare `-p`; propagates `grokEffort` error.
  - `grokFullPrompt(prompt, workdir)` == `config.SystemPrompt + "\n\n" + config.BuildWorkdirPreamble(workdir) + "\n" + prompt`.
  - `safeEnv` blocks `GROK_ANYTHING=x` and `XAI_API_KEY=x` (malicious-`.env` simulation: set via `t.Setenv`, assert absent from `safeEnv()` output; assert `PATH` survives).
- [ ] Implement: `GrokPreflight` (LookPath; `~/.grok/auth.json` exists — `os.UserHomeDir`, NOT `$GROK_HOME` since that prefix is now blocked/untrusted; error messages name `grok login`), `RunGrok` (temp file `os.CreateTemp("", "rival-grok-*.md")` + defer remove, `RunSubprocess` with `""` stdin prompt), `blockedEnvPrefixes` += `"GROK_"`, `"XAI_"` with why-comment.
- [ ] `go test ./internal/executor/` PASS.

### Task 3 — Parser + session rank

**Files:** Modify `rival/internal/parser/parser.go` (+test), `rival/internal/session/session.go` (+test).

- [ ] Failing tests: `ParseGrokArgs` follows `ParseGPT56SolArgs` pattern (same grammar: review/scope/`-re` effort override); `groupModelRank(config.GrokLabel)` gets its own slot after the existing named models, before default.
- [ ] Implement. `go test ./internal/parser/ ./internal/session/` PASS.

### Task 4 — `rival command grok`

**Files:** Create `rival/cmd/command_grok.go`.

- [ ] Mirror `cmd/command_codex.go` structurally: usage const, registration under `commandCmd`, flags `--workdir`/`--no-queue`, stdin raw-args read, `parser.ParseGrokArgs`, `executor.GrokPreflight()`, git-scope auto-detect for reviews (same as codex path), `config.ResolveEffort(config.GrokModel, parsed.Effort, config.DefaultReviewEffort)`, `session.NewQueued("grok", mode, …)`, `waitForQueueSlot`, `WithRunTimeout(ctx, 1)`, then **`executor.RunGrok(…, review = (mode == "review"), nil)`** — the review boolean comes from the derived mode (CRIT fix; a review session must never run unsandboxed).
- [ ] Test: a cmd-level test (pattern `gpt56_sol_command_test.go`) asserting the command path passes `review=true` for review-mode args and `review=false` for raw — via whatever seam that test file already uses; if none exists for codex, add a focused unit on the mode→review derivation helper.
- [ ] `go build ./...` green.

### Task 5 — `rival run grok`

**Files:** Create `rival/cmd/run_grok.go`.

- [ ] Mirror `cmd/run_codex.go` exactly: flags `--effort` (help text `low, medium, high`), `--workdir`, `--prompt-stdin` (bool), `--review` (**string scope flag**, mode="review" iff `Flags().Changed("review")`), `--no-queue`; mirror=`os.Stdout`; `RunGrok(…, review = (mode=="review"), os.Stdout)`.
- [ ] `go build ./...` green.

### Task 6 — Dashboard + web labels

**Files:** Modify `rival/internal/dashboard/session_list.go` (+test), `rival/internal/server/templates/index.html`, `rival/internal/server/server_test.go`.

- [ ] Failing tests: `cliLabel` for CLI `"grok"` → icon+label. Icon `𝕏` primary, `✖` fallback if width misaligns (test locks whichever ships).
- [ ] Implement icon + `cliLabel` case + `CLI_ICONS` entry (same glyph).
- [ ] `go test ./internal/dashboard/ ./internal/server/` PASS.

### Task 7 — Megareview opt-in selector

**Files:** Modify `rival/internal/config/config.go` (+test), `rival/internal/review/role.go` (+test), `rival/internal/review/runner.go` (+ dispatch tests), `rival/cmd/review.go`, `rival/cmd/command_megareview.go`, `rival/cmd/review_model_test.go`, `rival/internal/skills/rival-review/SKILL.md`.

- [ ] Failing tests: `ResolveReviewTargets(["grok"])` → `[{CLI:"grok", Model:GrokModel, Role:"bug_hunter"}]`; `["sol","grok"]` and `["grok","sol"]` both preserve **selector order** (and therefore judge preference — existing `preferredJudgeForTargets` semantics, deliberately unchanged; `grok,sol` → grok is preferred judge, test asserts this explicitly); duplicate dedup; unknown-selector error text lists `grok`; `RoleForCLI("grok")==RoleBugHunter`; `cmd/review_model_test.go` rows use `--model grok` (singular — the only flag that exists).
- [ ] Dispatch coverage (F10): extend/add runner tests exercising the grok arms of the preflight, reviewer-exec, judge-exec, and `modelForCLI` switches through the same injection seams the existing runner tests use; if the switches are only covered via integration today, add a minimal unit asserting each switch resolves `"grok"` without falling into the default/error arm.
- [ ] Implement: selector case; role case; runner switches (`GrokPreflight`, `RunGrok(…, review=true, …)` for reviewer AND judge arms); help/error text (`review.go` "sol, kimi-k3" strings, `command_megareview.go` usage); rival-review SKILL.md selector list gains `grok`.
- [ ] `go test ./internal/config/ ./internal/review/ ./cmd/` PASS.

### Task 8 — `/rival-grok` skill + embedding

**Files:** Create `rival/internal/skills/rival-grok/SKILL.md`; modify `rival/internal/skills/embed.go`, `embed_test.go`, `scripts/bump-skill-versions.sh`.

- [ ] Failing test: embed_test asserts `rival-grok` in `Names`/embedded FS with `version:` frontmatter.
- [ ] SKILL.md = copy of `rival-k3/SKILL.md` adapted: name/description for grok, Step 1 `rival command grok --detach --workdir "$(pwd)"` (Write-tool input file pattern, no heredoc), Step 2 watcher **exactly** `rival wait --log <rival_err>` — NO `--timeout` flag (F9: the bound comes from `RIVAL_QUEUE_TIMEOUT`+`RIVAL_RUN_TIMEOUT`), Step 3 end turn + present verbatim on notification. Same `version:` stamp as the other skills' current value.
- [ ] embed.go directive + `Names` entry; `SKILL_DIRS` entry in bump script.
- [ ] `go test ./internal/skills/` PASS.

### Task 9 — Live e2e + docs

**Files:** Modify `README.md`, `CHANGELOG.md`, `docs/runtime-reference.md`.

- [ ] `cd rival && make install`, then live (grok authenticated on this machine):
  - Raw: `printf 'say the word pineapple and stop' | rival run grok --prompt-stdin --no-queue` completes; session in `rival sessions`; TUI/web label aligned.
  - Command review path (CRIT regression check): drive `rival command grok` with review args in a scratch git repo and verify the session records mode `review` AND the child argv (visible in the session log) contains `--sandbox read-only`.
  - Sandbox proof (F6): in a scratch git repo (workdir), review-mode prompt instructs grok to create `<workdir>/rival-grok-sandbox-proof.txt`; assert file absent before AND after; transcript shows the blocked write. Clean up scratch repo.
  - Effort probe: one run `--effort low`, one with `-re ultra` (skill grammar) → argv shows `--effort high` (clamp verified).
  - Grok-only megareview live (F10): `rival review --model grok --no-queue` (or the megareview command equivalent) on a small scope — preflight, reviewer parse, judge (grok judges itself, sole reviewer), final formatting all succeed.
  - Detach: `rival command grok --detach` + `rival wait --log` exits 0.
- [ ] Docs: CHANGELOG Unreleased entry (grok provider, opt-in `--model grok`, env hardening `GROK_`/`XAI_`, sandbox fail-open caveat + macOS child-network caveat); README roster + auth (grok.com OAuth via `grok login`; `XAI_API_KEY` deliberately NOT supported — blocked env); runtime-reference invocation shape.
- [ ] Full gate: `cd rival && make test` green.

---

**Type-consistency check:** `GrokModel`/`GrokLabel` (Tasks 1,3,4,7) · `RunGrok(ctx, sess, prompt, effort, workdir string, review bool, mirror io.Writer)` (2,4,5,7) · `grokEffort` clamp table (2,9) · `ParseGrokArgs` (3,4) · CLI string `"grok"` (4,6,7) · `--model` singular (7,9) · watcher without `--timeout` (8) — all match Locked interfaces.

**Review gates:** `/rival-fable` after each stage (Stage A = Tasks 0–3, Stage B = 4–7, Stage C = 8–9); fix HIGH/CRITICAL before next stage; one final whole-branch review.

## Review deltas (v1.0 → v1.1, from Sol 3/10)

1. crit — command path now derives `review` from session mode; Task 4 test + Task 9 live check.
2. high — sandbox fail-open: claim narrowed in docs; live workdir write-block proof (no `~/.grok/sandbox.toml` management).
3. high — effort table corrected to grok-4.5's real menu (low/medium/high, default high) with explicit clamps.
4. high — default-effort bypass dissolved: grok's builtin default == `DefaultReviewEffort`; call sites stay codex-identical (documented in Locked interfaces).
5. high — "grok last" judge rule DROPPED; selector order wins (existing semantics), `grok,sol` case tested explicitly.
6. high — sandbox proof moved from `/tmp` (allowed by profile) to workdir; `--review` documented as string scope flag.
7. high — `GROK_`/`XAI_` added to `blockedEnvPrefixes` + malicious-`.env` tests; preflight auth = auth.json only; `GROK_DISABLE_AUTOUPDATER` dropped (flag suffices).
8. med — `--models` → `--model` everywhere.
9. med — skill watcher timeout removed; exact k3 watcher copied.
10. med — runner dispatch tests + live grok-only megareview added.
