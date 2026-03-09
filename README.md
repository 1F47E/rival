# codex — Claude Code Plugin

Run [OpenAI Codex CLI](https://github.com/openai/codex) from Claude Code as a subagent.

Forces `gpt-5.4` model with `xhigh` reasoning effort. Keeps main conversation context clean by running codex in a dedicated agent.

## Install

```bash
# Add this repo as a marketplace
claude plugin marketplace add https://github.com/1F47E/claude-codex-plugin

# Install the plugin
claude plugin install codex@claude-codex-plugin
```

## Prerequisites

- [Codex CLI](https://github.com/openai/codex) installed: `npm install -g @openai/codex`
- Authenticated: `codex login`

## Commands

### `/codex:run <prompt>` — Run any prompt

```
/codex:run explain the auth flow in this project
/codex:run find bugs in src/main.go
/codex:run list all TypeScript files and summarize the project structure
```

Run `/codex:run` with no arguments to see usage info.

### `/codex:rival [path or scope]` — Second opinion code review

Get a ruthless code review from GPT-5.4 covering architecture, API design, security, performance, concurrency, and Go/TS best practices.

```
/codex:rival                        # review entire project
/codex:rival src/api/               # review specific directory
/codex:rival the auth middleware     # review specific component
```

## How it works

Both commands dispatch to a `codex-runner` subagent that:

1. Verifies codex is installed and authenticated
2. Runs `codex exec` with the prompt:
   - Model: `gpt-5.4`
   - Reasoning: `xhigh`
   - Full auto mode (no approval prompts, no sandbox)
   - Ephemeral session (no persistence)
3. Returns the output to your Claude Code session

`/codex:run` passes your prompt verbatim. `/codex:rival` constructs a comprehensive review prompt targeting architecture, security, performance, and language-specific issues.

Output files are saved to `/tmp/codex-run.*` for reference.

## License

MIT
