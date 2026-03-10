# rival — Claude Code Plugin

<img src="assets/banner2.png" width="600px">

Dispatch prompts to external AI CLIs from Claude Code. Run GPT-5.4 via Codex or Gemini 2.5 Pro via Gemini CLI — as subagents that keep your main context clean.

**Zero Claude tokens.** All heavy lifting runs on your Codex/Gemini subscription, not your Claude usage.

## Install

```bash
# Add this repo as a marketplace
claude plugin marketplace add https://github.com/1F47E/rival

# Install the plugin
claude plugin install rival@rival
```

## Prerequisites

- [Codex CLI](https://github.com/openai/codex) installed: `npm install -g @openai/codex` + `codex login`
- [Gemini CLI](https://github.com/google-gemini/gemini-cli) installed: `npm install -g @google/gemini-cli` + set `GEMINI_API_KEY`

You only need the CLI for the commands you use — Codex for `/rival:codex` and `/rival:review`, Gemini for `/rival:gemini`.

## Commands

### `/rival:codex [-re <level>] <prompt>` — Run prompt via Codex CLI

```
/rival:codex explain the auth flow in this project
/rival:codex -re xhigh find bugs in src/main.go
/rival:codex list all TypeScript files and summarize the project structure
```

**Reasoning effort** (`-re`): `low`, `medium` (default), `high`, `xhigh`

### `/rival:review [-re <level>] [path or scope]` — Code review via Codex CLI

Ruthless code review from GPT-5.4 covering architecture, API design, security, performance, concurrency, and Go/TS best practices.

```
/rival:review                        # review entire project
/rival:review src/api/               # review specific directory
/rival:review -re high src/api/      # review with high reasoning effort
/rival:review the auth middleware     # review specific component
```

### `/rival:gemini [-m <model>] <prompt>` — Run prompt via Gemini CLI

```
/rival:gemini explain the auth flow in this project
/rival:gemini -m gemini-2.5-flash summarize this codebase
/rival:gemini find security issues in the API layer
```

**Models** (`-m`): `gemini-2.5-pro` (default), `gemini-2.5-flash`, `gemini-2.5-flash-lite`

## How it works

Each command dispatches to a dedicated runner subagent:

**Codex runner** (`/rival:codex`, `/rival:review`):
1. Verifies codex is installed and authenticated
2. Runs `codex exec` with the prompt (GPT-5.4, configurable reasoning effort, read-only sandbox, ephemeral session)
3. Returns the output to your Claude Code session

**Gemini runner** (`/rival:gemini`):
1. Verifies gemini CLI is installed
2. Runs `gemini` with the prompt (Gemini 2.5 Pro by default, sandbox mode, isolated config)
3. Returns the output to your Claude Code session

Temp files are created in private directories and auto-cleaned after each run.

## Security

### Codex runner
- **Strict input protocol** — mode header with optional effort line and `---` separator; rejects malformed requests
- **Randomized quoted heredoc** — prevents shell injection via crafted prompts
- **Read-only sandbox** — `--sandbox read-only` prevents Codex from writing to the filesystem
- **Ephemeral sessions** — `--ephemeral` ensures no session state persists between runs
- **Private temp directory** — created with `umask 077`; inaccessible to other users
- **Untrusted output labeling** — output is presented with an untrusted-output warning

### Gemini runner
- **Strict input protocol** — mode/model header with model allowlist validation and `---` separator
- **Model allowlist** — only `gemini-2.5-pro`, `gemini-2.5-flash`, `gemini-2.5-flash-lite` accepted (prevents injection via model arg)
- **Randomized quoted heredoc** — same shell injection prevention as Codex runner
- **Sandbox mode** — `--sandbox` uses macOS seatbelt (note: not equivalent to Codex's read-only sandbox)
- **Isolated config** — runs with a temp `GEMINI_HOME` to prevent user settings/extensions/hooks from loading
- **Weaker prompt boundary** — Gemini CLI may preprocess `/` slash commands and `@include` directives in prompt text. This is a known difference from Codex.

## Version

3.0.0

## License

MIT
