# Claude Code CLI in Docker

Run Claude Code CLI inside a Docker container as the 3rd reviewer in rival megareview (alongside Codex and Gemini on host).

## Architecture

```
Host: rival binary
├── Codex CLI (native, host)
├── Gemini CLI (native, host)
└── Claude CLI (Docker container)
    ├── workdir mounted as /workspace
    └── OAuth token passed via env var
```

## Setup

### 1. Build the image

Happens automatically on first run, or manually:

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

Image is ~200MB. Runs as non-root user `claude` (required — Claude CLI refuses `--dangerously-skip-permissions` as root).

### 2. Authenticate

Start a temporary container and run interactive login:

```bash
docker run -d --name rival-claude-login \
  --user claude \
  --entrypoint sh rival-claude -c "sleep 3600"

docker exec -it rival-claude-login claude login
```

This prints an auth URL. Open it in your browser, authorize, and paste the `localhost:...` redirect URL back.

Extract the OAuth token:

```bash
docker exec rival-claude-login cat /home/claude/.claude/.credentials.json
# grab the accessToken field (starts with sk-ant-oat01-...)
```

Clean up:

```bash
docker rm -f rival-claude-login
```

### 3. Configure rival

Export the token:

```bash
export RIVAL_CLAUDE_TOKEN=sk-ant-oat01-YOUR-TOKEN-HERE
```

Enable docker mode in `~/.rival/config.yaml`:

```yaml
claude:
  mode: docker
```

### 4. Run

```bash
# Single Claude review
rival command claude --workdir /path/to/project 'review'

# Full megareview (Codex + Gemini + Claude)
rival command megareview --workdir /path/to/project
```

## How it works

1. `rival` detects `claude.mode: docker` in config
2. `RunClaudeDocker()` runs `docker run --rm -i` with:
   - `-v <workdir>:/workspace` — mounts project dir
   - `-e ANTHROPIC_AUTH_TOKEN=<token>` — passes OAuth token
   - Claude CLI flags: `--model`, `--effort`, `--output-format text`, `--dangerously-skip-permissions`
3. Prompt is piped to stdin, stdout is captured to session log

## Gotchas

- **OAuth tokens expire** — re-run `claude login` in a temp container when you get 401s
- **Non-root required** — the Dockerfile creates a `claude` user; running as root causes `--dangerously-skip-permissions` to fail
- **Token env var** is `RIVAL_CLAUDE_TOKEN` (not `ANTHROPIC_API_KEY`)
- **Native mode** (default, no config or `claude.mode: native`) runs `claude` binary directly from host, no Docker, no token env var needed
- **Config location**: the embedded Dockerfile is in `rival/internal/executor/claude_docker.go`, not a standalone file
