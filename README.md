# rival

<img src="assets/banner2.png" width="600px">

Dispatch prompts to external AI CLIs from Claude Code. Run GPT-5.4 via Codex or Gemini 3.1 Pro via Gemini CLI — as isolated subagents that keep your main context clean.

**Zero Claude tokens.** All heavy lifting runs on your Codex/Gemini subscription, not your Claude usage.

![TUI Dashboard](rival-tui.gif)

## Install

### Binary

```bash
cd rival && make install
```

Or with `go install`:

```bash
go install github.com/1F47E/rival@latest
```

### Skills

```bash
./scripts/install-skills.sh
```

This symlinks the Claude Code skills into `~/.claude/skills/`. After installation, `/rival-codex`, `/rival-gemini`, and `/rival-megareview` are available in Claude Code.

### Prerequisites

- [Codex CLI](https://github.com/openai/codex): `npm install -g @openai/codex` + `codex login`
- [Gemini CLI](https://github.com/google-gemini/gemini-cli): `npm install -g @google/gemini-cli` + set `GEMINI_API_KEY`

You only need the CLI for the commands you use. Megareview requires both.

## Usage

### Claude Code Skills

```
/rival-codex explain the auth flow in this project
/rival-codex -re xhigh find bugs in src/main.go
/rival-codex review                        — code review of entire project
/rival-codex review src/api/               — review specific scope
/rival-codex -re xhigh review src/api/     — review with xhigh reasoning
```

```
/rival-gemini explain the auth flow
/rival-gemini -re high analyze this complex algorithm
/rival-gemini review                       — code review of entire project
/rival-gemini review src/api/              — review specific scope
```

```
/rival-megareview                          — review entire project with BOTH CLIs in parallel
/rival-megareview src/api/                 — review specific scope
/rival-megareview -re xhigh src/api/       — both CLIs, max reasoning effort
```

**Reasoning effort** (`-re`): `low`, `medium`, `high` (default), `xhigh`

### How Reviews Work

When you run a review, Codex/Gemini get **full access to your project**. They don't just see a diff — they run as CLI tools inside your workdir with tool use enabled, so they can:

- Read any file in the project
- Follow imports and trace dependencies
- Explore the full codebase to understand context
- Run commands to inspect project structure

**No scope = full project review.** Running `/rival-codex review` with no arguments reviews the entire project. The reviewer will explore all directories and files it considers relevant.

The **scope** is a focus hint, not a restriction. `review src/api/` tells the reviewer to focus on `src/api/`, but it can (and will) read other files to understand the code in context. It does **not** run `git diff` — it works with the full project tree as-is.

This means you can use natural language for the scope:

```
/rival-codex review the files changed in the last commit
/rival-codex review the authentication middleware
/rival-megareview -re xhigh the new payment flow in src/billing/
```

The reviewer will figure out what to look at, explore the relevant code, and give you a review with full project understanding.

### Direct CLI

```bash
# Run with prompt from stdin
echo 'explain the auth flow' | rival command codex --workdir .
echo 'explain the auth flow' | rival command gemini --workdir .

# Review via megareview (both CLIs in parallel)
echo 'src/api/' | rival command megareview --workdir .
```

### TUI Dashboard

Monitor running and past sessions in a full-screen terminal UI:

```bash
rival tui
```

**List view** shows all sessions with status, CLI (◈ codex / ✦ gemini / ◈✦ mega), model, effort, elapsed time, workdir, and prompt preview. Megareview sessions are grouped into a single row.

**Detail view** shows full metadata, prompt, and live-streaming log output. For megareview groups, both Codex and Gemini logs are shown side by side.

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
    │ /rival-codex review src/
    ▼
Claude skill (context: fork, disable-model-invocation)
    │
    │ stdin heredoc → rival command codex --workdir $(pwd)
    ▼
rival binary
    ├─ parses arguments (-re flag, review/prompt mode)
    ├─ builds review prompt with scope injection
    ├─ spawns codex/gemini via subprocess
    ├─ pipes prompt to stdin, tees stdout to log file
    ├─ writes session JSON + live log to ~/.rival/sessions/
    └─ returns output to skill → back to Claude Code

Megareview:
    rival binary
    ├─ generates shared GroupID (UUID)
    ├─ spawns codex + gemini concurrently (goroutines)
    ├─ each gets its own session with shared GroupID
    ├─ waits for both, prints combined output
    └─ TUI groups them into a single display row

Second terminal:
    rival tui
      ├─ watches ~/.rival/sessions/ via fsnotify (.json + .log)
      ├─ groups sessions by GroupID for megareview display
      ├─ live-refreshes every second while sessions are running
      └─ x key sends SIGTERM to kill stuck sessions
```

### Key design decisions

- **Full project access**: reviewers run as AI CLI tools with tool use — they explore your codebase, not just diffs
- **Isolated execution**: skills use `context: fork` + `disable-model-invocation` — zero impact on your Claude context
- **Stdin piping**: prompts passed via heredoc, never shell-quoted into argv (prevents injection)
- **Env filtering**: child processes get a sanitized environment (blocks proxy/preload vars from .env)
- **Fault tolerant**: megareview continues if one CLI fails, reports the error inline

## Uninstall

```bash
./scripts/uninstall-skills.sh
rm "$(go env GOPATH)/bin/rival"
```

## License

MIT
