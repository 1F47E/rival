# Idea — rival antislop

**Status:** captured — 2026-08-01

Claude Code's built-in `/simplify` skill does a quality-only pass (reuse,
simplification, efficiency, altitude) on changed code and applies the fixes —
but only the session's own Claude model can run it. We want the same pass (and
a harder one) runnable by rival's independent reviewer models (Sol first,
Fable optionally), so the model that wrote the code isn't the only one judging
its quality.

Two separate targets:

- **Code** — the changed files (gitscope auto-detect) or an explicit scope.
- **Plans** — a plan/spec markdown file, slotting into the existing workflow
  gate "antislop the plan v1.0 before any reviewer sees it".

Beyond `/simplify`'s four angles, the pass battles AI slop and
over-engineering explicitly:

- no backward-compatibility hoarding (kill shims/legacy paths unless a named
  external consumer needs them)
- don't reinvent what a well-established library already does
- enforce DRY
- the five highest-signal slop signatures we rated 8+/10: comment slop,
  silent-fallback slop, wrapper/pass-through slop, speculative-generality
  slop (code), ceremony padding (plans)

Report-only: rival reviewers stay read-only; the main session applies cuts.
Name: **antislop** (`rival command antislop`, `/rival-antislop`,
`/rival-antislop-plan`).
