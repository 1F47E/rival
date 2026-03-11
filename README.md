# rival

<img src="assets/banner2.png" width="600px">

Dispatch prompts to external AI CLIs from Claude Code. Run GPT-5.4 via Codex or Gemini 3.1 Pro via Gemini CLI — as isolated subagents that keep your main context clean.

**Zero Claude tokens.** All heavy lifting runs on your Codex/Gemini subscription, not your Claude usage.

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

This symlinks the Claude Code skills into `~/.claude/skills/`. After installation, `/rival-codex` and `/rival-gemini` are available in Claude Code.

### Prerequisites

- [Codex CLI](https://github.com/openai/codex): `npm install -g @openai/codex` + `codex login`
- [Gemini CLI](https://github.com/google-gemini/gemini-cli): `npm install -g @google/gemini-cli` + set `GEMINI_API_KEY`

You only need the CLI for the commands you use.

## Usage

### Claude Code Skills

```
/rival-codex explain the auth flow in this project
/rival-codex -re xhigh find bugs in src/main.go
/rival-codex review                        — ruthless code review of entire project
/rival-codex review src/api/               — review specific scope
/rival-codex -re xhigh review src/api/     — review with xhigh reasoning
```

```
/rival-gemini explain the auth flow
/rival-gemini -re high analyze this complex algorithm
/rival-gemini review                       — code review of entire project
/rival-gemini review src/api/              — review specific scope
```

**Reasoning effort** (`-re`): `low`, `medium` (default), `high`, `xhigh`

### Direct CLI

```bash
# Run with prompt from stdin
echo 'explain the auth flow' | rival run codex --prompt-stdin --workdir "$PWD"
echo 'explain the auth flow' | rival run gemini --prompt-stdin --workdir "$PWD"

# Review mode
rival run codex --review "src/api/" --effort xhigh --workdir "$PWD"
rival run gemini --review "" --workdir "$PWD"   # reviews entire project
```

### TUI Dashboard

Monitor running and past sessions in a second terminal:

```bash
rival tui
```

Two-pane layout: session list on the left, live log + metadata on the right. Keys: `j/k` navigate, `tab` switch panes, `q` quit.

### Session List

```bash
rival sessions              # all sessions
rival sessions --active     # running only
rival sessions --recent 10  # last 10
```

## Architecture

```
Claude Code main session
    │
    │ /rival-codex review src/
    ▼
Claude skill (context: fork)
    │
    │ stdin heredoc → rival command codex --workdir $(pwd)
    ▼
rival binary
    ├─ parses arguments (same grammar as old plugin commands)
    ├─ validates flags/effort
    ├─ builds review prompt if needed
    ├─ executes codex/gemini via exec.Command
    ├─ writes session JSON + live log to ~/.rival/sessions/
    └─ returns final output

Second terminal:
    rival tui
      ├─ watches ~/.rival/sessions/ via fsnotify
      └─ shows live logs, status, durations, errors
```

- Skills run with `context: fork` for isolated subagent execution
- Prompts are passed via stdin pipes, never shell-quoted into argv
- Session storage tracks metadata only (no full prompts by default)

## Uninstall

```bash
./scripts/uninstall-skills.sh
# Remove binary from GOPATH/bin
rm "$(go env GOPATH)/bin/rival"
```

## License

MIT
