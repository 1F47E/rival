# Grok CLI as a rival provider

**Date:** 2026-07-23
**Scope:** /Users/kass/dev/claude-codex-plugin/rival
**Status:** approved

## Problem(s)

1. Rival dispatches to codex, antigravity, gemini, claude/fable, and opencode (incl. Kimi K3), but not to the xAI Grok CLI, even though `grok` 0.2.111 is installed (`~/.local/bin/grok`) and authenticated (grok.com OAuth in `~/.grok/auth.json`). There is no `case "grok"` anywhere in `rival/internal/config/config.go` (engine/model routing, `EngineLabel` at config.go:88, `ResolveReviewTargets` at config.go:301) or `rival/internal/executor/`. The user wants Grok as another independent reviewer/runner.
2. Every existing executor delivers the prompt to the child via stdin (`RunSubprocess`, rival/internal/executor/subprocess.go:157-164; codex appends a trailing `-` arg, codex.go:68). Grok's headless mode explicitly does **not** read the prompt from stdin (bundled doc `~/.grok/docs/user-guide/14-headless-mode.md`, "Piping Input and Output") — it needs `-p <prompt>` (argv, risks ARG_MAX on big review diffs) or `--prompt-file <path>`. The integration must bridge this without disturbing the shared subprocess plumbing.

## Goals

1. `rival command grok` + `rival run grok` work end-to-end with the standard plumbing: queue, `--detach`, `RIVAL_RUN_TIMEOUT`, session tracking, log teeing. (Problem 1)
2. `/rival-grok` skill (async detach + `rival wait` watcher pattern), installed via `rival install`. (Problem 1)
3. Opt-in megareview selector `grok` (like k3): `--models grok` resolves; NOT in the default roster. (Problem 1)
4. Review mode is mechanically read-only via grok's native `--sandbox read-only`; raw run mode is full-auto via `--yolo`. (Problem 1)
5. Prompt delivered via a temp `--prompt-file`, created and cleaned up inside the grok executor; `RunSubprocess` untouched. (Problem 2)
6. Effort passes through 1:1 (grok natively accepts `low/medium/high/xhigh/max`); rival's `ultra` maps to `max`; default = rival's global default `xhigh`.

## Non-goals

- No change to the default megareview roster (stays Sol + K3 after the 2026-07-23 model-surface reduction, commit 1c2c2ec; consilium judge priority Sol → K3).
- No grok participation in plan review (`command_plan.go`, `rival-plan*` skills stay `sol,fable`).
- No `XAI_API_KEY` env stripping or `RIVAL_GROK_AUTH` mode: grok's stored OAuth session token takes precedence over the env key (doc 02-authentication.md:53, :254-260) — the claude-CLI billing footgun does not exist here, and `XAI_API_KEY` is not set on this machine anyway.
- No grok-specific quota signatures in `quota.go` initially — grok's 429 envelope format is unverified; add when observed in the wild.
- No `RunSubprocess` extension for prompt files (option (b)); temp file lives in the executor.

## Grok execution contract

Verified against the installed binary (`grok --help`, `grok models`, bundled docs 14/18/02):

- Headless: `grok -p`/`--prompt-file` runs one prompt, prints to stdout, exits (0 ok / 1 error / 130 SIGINT / 143 SIGTERM).
- Only model on this account: `grok-4.5` (default). Constant `GrokModel = "grok-4.5"`, `GrokLabel = "grok"`.
- Invocation built by `grokRunArgs(effort, workdir, review bool)`:

```
grok --prompt-file <tmpfile> \
     -m grok-4.5 \
     --effort <low|medium|high> \
     --output-format plain \
     --no-auto-update \
     --cwd <workdir>            # only when workdir != ""
     # review mode:  --sandbox read-only --yolo
     # raw run mode: --yolo
```

Why `--yolo` in both modes: headless permission prompts would stall a detached run; in review mode the `read-only` Seatbelt/Landlock profile kernel-blocks writes anyway (writes allowed only to `~/.grok/` + temp dirs — doc 18-sandbox.md). Known limitation, documented not mitigated: child-process network blocking in `read-only` is Linux-only, a no-op on macOS.

- Prompt file: `os.CreateTemp("", "rival-grok-*.md")`, write `config.SystemPrompt + "\n\n" + config.BuildWorkdirPreamble(workdir) + "\n" + prompt` (same preamble shape as codex.go:46), `defer os.Remove`. Pass `""` as the `RunSubprocess` prompt (stdin unused; grok ignores stdin).
- Env: child gets the standard filtered env with `GROK_` and `XAI_` prefixes newly blocked (see Env hardening below).
- Preflight `GrokPreflight()`: `exec.LookPath("grok")`, then `~/.grok/auth.json` exists. No network call. Auth failure at runtime surfaces via grok's stderr in the teed log.
- Effort mapping in the executor arg builder: grok-4.5's advertised menu (models_cache.json `reasoning_efforts`) is exactly `low/medium/high`, default `high`. Mapping: `low/medium/high` verbatim; `xhigh`/`ultra`/`max` clamp to `high`; `minimal`/`none` clamp to `low`; anything else → error. Default effort = `high` (grok's `builtinModelEffort` entry equals `DefaultReviewEffort`, so the standard call-site fallback yields the right value with no special-casing).
- Env hardening: add `GROK_` and `XAI_` to `blockedEnvPrefixes` (subprocess.go) — godotenv loads a reviewed repo's `.env` globally (main.go:18), and unblocked `GROK_*` vars could redirect the authenticated grok runtime (proxy base URL, `GROK_HOME`, auth helpers). Consequence: `XAI_API_KEY` fallback auth is dropped entirely; preflight auth = `~/.grok/auth.json` exists, full stop. No `GROK_DISABLE_AUTOUPDATER` env var — the `--no-auto-update` flag covers it.
- Sandbox honesty: grok's built-in profiles fail OPEN (warning + continue) when the kernel sandbox can't be applied (doc 18-sandbox.md, "Enforcement failure"). Accepted with the claim narrowed in docs; enforcement is proven live on the primary platform (macOS Seatbelt) by an e2e that has review-mode grok attempt a write INSIDE the workdir (not /tmp — the read-only profile legitimately allows /tmp writes) and asserts the file does not appear.

## Megareview wiring

- `ResolveReviewTargets`: selector `grok` → `{CLI: "grok", Model: GrokModel, Role: bug_hunter}`; update the valid-selectors error strings (config.go:338, :645, :651) and `--model` help (cmd/review.go:39 `StringSliceP("model","m",…)`, error at review.go:106, usage in cmd/command_megareview.go:21-30).
- Judge selection keeps existing semantics: first requested reviewer that passed preflight / produced a successful review judges (`preferredJudgeForTargets`/`pickJudge` order = per-invocation target order). No grok special-casing — `--model grok,sol` makes grok the preferred judge by selector order, same as k3 today.
- `RoleForCLI` (review/role.go:13): `case "grok": return RoleBugHunter`.
- runner.go: preflight switch (:84), `runReviewer` exec switch (:435), `runConsilium` judge switch (:517, so a grok-containing roster can never hit an unhandled judge case), `modelForCLI` (:600).
- Review invocation always uses review mode args (`--sandbox read-only --yolo`).

## File-level changes

| File | Change |
|---|---|
| `rival/internal/config/config.go` | Add `GrokModel`/`GrokLabel` consts; cases in `ModelLabel`, `EngineLabel`, `replaceConcreteModelIDs`, `publicReviewHeader`, `ResolveReviewTargets` (+ error strings), `builtinModelEffort`, `knownEffortModel`, `validConfiguredModelEffort`, `ResolveEffort`, `DefaultEffortForModel` (default xhigh). |
| `rival/internal/executor/grok.go` (new) | `GrokPreflight`, `RunGrok(ctx, sess, prompt, effort, workdir, review, mirror)`, `grokRunArgs`, `grokEffort`; temp prompt-file lifecycle. Modeled on codex.go. |
| `rival/internal/executor/subprocess.go` | Add `"GROK_"` and `"XAI_"` to `blockedEnvPrefixes` with a why-comment (repo `.env` must not reconfigure the authenticated grok runtime). |
| `rival/cmd/command_grok.go` (new) | `rival command grok` — mirror command_codex.go: flags `--workdir`/`--no-queue`, stdin prompt read, `parser.ParseGrokArgs`, preflight, `session.NewQueued("grok", …)`, queue wait, `WithRunTimeout`, `PublicRuntimeLog`. |
| `rival/cmd/run_grok.go` (new) | `rival run grok` — mirror run_codex.go: `--effort --workdir --prompt-stdin --review --no-queue`, stdout mirror; `--review` → review mode args, raw → `--yolo`. |
| `rival/internal/parser/parser.go` | `ParseGrokArgs` on the standard effort ladder (pattern: `ParseGPT56SolArgs`, parser.go:24). |
| `rival/internal/session/session.go` | `groupModelRank` case for `GrokLabel` (session.go:261). |
| `rival/internal/dashboard/session_list.go` | `iconGrok` glyph + `case "grok"` in `cliLabel` (session_list.go:79-107). Icon: `𝕏 grok`; if the double-width glyph misaligns the TUI table in manual check, fall back to `✖ grok`. |
| `rival/internal/server/templates/index.html` | Add `grok` entry to `CLI_ICONS` (index.html:970). |
| `rival/internal/review/role.go` | `RoleForCLI` grok case. |
| `rival/internal/review/runner.go` | Grok cases in preflight, reviewer exec, judge exec, `modelForCLI`. |
| `rival/internal/skills/embed.go` | `//go:embed all:rival-grok` + `"rival-grok"` in `Names`. |
| `rival/internal/skills/rival-grok/SKILL.md` (new) | Copy rival-k3 structure (detach → background `rival wait --log` → present verbatim); `rival command grok --detach --workdir "$(pwd)"`. |
| `rival/internal/skills/rival-review/SKILL.md` | Add `grok` to selector list in argument-hint + usage. |
| `scripts/bump-skill-versions.sh` | Append rival-grok dir to `SKILL_DIRS`. |
| `rival/cmd/command_megareview.go`, `rival/cmd/review.go` | Selector help/error text. |
| `README.md`, `CHANGELOG.md`, `docs/runtime-reference.md` | Provider roster docs. |

## Tests

- `rival/internal/executor/grok_test.go` (new, pattern codex_test.go): `grokRunArgs` table — effort mapping incl. `ultra`→`max` and invalid-effort error; review vs raw flag sets; workdir/`--cwd` presence; prompt-file arg present and temp file contents include system prompt + preamble.
- `rival/internal/config/config_test.go`: `ModelLabel`/`EngineLabel`/`ResolveReviewTargets("grok")`/effort-resolution rows.
- `rival/internal/review/role_test.go`: grok → bug_hunter.
- `rival/internal/dashboard/session_list_test.go`, `rival/internal/server/server_test.go`: label/icon rows.
- `rival/internal/session/session_test.go`: `groupModelRank` row.
- `rival/cmd/review_model_test.go`: `--models grok` selection + invalid-selector error text.
- `rival/internal/skills/embed_test.go`: rival-grok embedded + version stamp present.
- Manual: `rival run grok --prompt-stdin` happy path; `rival command grok --detach` + `rival wait`; `/rival-grok` skill e2e; review-mode write attempt blocked by sandbox (ask grok to touch a file, verify refusal/failure); TUI + web dashboard render of a grok session.

## Failure modes & decisions

| Failure | Behaviour |
|---|---|
| `grok` binary missing | Preflight error before session creation: "grok CLI not found in PATH". |
| Not logged in | Preflight error when `~/.grok/auth.json` missing (message points at `grok login`); if auth breaks at runtime, grok exits 1, session fails with teed stderr. |
| Prompt temp file write fails | `RunGrok` returns error before spawning; session fails. Temp file always removed via defer. |
| Grok 429/quota | Exits 1, session fails with raw error (no `quotaSignatures` entry yet — out of scope until envelope observed). |
| Huge prompt | No ARG_MAX risk — prompt travels via file, not argv. |
| Run exceeds `RIVAL_RUN_TIMEOUT` | Existing plumbing kills child, fails session, frees queue slot (unchanged). |
| macOS review sandbox | FS writes kernel-blocked (Seatbelt); child network NOT blocked (macOS no-op) — accepted, documented in README section. |
| Sandbox can't be applied (built-in profiles fail open) | Accepted + documented (no fail-closed mechanism without managing the user's `~/.grok/sandbox.toml`); live e2e proves enforcement on macOS by asserting a review-mode workdir write is blocked. |
| `grok models` roster changes | Model pinned by const; `-m grok-4.5` errors loudly if retired → bump const. |

## Out of scope

- Grok in the default megareview roster or as a preferred consilium judge (`pickJudge` order unchanged; judge case exists only as a safety net).
- Plan-review participation.
- Quota-signature detection for grok.
- Streaming/JSON output parsing (`--output-format plain` only; sessionId/cost fields unused).
- Any Docker/containerized grok setup.

## Rollout

- P1: executor + config + parser + `command grok`/`run grok` + session/dashboard/web labels + unit tests (`go build ./... && make test`).
- P2: megareview opt-in selector + review runner wiring + selector help/error text + tests.
- P3: `/rival-grok` skill + rival-review skill text + bump-skill-versions + README/CHANGELOG/runtime-reference docs; live e2e (raw run, detached review, sandbox write-block proof).

Each phase = one commit, gated on a clean `/rival-fable` review.
