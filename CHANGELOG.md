# Changelog

All notable changes to **rival** are documented here. Versions follow [semver](https://semver.org/); every release is git-tagged.

## [v3.14.2] — 2026-06-13

### Fixed — `rival wait` / `detach` review findings
A megareview of the v3.14.0 wait/detach code surfaced four real issues, all fixed:

- **`rival wait --log` no longer tracks stale sessions.** `parseLogFile` matched
  every `"session"` field in the stderr, including ones that startup maintenance
  (`ReapOrphans`, queue reaper) logs there — so wait could watch old sessions and
  mis-report a healthy run as failed/crashed. It now only collects IDs from this
  run's `"message":"starting …"` lines.
- **No false "crashed" on a finalize race.** When the rival process is detected
  dead, wait now re-reads the session JSON once before declaring a crash — the
  process can finalize the session and exit between the status read and the
  liveness check.
- **`rival wait` default timeout derived from config.** Was a fixed 75m, below a
  legitimate worst-case megareview (queue wait + 2× run). New `config.MaxRunWait()`
  = `RIVAL_QUEUE_TIMEOUT + 2×RIVAL_RUN_TIMEOUT + 5m` (95m by default), used as the
  default and inherited by the skills (they no longer hardcode `--timeout`).
- **Windows build fixed.** `detach.go`'s `syscall.SysProcAttr{Setsid}` is Unix-only;
  split into build-tagged `detach_unix.go` / `detach_other.go` so the package
  compiles for `GOOS=windows`. (Releases remain darwin+linux.)

## [v3.14.1] — 2026-06-13

### Changed — `/rival-fable` skill disabled
The `rival-fable-only` skill is removed on install (added to `Deprecated`, no
longer embedded) — Claude Fable 5 is currently unavailable upstream. The
`rival command fable` executor and `FableModel` stay in the binary, so the skill
can be re-enabled later by re-adding it to `Names` + the embed list. The default
Claude model is unchanged: `claude-opus-4-8[1m]` (`ClaudeModel`); nothing
defaulted to fable.

## [v3.14.0] — 2026-06-13

### Fixed — reviews no longer die when the launching context ends
The v3.13.0 background-shell skill pattern had a fatal flaw: when a Claude Code
skill's forked context completed, Claude Code tore down the fork's background
shells with a **process-group kill**, SIGTERM-ing the rival process underneath —
mid-review or while waiting in the queue (`cancelled while queued`). The review
died even though the queue itself behaved correctly.

### Added — `rival command … --detach`
- New `--detach` flag on all `rival command` subcommands: rival **re-execs
  itself into its own process session** (`setsid`), prints
  `rival: detached pid=N` to stderr, and the parent exits immediately. The
  launching shell returns at once; no process-group kill can reach the review.
- Stdin/stdout/stderr redirections are inherited by the detached child, so the
  `< prompt > out 2> err` contract is unchanged.
- `RIVAL_DETACHED=1` guards against re-exec loops.

### Added — `RIVAL_RUN_TIMEOUT` (no run can hang forever)
Previously nothing bounded the provider run itself: a hung CLI (network stall,
deadlock) meant the detached rival never exited and any waiter blocked forever.
Now every run is bounded by `RIVAL_RUN_TIMEOUT` (default **30m**), and the clock
starts **after** the queue slot is acquired so queue wait never eats the budget.
On deadline the provider child is killed, the session fails with
`run timeout after <d> (RIVAL_RUN_TIMEOUT) — provider CLI did not finish`, and
the queue slot is freed. `RIVAL_RUN_TIMEOUT=0` disables it. The megareview
pipeline gets 2× the budget (reviewers phase + judge phase).

### Added — `rival wait` subcommand
One clean primitive that blocks until a run reaches a terminal state, used by
the skills' background watcher and available to any script/CI:

- `rival wait --log <stderr-file>` — parse the detached rival PID + session IDs
  from a run's stderr, poll the **rival process** for liveness (PID-reuse-proof
  via `procinfo` start-time pinning), then summarize the sessions when it exits.
  Detects a crashed rival (process dead, sessions not finalized).
- `rival wait <session-id>...` — terminal-status-only mode.
- Exit codes: `0` all completed · `2` some failed · `3` rival crashed · `4`
  timed out. Default `--timeout` 75m.

### Changed — skills are now async (launch + background watch, never block)
All active skills (`rival-review`, `rival-codex-only`, `rival-antigravity-only`,
`rival-fable-only`, `rival-plan`) rewritten to **not block the session**. They
drop `context: fork`, launch rival `--detach` (foreground, returns in seconds),
arm a **background `rival wait --log …` watcher** (`run_in_background: true`),
then hand control back immediately and end the turn. When the watcher exits, the
harness wakes the session and the skill presents the output verbatim — possibly
several turns later. This removes the previous design's two hang paths (no
provider-run timeout, bare-PID polling with "never stop polling") and frees the
session for the whole review. Cancel = `kill <pid>`; status on demand =
`tail <err>`. The detached run + result files survive the context ending, so a
lost watcher can be resumed with `rival wait --log <err>`.

### Fixed — claude/fable bills the subscription, not API credits
The claude CLI silently prefers an inherited `ANTHROPIC_API_KEY` over its own
`/login` (Pro/Max) auth, so rival runs were billed to API credits whenever the
shell exported the key. Auth is now **explicit**:

- Default (`RIVAL_CLAUDE_AUTH` unset / `subscription`): `ANTHROPIC_API_KEY` and
  `ANTHROPIC_AUTH_TOKEN` are stripped from the subprocess env — the CLI uses
  its subscription login.
- `RIVAL_CLAUDE_AUTH=api`: opt-in API billing; requires `ANTHROPIC_API_KEY`,
  hard error when empty. Any other value is a hard error.
- Auth mode is logged per run and shown in the TUI detail view (Account field).
- Auth/billing failures (`Credit balance is too low`, `Please run /login`,
  invalid key, expired OAuth) now append an actionable, mode-specific hint to
  the output instead of a bare CLI error.

### TUI
- Detail view shows the full error message word-wrapped (was a single truncated
  line); applied to single and group views.
- Fable sessions store `cli: "claude"` (fable is a model inside the Claude Code
  CLI, not a CLI) — TUI/web/`rival sessions` show `⬡ claude` + model
  `claude-fable-5`. Old sessions with `cli: "fable"` still render as claude via
  a read-compat fallback.

## [v3.13.0] — 2026-06-11

### Added — cross-process review queue
Multiple Claude Code clients (or terminals) invoke `rival` as independent processes. They now serialize through a **cross-process FIFO queue** instead of all firing their provider CLIs at once and hitting rate limits.

- **One review at a time by default.** Coordination is via ticket files in `~/.rival/queue/` guarded by an `flock` — no daemon. Each waiting process scans, reaps dead tickets, and promotes itself in a single locked critical section.
- **Position reported to the waiting client.** A queued review prints `rival queue: position 2/3 (1 running), waiting 1m12s` to stderr; skills relay it to the user.
- **Visible in TUI and web dashboard.** Queued sessions show a `◌ queued #N` row with a live wait timer, plus a queued counter. The TUI `x` key cancels a queued session too.
- **New `rival queue` subcommand** — `rival queue` (list position/state/mode/wait/workdir), `rival queue clear` (remove dead tickets), `rival queue clear --force` (remove all; live waiters re-queue at the tail).
- **Config:** `RIVAL_MAX_CONCURRENT` (default 1), `RIVAL_QUEUE_TIMEOUT` (default 30m, wait-only), `RIVAL_NO_QUEUE` / `--no-queue` to bypass.
- A megareview holds **one** slot for its whole pipeline (both reviewers + the consilium judge).

### Changed — skills run rival in the background
A queued-then-run sequence can exceed the 600s foreground Bash cap. All skills now launch rival with `run_in_background: true`, redirect stdout/stderr to separate temp files, poll the stderr file for queue-position lines, and present stdout verbatim on exit. Skill `allowed-tools` updated to `Bash, Read, KillShell`. All active skills bumped to v3.13.0; `rival-fable-only` added.

### Reliability
- **PID-reuse-safe liveness.** Tickets and sessions record the owning process's start time (new `internal/procinfo` package: macOS `sysctl KERN_PROC`, Linux boot-relative `/proc/<pid>/stat`). A recycled PID — a different process with a different start time — is correctly treated as dead, so a crashed holder's slot is reclaimed and never wedges. Degrades to a bare existence check on unsupported platforms.
- **SIGKILL-survivor handling.** If rival is killed mid-review but its provider-CLI child survives, the slot stays held (the child still consumes quota) until that child exits — covers reviewers and the consilium judge.
- No heartbeats by design: laptop sleep/wake can never false-reap a live holder.

## [v3.12.0] — 2026-06-10
- Claude bumped to `opus-4-8`; CI is now the sole release publisher.

## [v3.11.0] — 2026-06-09
- Web dashboard server, `/rival-plan`, provider quota/429 detection; megareview is now Codex + Antigravity (Gemini dropped from the default review).

## [v3.10.0] — 2026-03-21
- Rival skills became directly Skill-tool invocable (the old Agent-tool/binary workaround is no longer needed).

---

Older releases (v3.9.0 and earlier) predate this changelog; see `git log` and tags for history.
