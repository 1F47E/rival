# rival

<img src="assets/banner2.png" width="600px">

Dispatch prompts to external AI models from your coding session as isolated subagents that keep the main context clean. The default `/rival-review` runs Sol, DeepSeek V4 Pro, Kimi K2.7 Code, and GLM-5.2 in parallel and merges their findings with a consilium judge. Use `-m/--model` to run an exact subset for one review.

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

`rival install` copies the slash-command skills (embedded in the binary) into `~/.claude/skills/`. After that, `/rival-review`, `/rival-sol`, `/rival-plan`, `/rival-plan-sol`, `/rival-plan-fable`, and `/rival-fable` are available. Install also removes superseded skill names.

Use `rival install --force` to overwrite without prompting.

### Prerequisites

- [Sol runtime](https://github.com/openai/codex): install and authenticate it, then use `/rival-review`, `/rival-sol`, `/rival-plan`, or `/rival-plan-sol`
- Antigravity CLI (`agy`): install + authenticate to a quota-bearing account — optional, available through the standalone binary command
- DeepSeek V4 Pro, Kimi K2.7 Code, and GLM-5.2 runtime: install [opencode](https://opencode.ai) and set `RIVAL_OPENCODE_API_KEY` to a Zen key
- [Gemini CLI](https://github.com/google-gemini/gemini-cli): `npm install -g @google/gemini-cli` + set `GEMINI_API_KEY` — optional, only for the standalone `rival command gemini`
- [Opus/Fable runtime](https://docs.anthropic.com/en/docs/claude-code/overview): install + authenticate (or use Docker — see below), then use `/rival-plan`, `/rival-plan-fable`, or `/rival-fable`

You only need the runtimes for the models you use. **Megareview uses four curated models** — any unavailable selection is skipped, and the run proceeds when at least one succeeds. Put `RIVAL_OPENCODE_API_KEY` in `~/.zshenv` so non-interactive shells inherit it.

## Usage

### Slash-command skills

**Default review** (runs Sol + DeepSeek V4 Pro + Kimi K2.7 Code + GLM-5.2):

```
/rival-review                              — all four models; auto-detect changed files
/rival-review -m sol src/api/              — Sol only
/rival-review -m deepseek src/api/         — DeepSeek V4 Pro only
/rival-review -m kimi src/api/             — Kimi K2.7 Code only
/rival-review -m glm src/api/              — GLM-5.2 only
/rival-review -m deepseek,kimi src/api/    — exactly those two models
/rival-review -re ultra src/api/           — highest supported effort
```

**Single-model skills** (use only when you want one specific model):

```
/rival-sol explain the auth flow in this project
/rival-sol -re ultra find bugs in src/main.go
/rival-sol review                          — review (auto-detects changed files via git)
/rival-sol review src/api/                 — review specific scope
```

**Code review via Fable** (medium effort by default):

```
/rival-fable                               — review changed files with Fable (auto-detects via git)
/rival-fable src/api/                      — review a specific scope
```

**Plan/spec review** (single path to a markdown plan, rated 1-10):

```
/rival-plan path/to/plan.md                 — review with Sol + Fable at ultra effort
/rival-plan-sol path/to/plan.md             — rate the plan 1-10 with Sol at ultra effort
/rival-plan-fable path/to/plan.md           — same, with Fable (low effort by default)
```

Each model rates the plan 1-10 and returns numbered findings. `/rival-plan` runs Sol and Fable in parallel at **ultra** and shows both results. `/rival-plan-sol` also always uses **ultra**; `/rival-plan-fable` defaults to **low**. The native binary defaults to high for Sol and accepts an explicit override, for example `rival command plan --model sol --effort ultra`.

**Model selection** (`-m`, `--model`): `sol`, `deepseek`, `kimi`, and `glm`; comma-separated or repeated. An explicit list replaces the complete roster. **Reasoning effort** (`-re`): `low`, `medium`, `high` (default), `ultra`.

### How Reviews Work

When you run a review, the selected models get **read-only access to your project**. They don't just see a diff — they run as isolated agents inside your workdir with safe exploration tools enabled, so they can:

- Read any file in the project
- Follow imports and trace dependencies
- Explore the full codebase to understand context
- Run inspection commands inside a read-only sandbox without modifying project files

**Smart scope detection.** Running `/rival-review` with no arguments auto-detects what to review via git:
1. **Dirty files** (staged + unstaged + untracked new files) → reviews those files
2. **Last commit** (if working tree is clean) → reviews files from HEAD
3. **Full project** → only if not a git repo or no changes found

The **scope** is a focus hint, not a restriction. `review src/api/` tells the reviewer to focus on `src/api/`, but it can (and will) read other files to understand the code in context. Explicit scope bypasses git detection entirely.

This means you can use natural language for the scope:

```
/rival-sol review the files changed in the last commit
/rival-sol review the authentication middleware
/rival-review -m deepseek -re ultra the new payment flow in src/billing/
```

The reviewer will figure out what to look at, explore the relevant code, and give you a review with full project understanding.

### Roles & Consilium (megareview)

Megareview assigns **specialized roles** to each reviewer:

- **Sol → Bug Hunter** — an independent correctness pass and the first default consilium-judge candidate.
- **DeepSeek V4 Pro → Bug Hunter** — the primary correctness reviewer for concrete code-level defects, broken state transitions, races, and missing edge cases.
- **Kimi K2.7 Code → Architecture & Security** — follows multi-step, cross-file flows and looks for architectural regressions, incomplete refactors, and security gaps.
- **GLM-5.2 → Code Quality** — uses its large-context strength to inspect broad schemas, many tables, and report-generation paths for maintainability and correctness risks.

DeepSeek V4 Pro, Kimi K2.7 Code, and GLM-5.2 share one Zen credential/quota. Under load those three may hit a 429 together; Rival skips failed reviewers and proceeds when at least one remains.

All reviewers emit **structured JSON** with file, line, severity, category, confidence (1-10), and fix suggestions.

If a reviewer hits a provider quota/rate limit, Rival detects it from the captured log and reports that concrete model as **skipped** with a reason, rather than silently counting it as a clean empty review.

Role prompts can be customized via `~/.rival/config.yaml`:

```yaml
roles:
  bug_hunter: |
    Your custom bug hunter instructions...
  code_quality: |
    Your custom code quality instructions...
```

A separate **consilium pass** runs with the highest-priority selected model that
successfully reviewed (default priority: Sol → DeepSeek V4 Pro → Kimi K2.7 Code → GLM-5.2), then:
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
  Found by: deepseek-v4-pro, kimi-k2.7-code

[HIGH] file.go:100 — Title
  ...

Recommendation: request_changes — ...

Reviewed by: sol (bug_hunter), deepseek-v4-pro (bug_hunter), kimi-k2.7-code (arch_security), glm-5.2 (code_quality)
Judge: sol (consilium)
Findings: 5 (threshold: 6)
```

For an explicit multi-model selection, requested order controls judge priority. A single-model review uses that same model for the review and consilium phases. If a reviewer fails to produce structured JSON, the consilium receives a stub with a 2KB debug tail instead of the full raw output (prevents prompt overflow).

### Direct CLI

```bash
# Run Sol with a prompt from stdin
echo 'explain the auth flow' | rival command sol --workdir .
echo 'explain the auth flow' | rival command antigravity --workdir .

# Review via the default four-model roster
echo 'src/api/' | rival command megareview --workdir .

# Exact one-model review (native binary flag)
echo 'src/api/' | rival command megareview --model sol --workdir .
echo 'src/api/' | rival command megareview --model deepseek --workdir .
rival review --model kimi src/api/

# Rate a plan/spec doc 1-10 (Sol + Fable by default, high effort)
echo 'docs/plan.md' | rival command plan --workdir .
echo 'docs/plan.md' | rival command plan --model sol --effort ultra --workdir .
echo 'docs/plan.md' | rival command plan --model fable --effort low --workdir .
```

### TUI Dashboard

Monitor running and past sessions in a full-screen terminal UI:

```bash
rival tui
```

**List view** shows all sessions with status, concrete model, effort, elapsed time, workdir, and prompt preview. Multi-session runs are grouped into one row: the megareview glyph repeats per selected reviewer (`❯ mega` through `❯❯❯❯ mega`). Plan reviews (`/rival-plan`, `/rival-plan-sol`, `/rival-plan-fable`) show `▤ plan`.

**Detail view** shows full metadata, prompt, and live-streaming log output. Group titles and metadata are derived from the sessions: a default megareview lists `sol+deepseek-v4-pro+kimi-k2.7-code+glm-5.2`, while an explicit subset shows only its selected models. A dual plan group lists `sol+fable`. All member logs and model-specific failures are shown; `o` opens one combined public log for a grouped run.

#### Keys

| Key | List View | Detail View |
|-----|-----------|-------------|
| `j/k` or `↑/↓` | Navigate sessions | — |
| `Enter` | Open detail view | — |
| `Esc` | — | Back to list |
| `g` / `G` | Jump to top / bottom | — |
| `p` | — | Toggle full prompt |
| `o` | — | Open log (all logs for a group) |
| `x` | — | Kill running session |
| `q` | Quit | Quit |

### Web Dashboard

```bash
rival server --port 3333
```

The browser dashboard uses the same stable grouping and requested model order as
the TUI. It shows public model names and icons, labels judge output separately,
and includes each member's log and failure reason.

### Session Management

```bash
rival sessions              # recent sessions as a text table
rival version               # show version
```

### Review Queue

Multiple coding-agent sessions (or terminals) invoke `rival` as independent
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
caller's teardown (the host may kill a skill's shells with a process-group kill
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
Host coding session
    │
    │ /rival-review
    ▼
Slash-command skill (async — does not block the session)
    │ 1. prompt tempfile → rival command megareview --detach --workdir $(pwd)
    │    (parent prints "rival: detached pid=N" and exits in seconds)
    │ 2. arm background watcher:  rival wait --log <err> --timeout 100m
    │ 3. hand back to the session and END the turn
    ▼
rival binary (own process session — survives the skill/fork teardown)
    ├─ parses model selection (-m), effort (-re), and review scope
    ├─ builds review prompt with scope injection
    ├─ waits for a queue slot, then bounds the run by RIVAL_RUN_TIMEOUT
    ├─ spawns the selected models via isolated subprocesses
    ├─ pipes prompt to stdin, tees stdout to log file
    └─ writes session JSON + live log to ~/.rival/sessions/
         │
         ▼  (rival exits → background `rival wait` exits → harness wakes session)
    The host reads the output file and presents the review verbatim

Megareview (roles + consilium):
    rival binary
    ├─ generates shared GroupID (UUID)
    ├─ assigns roles: Sol/DeepSeek=bug_hunter, Kimi=arch_security, GLM=code_quality
    ├─ spawns the exact selected roster concurrently with role-specific prompts
    ├─ skips any reviewer that hits a provider quota/rate limit (429)
    ├─ parses structured JSON output from each reviewer
    ├─ runs the highest-priority successful selected model as consilium judge
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

## Opus and Fable: Native vs Docker

The Opus/Fable runtime auto-detects its execution mode:

- **Native** (default): uses the authenticated host runtime. No extra config needed.
- **Docker**: otherwise runs inside a container with a separate subscription.

### Auth: subscription by default, API key only if explicit

Native Opus/Fable runs bill your **CLI subscription login** (Pro/Max)
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

See the [full Opus/Fable Docker setup](docs/opus-fable-docker-setup.md) for architecture and troubleshooting details.

1. Build the image (auto-builds on first run, or manually):
   ```bash
   docker build -t rival-opus-fable -f - . <<'EOF'
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
   docker run -d --name rival-opus-fable-login --user claude --entrypoint sh rival-opus-fable -c 'sleep 3600'
   docker exec -it rival-opus-fable-login claude login
   # Opens auth URL → authorize in browser → paste localhost redirect back
   docker exec rival-opus-fable-login cat /home/claude/.claude/.credentials.json
   # Copy the accessToken value (starts with sk-ant-oat01-...)
   docker rm -f rival-opus-fable-login
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
- The Docker image runs as non-root user `claude` (required by the runtime)
- Your workdir is mounted as `/workspace` inside the container
- To rebuild: `docker rmi rival-opus-fable`, next run rebuilds automatically
- TUI shows `⬡ opus/dk` for Docker sessions and `⬡ opus` for native

## Models

| Model | Default Effort | Used by |
|-------|----------------|---------|
| Sol | high generally; ultra in plan skills | `/rival-review`, `/rival-sol`, `/rival-plan`, `/rival-plan-sol` |
| `deepseek-v4-pro` | high; `ultra` maps to max | `/rival-review` |
| `kimi-k2.7-code` | model default | `/rival-review` |
| `glm-5.2` | high; `ultra` maps to max | `/rival-review` |
| `gemini-3.5-flash` | xhigh | standalone binary command |
| `gemini-3.1-pro-preview` | xhigh | standalone binary command |
| Opus | max | standalone binary command |
| Fable | medium for code review; low alone, ultra when paired | `/rival-fable`, `/rival-plan`, `/rival-plan-fable` |

## Uninstall

```bash
rm -rf ~/.claude/skills/rival-sol ~/.claude/skills/rival-plan ~/.claude/skills/rival-plan-sol ~/.claude/skills/rival-fable ~/.claude/skills/rival-plan-fable ~/.claude/skills/rival-review ~/.claude/skills/rival-antigravity-only
brew uninstall rival        # if installed via brew
# or: rm "$(go env GOPATH)/bin/rival"   # if installed from source
```

## License

MIT
