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

`rival install` copies the Claude Code skills (embedded in the binary) into `~/.claude/skills/`. After that, `/rival-review`, `/rival-codex-only`, `/rival-antigravity-only`, and `/rival-plan` are available in Claude Code. (Install also removes the deprecated `/rival-gemini-only` and `/rival-claude-only` skills.)

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

**Plan/spec review** (single path to a markdown plan, rated 1-10 by Codex):

```
/rival-plan path/to/plan.md                — rate the plan 1-10, surface bugs + gaps
```

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

# Rate a plan/spec doc 1-10 with Codex
echo 'docs/plan.md' | rival command plan --workdir .
```

### TUI Dashboard

Monitor running and past sessions in a full-screen terminal UI:

```bash
rival tui
```

**List view** shows all sessions with status, CLI (◈ codex / △ antigravity / ⬡ claude / ▤ plan / ◈△ mega), model, effort, elapsed time, workdir, and prompt preview. Megareview sessions are grouped into a single row. Claude sessions show `⬡ claude` for native or `⬡ claude/dk` for Docker mode. Plan reviews show `▤ plan`.

**Detail view** shows full metadata (including Mode and Account/subscription type for Claude), prompt, and live-streaming log output. For megareview groups, all reviewer logs are shown.

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

## Architecture

```
Claude Code main session
    │
    │ /rival-review
    ▼
Claude skill (context: fork)
    │
    │ stdin heredoc → rival command megareview --workdir $(pwd)
    ▼
rival binary
    ├─ parses arguments (-re flag, review/prompt mode)
    ├─ builds review prompt with scope injection
    ├─ spawns codex/antigravity via subprocess
    ├─ pipes prompt to stdin, tees stdout to log file
    ├─ writes session JSON + live log to ~/.rival/sessions/
    └─ returns output to skill → back to Claude Code

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
- **Isolated execution**: skills use `context: fork` — runs in subagent, zero impact on your Claude context
- **Stdin piping**: prompts passed via heredoc, never shell-quoted into argv (prevents injection)
- **Env filtering**: child processes get a sanitized environment (blocks proxy/preload vars from .env)
- **Fault tolerant**: megareview continues if one CLI fails, reports the error inline
- **Consilium overflow protection**: reviewer outputs that fail JSON parsing are replaced with a stub + 2KB debug tail, preventing oversized judge prompts

## Claude: Native vs Docker

Claude auto-detects its execution mode:

- **Native** (default): if `claude` CLI is on PATH, uses it directly. No extra config needed.
- **Docker**: if `claude` CLI is not available, runs inside a Docker container with a separate Anthropic subscription.

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
| Codex | `gpt-5.5` | xhigh | megareview, consilium judge, `/rival-codex-only`, `/rival-plan` |
| Antigravity | `gemini-3.5-flash` | xhigh | megareview, judge fallback, `/rival-antigravity-only` |
| Gemini | `gemini-3.1-pro-preview` | xhigh | standalone `rival command gemini` only |
| Claude | `claude-opus-4-6[1m]` | max | standalone only |

## Uninstall

```bash
rm -rf ~/.claude/skills/rival-codex-only ~/.claude/skills/rival-antigravity-only ~/.claude/skills/rival-plan ~/.claude/skills/rival-review
brew uninstall rival        # if installed via brew
# or: rm "$(go env GOPATH)/bin/rival"   # if installed from source
```

## License

MIT
