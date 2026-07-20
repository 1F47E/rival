# Runtime and model reference

This document describes Rival v3.23.0's native command surface, model roster,
authentication inputs, and effort resolution. Slash-command skills delegate to
these same commands and runtimes.

## Code-review roster

`rival review [scope]` runs a multi-model review and merges successful results
through a consilium judge.

- The default roster is **Sol + DeepSeek V4 Pro**.
- `--model` replaces the complete roster for that invocation; it does not add
  to the defaults.
- Accepted selectors are `sol`, `deepseek-v4-pro` (`deepseek` or
  `deepseek-pro`), and `kimi-k3` (`k3`). Selectors may be comma-separated or
  repeated.
- Kimi K3 is opt-in and always runs at its provider's only supported reasoning
  level, `max`.
- An unavailable selected runtime is reported as skipped. The review can
  continue when at least one selected model succeeds.

Examples:

```bash
rival review
rival review src/api/
rival review --model sol src/api/
rival review --model deepseek,k3 --effort ultra src/api/
```

Opus and Fable are direct-prompt and plan-review models; they are not members of
the default code-review roster.

## Native commands

Use `--prompt-stdin` for an arbitrary direct prompt and `--review` for the
single-model review template:

```bash
printf '%s\n' 'explain the auth flow' |
  rival run sol --prompt-stdin --workdir .
rival run sol --review src/api/ --workdir .

printf '%s\n' 'inspect this project' |
  rival run k3 --prompt-stdin --workdir .
rival run k3 --review src/api/ --workdir .

printf '%s\n' 'explain the auth flow' |
  rival run opus --prompt-stdin --workdir .
rival run opus --review src/api/ --workdir .
```

Fable's skill-facing command reads its argument grammar from stdin:

```bash
printf '%s\n' 'review src/api/' |
  rival command fable --workdir .
printf '%s\n' '-re high review src/api/' |
  rival command fable --workdir .
```

Native plan review accepts one file path on stdin. An omitted effort resolves
separately for each selected model:

```bash
printf '%s\n' 'docs/plan.md' |
  rival command plan --model sol,fable --workdir .
printf '%s\n' 'docs/plan.md' |
  rival command plan --model fable --effort high --workdir .
```

Operational views are available through `rival tui`, `rival sessions`, and
`rival server --port 3333`. The web server listens on `127.0.0.1` and tries up
to ten subsequent ports when the requested port is occupied.

## Authentication

Rival launches installed provider CLIs; it does not replace their accounts.

| Model | Runtime and required authentication |
|---|---|
| Sol | Codex CLI. Run `codex login` for browser-based ChatGPT authentication (preferred), or pipe an OpenAI API key to `codex login --with-api-key`. `codex login status` must succeed. |
| DeepSeek V4 Pro | OpenCode plus an OpenCode Zen key exported as `RIVAL_OPENCODE_API_KEY`. Rival injects the key only into the child provider configuration. |
| Kimi K3 | OpenCode plus `MOONSHOT_API_KEY`. Export it or place it in a gitignored project `.env`; Rival searches upward from the workdir. |
| Opus and Fable, native | Claude Code CLI. Subscription login is the default. To opt into API billing, set both `RIVAL_CLAUDE_AUTH=api` and a funded `ANTHROPIC_API_KEY`. |
| Opus and Fable, Docker fallback | `RIVAL_CLAUDE_TOKEN` containing the OAuth access token extracted by the flow in [Opus and Fable in Docker](opus-fable-docker-setup.md). |

For API-key-based Sol authentication, let Codex store the credential and then
remove it from the immediate shell:

```bash
export OPENAI_API_KEY='your-key'
printenv OPENAI_API_KEY | codex login --with-api-key
unset OPENAI_API_KEY
codex login status
```

For native Opus/Fable runs, Rival strips inherited Anthropic key variables in
the default subscription mode so an unrelated shell variable cannot silently
switch billing to API credits. Docker is selected only when the `claude`
executable is not on `PATH`.

Never commit provider keys or OAuth tokens. A project `.env` used for K3 must be
listed in `.gitignore`.

## Per-model effort defaults

Configure stable model labels in `~/.rival/config.yaml`:

```yaml
efforts:
  sol: high
  deepseek-v4-pro: high
  kimi-k3: max
  opus: xhigh
  fable: medium
```

Effort precedence is:

1. an explicit invocation value (`--effort` or skill `-re`);
2. the matching entry in `~/.rival/config.yaml`;
3. that command's built-in fallback.

Non-K3 configuration values may be `low`, `medium`, `high`, `xhigh`, or
`ultra`. Individual command help may expose a smaller relevant subset. Kimi K3
must be `max`; an invocation-level effort is normalized to `max`.

The general built-in defaults are Sol `high`, DeepSeek V4 Pro `high`, Opus
`xhigh`, Fable `medium`, and Kimi K3 `max`. Plan review preserves its
surface-specific fallbacks: Sol is `high`, Fable alone is `low`, and a native
Sol/Fable pair is `high` for both. The installed paired plan skills explicitly
request `ultra`, so that explicit skill value wins over configured defaults.

Invalid model labels or effort values in `~/.rival/config.yaml` stop the command
before sessions or queue entries are created.
