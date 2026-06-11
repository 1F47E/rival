# Changelog

All notable changes to **rival** are documented here. Versions follow [semver](https://semver.org/); every release is git-tagged.

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
