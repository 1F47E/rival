# TUI detail view: sanitized scrollable logs + bubbletea v2 migration

**Date:** 2026-07-31
**Scope:** rival/ (Go module github.com/1F47E/rival)
**Status:** shipped (reconciled with as-built 2026-07-31)

## As-built notes

Shipped on branch `feat/tui-detail-viewport` in four commits: `43ce625` sanitizer, `8a64a4a` v2 migration, `bfaeade` viewport, `d9cb61a` server sanitization.

Two scope additions the spec did not anticipate, both discovered by tests the spec did call for:

1. **Pre-existing list-view overflow (P3).** The frame width invariant caught `calcColumns` in `session_list.go`: fixed columns consume 75 cells plus a 10-cell floor, so rows rendered **85 cells wide in an 80-column terminal** — the same hard-wrap corruption this work targeted, already on master in the list view. Fixed with a `fitWidth` display-width clip on header, rows and the load-more line. Not a `calcColumns` redesign; narrow terminals clip rather than drop columns.
2. **Server-side log sanitization (P4, new stage).** An audit of `internal/server` found the same *content* problems on the web path: `readLogTail` seeked to a raw byte offset (splitting runes and ANSI escapes), raw escapes/CR frames reached the browser, and — the real one — `replaceConcreteModelIDs` is a plain `ReplaceAll`, so ANSI interleaved in a model id defeats redaction and the internal id renders legibly (the ESC is invisible in a browser). Fixed by sanitizing *before* redacting and aligning the tail to a line boundary.

Consequently the sanitizer lives in a new shared package **`internal/logfmt`**, not in `internal/dashboard` as the file table below says. Tab expansion is a separate exported step (`ExpandTabs`): terminals need it before wrapping, the web page has CSS `tab-size`. Note that tab is `0x09` and thus below the control-char threshold — it needs an explicit whitelist in the filter or both paths silently eat tabs.

The web frontend was audited and needed **no** work on: HTML escaping (no `innerHTML` anywhere; all writes via `.textContent`), detail completeness (it shows more fields than the TUI), and group coherence. It has no equivalent of the TUI's width bug — CSS `pre-wrap` handles wrapping.

## Problem(s)

1. **Detail view renders garbage.** `wrapLogLines` (`rival/internal/dashboard/detail_view.go:313-340`) reads the raw session log and wraps by `[]rune` count. Logs contain tabs (Go code snippets — 1 rune, up to 8 columns), and CLI tools can emit `\r` and ANSI escapes. Rendered lines overflow the terminal width, the terminal hard-wraps them itself, and bubbletea's line-diff repaint desyncs — stale list-view rows bleed through the detail content (screenshot-confirmed by user).
2. **Log output is not scrollable.** `renderSingleDetailView` (`detail_view.go:65-91`) shows only the tail that fits the window; everything above is unreachable. The group view (`detail_view.go:94-165`) budget-splits the window across members, giving each a few-line sliver.
3. **Stale TUI stack.** The TUI runs bubbletea v1.3.10 / lipgloss v1.1.0; the charm ecosystem's stable current line is v2 (`charm.land/bubbletea/v2` v2.0.8, `charm.land/lipgloss/v2` v2.0.5, `charm.land/bubbles/v2` v2.1.1). User asked to update. Adding the viewport from v0.x bubbles onto v1 would entrench the old line.
4. **Duplicate metadata field.** `detail_view.go:35-36` renders both "Reviewer" and "Model" with the identical `config.EngineLabel(s.CLI, s.Model)` value.

## Goals

1. Sanitize log content (ANSI/`\r`/tabs/control chars) and wrap by display width so the repaint can never desync. (P1)
2. Scrollable log pane in the detail view via `bubbles/v2` viewport; sticky tail-follow for running sessions. (P2)
3. Group detail becomes one scrollable content with all members' full logs. (P2)
4. Migrate `cmd/tui.go` + `internal/dashboard/` to bubbletea v2 / lipgloss v2 / bubbles v2 (`charm.land/*`). (P3)
5. Drop the duplicate Reviewer/Model field. (P4)

## Non-goals

- No redesign of the list view, banner, colors, or layout beyond the detail pane mechanics.
- No horizontal scrolling, search, or line numbers in the log pane (viewport v2 supports them; not now).
- No streaming/tailing infrastructure — keep the existing re-read-file-on-tick model.
- No rendering of ANSI colors from the underlying CLI logs — sanitization strips them; logs display monochrome (styled headings only).

## Migration surface (verified against upstream UPGRADE_GUIDE_V2.md files)

- Imports: `github.com/charmbracelet/bubbletea` → `charm.land/bubbletea/v2`; lipgloss → `charm.land/lipgloss/v2`; new `charm.land/bubbles/v2/viewport`.
- `Model.View()` returns `tea.View` (not `string`): build via `tea.NewView(content)`, set `v.AltScreen = true` there; `tea.WithAltScreen()` program option is removed (`cmd/tui.go:17`).
- Key handling: `case tea.KeyMsg:` → `case tea.KeyPressMsg:`; `msg.String()` matching stays; space is `"space"` (unused by us).
- lipgloss v2: `NewStyle/Bold/Foreground/Background/Render` unchanged; `Color("#hex")` now returns `color.Color` — our `styles.go` usage is compatible as-is.
- viewport v2: `viewport.New(viewport.WithWidth(w), viewport.WithHeight(h))`; `SetWidth/SetHeight/Width()/Height()` methods (fields are gone); `SetContent`, `AtBottom`, `GotoTop`, `GotoBottom`, `ScrollPercent`, `Update` remain.
- go.mod: `go 1.25.0` bump required by bubbles/v2 (local toolchain go1.26.0).
- `charmbracelet/x/ansi` (≥ v0.11.7 transitively via bubbles/v2) becomes a direct dep for `ansi.Strip`, `ansi.Hardwrap`, `ansi.StringWidth`.

## Detail view pipeline

```
~/.rival/sessions/<id>.log
  → os.ReadFile                      (whole file, as today)
  → config.PublicRuntimeLog          (existing public-name transform)
  → sanitizeLog: per line —
      keep segment after last \r → ansi.Strip → \t→4sp → drop ctrl runes (<0x20, 0x7f)
  → ansi.Hardwrap(text, width, true) (display-width-aware, CJK-safe)
  → viewport.SetContent
```

Layout: meta block (title, fields, error, prompt, "Log" heading) static on top; viewport fills the rest (`max(1, contentHeight - metaLines)`). One `syncDetailViewport(resetToBottom bool)` method owns this math, sharing a `contentHeight()` helper with `View()` so Update and View can never disagree (disagreement = the stale-frame bug class). Sticky follow: capture `AtBottom()` before `SetContent`; `resetToBottom || wasAtBottom` → `GotoBottom()`.

Group content: per member — `=== <groupLogLabel> ===` heading (+ `(FAILED)` suffix), wrapped `PublicRuntimeError` in red if failed, full sanitized log, blank separator. Budget-split logic deleted.

Keys in detail mode: unconsumed keys fall through to `viewport.Update` (default keymap: `j/k/up/down`, `pgup/b`, `pgdn/f/space`, `u/d`); `g`/`G` → `GotoTop()`/`GotoBottom()`; `p/o/x/esc/backspace/q` keep current meanings. List-mode bindings unchanged. We pre-wrap and do NOT use viewport v2's `SoftWrap` (explicit wrap is deterministic and reused by the group builder).

## File-level changes

| File | Change |
|---|---|
| `rival/go.mod` | `go 1.25.0`; replace bubbletea/lipgloss with `charm.land/*/v2`; add `charm.land/bubbles/v2`, direct `charmbracelet/x/ansi`. |
| `rival/cmd/tui.go` | Drop `tea.WithAltScreen()` option (moves into `View()`); v2 import path. |
| `rival/internal/dashboard/logview.go` (new) | `sanitizeLogLine`, `sanitizeLog`, rewritten `wrapLogLines` (Hardwrap), `buildLogContent`, `buildGroupLogContent`. |
| `rival/internal/dashboard/detail_view.go` | Delete old `wrapLogLines`, `groupLogLines`, tail/budget math; new `renderDetailMeta` (single+group, meta only, clamp ≤ height-3); drop duplicate Reviewer field. |
| `rival/internal/dashboard/model.go` | v2 Model interface (`View() tea.View` with `AltScreen=true`); `logView viewport.Model` field; `syncDetailViewport` + `contentHeight()`; key routing (`tea.KeyPressMsg`, viewport fall-through, g/G); sync calls on enter/p/resize/tick/SessionEvent. |
| `rival/internal/dashboard/session_list.go` | No behavior change; only touched if v2 compile requires it. |
| `rival/internal/dashboard/styles.go` | v2 lipgloss import; styles otherwise unchanged. |
| `rival/internal/dashboard/logview_test.go` (new) | Sanitizer + display-width wrap tests. |
| `rival/internal/dashboard/detail_view_test.go` | Retarget to `renderDetailMeta`/`buildGroupLogContent`; keep tiny-terminal test; add single-"Model"-field regression; sticky-bottom model test. |

## Tests

- **Unit — `logview_test.go`** (table-driven, stdlib): tab→4 spaces; `"10%\r99%"` → `"99%"`; CSI (`\x1b[31m`) stripped; OSC BEL- and ST-terminated stripped; `\x08`/`\x00` dropped; `\n` preserved; plain text unchanged. Wrap test: temp log with tabbed Go code, `strings.Repeat("あ", 50)`, ANSI overlong line → every output line `ansi.StringWidth(line) <= width` (direct regression for Problem 1).
- **Unit — `detail_view_test.go`**: group content contains every member's heading and body (replaces budget test); "Model" field appears exactly once; tiny-terminal (`WindowSizeMsg{80,1}`) doesn't panic, viewport height ≥ 1.
- **Model — sticky-bottom**: detail mode + temp log; append + tick → `AtBottom()`; scroll up, append + tick → `!AtBottom()` with offset held.
- **Manual**: `rival tui` on real sessions — codex log with Go tabs renders clean; scroll long log; megareview group fully reachable; live run follows tail; resize mid-detail.

## Failure modes & decisions

| Failure / decision point | Behaviour |
|---|---|
| Log file unreadable/empty | Existing behavior kept: `(empty log)` placeholder. |
| Very large log | Read in full each refresh, as today; not optimized until a real case shows lag. `o` opens the full log externally regardless. |
| Terminal too small (height ≤ meta) | Meta clamped to height-3; viewport floor 1 line; no panic. |
| Content shrinks while scrolled | viewport `SetContent` clamps offset; `AtBottom` semantics preserved. |
| Completed session (no tick) | Log doesn't refresh — fine, it no longer grows; `SessionEvent` covers status flips. |
| Ambiguous-width runes / emoji ZWJ | May be off by one cell in exotic terminals; accepted (strictly better than today). |
| bubbles v2.x needs bubbletea ≥ v2.0.7 | Pin all three `charm.land` modules together in one commit. |

## Out of scope

- List view UX changes, theming, mouse support.
- Log search/filter, horizontal scroll, ANSI color passthrough.
- Any non-TUI rival code (review runner, server, config).

## Rollout

- **P1** — `logview.go` + tests on the current v1 stack (pure functions; fixes the garble on its own).
- **P2** — v2 migration (go.mod, imports, `View() tea.View`, `KeyPressMsg`) with the TUI compiling and behaving as before.
- **P3** — viewport wiring: `syncDetailViewport`, key routing, group content, meta refactor + dup-field fix, test retargets.

Each phase = one commit, gated on clean rival-fable review + green `go build ./... && go test ./...`.
