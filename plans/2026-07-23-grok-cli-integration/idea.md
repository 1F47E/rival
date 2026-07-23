# Idea — Grok CLI as a rival provider

**Date:** 2026-07-23
**Status:** done

Integrate the xAI Grok CLI (`grok`, installed at ~/.local/bin/grok, v0.2.111) into rival the same way as the other provider CLIs (codex, antigravity, opencode/k3, claude/fable): `rival command grok`, `rival run grok`, a `/rival-grok` skill (async detach + wait pattern), session tracking with its own cli label/icon, review-mode read-only sandboxing, and likely a megareview selector.

Why now: user wants Grok as another independent reviewer/runner alongside the existing roster.

First-guess scope: new executor `internal/executor/grok.go` (headless `grok -p`/`--prompt-file`, `--output-format`, `--effort`, `--sandbox read-only` for reviews, `--yolo` for run mode), config model `grok-4.5`, cmd wiring, skill, dashboard/web labels, tests. Gotcha: grok does NOT read the prompt from stdin — needs `--prompt-file` or argv, unlike the stdin-piping subprocess pattern.
