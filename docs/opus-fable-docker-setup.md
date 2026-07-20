# Opus and Fable in Docker

Run the Opus/Fable runtime through Rival inside a Docker container. Rival always
prefers a native `claude` executable on `PATH`; Docker is the automatic fallback
when that executable is unavailable.

## Architecture

```
Host: rival binary
└── Opus/Fable runtime (Docker container)
    ├── workdir mounted read-write as /workspace
    └── OAuth token passed via env var
```

The container runs Claude Code with `--dangerously-skip-permissions`. Treat an
Opus/Fable invocation as a write-capable agent: it can modify the mounted
project and run commands with the container user's access. Review changes before
committing, and do not mount a broader directory than the intended workdir.

## Setup

### 1. Build the image

Rival builds the image automatically on the first Docker-fallback run after
authentication is configured. To build the same image manually:

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

The image runs as the non-root `claude` user because the runtime refuses
`--dangerously-skip-permissions` as root.

### 2. Authenticate

Start a temporary container and run interactive login:

```bash
docker run -d --name rival-opus-fable-login \
  --user claude \
  --entrypoint sh rival-opus-fable -c "sleep 3600"

docker exec -it rival-opus-fable-login claude login
```

This prints an auth URL. Open it in your browser, authorize, and paste the
`localhost:...` redirect URL back.

Extract the OAuth token:

```bash
docker exec rival-opus-fable-login cat /home/claude/.claude/.credentials.json
# Copy the accessToken field.
```

The credentials output is secret. Do not paste it into an issue, commit it, or
leave it in a project file.

Clean up:

```bash
docker rm -f rival-opus-fable-login
```

### 3. Export the token

```bash
export RIVAL_CLAUDE_TOKEN=sk-ant-oat01-YOUR-TOKEN-HERE
```

### 4. Run

```bash
# Arbitrary Opus prompt
printf '%s\n' 'explain the auth flow' |
  rival run opus --prompt-stdin --workdir /path/to/project

# Single-model Opus review
rival run opus --review src/api/ --workdir /path/to/project

# Fable review (the skill-facing argument grammar is read from stdin)
printf '%s\n' 'review src/api/' |
  rival command fable --workdir /path/to/project
```

The installed `/rival-fable`, `/rival-plan`, and `/rival-plan-fable` skills use
the same runtime selection and Docker fallback.

### 5. Optional effort defaults

Set per-model defaults in `~/.rival/config.yaml`:

```yaml
efforts:
  opus: xhigh
  fable: medium
```

An explicit `--effort` or skill `-re` value wins, followed by this file and then
the command-specific fallback. In the Claude runtime, Rival maps `high`,
`xhigh`, and `ultra` to Claude's `max`; `low` and `medium` stay distinct.

## How it works

1. Rival uses Docker only when the native `claude` executable is unavailable.
2. The Docker executor runs `docker run --rm -i` with:
   - `-v <workdir>:/workspace` and `-w /workspace` — mounts and selects the
     project directory
   - `-e ANTHROPIC_AUTH_TOKEN=<token>` — passes OAuth token
   - Runtime flags including `--model`, `--effort`, `--output-format text`,
     `--no-session-persistence`, and `--dangerously-skip-permissions`
3. Rival pipes the prompt to stdin and captures stdout in the session log.

## Gotchas

- **OAuth tokens expire** — repeat the temporary-container login flow after an
  authentication failure.
- **The mount is writable** — the runtime can change files under the workdir.
- **Non-root is required** — running the container as root causes
  `--dangerously-skip-permissions` to fail.
- **Docker auth is separate** — the Docker fallback requires
  `RIVAL_CLAUDE_TOKEN`, not `ANTHROPIC_API_KEY`. Native subscription/API
  selection through `RIVAL_CLAUDE_AUTH` does not configure the container.
- **Native mode wins** — if `claude` is on `PATH`, Rival uses it instead of
  Docker.
- **The Dockerfile is embedded** — its source is
  `rival/internal/executor/claude_docker.go`, not a standalone project file.
