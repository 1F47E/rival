---
description: Run OpenAI Codex CLI as a subagent (gpt-5.4, xhigh reasoning)
argument-hint: "review" or a prompt for codex to execute
---

# Codex CLI Runner

Run OpenAI Codex CLI from Claude Code. All work happens in a subagent to keep the main context clean.

## Instructions

Dispatch to the `codex:codex-runner` agent immediately. Pass the full user argument as the prompt.

**Arguments received:** $ARGUMENTS

**Determine mode from arguments:**

- If arguments start with `review` → tell the agent: "Run codex review mode. Extra args: $ARGUMENTS"
- Otherwise → tell the agent: "Run codex exec with this prompt: $ARGUMENTS"

**Launch the agent now.** Do not do any work yourself — the agent handles everything.

After the agent returns, present its output to the user. If the agent reports an error, show it clearly.
