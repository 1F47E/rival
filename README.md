# codex — Claude Code Plugin

Run [OpenAI Codex CLI](https://github.com/openai/codex) from Claude Code as a subagent.

Forces `gpt-5.4` model with `xhigh` reasoning effort. Keeps main conversation context clean by running codex in a dedicated agent.

## Install

```bash
# Add this repo as a marketplace
claude plugin marketplace add github:1F47E/claude-codex-plugin

# Install the plugin
claude plugin install codex@claude-codex-plugin
```

## Prerequisites

- [Codex CLI](https://github.com/openai/codex) installed: `npm install -g @openai/codex`
- Authenticated: `codex login`

## Usage

```
/codex explain the auth flow in this project
/codex find bugs in src/main.go
/codex list all TypeScript files and summarize the project structure
```

Run `/codex` with no arguments to see usage info.

## How it works

The `/codex` command dispatches to a `codex-runner` subagent that:

1. Verifies codex is installed and authenticated
2. Runs `codex exec` with your prompt:
   - Model: `gpt-5.4`
   - Reasoning: `xhigh`
   - Full auto mode (no approval prompts)
   - Ephemeral session (no persistence)
3. Returns the output to your Claude Code session

Output files are saved to `/tmp/codex-run-*.txt` for reference.

## License

MIT
