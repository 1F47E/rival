# Opus and Fable in Docker

Run the Opus/Fable runtime through Rival inside a Docker container.

## Architecture

```
Host: rival binary
└── Opus/Fable runtime (Docker container)
    ├── workdir mounted as /workspace
    └── OAuth token passed via env var
```

## Setup

### 1. Build the image

Happens automatically on first run, or manually:

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

Image is ~200MB. Runs as non-root user `claude` because the runtime refuses `--dangerously-skip-permissions` as root.

### 2. Authenticate

Start a temporary container and run interactive login:

```bash
docker run -d --name rival-opus-fable-login \
  --user claude \
  --entrypoint sh rival-opus-fable -c "sleep 3600"

docker exec -it rival-opus-fable-login claude login
```

This prints an auth URL. Open it in your browser, authorize, and paste the `localhost:...` redirect URL back.

Extract the OAuth token:

```bash
docker exec rival-opus-fable-login cat /home/claude/.claude/.credentials.json
# grab the accessToken field (starts with sk-ant-oat01-...)
```

Clean up:

```bash
docker rm -f rival-opus-fable-login
```

### 3. Export the token

Export the token:

```bash
export RIVAL_CLAUDE_TOKEN=sk-ant-oat01-YOUR-TOKEN-HERE
```

### 4. Run

```bash
# Single Opus review
printf 'review\n' | rival command opus --workdir /path/to/project
```

## How it works

1. `rival` uses Docker when the native runtime executable is unavailable
2. The Docker executor runs `docker run --rm -i` with:
   - `-v <workdir>:/workspace` — mounts project dir
   - `-e ANTHROPIC_AUTH_TOKEN=<token>` — passes OAuth token
   - Runtime flags: `--model`, `--effort`, `--output-format text`, `--dangerously-skip-permissions`
3. Prompt is piped to stdin, stdout is captured to session log

## Gotchas

- **OAuth tokens expire** — re-run `claude login` in a temp container when you get 401s
- **Non-root required** — the Dockerfile creates a `claude` user; running as root causes `--dangerously-skip-permissions` to fail
- **Token env var** is `RIVAL_CLAUDE_TOKEN` (not `ANTHROPIC_API_KEY`)
- **Native mode** runs the host executable whenever it is available; Docker is the automatic fallback
- **Config location**: the embedded Dockerfile is in `rival/internal/executor/claude_docker.go`, not a standalone file
