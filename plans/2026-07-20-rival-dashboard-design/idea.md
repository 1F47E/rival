# Rival web dashboard — design review follow-ups (main page + details drawer)

**Date:** 2026-07-20
**Status:** pending review

Live audit of the rebuilt server dashboard (commit `3dc2b8e`) against the real
~1,744-session history, measured programmatically (computed styles, WCAG
contrast math, DOM scroll state) — no eyeballing.

Headline findings:

- **Details drawer logs open on the oldest lines of the tail** and the 3s live
  refresh resets scroll — the verdict is always at the bottom, so every user
  manually scrolls every card, every time. Worst functional flaw.
- **Group meta contradicts itself**: "Status: failed" next to "Exit code: 0"
  (the primary member's), raw exit `-1`, error row suppressed for all groups.
- **Model column duplicates Reviewer column** for ~95% of rows (solo sessions)
  because the API rewrites both `cli` and `model` to the same label — 347px of
  prime table width showing nothing, and icon mapping keyed on label strings
  misses historical retired-model sessions (`retired-model` → `·`).
- **Small text fails WCAG AA**: `--subtle #5f697c` measures 3.41–3.58:1
  (needs 4.5:1) on the smallest text (8–10px labels, headers, tags).
- **Mobile**: 1100px-min table in a 349px viewport — Workdir and Prompt, the
  only content columns, sit off-screen right.
- Polish: mega icon `❯❯❯❯` noise, favicon 404 on every load, judge card
  indistinguishable from reviewer card, full UUID in drawer title, elapsed
  ticker runs in hidden tabs.

TUI explicitly out of scope (user call, 2026-07-20).

Details → `spec.md`.
