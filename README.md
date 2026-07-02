# rival

<img src="assets/banner2.png" width="600px">

Dispatch prompts to external AI CLIs from Claude Code. Run GPT-5.5 via Codex, Gemini via Antigravity, or Claude Opus 4.6 (1M) via Claude Code CLI — as isolated subagents that keep your main context clean. The default `/rival-review` runs Codex + Antigravity in parallel and merges their findings with a consilium judge.

## Install

### Homebrew (recommended)

```bash
brew install 1F47E/tap/rival
rival install
```

### From source

```bash
cd rival && make install
rival install
```

> **Note:** `go install` is not supported due to the repo's subdirectory layout. Use Homebrew or build from source.

`rival install` copies the Claude Code skills (embedded in the binary) into `~/.claude/skills/`. After that, `/rival-review`, `/rival-codex-only`, `/rival-antigravity-only`, `/rival-plan`, `/rival-plan-codex`, and `/rival-plan-fable` are available in Claude Code. (Install also removes the deprecated `/rival-gemini-only` and `/rival-claude-only` skills.)

Use `rival install --force` to overwrite without prompting.

### Prerequisites

- [Codex CLI](https://github.com/openai/codex): `npm install -g @openai/codex` + `codex login` — used by megareview, `/rival-codex-only`, and `/rival-plan`
- Antigravity CLI (`agy`): install + authenticate to a quota-bearing account — used by megareview and `/rival-antigravity-only`
- [Gemini CLI](https://github.com/google-gemini/gemini-cli): `npm install -g @google/gemini-cli` + set `GEMINI_API_KEY` — optional, only for the standalone `rival command gemini`
- [Claude Code CLI](https://docs.anthropic.com/en/docs/claude-code/overview): install + authenticate (or use Docker — see below) — optional standalone

You only need the CLIs for the commands you use. **Megareview uses Codex + Antigravity.**

## Usage

### Claude Code Skills

**Default review** (runs Codex + Antigravity + consilium judge):

```
/rival-review                              — review with Codex + Antigravity (auto-detects changed files)
/rival-review src/api/                     — review specific scope (bypasses git detection)
/rival-review -re xhigh src/api/           — both CLIs, max reasoning effort
```

**Single-CLI skills** (use only when you want one specific CLI):

```
/rival-codex-only explain the auth flow in this project
/rival-codex-only -re xhigh find bugs in src/main.go
/rival-codex-only review                   — review (auto-detects changed files via git)
/rival-codex-only review src/api/          — review specific scope
```

```
/rival-antigravity-only explain the auth flow
/rival-antigravity-only -re high analyze this complex algorithm
/rival-antigravity-only review             — review (auto-detects changed files via git)
/rival-antigravity-only review src/api/    — review specific scope
```

**Plan/spec review** (single path to a markdown plan, rated 1-10):

```
/rival-plan path/to/plan.md                — codex + claude-fable in parallel, both results shown
/rival-plan-codex path/to/plan.md          — codex only
/rival-plan-fable path/to/plan.md          — claude-fable only
```

`/rival-plan` runs both engines concurrently and prints each one's 1-10 rating + findings; an engine that is unavailable is skipped, not fatal. The `-codex` / `-fable` variants run a single engine.

**Reasoning effort** (`-re`): `low`, `medium`, `high`, `xhigh` (default). Plan review is fixed at `xhigh`.

### How Reviews Work

When you run a review, Codex and Antigravity get **full access to your project**. They don't just see a diff — they run as CLI tools inside your workdir with tool use enabled, so they can:

- Read any file in the project
- Follow imports and trace dependencies
- Explore the full codebase to understand context
- Run commands to inspect project structure

**Smart scope detection.** Running `/rival-review` with no arguments auto-detects what to review via git:
1. **Dirty files** (staged + unstaged + untracked new files) → reviews those files
2. **Last commit** (if working tree is clean) → reviews files from HEAD
3. **Full project** → only if not a git repo or no changes found

The **scope** is a focus hint, not a restriction. `review src/api/` tells the reviewer to focus on `src/api/`, but it can (and will) read other files to understand the code in context. Explicit scope bypasses git detection entirely.

This means you can use natural language for the scope:

```
/rival-codex-only review the files changed in the last commit
/rival-codex-only review the authentication middleware
/rival-review -re xhigh the new payment flow in src/billing/
```

The reviewer will figure out what to look at, explore the relevant code, and give you a review with full project understanding.

### Roles & Consilium (megareview)

Megareview assigns **specialized roles** to each reviewer:

- **Codex → Bug Hunter** — finds concrete code-level defects: logic bugs, broken state transitions, race conditions, missing edge cases. Optimizes for true positives with high confidence.
- **Antigravity → Architecture & Security** — attacks from angles a bug hunter misses: architectural regressions, broken cross-file flows, incomplete refactors, concurrency issues, security problems, silent failure gaps.

All reviewers emit **structured JSON** with file, line, severity, category, confidence (1-10), and fix suggestions.

If a reviewer hits a provider quota/rate limit (a 429 — `agy` exits 0 with empty output on this), rival detects it from the captured log and reports that reviewer as **skipped** with a reason, rather than silently counting it as a clean empty review.

Role prompts can be customized via `~/.rival/config.yaml`:

```yaml
roles:
  bug_hunter: |
    Your custom bug hunter instructions...
  code_quality: |
    Your custom code quality instructions...
```

A separate **consilium judge** (runs via Codex) then:
- Merges duplicate findings (same file + line + problem → single finding with all reporters in `found_by`)
- Applies consensus bonus (+2 confidence for findings reported by 2+ reviewers)
- Filters by confidence threshold (default: ≥6)
- Sorts by severity (critical first), then confidence
- Produces a unified verdict: `approve`, `request_changes`, or `comment`

```
═══ RIVAL REVIEW ═══

Summary: ...

[CRITICAL] file.go:42 — Title
  Description...
  Fix: ...
  Found by: codex, antigravity

[HIGH] file.go:100 — Title
  ...

Recommendation: request_changes — ...

Reviewed by: codex (bug_hunter), antigravity (arch_security)
Judge: codex (consilium)
Findings: 5 (threshold: 6)
```

The consilium judge runs via Codex, falling back to Antigravity if Codex is unavailable. If only one reviewer is available, the consilium judge falls back to whichever CLI is present. If a reviewer fails to produce structured JSON, the consilium receives a stub with a 2KB debug tail instead of the full raw output (prevents prompt overflow).

### Direct CLI

```bash
# Run with prompt from stdin
echo 'explain the auth flow' | rival command codex --workdir .
echo 'explain the auth flow' | rival command antigravity --workdir .

# Review via megareview (Codex + Antigravity in parallel)
echo 'src/api/' | rival command megareview --workdir .

# Rate a plan/spec doc 1-10 (codex + claude-fable by default)
echo 'docs/plan.md' | rival command plan --workdir .
echo 'docs/plan.md' | rival command plan --cli codex --workdir .   # codex only
echo 'docs/plan.md' | rival command plan --cli fable --workdir .   # claude-fable only
```

### TUI Dashboard

Monitor running and past sessions in a full-screen terminal UI:

```bash
rival tui
```

**List view** shows all sessions with status, CLI (◈ codex / △ antigravity / ⬡ claude / ▤ plan / ◈△ mega), model, effort, elapsed time, workdir, and prompt preview. Multi-session runs are grouped into a single row: megareview shows `◈△ mega`, and a dual `/rival-plan` (codex + claude-fable) shows `▤ plan`. Claude sessions show `⬡ claude` for native or `⬡ claude/dk` for Docker mode. Single-engine plan reviews (`/rival-plan-codex`, `/rival-plan-fable`) show `▤ plan`.

**Detail view** shows full metadata (including Mode and Account/subscription type for Claude), prompt, and live-streaming log output. Group titles and metadata are derived from the sessions: a megareview group is titled `Megareview` with CLI `codex+antigravity`, a dual plan group is titled `Plan Review` with CLI `codex+claude-fable` and mode `plan`. All member logs are shown.

#### Keys

| Key | List View | Detail View |
|-----|-----------|-------------|
| `j/k` or `↑/↓` | Navigate sessions | — |
| `Enter` | Open detail view | — |
| `Esc` | — | Back to list |
| `g` / `G` | Jump to top / bottom | — |
| `p` | — | Toggle full prompt |
| `o` | — | Open log file in editor |
| `x` | — | Kill running session |
| `q` | Quit | Quit |

### Session Management

```bash
rival sessions              # all sessions as JSON
rival version               # show version
```

### Review Queue

Multiple Claude Code clients (or terminals) invoke `rival` as independent
processes. Without coordination they launch their provider CLIs at once and hit
rate limits. rival serializes them through a **cross-process FIFO queue** — by
default one review runs at a time; the rest wait their turn.

- Each waiting review prints its position to stderr while it waits:
  `rival queue: position 2/3 (1 running), waiting 1m12s`. Skills relay this to you.
- Queued sessions show up in the TUI and web dashboard with a `◌ queued #N` row
  and a growing wait time, alongside a queued counter.
- No daemon: coordination is via ticket files in `~/.rival/queue/` guarded by an
  flock. A crashed holder is reaped automatically (its slot frees when both the
  rival process and any surviving provider CLI child are gone).

```bash
rival queue                 # list tickets (position, state, mode, wait, workdir)
rival queue clear           # remove dead tickets
rival queue clear --force   # remove ALL tickets (live waiters re-queue at the tail)
```

**Config (env vars):**

| Var | Default | Effect |
|-----|---------|--------|
| `RIVAL_MAX_CONCURRENT` | `1` | How many reviews may run at once |
| `RIVAL_QUEUE_TIMEOUT` | `30m` | Max wait for a slot before the review fails |
| `RIVAL_RUN_TIMEOUT` | `30m` | Max run time once a slot is held; kills a hung provider CLI (`0` disables) |
| `RIVAL_NO_QUEUE` | unset | Set to bypass the queue entirely |

Every command also accepts `--no-queue` to skip the queue for that invocation.

**`--detach`** — `rival command …` re-execs itself into its own process session
(`setsid`), prints `rival: detached pid=N` to stderr, and the parent exits
immediately. The launching shell returns at once and the run survives the
caller's teardown (Claude Code kills a skill's shells with a process-group kill
when it ends, which used to SIGTERM a running or queued review).

**`rival wait`** — blocks until a run reaches a terminal state. Skills arm it as
a background watcher; it's also usable from scripts/CI:

```bash
rival wait --log <stderr-file>   # parse detached pid + session IDs, wait, summarize
rival wait <session-id>...       # terminal-status-only mode
# exit: 0 completed · 2 failed · 3 rival crashed · 4 timed out
```

**Never hangs:** a run is bounded by `RIVAL_RUN_TIMEOUT`; `rival wait` detects a
crashed rival (process dead, sessions unfinalized) and its own `--timeout`. The
skills launch detached, watch in the background, and never block your session.

> **NFS note:** the queue relies on `flock`, which is unreliable over NFS-mounted
> home directories. On such hosts set `RIVAL_NO_QUEUE=1`.

## Architecture

```
Claude Code main session
    │
    │ /rival-review
    ▼
Claude skill (async — does not block the session)
    │ 1. prompt tempfile → rival command megareview --detach --workdir $(pwd)
    │    (parent prints "rival: detached pid=N" and exits in seconds)
    │ 2. arm background watcher:  rival wait --log <err> --timeout 100m
    │ 3. hand back to the session and END the turn
    ▼
rival binary (own process session — survives the skill/fork teardown)
    ├─ parses arguments (-re flag, review/prompt mode)
    ├─ builds review prompt with scope injection
    ├─ waits for a queue slot, then bounds the run by RIVAL_RUN_TIMEOUT
    ├─ spawns codex/antigravity via subprocess
    ├─ pipes prompt to stdin, tees stdout to log file
    └─ writes session JSON + live log to ~/.rival/sessions/
         │
         ▼  (rival exits → background `rival wait` exits → harness wakes session)
    Claude reads the output file and presents the review verbatim

Megareview (roles + consilium):
    rival binary
    ├─ generates shared GroupID (UUID)
    ├─ assigns roles: codex=bug_hunter, antigravity=arch_security
    ├─ spawns codex + antigravity concurrently with role-specific prompts
    ├─ skips any reviewer that hits a provider quota/rate limit (429)
    ├─ parses structured JSON output from each reviewer
    ├─ spawns codex again as consilium judge
    │   ├─ merges duplicates, applies consensus bonus
    │   ├─ filters by confidence threshold (≥6)
    │   └─ produces unified verdict with found_by attribution
    ├─ prints formatted review to stdout
    └─ TUI groups all sessions by GroupID

Second terminal:
    rival tui
      ├─ watches ~/.rival/sessions/ via fsnotify (.json + .log)
      ├─ groups sessions by GroupID for megareview display
      ├─ live-refreshes every second while sessions are running
      └─ x key sends SIGTERM to kill stuck sessions
```

### Key design decisions

- **Full project access**: reviewers run as AI CLI tools with tool use — they explore your codebase, not just diffs
- **Async, non-blocking**: skills launch rival detached and watch it in the background — your session is never blocked for the run, and a hung run can never hang the session (`RIVAL_RUN_TIMEOUT` + `rival wait` crash/timeout detection)
- **Stdin piping**: prompts passed via heredoc, never shell-quoted into argv (prevents injection)
- **Env filtering**: child processes get a sanitized environment (blocks proxy/preload vars from .env)
- **Fault tolerant**: megareview continues if one CLI fails, reports the error inline
- **Consilium overflow protection**: reviewer outputs that fail JSON parsing are replaced with a stub + 2KB debug tail, preventing oversized judge prompts

## Claude: Native vs Docker

Claude auto-detects its execution mode:

- **Native** (default): if `claude` CLI is on PATH, uses it directly. No extra config needed.
- **Docker**: if `claude` CLI is not available, runs inside a Docker container with a separate Anthropic subscription.

### Auth: subscription by default, API key only if explicit

Native claude/fable runs bill your **claude CLI subscription login** (Pro/Max)
by default. An `ANTHROPIC_API_KEY` exported in your shell is **stripped from
the child environment** — without this, the claude CLI silently prefers the env
key and bills API credits even though you're logged in with a subscription.

| `RIVAL_CLAUDE_AUTH` | Behavior |
|---------------------|----------|
| unset / `subscription` / `sub` | Use the CLI's `/login` auth; `ANTHROPIC_API_KEY` / `ANTHROPIC_AUTH_TOKEN` are stripped from the subprocess env |
| `api` | Bill the API key; **requires** `ANTHROPIC_API_KEY` to be set, hard error if empty |
| anything else | Hard error — auth is never guessed |

The active mode is logged per run (`auth=subscription`) and shown in the TUI
detail view (Account field). If a run fails on auth/billing, rival appends an
actionable hint: not logged in → run `claude` and `/login`; api mode → check
the key and its credit balance.

### Docker Setup

1. Build the image (auto-builds on first run, or manually):
   ```bash
   docker build -t rival-claude -f - . <<'EOF'
   FROM node:22-slim
   RUN npm install -g @anthropic-ai/claude-code && \
       useradd -m -s /bin/bash claude
   USER claude
   WORKDIR /workspace
   ENTRYPOINT ["claude"]
   EOF
   ```

2. Authenticate via interactive login in a temp container:
   ```bash
   docker run -d --name rival-claude-login --user claude --entrypoint sh rival-claude -c 'sleep 3600'
   docker exec -it rival-claude-login claude login
   # Opens auth URL → authorize in browser → paste localhost redirect back
   docker exec rival-claude-login cat /home/claude/.claude/.credentials.json
   # Copy the accessToken value (starts with sk-ant-oat01-...)
   docker rm -f rival-claude-login
   ```

3. Export the token:
   ```bash
   export RIVAL_CLAUDE_TOKEN=sk-ant-oat01-YOUR-TOKEN-HERE
   ```

4. Optionally set subscription type in `~/.rival/config.yaml`:
   ```yaml
   claude:
     subscription: team    # or "personal" — shown in TUI
   ```

### Notes

- OAuth tokens expire — re-run the login flow if you get 401 errors
- The Docker image runs as non-root user `claude` (required by Claude CLI)
- Your workdir is mounted as `/workspace` inside the container
- To rebuild: `docker rmi rival-claude`, next run rebuilds automatically
- TUI shows `⬡ claude/dk` for Docker sessions, `⬡ claude` for native

## Models

| CLI | Model | Default Effort | Used by |
|-----|-------|---------------|---------|
| Codex | `gpt-5.5` | xhigh | megareview, consilium judge, `/rival-codex-only`, `/rival-plan`, `/rival-plan-codex` |
| Antigravity | `gemini-3.5-flash` | xhigh | megareview, judge fallback, `/rival-antigravity-only` |
| Gemini | `gemini-3.1-pro-preview` | xhigh | standalone `rival command gemini` only |
| Claude | `claude-opus-4-8[1m]` | max | standalone only |
| claude-fable | `claude-fable-5` | max | `/rival-plan`, `/rival-plan-fable` |

## Uninstall

```bash
rm -rf ~/.claude/skills/rival-codex-only ~/.claude/skills/rival-antigravity-only ~/.claude/skills/rival-plan ~/.claude/skills/rival-plan-codex ~/.claude/skills/rival-plan-fable ~/.claude/skills/rival-review
brew uninstall rival        # if installed via brew
# or: rm "$(go env GOPATH)/bin/rival"   # if installed from source
```

## License

MIT
