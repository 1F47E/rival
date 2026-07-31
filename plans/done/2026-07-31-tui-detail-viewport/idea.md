# Idea: TUI detail view — fix garble, scrollable logs, bubbletea v2

**Date:** 2026-07-31
**Status:** done

User hit the rival TUI detail view (enter on a session) rendering garbage: stale list rows bleeding through log content (screenshot evidence). Root cause found: `wrapLogLines` wraps raw log content by rune count — tabs/ANSI/`\r` make rendered lines wider than the terminal, desyncing bubbletea's line-diff repaint.

User wants: (1) the view not broken/ugly, (2) scrollable agent output in detail view (live-follow optional; it reads `~/.rival/sessions/<id>.log`), (3) "maybe update bubble tea to fresh one" → migrate TUI to the stable v2 ecosystem (bubbletea v2, bubbles v2 viewport, lipgloss v2).

Scope guess: all inside `rival/internal/dashboard/` + `cmd/tui.go` + go.mod. Full auto, spec route.
