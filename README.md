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

You only need the CLI for the commands you use — Codex for `/rival:codex`, Gemini for `/rival:gemini`.

## Commands

### `/rival:codex [-re <level>] [review [scope] | <prompt>]` — Run prompt via Codex CLI

```
/rival:codex explain the auth flow in this project
/rival:codex -re xhigh find bugs in src/main.go
/rival:codex review                        — ruthless code review of entire project
/rival:codex review src/api/               — review specific scope
/rival:codex -re xhigh review src/api/     — review with xhigh reasoning
```

**Reasoning effort** (`-re`): `low`, `medium` (default), `high`, `xhigh`

### `/rival:gemini [-m <model>] [-re <level>] [review [scope] | <prompt>]` — Run prompt via Gemini CLI

```
/rival:gemini explain the auth flow in this project
/rival:gemini -m gemini-2.5-flash summarize this codebase
/rival:gemini -re high analyze this complex algorithm
/rival:gemini review                                   — ruthless code review of entire project
/rival:gemini review src/api/                          — review specific scope
/rival:gemini -m gemini-2.5-flash -re high review src/ — all flags combined
```

**Models** (`-m`): `gemini-2.5-pro` (default), `gemini-2.5-flash`, `gemini-2.5-flash-lite`

**Reasoning effort** (`-re`): `low`, `medium` (default), `high`, `xhigh` — controls Gemini's thinking budget

## How it works

Each command dispatches to a dedicated runner subagent:

**Codex runner** (`/rival:codex`):
1. Verifies codex is installed and authenticated
2. Runs `codex exec` with the prompt (GPT-5.4, configurable reasoning effort, read-only sandbox, ephemeral session)
3. Returns the output to your Claude Code session

**Gemini runner** (`/rival:gemini`):
1. Verifies gemini CLI is installed
2. Writes a `settings.json` with the requested thinking budget into an isolated config dir
3. Runs `gemini` with the prompt (Gemini 2.5 Pro by default, sandbox mode, isolated config)
4. Returns the output to your Claude Code session

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
- **Strict input protocol** — mode/model/effort header with model allowlist and effort validation, `---` separator
- **Model allowlist** — only `gemini-2.5-pro`, `gemini-2.5-flash`, `gemini-2.5-flash-lite` accepted (prevents injection via model arg)
- **Randomized quoted heredoc** — same shell injection prevention as Codex runner
- **Sandbox mode** — `--sandbox` uses macOS seatbelt (note: not equivalent to Codex's read-only sandbox)
- **Isolated config** — runs with a temp `GEMINI_HOME` to prevent user settings/extensions/hooks from loading
- **Weaker prompt boundary** — Gemini CLI may preprocess `/` slash commands and `@include` directives in prompt text. This is a known difference from Codex.

## Version

3.1.0

## License

MIT
