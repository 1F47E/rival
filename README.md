# rival

<img src="assets/banner2.png" width="600px">

Dispatch prompts to external AI models from your coding session as separate
reviewer processes. Their repository exploration and tool traces stay out of
your primary agent's context; Rival returns the final review. The default
`/rival-review` runs Sol and DeepSeek V4 Pro in parallel and merges their
findings with a consilium judge. Use `-m/--model` to run an exact subset or add
Kimi K3 as an opt-in reviewer.

## TL;DR — why Rival?

Rival is an orchestration layer for independent AI review, not another model.
One coding agent tends to preserve its own assumptions when reviewing its work;
Rival asks different models, with complementary roles, to inspect the actual
repository and then turns their overlap and disagreements into one verdict.

- **Catch different blind spots:** correctness, architecture/security, and code
  quality reviewers explore the codebase instead of seeing only a pasted diff.
- **Get one usable answer:** a consilium pass merges duplicates, attributes
  agreement, filters weak findings, and reports skipped or failed reviewers.
- **Keep long work out of the way:** reviews run detached with queue and timeout
  controls, while terminal and web dashboards retain progress and bounded logs.
- **Use only the providers you have:** select one model or an exact subset; the
  review can continue when another selected provider is unavailable.

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

> **Note:** run `make install` from the repository's `rival/` subdirectory.
> It writes to `GOBIN` or `$(go env GOPATH)/bin`; make sure that directory
> appears before any older Rival installation in `PATH`. Remote
> `go install github.com/1F47E/rival@latest` is not supported because the Go
> module lives in a repository subdirectory.

`rival install` copies the slash-command skills (embedded in the binary) into `~/.claude/skills/`. After that, `/rival-review`, `/rival-sol`, `/rival-plan`, `/rival-plan-sol`, `/rival-plan-fable`, `/rival-fable`, and `/rival-k3` are available. Install also removes superseded skill names.

Use `rival install --force` to overwrite without prompting.

For a Homebrew installation, this upgrades the binary and then refreshes the
embedded skills:

```bash
rival update
# equivalent: brew upgrade 1F47E/tap/rival && rival install --force
```

Source installs update with `git pull`, `make install` from `rival/`, and
`rival install --force`. Rival performs a cached GitHub release check at
startup; set `RIVAL_NO_UPDATE_CHECK=1` to disable it.

### Prerequisites

- [Codex CLI](https://github.com/openai/codex), authenticated with ChatGPT
  (recommended) or an OpenAI API key, provides Sol.
- [OpenCode](https://opencode.ai/docs) plus an
  [OpenCode Zen key](https://opencode.ai/auth) in
  `RIVAL_OPENCODE_API_KEY` provides DeepSeek V4 Pro.
- [OpenCode](https://opencode.ai/docs) plus a
  [Kimi API key](https://platform.kimi.ai/console/api-keys) in
  `MOONSHOT_API_KEY` provides opt-in Kimi K3.
- [Opus/Fable runtime](https://docs.anthropic.com/en/docs/claude-code/overview): install + authenticate (or use Docker — see below), then use `/rival-plan`, `/rival-plan-fable`, or `/rival-fable`

You only need the runtimes for the models you use. **The default review uses two
curated models** — any unavailable selection is skipped, and the run proceeds
when at least one succeeds. Put `RIVAL_OPENCODE_API_KEY` in the environment file
loaded by non-interactive shells (for example `~/.zshenv` for zsh).

## For Claude Code users

Rival is installed into Claude Code as slash-command skills, but those skills
delegate to local model CLIs. This is the shortest clean setup.

1. Install Rival and its skills:

   ```bash
   brew install 1F47E/tap/rival
   rival install
   ```

2. Install Codex for Sol:

   ```bash
   npm install -g @openai/codex
   codex login
   codex login status
   ```

   Browser-based ChatGPT login is preferred: it uses your eligible ChatGPT
   plan and does not put an API key in the project or Rival config. If you
   intentionally prefer usage-based API billing, create a key in the
   [OpenAI dashboard](https://platform.openai.com/api-keys), then let Codex
   store that authentication:

   ```bash
   export OPENAI_API_KEY='your-key'
   printenv OPENAI_API_KEY | codex login --with-api-key
   unset OPENAI_API_KEY
   codex login status
   ```

   Rival supports both Codex login methods. Do not put the OpenAI key in the
   reviewed repository; Rival only needs the resulting Codex login.

3. Install OpenCode for DeepSeek and K3:

   ```bash
   curl -fsSL https://opencode.ai/install | bash
   # macOS alternative:
   brew install anomalyco/tap/opencode
   ```

4. For the default DeepSeek reviewer, create a key at
   [OpenCode Zen](https://opencode.ai/auth) and expose it to non-interactive
   shells:

   ```bash
   # ~/.zshenv (zsh example)
   export RIVAL_OPENCODE_API_KEY='your-zen-key'
   ```

5. For Kimi K3, create a key in the
   [Kimi API console](https://platform.kimi.ai/console/api-keys). Put it in the
   project `.env` so Rival can find it when Claude Code runs from any
   subdirectory:

   ```dotenv
   MOONSHOT_API_KEY=your-kimi-api-key
   ```

   Add `.env` to `.gitignore` and never commit it. Exporting
   `MOONSHOT_API_KEY` from your shell is also supported. Rival injects the key
   into OpenCode's built-in `moonshotai/kimi-k3` provider at runtime, so no
   custom OpenCode provider config is required.

Run `rival install --force` after every Rival upgrade, then restart or reload
Claude Code so it discovers the refreshed skills.

## Usage

### Slash-command skills

**Default review** (runs Sol + DeepSeek V4 Pro):

```
/rival-review                              — both default models; auto-detect changed files
/rival-review -m sol src/api/              — Sol only
/rival-review -m deepseek src/api/         — DeepSeek V4 Pro only
/rival-review -m k3 src/api/               — Kimi K3 only (requires MOONSHOT_API_KEY)
/rival-review -m deepseek,k3 src/api/      — exactly those two models
/rival-review -re ultra src/api/           — override compatible model defaults
```

**Single-model skills** (use only when you want one specific model):

```
/rival-sol explain the auth flow in this project
/rival-sol -re ultra find bugs in src/main.go
/rival-sol review                          — review (auto-detects changed files via git)
/rival-sol review src/api/                 — review specific scope
```

**Code review via Fable** (built-in medium default, configurable):

```
/rival-fable                               — review changed files with Fable (auto-detects via git)
/rival-fable src/api/                      — review a specific scope
```

**Kimi K3** (thinking-only model, always max reasoning, via opencode's Moonshot provider):

```
/rival-k3 explain the auth flow in this project
/rival-k3 review                           — review changed files with Kimi K3 (auto-detects via git)
/rival-k3 review src/api/                  — review a specific scope
```

> `review` runs under the same mechanical read-only sandbox as the megareview
> reviewers. Raw prompts run **full auto** — the agent can edit files and run
> commands in the workdir (native file tools are denied outside it); rival
> strips known credential env vars (`OPENAI_API_KEY`, `ANTHROPIC_API_KEY`, the
> whole `AWS_*` family, tokens, …) from that child as blast-radius reduction.
> Not containment: bash runs as you.

**Plan/spec review** (single path to a markdown plan, rated 1-10):

```
/rival-plan path/to/plan.md                 — review with Sol + Fable at ultra effort
/rival-plan-sol path/to/plan.md             — rate the plan 1-10 with Sol at ultra effort
/rival-plan-fable path/to/plan.md           — same, with Fable (low fallback unless configured)
```

Each model rates the plan 1-10 and returns numbered findings. `/rival-plan` runs
Sol and Fable in parallel at **ultra** and shows both results.
`/rival-plan-sol` also always uses **ultra**. `/rival-plan-fable` uses the
configured Fable effort when present, otherwise its low fallback. The native
binary accepts an explicit override, for example
`rival command plan --model sol --effort ultra`.

**Model selection** (`-m`, `--model`): `sol`, `deepseek`, and `k3` (Kimi K3,
needs `MOONSHOT_API_KEY`); comma-separated or repeated. An explicit list
replaces the complete roster. **Reasoning effort** (`-re`): `low`, `medium`,
`high`, or `ultra`. When omitted, each model uses its own configured default.

### Model effort defaults

Set defaults by stable model label in `~/.rival/config.yaml`:

```yaml
efforts:
  sol: high
  deepseek-v4-pro: high
  kimi-k3: max
  opus: xhigh
  fable: medium
```

An explicit `--effort` or `-re` wins for every compatible selected model.
Otherwise Rival uses the configured model value, then that command's built-in
fallback. Kimi K3 is fixed at `max`, the only reasoning level its provider
supports. The paired plan skills deliberately request `ultra`; an explicit
skill choice therefore still wins over this file. When a grouped run uses
different per-model efforts, the dashboard summary shows `mixed` and each
member detail shows its actual effort.

Invalid model names or effort values fail before Rival creates sessions or
touches the queue, with the config path and accepted values in the error.

### How Reviews Work

The curated `/rival-review` reviewers and `/rival-k3 review` mode get
mechanically **read-only access to your project**. They don't just see a diff:
they run as separate agents inside your workdir with safe exploration tools
enabled, so they can:

- Read any file in the project
- Follow imports and trace dependencies
- Explore the full codebase to understand context
- Search, list, and inspect project files without modifying them; Sol can also
  run read-only inspection commands

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
- **Kimi K3 → Bug Hunter (opt-in)** — adds a Moonshot-backed correctness pass
  when selected with `-m k3`; it is not part of the default two-model roster.

DeepSeek V4 Pro uses the OpenCode Zen credential and quota. Kimi K3 uses the
separate `MOONSHOT_API_KEY` credential and Moonshot quota. Rival skips failed
reviewers and proceeds when at least one remains.

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
successfully reviewed (default priority: Sol → DeepSeek V4 Pro), then:
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
  Found by: sol, deepseek-v4-pro

[HIGH] file.go:100 — Title
  ...

Recommendation: request_changes — ...

Reviewed by: sol (bug_hunter), deepseek-v4-pro (bug_hunter)
Judge: sol (consilium)
Findings: 5 (threshold: 6)
```

For an explicit multi-model selection, requested order controls judge priority. A single-model review uses that same model for the review and consilium phases. If a reviewer fails to produce structured JSON, the consilium receives a stub with a 2KB debug tail instead of the full raw output (prevents prompt overflow).

### Terminal CLI

```bash
# Run Sol with a prompt from stdin
echo 'explain the auth flow' | rival run sol --prompt-stdin --workdir .
echo 'explain the auth flow' | rival run k3 --prompt-stdin --workdir .   # needs MOONSHOT_API_KEY

# Review via the default two-model roster
rival review src/api/

# Exact one-model reviews
rival review --model sol src/api/
rival review --model deepseek src/api/
rival run k3 --review src/api/ --workdir .

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

**Detail view** shows full metadata, prompt, and live-streaming log output.
Group titles and metadata are derived from the sessions: a default megareview
lists `sol+deepseek-v4-pro`, while an explicit subset shows only its selected
models. A dual plan group lists `sol+fable`. All member logs and model-specific
failures are shown; `o` opens one combined public log for a grouped run.

#### Keys

| Key | List View | Detail View |
|-----|-----------|-------------|
| `j/k` or `↑/↓` | Navigate sessions | — |
| `Enter` | Open detail view | — |
| `Esc` | — | Back to list |
| `g` / `G` | Jump to top / bottom | — |
| `p` | — | Toggle full prompt |
| `o` | — | Open log (all logs for a group) |
| `x` | — | Kill running or queued session |
| `q` | Quit | Quit |

### Web Dashboard

```bash
rival server --port 3333
```

The server binds to `127.0.0.1` only. If the requested port is occupied, Rival
tries the next ten ports and prints the selected local URL.

The self-contained browser dashboard uses the same stable grouping and requested
model order as the TUI. The newest 100 grouped runs load first, with pagination
for older history. A fixed detail drawer shows prompt metadata, per-member
status, queue position, duration, exit code, errors, and a bounded live log
tail. Details remain usable for queued, running, completed, failed, grouped,
empty-log, and missing-log sessions, including direct links to older runs.
Each member log response is limited to the newest 256 KiB.

### Session Management

```bash
rival sessions              # all sessions as a text table
rival sessions --recent 20  # newest 20
rival sessions --active     # running sessions only
rival version                # show version
```

### Review Queue

Multiple coding-agent sessions (or terminals) invoke `rival` as independent
processes. Without coordination they launch their provider CLIs at once and hit
rate limits. rival coordinates them through a **cross-process FIFO queue** — by
default two reviews run at a time; the rest wait their turn. Set
`RIVAL_MAX_CONCURRENT=1` for strict serialization on quota-constrained accounts.

- Each waiting review prints its position to stderr while it waits:
  `rival queue: position 2/3 (1 running), waiting 1m12s`. Skills relay this to you.
- Queued sessions show up in the TUI and web dashboard with a `◌ queued #N` row
  and a growing wait time, alongside a queued counter.
- No daemon: coordination is via ticket files in `~/.rival/queue/` guarded by a
  file lock (`flock`). A crashed holder is reaped automatically (its slot frees when both the
  rival process and any surviving provider CLI child are gone).

```bash
rival queue                 # list tickets (position, state, mode, wait, workdir)
rival queue clear           # remove dead tickets
rival queue clear --force   # remove ALL tickets (live waiters re-queue at the tail)
```

**Config (env vars):**

| Var | Default | Effect |
|-----|---------|--------|
| `RIVAL_MAX_CONCURRENT` | `2` | How many reviews may run at once |
| `RIVAL_QUEUE_TIMEOUT` | `30m` | Max wait for a slot before the review fails |
| `RIVAL_RUN_TIMEOUT` | `30m` | Max run time once a slot is held; kills a hung provider CLI (`0` disables) |
| `RIVAL_NO_QUEUE` | unset | Set to bypass the queue entirely |

`RIVAL_RUN_TIMEOUT` is a per-provider-phase baseline. A megareview receives
twice that budget for its two sequential phases (parallel reviewers, then the
consilium judge); a plan or standalone run receives one baseline.

Provider-invoking `review`, `run`, and `command` subcommands also accept
`--no-queue` to skip the queue for that invocation.

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

**Bounded by default:** a run is bounded by `RIVAL_RUN_TIMEOUT`; `rival wait`
detects a crashed rival (process dead, sessions unfinalized) and has its own
`--timeout`. The skills launch detached, watch in the background, and do not
block your session. Setting `RIVAL_RUN_TIMEOUT=0` intentionally disables the run
bound.

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
    │ 2. arm background watcher:  rival wait --log <err>
    │ 3. hand back to the session and END the turn
    ▼
rival binary (own process session — survives the skill/fork teardown)
    ├─ parses model selection (-m), effort (-re), and review scope
    ├─ builds review prompt with scope injection
    ├─ waits for a queue slot, then bounds the run by RIVAL_RUN_TIMEOUT
    ├─ spawns the selected models in separate subprocesses
    ├─ pipes prompt to stdin, tees stdout to log file
    └─ writes session JSON + live log to ~/.rival/sessions/
         │
         ▼  (rival exits → background `rival wait` exits → harness wakes session)
    The host reads the output file and presents the review verbatim

Megareview (roles + consilium):
    rival binary
    ├─ generates shared GroupID (UUID)
    ├─ assigns the bug_hunter role to Sol, DeepSeek, and opt-in Kimi K3
    ├─ spawns the exact selected roster concurrently with role-specific prompts
    ├─ skips any reviewer that hits a provider quota/rate limit (429)
    ├─ parses structured JSON output from each reviewer
    ├─ runs the highest-priority successful selected model as consilium judge
    │   ├─ merges duplicates, applies consensus bonus
    │   ├─ filters by confidence threshold (≥6)
    │   └─ produces unified verdict with found_by attribution
    ├─ prints formatted review to stdout
    └─ TUI and web dashboards group all sessions by GroupID

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
- **Safe stdin piping**: skills create input files with the host Write tool
  (never echo/printf/heredoc), then redirect them to Rival's stdin
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
| Sol | high; ultra is explicit in plan skills | `/rival-review`, `/rival-sol`, `/rival-plan`, `/rival-plan-sol` |
| `deepseek-v4-pro` | high; `ultra` maps to max | `/rival-review` |
| Opus | xhigh | standalone binary command |
| Fable | medium for code review; plan commands have surface-specific fallbacks | `/rival-fable`, `/rival-plan`, `/rival-plan-fable` |
| `kimi-k3` | max (only level the model supports) | `/rival-k3`, `/rival-review -m k3` |

These are built-in fallbacks. The `efforts` map in `~/.rival/config.yaml`
overrides any omitted model effort; an explicit command or skill effort wins.

## Uninstall

```bash
rm -rf ~/.claude/skills/rival-*
brew uninstall rival        # if installed via brew
# source install:
RIVAL_BIN_DIR="$(go env GOBIN)"
[ -n "$RIVAL_BIN_DIR" ] || RIVAL_BIN_DIR="$(go env GOPATH)/bin"
rm "$RIVAL_BIN_DIR/rival"
```

## License

MIT
