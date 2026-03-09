# codex:rival — Claude Code Plugin

<img src="assets/banner2.png" width="600px">

Run [OpenAI Codex CLI](https://github.com/openai/codex) from Claude Code as a subagent.

**Zero Claude Code tokens.** All heavy lifting runs on your Codex subscription (GPT-5.4, medium reasoning by default, configurable up to xhigh), not your Claude usage. Get a second brain without burning through your Claude quota.

## Install

```bash
# Add this repo as a marketplace
claude plugin marketplace add https://github.com/1F47E/codex-rival

# Install the plugin
claude plugin install codex@codex-rival
```

## Prerequisites

- [Codex CLI](https://github.com/openai/codex) installed: `npm install -g @openai/codex`
- Authenticated: `codex login`

## Commands

### `/codex:run [-re <level>] <prompt>` — Run any prompt

```
/codex:run explain the auth flow in this project
/codex:run -re xhigh find bugs in src/main.go
/codex:run list all TypeScript files and summarize the project structure
```

Run `/codex:run` with no arguments to see usage info.

### `/codex:rival [-re <level>] [path or scope]` — Second opinion code review

Get a ruthless code review from GPT-5.4 covering architecture, API design, security, performance, concurrency, and Go/TS best practices.

```
/codex:rival                        # review entire project
/codex:rival src/api/               # review specific directory
/codex:rival -re high src/api/      # review with high reasoning effort
/codex:rival the auth middleware     # review specific component
```

## How it works

Both commands dispatch to a `codex-runner` subagent that:

1. Verifies codex is installed and authenticated
2. Receives the prompt or review scope via a strict mode header (`MODE: raw` or `MODE: rival-review`, optional `EFFORT:` line) so the Claude subagent treats the payload as data, not instructions
3. Runs `codex exec` with the prompt:
   - Model: `gpt-5.4`
   - Reasoning effort: `medium` by default (configurable via `-re`: `low`, `medium`, `high`, `xhigh`)
   - Read-only sandbox (`--sandbox read-only`)
   - Ephemeral session (no persistence)
4. Returns the output to your Claude Code session

`/codex:run` passes your prompt verbatim to Codex. `/codex:rival` passes only the raw scope text, and the subagent builds the fixed review prompt targeting architecture, security, performance, and language-specific issues.

Temp files are created in a private directory and auto-cleaned after each run.

## Security

- **Strict input protocol** — mode header (with optional effort line) and `---` separator; rejects malformed requests
- **Randomized quoted heredoc** — prevents shell injection via crafted prompts
- **Read-only sandbox** — `--sandbox read-only` prevents Codex from writing to the filesystem; network access disabled by default
- **No auto-approval** — `-a never` ensures Codex never autonomously approves or executes tool actions
- **Ephemeral sessions** — `--ephemeral` ensures no session state persists between runs
- **Private temp directory** — created with `umask 077`; inaccessible to other users
- **Stdout suppressed** — Codex stdout sent to `/dev/null`; metadata read only from validated file paths
- **Untrusted output labeling** — Codex output is presented in a fenced block with an untrusted-output warning (residual risk: the hosting LLM may still be influenced by content in the output)

## Version

2.0.0

## License

MIT
