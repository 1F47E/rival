# Plan v1.0 — TUI detail view: sanitized scrollable logs + bubbletea v2

**Date:** 2026-07-31
**Spec:** [spec.md](spec.md)
**Status:** draft (pre-review)

Three commits, in order. P1 fixes the user-visible garble on the existing stack; P2 migrates; P3 adds scrolling. Each stage ends green on `go build ./... && go test ./...` and passes a rival-fable review before the next starts.

**Verified API facts** (checked in module cache, not assumed):
- `charm.land/bubbles/v2@v2.1.1/viewport`: `New(opts ...Option) Model`, `WithWidth(int)`, `WithHeight(int)`, `SetWidth/SetHeight/Width()/Height()`, `SetContent(string)`, `AtBottom() bool`, `GotoTop()/GotoBottom() []string`, `SetYOffset(int)/YOffset() int`, `ScrollPercent() float64`, `Update(tea.Msg) (Model, tea.Cmd)`, `View() string`.
- Default keymap binds `pgdown/space/f`, `pgup/b`, `u/ctrl+u`, `d/ctrl+d`, `up/k`, `down/j`, `left/h`, `right/l`.
- `charmbracelet/x/ansi`: `Strip(string) string`, `Hardwrap(string, int, bool) string`, `StringWidth(string) int`.
- Latest: bubbletea v2.0.8, lipgloss v2.0.5, bubbles v2.1.1 (module paths `charm.land/*`). bubbles/v2 requires `go 1.25.0`.

---

## P1 — Sanitizer (fixes the garble, no dependency changes)

### Task 1.1 — `rival/internal/dashboard/logview.go` (new)

```go
func sanitizeLogLine(line string) string
```
In order: (1) keep only the segment after the **last** `\r` — collapses progress-bar rewrites instead of concatenating frames; (2) `ansi.Strip(line)`; (3) expand `\t` → 4 spaces; (4) drop runes `< 0x20` or `== 0x7f`.

```go
func sanitizeLog(raw string) string   // split on \n, sanitize each line, rejoin
func wrapLogLines(s *session.Session, wrapWidth int) []string  // moved here from detail_view.go
```
`wrapLogLines`: `os.ReadFile` → `config.PublicRuntimeLog(s.CLI, s.Model, ...)` → `sanitizeLog` → `ansi.Hardwrap(text, wrapWidth, true)` → split on `\n`. Preserve existing contract: nil on read error, nil on empty. Guard `wrapWidth <= 0` (return sanitized unwrapped lines).

Read the whole file, as today — no size cap. A tail cap was considered and cut: the current code already reads the full log on every render with no reported problem, and the cap buys an unmeasured win for a partial-line edge case. Revisit only if a real multi-MB log shows lag.

Note `x/ansi` is already an indirect dep; importing it here promotes it to direct in go.mod.

### Task 1.2 — delete the old `wrapLogLines` from `detail_view.go:313-340`

Everything else in that file stays untouched this stage; callers (`renderSingleDetailView`, `groupLogLines`) keep compiling against the new signature (identical).

### Task 1.3 — `rival/internal/dashboard/logview_test.go` (new)

Table-driven, stdlib `testing`, `t.Run` subtests:
- `TestSanitizeLog`: tab→4 spaces; `"10%\r99%"` → `"99%"`; CSI `"\x1b[31mred\x1b[0m"` → `"red"`; OSC BEL `"\x1b]0;title\x07text"` → `"text"`; OSC ST `"\x1b]8;;url\x1b\\link"` → `"link"`; `\x08`/`\x00` dropped; embedded `\n` preserved across lines; plain text unchanged.
- `TestWrapLogLinesDisplayWidth`: write a temp log (`t.TempDir()`) containing tabbed Go code, `strings.Repeat("あ", 50)`, and an ANSI-colored overlong line; build a `session.Session` pointing at it; assert **every** returned line satisfies `ansi.StringWidth(line) <= width`. This is the regression test for Problem 1.

**Gate:** `go test ./internal/dashboard/`, then `/rival-fable`. Commit: `fix(tui): sanitize log content and wrap by display width`.

---

## P2 — bubbletea v2 migration (behavior-preserving)

### Task 2.1 — `rival/go.mod`

`go 1.25.0`; drop `github.com/charmbracelet/bubbletea` + `lipgloss`; add `charm.land/bubbletea/v2@v2.0.8`, `charm.land/lipgloss/v2@v2.0.5`, `charm.land/bubbles/v2@v2.1.1`; keep `charmbracelet/x/ansi` direct. `go mod tidy`.

### Task 2.2 — import rewrite

`cmd/tui.go`, `internal/dashboard/{model,styles,detail_view,session_list}.go` → `charm.land/*/v2` paths. `styles.go` needs no other change: `NewStyle/Bold/Foreground/Background/Italic/Render` and `Color("#hex")` are source-compatible in lipgloss v2 (Color returns `color.Color`; we only pass it to `Foreground`/`Background`).

### Task 2.3 — `cmd/tui.go`

Remove `tea.WithAltScreen()` — the option no longer exists; alt-screen moves to the View. `tea.NewProgram(m)`, `p.Run()` unchanged.

### Task 2.4 — `internal/dashboard/model.go` v2 conformance

- `func (m Model) View() tea.View`: build the same string as today, then `v := tea.NewView(content); v.AltScreen = true; return v`.
- `case tea.KeyMsg:` → `case tea.KeyPressMsg:`. All `msg.String()` matches stay as-is (no `" "`/space binding in use).
- `lipgloss.JoinVertical` / `lipgloss.Width` usage in `model.go:423,500` — verify still exported in v2 during the port; if `Width` moved, substitute `ansi.StringWidth`.
- Everything else (Init, Update flow, tick, watcher, `clipLines`) unchanged.

**Gate:** `go build ./... && go test ./...`; manual `rival tui` smoke — list renders, enter/esc, q quits, alt-screen restores the terminal. Then `/rival-fable`. Commit: `chore(tui): migrate to bubbletea/lipgloss/bubbles v2`.

---

## P3 — Scrollable viewport + meta refactor

### Task 3.1 — content builders in `logview.go`

```go
func buildLogContent(s *session.Session, width int) string
func buildGroupLogContent(item *displayItem, width int) string
```
Single: wrapped lines joined, or `labelStyle.Render("(empty log)")`. Group: per member — `titleStyle.Render("=== " + groupLogLabel(sess) + " ===")` (keep the existing `(FAILED)` suffix), then wrapped `config.PublicRuntimeError` in `failedStyle` when failed, then the member's wrapped log or `(empty log)`, then a blank separator. No line budgets.

### Task 3.2 — `detail_view.go` meta/log split

- New `renderDetailMeta(item *displayItem, width, height int, promptExpanded bool) string` — same single/group dispatch as today's `renderDetailView`, returning **only** meta (title, fields, error, prompt) plus the `titleStyle.Render("Log")` heading. Clamp meta to `height-3` lines so the viewport always gets ≥3; when clamped mid-prompt keep the existing `"... (p to expand)"` hint.
- Delete `renderDetailView`, `renderSingleDetailView`'s log tail math, `renderGroupDetailView`'s `minLogLines`/`maxMetaLines` budget logic, and `groupLogLines`. Keep `groupLogLabel` (used by `buildGroupLogContent` and `createPublicGroupLogView`), `renderErrorSection`, `renderPromptSection`, `addField`, `addStyledField`, `wrapText`, `renderedLines`.
- Drop the duplicate `Reviewer` field at `detail_view.go:35-36`; keep one `Model` row.

### Task 3.3 — `model.go` viewport wiring

- Struct: add `logView viewport.Model`. `New()` initializes `viewport.New(viewport.WithWidth(0), viewport.WithHeight(0))`.
- Extract `func (m Model) contentHeight() int` from `View()`'s existing `m.height - headerLines - 1` math; **both** `View()` and `syncDetailViewport` must call it — divergence here is precisely what produces stale-frame bleed.
- New `func (m *Model) syncDetailViewport(resetToBottom bool)`:
  1. bail if not in detail mode or no selected item;
  2. `meta := renderDetailMeta(item, m.width, m.contentHeight(), m.promptExpanded)`; `metaLines := strings.Count(meta, "\n") + 1`;
  3. `m.logView.SetWidth(m.width)`; `m.logView.SetHeight(max(1, m.contentHeight()-metaLines))`;
  4. `wasAtBottom := m.logView.AtBottom()`; build content (group vs single); `SetContent`;
  5. `if resetToBottom || wasAtBottom { m.logView.GotoBottom() }`.
- Update routing (`Model.Update` must take a pointer receiver or reassign `m` — it currently returns `m` by value; keep that style and call `(&m).syncDetailViewport(...)`):
  - `"enter"` → after `m.viewMode = viewDetail`, `syncDetailViewport(true)` (open at tail: live output and final verdicts both live at the end).
  - `"p"` → after toggle, `syncDetailViewport(false)` (meta height changed).
  - `"g"`/`"G"` → detail mode: `m.logView.GotoTop()` / `GotoBottom()`; list mode unchanged.
  - `"esc"/"backspace"/"o"/"x"/"q"` unchanged.
  - Default: in detail mode, forward unhandled keys to `m.logView, cmd = m.logView.Update(msg)` and return that cmd — gives `j/k/up/down/pgup/pgdn/space/u/d` free.
  - `tea.WindowSizeMsg`, `SessionEvent`, `tickMsg` → if in detail mode, `syncDetailViewport(false)` before returning.
- `View()` detail branch: `content = clipLines(meta+"\n"+m.logView.View(), contentHeight)`; **View no longer reads the log file** — Update owns content. Keep `clipLines` as a guard.
- Help bar (detail): `"  j/k: scroll  g/G: top/bottom  p: prompt  o: open log  x: stop  esc: back  q: quit"`.

### Task 3.4 — tests

- `detail_view_test.go`: retarget `TestGroupDetailReservesSpaceForEveryPlanLog` → assert `buildGroupLogContent(item, 80)` contains both member headings and both bodies (the budget invariant is gone); retarget `TestSingleDetailStillShowsItsEffort` → `renderDetailMeta`, plus assert `"Model"` appears exactly once; keep `TestDetailViewHandlesTinyTerminal`, driving it through `Update(tea.WindowSizeMsg{Width: 80, Height: 1})` then `View()` and asserting no panic and `logView.Height() >= 1`.
- New sticky-bottom model test: Model in detail mode over a temp log; `syncDetailViewport(true)`; append lines; send `tickMsg` → `AtBottom()` true. Then send `tea.KeyPressMsg` for `k` (scroll up), append, tick → `AtBottom()` false and `YOffset()` unchanged.

**Gate:** `go build ./... && go test ./...`; manual checklist below. Then `/rival-fable`. Commit: `feat(tui): scrollable log viewport in detail view`.

---

## Verification (end to end)

1. `cd rival && go build ./... && go test ./...` — green.
2. `rival tui` against real `~/.rival/sessions`: open a **codex** session whose log has Go code (tabs) — content readable, **no list rows bleeding through**.
3. Scroll a long log with `j/k`, `pgup/pgdn`, `g`/`G`; `esc` back to list; re-enter — opens at bottom.
4. Open a megareview/plan group — every member's heading + full log reachable by scrolling.
5. Live: start a review, open its detail — tail follows while at bottom; scroll up and confirm position holds as the log grows; scroll back to bottom and confirm follow resumes.
6. Resize the terminal while in detail — re-wraps, no corruption; shrink to a few rows — no panic.
7. `q` restores the terminal cleanly (alt-screen via `View.AltScreen`).

## Risks

| Risk | Mitigation |
|---|---|
| v2 migration touches every TUI file at once | P2 is behavior-preserving and separately gated; P1 already fixed the bug if P2 has to be reverted. |
| Height math drift between View and Update | Single `contentHeight()` helper used by both — mandatory, not stylistic. |
| viewport clips (never wraps) long lines | We pre-wrap to width before `SetContent`; `SoftWrap` stays off. |
| `Model.Update` value receiver vs `syncDetailViewport` pointer | Use `(&m).syncDetailViewport(...)` and return the modified `m`; covered by the sticky-bottom test. |
| lipgloss v2 removed `Renderer`/`AdaptiveColor` | `styles.go` uses neither — plain `Color("#hex")` only. |
| 1s tick re-reads + re-wraps the log | Unchanged in shape from today's per-render read; deliberately not optimized until measured. |
