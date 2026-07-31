# Plan v1.1 — TUI detail view: sanitized scrollable logs + bubbletea v2

**Date:** 2026-07-31
**Spec:** [spec.md](spec.md)
**Status:** approved (Sol-reviewed, v1.0 rated 4/10 → findings addressed)
**Supersedes:** [plan-v1.0.md](plan-v1.0.md)

**Sol review disposition** (10 findings: 4 high, 6 med). Fixed here: #1 tidy/dependency ordering, #2 key routing, #3 help-bar overflow, #4 sticky-bottom capture order, #5 CRLF, #6 lipgloss v1 test import, #9 CI Go pin. Deliberately not fixed: **#7** (deleting the selected session from disk mid-detail — requires manual `rm` from `~/.rival/sessions`; the nil-item guard in Task 3.3 covers the crash risk), **#8** (ANSI-colored *runtime banner* defeating `PublicRuntimeLog` prefix matching — real ordering issue, but no reviewer CLI colors its banner line; revisit if one starts), **#10** (test-strength nit — folded partially into Task 3.4's content assertion instead of a separate change).

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
In order: (1) **trim a single trailing `\r`** (CRLF line ending) — see below; (2) keep only the segment after the **last** remaining `\r` — collapses progress-bar rewrites instead of concatenating frames; (3) `ansi.Strip(line)`; (4) expand `\t` → 4 spaces; (5) drop runes `< 0x20` or `== 0x7f`.

**Sol #5 — CRLF must be handled before the last-`\r` rule.** `sanitizeLog` splits on `\n`, so every line of a CRLF log ends in `\r`. Applying "keep everything after the last `\r`" first would blank *every* line. Order is therefore: strip one trailing `\r`, and only then apply the progress-frame rule to any `\r` still embedded in the line. A line that is only a progress frame ending in `\r` (e.g. `"working...\r"`) correctly collapses to `"working..."` rather than to empty.

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
- **CRLF regression cases (Sol #5)**: `"alpha\r\nbeta\r\n"` → `"alpha\nbeta\n"` (neither line blanked); trailing-only frame `"working...\r"` → `"working..."`; combined `"10%\r99%\r\n"` → `"99%"`.
- `TestWrapLogLinesDisplayWidth`: write a temp log (`t.TempDir()`) containing tabbed Go code, `strings.Repeat("あ", 50)`, and an ANSI-colored overlong line; build a `session.Session` pointing at it; assert **every** returned line satisfies `ansi.StringWidth(line) <= width`. This is the regression test for Problem 1.

**Gate:** `go test ./internal/dashboard/`, then `/rival-fable`. Commit: `fix(tui): sanitize log content and wrap by display width`.

---

## P2 — bubbletea v2 migration (behavior-preserving)

### Task 2.1 — `rival/go.mod`

`go 1.25.0`; drop `github.com/charmbracelet/bubbletea` + `lipgloss`; add `charm.land/bubbletea/v2@v2.0.8`, `charm.land/lipgloss/v2@v2.0.5`; keep `charmbracelet/x/ansi` direct.

**Sol #1 — do NOT add `charm.land/bubbles/v2` here.** Nothing imports it until P3, so P2's `go mod tidy` would strip the requirement right back out and P3's build gate would fail on a missing module. `bubbles/v2` is added in **P3 (Task 3.3)**, where the viewport import actually appears, and `go mod tidy` runs there after the import exists.

### Task 2.2 — import rewrite

`cmd/tui.go`, `internal/dashboard/{model,styles,detail_view,session_list}.go` **and `internal/dashboard/session_list_test.go`** → `charm.land/*/v2` paths. `styles.go` needs no other change: `NewStyle/Bold/Foreground/Background/Italic/Render` and `Color("#hex")` are source-compatible in lipgloss v2 (Color returns `color.Color`; we only pass it to `Foreground`/`Background`).

**Sol #6 — the test file is a real import site.** `session_list_test.go:11` imports `github.com/charmbracelet/lipgloss` and calls `lipgloss.Width(icon)` at line 44. Omitting it would keep lipgloss **v1 in the module graph after tidy** and make the icon-width test exercise a different library than production. Rewrite it to `charm.land/lipgloss/v2` (or `ansi.StringWidth`, the same primitive production uses). **Verify:** `grep -r "github.com/charmbracelet/lipgloss" rival/` returns nothing and `go.mod` lists no v1 lipgloss.

### Task 2.3 — CI toolchain (Sol #9)

`.github/workflows/release.yml:24` pins `go-version: "1.24"`. `bubbles/v2` requires `go 1.25.0`, so releases would depend on implicit toolchain auto-download and break where that's disabled. Bump the workflow to `"1.25"` in the same commit as the go.mod bump. Check every workflow, not just release.yml: `grep -rn "go-version" .github/workflows/`.

### Task 2.4 — `cmd/tui.go`

Remove `tea.WithAltScreen()` — the option no longer exists; alt-screen moves to the View. `tea.NewProgram(m)`, `p.Run()` unchanged.

### Task 2.5 — `internal/dashboard/model.go` v2 conformance

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
- Add `charm.land/bubbles/v2@v2.1.1` and run `go mod tidy` **here** (Sol #1) — this task introduces the first real viewport import.
- New `func (m *Model) syncDetailViewport(resetToBottom bool)` — **capture-before-resize ordering is mandatory (Sol #4):**
  1. if not in detail mode, or no selected item, `m.logView.SetContent("")` and return (also covers Sol #7: no stale log under a vanished session);
  2. **`wasAtBottom := m.logView.AtBottom()` — FIRST, before any width/height change.** Shrinking the viewport (terminal resize, prompt expansion via `p`, growing metadata) pushes the old offset above the new max-bottom, so a post-resize `AtBottom()` reports false and silently kills tail-follow even though the user never scrolled;
  3. `meta := renderDetailMeta(item, m.width, m.contentHeight(), m.promptExpanded)`; `metaLines := strings.Count(meta, "\n") + 1`;
  4. `m.logView.SetWidth(m.width)`; `m.logView.SetHeight(max(1, m.contentHeight()-metaLines))`;
  5. build content (group vs single); `SetContent`;
  6. `if resetToBottom || wasAtBottom { m.logView.GotoBottom() }`.
- Update routing (`Model.Update` returns `m` by value; keep that style and call `(&m).syncDetailViewport(...)`).
  **Sol #2 — detail keys must be routed BEFORE the existing switch.** `model.go:176-188` matches `"j","down"` and `"k","up"` unconditionally; the `if m.viewMode == viewList` test lives *inside* each case, so in detail mode those cases still swallow the keypress and a `default:` branch would never see it — the advertised scroll keys would do nothing. Structure the `tea.KeyPressMsg` handler as:
  1. **first**, handle the detail-mode keys this feature owns explicitly: `esc`/`backspace` (back), `p`, `o`, `x`, `q`/`ctrl+c`, and `g`/`G` → `m.logView.GotoTop()`/`GotoBottom()`;
  2. **then**, if `m.viewMode == viewDetail`, forward *everything else* to `m.logView, cmd = m.logView.Update(msg)` and **return immediately** — never falling through to the list switch. This is what actually delivers `j/k/up/down/pgup/pgdn/space/u/d`;
  3. only if still in list mode, run the existing list switch unchanged.
  - `"enter"` (list mode) → `m.viewMode = viewDetail`, then `syncDetailViewport(true)` (open at tail: live output and final verdicts both live at the end).
  - `"p"` → after toggle, `syncDetailViewport(false)` (meta height changed — exercises the #4 ordering).
  - `tea.WindowSizeMsg`, `SessionEvent`, `tickMsg` → if in detail mode, `syncDetailViewport(false)` before returning.
- `View()` detail branch: `content = clipLines(meta+"\n"+m.logView.View(), contentHeight)`; **View no longer reads the log file** — Update owns content. Keep `clipLines` as a guard.
- **Help bar must fit the terminal (Sol #3).** The v1.0 detail string is **83 display cells** (measured) — wider than an 80-column terminal. `lipgloss.JoinVertical` pads every other row to the widest line, so an over-wide help bar drags the whole frame over-width and reintroduces exactly the hard-wrap/stale-repaint failure this work exists to kill. Render help responsively: full text when it fits `m.width`, else a short form (`"  j/k: scroll  g/G: top/bottom  esc: back  q: quit"`, 51 cells), else truncate to `m.width` by display width. Never emit a help line wider than `m.width`.

### Task 3.4 — tests

- `detail_view_test.go`: retarget `TestGroupDetailReservesSpaceForEveryPlanLog` → assert `buildGroupLogContent(item, 80)` contains both member headings and both bodies (the budget invariant is gone); retarget `TestSingleDetailStillShowsItsEffort` → `renderDetailMeta`, plus assert `"Model"` appears exactly once; keep `TestDetailViewHandlesTinyTerminal`, driving it through `Update(tea.WindowSizeMsg{Width: 80, Height: 1})` then `View()` and asserting no panic and `logView.Height() >= 1`.
- **Frame width invariant (Sol #3, the general form).** New `TestViewNeverExceedsWidth`: drive a Model through `Update` to detail mode over a log containing tabs, CJK, and ANSI; call `View()` at widths 60, 80, and 120; assert **every** line of the rendered frame satisfies `ansi.StringWidth(line) <= m.width`. Run the same assertion in list mode. This catches the help bar, the rune-count `truncate`/`truncatePath`/`wrapText` helpers, and any future over-wide row in one test — it is the direct guard on the original bug, not just on the log pane.
- **Sticky-bottom model test (Sol #4 + #10).** Enter detail **through `Update` (`enter` key), not by calling `syncDetailViewport` directly**, so the test exercises the real path. Then:
  - append unique marker lines to the temp log, send `tickMsg` → assert `AtBottom()` **and** that `logView.GetContent()` contains the new marker (per Sol #10: asserting only `AtBottom`/`YOffset` passes even if the tick never re-reads the file, so the follow assertion would be vacuous);
  - send `tea.KeyPressMsg` for `k`, append another marker, tick → `AtBottom()` false, `YOffset()` unchanged, new marker still present in content;
  - **resize while pinned to bottom**: back to bottom, then `Update(tea.WindowSizeMsg{Width: 80, Height: <smaller>})` → assert still `AtBottom()`. This is the regression test for the capture-before-resize ordering; with the v1.0 ordering it fails.
  - **prompt expansion while pinned**: at bottom, send `p` → assert still `AtBottom()`.

**Gate:** `go build ./... && go test ./...`; manual checklist below. Then `/rival-fable`. Commit: `feat(tui): scrollable log viewport in detail view`.

---

## Verification (end to end)

1. `cd rival && go build ./... && go test ./...` — green.
2. `rival tui` against real `~/.rival/sessions`: open a **codex** session whose log has Go code (tabs) — content readable, **no list rows bleeding through**.
3. Scroll a long log with `j/k`, `pgup/pgdn`, `g`/`G`; `esc` back to list; re-enter — opens at bottom.
4. Open a megareview/plan group — every member's heading + full log reachable by scrolling.
5. Live: start a review, open its detail — tail follows while at bottom; scroll up and confirm position holds as the log grows; scroll back to bottom and confirm follow resumes.
6. Resize the terminal while in detail — re-wraps, no corruption; shrink to a few rows — no panic.
7. Run the TUI in an **80-column** terminal specifically — the help bar and every frame line must fit without the terminal wrapping them (Sol #3).
8. `q` restores the terminal cleanly (alt-screen via `View.AltScreen`).
9. Dependency hygiene: `grep -r "github.com/charmbracelet/\(lipgloss\|bubbletea\)" rival/` is empty, and `go.mod` has no v1 lipgloss/bubbletea (Sol #6).
10. CI: `grep -rn "go-version" .github/workflows/` shows 1.25 everywhere; push and confirm the release workflow builds (Sol #9).

## Risks

| Risk | Mitigation |
|---|---|
| v2 migration touches every TUI file at once | P2 is behavior-preserving and separately gated; P1 already fixed the bug if P2 has to be reverted. |
| Height math drift between View and Update | Single `contentHeight()` helper used by both — mandatory, not stylistic. |
| viewport clips (never wraps) long lines | We pre-wrap to width before `SetContent`; `SoftWrap` stays off. |
| `Model.Update` value receiver vs `syncDetailViewport` pointer | Use `(&m).syncDetailViewport(...)` and return the modified `m`; covered by the sticky-bottom test. |
| lipgloss v2 removed `Renderer`/`AdaptiveColor` | `styles.go` uses neither — plain `Color("#hex")` only. |
| 1s tick re-reads + re-wraps the log | Unchanged in shape from today's per-render read; deliberately not optimized until measured. |
| Detail key routing runs before the list switch | Deliberate (Sol #2) — the existing `j/k` cases would otherwise swallow scroll keys. Detail mode must `return` after forwarding, never fall through. |
| Over-wide help bar re-breaks rendering | `TestViewNeverExceedsWidth` asserts the invariant on the whole frame at 60/80/120 columns (Sol #3). |
| Accepted-risk findings (Sol #7, #8) | Selected-session-deleted-from-disk and ANSI-colored runtime banner: both low-likelihood, neither fixed. See disposition note at top. |
