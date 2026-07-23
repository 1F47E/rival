# Rival web dashboard — main page + details drawer design fixes

**Date:** 2026-07-20
**Scope:** /Users/kass/dev/claude-codex-plugin (Go module in `rival/`); web dashboard only — `rival/internal/server/` (`server.go`, `templates/index.html`, `server_test.go`). TUI explicitly out of scope (user call).
**Status:** pending review
**Revision:** 1 — findings measured live against the production session history (1,744 sessions / 1,319 groups) served from commit `3dc2b8e`: DOM metrics via headless Chrome, WCAG 2.x contrast math, API payload inspection. Line citations are landmarks — re-grep by symbol name before implementation.

## Interaction with in-flight work

`plans/2026-07-20-sessionview-unification` rewrites `server.go` grouping (its
P2–P3) and touches `index.html` (its Sol #6 elapsed fix). This spec's P2 touches
`publicSession`/`publicSessions` (`server/server.go:49-69,476-503`) — a disjoint
function set, but the same file; both specs touch `index.html`. **Rule: whichever
lands second rebases; do not interleave phases of the two specs.**

## Problems (all verified)

### A. Details drawer — logs open on the oldest lines, live refresh yanks scroll

The log API deliberately returns the **latest** 256 KiB tail
(`X-Rival-Log-Truncated`), but `loadMemberLog` (`index.html:1423-1462`) writes
`textContent` and never adjusts scroll. Measured on a 5-member megareview: every
card at `scrollTop: 0`, `scrollHeight` up to **106,841px** vs `clientHeight`
440px. The user lands on the *oldest* lines of the tail; the review verdict —
always at the end — requires manually scrolling every card to the bottom. Worse:
the 3s live refresh (`refreshDetailLogs`) replaces `textContent`, resetting
scroll, so a user watching a running review can never stay at the bottom.

### B. Details drawer — group meta contradicts the group status

`renderDetailMeta` (`index.html:1277-1309`) pulls Exit code / Output / Duration
from the **primary member** even for groups. Verified on a failed megareview:
drawer shows "Status: failed" next to "Exit code: 0" and "Output: 1.6 KiB · 1
lines" — the primary (sol) completed; the *consilium judge* failed. Also:

- Exit code `-1` renders raw — an internal kill/timeout sentinel, meaningless to
  a user. `137` (SIGTERM kill from the TUI) likewise deserves a label.
- The Error row is suppressed for **all** groups (`index.html:1308`,
  `!group.is_group`). A failed 1-member (degraded preflight) plan group shows no
  error in the overview; the user must open the log card to learn why.

### C. Reviewer and Model columns/meta are duplicates (API root cause)

`publicSessions` (`server/server.go:476-503`) rewrites **both** `CLI` and
`Model` to the same `config.EngineLabel`. Consequences, all verified live:

- Solo rows render identical adjacent cells — "☾kimi-k3" | "kimi-k3" —
  burning Reviewer (142px) + Model (205px) at 1440px viewport. Only group rows
  ("sol + kimi-k3") carry information in Model.
- Same duplication in the drawer: "Reviewers: sol+kimi-k3" vs "Model: sol +
  kimi-k3" — one list, two separator styles.
- The icon map (`CLI_ICONS`, `index.html:970-975`) is keyed on **label strings**,
  so any label not enumerated falls back to `·` — verified: historical
  `retired-model` solo rows get no icon. The UI cannot key icons on transport
  because the DTO erased it.
- Deliberate contract note: `model` must **not** be restored to the raw runtime
  model id — public naming (`sol`/`fable`, not internal ids) is an intentional
  privacy/normalization decision (`server/server.go:47-48,473-475`).

### D. Small text fails WCAG AA

Measured contrast of `--subtle #5f697c`: **3.41:1** on `--panel #0d111c`,
**3.58:1** on `--bg #070a12`. AA requires 4.5:1 for normal text. It is used for
the smallest text on the page: table headers (9px), stat labels (10px),
section-heading, member-facts (9px), role-tag (8px), version, page-summary,
last-updated, prompt/log notes. `#7a8398` measures 4.96:1 / 5.21:1 — passes.

### E. Mobile: content columns scroll off-screen

At 375px: `.table-scroll` client 349px vs table `min-width: 1100px` (verified).
Status/Reviewer/Model are visible; **Workdir and Prompt — the only columns with
actual content — are off-screen right.** No responsive column prioritization
exists below 700px. The drawer itself is fine (full-width, single-column meta
grid, no overflow — verified).

### F. Polish items

1. Megareview reviewer icon is `❯`.repeat(min(count,4)) (`index.html:1065`) —
   up to four identical glyphs of noise; the text already says "4-model review".
2. No favicon → 404 console error on every page load (verified).
3. Judge card name is identical to the reviewer card ("sol" twice in one
   megareview drawer, verified); only the 8px JUDGE tag disambiguates — the tag
   that also fails contrast (D).
4. Drawer title shows the full 36-char UUID at 10px; the TUI truncates to 8.
5. `setInterval(updateElapsedCells, 1000)` (`index.html:1616`) runs even in
   hidden tabs (the poll loop already checks `document.hidden`; the ticker
   doesn't).
6. `byId('detail-title').childNodes[0].nodeValue = …` (`index.html:1283`) —
   fragile positional DOM write; replace with a dedicated span.

## Goals

1. Log cards behave like a tail: open at the bottom, stay at the bottom during
   live refresh unless the user scrolled up. (A)
2. Group overview never contradicts member reality; failures explain themselves
   in the overview; no internal sentinels rendered raw. (B)
3. One source of truth for "who reviewed": API exposes `label` + `transport`;
   solo rows stop duplicating columns; icons keyed by transport with a default.
   (C)
4. All text ≥ WCAG AA 4.5:1. (D)
5. Mobile shows the content columns first. (E)
6. Zero new console errors; polish batch. (F)

## Non-goals

- TUI changes (explicitly deferred by user; parity tracked separately).
- Restoring raw runtime model ids into the public DTO (privacy decision stands).
- Restructuring endpoints (`/api/run?id=` vs path style) — cosmetic, skip.
- Server-side rendering / framework rewrite; the self-contained page stays.
- Touching grouping logic, `sessionview`, the session store format, or the
  pagination contract.
- Changing log-tail bounds (256 KiB) or prompt bounds.

## Design

### Fix A — stick-to-bottom log cards (index.html only)

Standard log-tail UX, per `.log-output` card:

- On first content write for a card: `scrollTop = scrollHeight` (land on the
  verdict).
- On every refresh write: capture `wasAtBottom = scrollTop + clientHeight >=
  scrollHeight - 40` **before** replacing `textContent`; if `wasAtBottom`, pin
  to bottom after the write; otherwise preserve the user's scroll offset
  (proportionally, since the tail window shifts under them — keep it simple:
  preserve absolute offset, clamped).
- State lives in the DOM (per-element), not in `state` — cards are rebuilt by
  `renderLogCards` on signature change, and rebuilt cards default to bottom,
  which is the desired behaviour for a newly opened/status-changed drawer.

### Fix B — honest group overview (index.html)

- For `group.is_group`: drop the Exit code, Output, and (primary's) Ended/Duration
  rows from the overview — they exist per member in each log card's facts line.
  Keep them for solo sessions. Queue position stays (it's group-relevant).
- Show the Error row when **any member** failed and `primary.error` is set —
  i.e. drop the blanket `!group.is_group` suppression for 1-member groups, and
  for multi-member groups surface the first failing member's error prefixed
  with its label ("sol: …"). Members' errors remain authoritative in cards.
- Map sentinels at render time: exit `-1` → "killed/timeout", `137` → "killed",
  others raw. (Server keeps sending numbers; this is presentation-only.)

### Fix C — label/transport split (server + index.html)

Additive, no breaking change:

```go
// publicSession gains one field:
Transport string `json:"transport"` // raw CLI id: codex|claude|opencode
```

`cli` and `model` continue to carry the public label (privacy contract
unchanged). `sessionGroup` gains the same field for the primary member.

UI consequences:

- `CLI_ICONS` keyed by **transport** (`codex`→◈, `claude`→⬡,
  `opencode`→❯) with a documented default glyph — no more per-label entries,
  no more `·` for unmapped historical sessions.
- Solo row: Reviewer cell = transport icon + label (unchanged visually);
  **Model cell renders the mode** (`review`/`plan`/`docker`) instead of the
  duplicated label. Group rows: Model cell keeps the "a + b + c" list.
- Drawer: the "Reviewers" and "Model" meta rows collapse into one "Reviewers"
  row for groups (drop the separator-variant duplicate); solo keeps one
  Reviewer row and gains Mode (already present).

### Fix D — contrast (index.html)

- `--subtle: #5f697c` → `#7a8398` (4.96:1 on panel, 5.21:1 on bg — AA pass
  with margin, still visually quieter than `--muted #8c96aa` at 6.34:1).
- Bump the two tiniest sizes: `th` 9px→10px, `.role-tag` 8px→9px. Everything
  else already passes (td 6.34:1, prompt-preview 4.82:1, badges 6.85–10.68:1).

### Fix E — mobile column priority (index.html)

Below 700px: hide Effort and Workdir columns (CSS `display:none` on the
`col-effort`/`col-workdir` `th`/`td` via a media-query class), leaving
Status/Reviewer/Model/Time/Prompt — Prompt finally visible without horizontal
scrolling for the common case; the table still scrolls when it must. No markup
restructure, no card layout rewrite (drawer already handles detail needs).

### Fix F — polish batch (index.html)

1. Mega icon → single `❯` (count already in the "N-model review" text).
2. Inline favicon: `<link rel="icon" href="data:image/svg+xml,…">` — a simple
   `❯` glyph on the violet brand color; kills the 404.
3. Judge/log-card name includes the role: `sol · judge` (name span), keeping
   the JUDGE tag.
4. Drawer title: short id (first 8 chars, matching TUI) with `title=` full id
   and click-to-copy.
5. Gate the 1s elapsed ticker on `!document.hidden` (start/stop on
   `visibilitychange`, which already exists for the poll).
6. Give the drawer title its own `<span id="detail-title-text">`; stop writing
   `childNodes[0]`.

## File-level changes

| File | Change |
|---|---|
| `rival/internal/server/templates/index.html` | Fixes A, B, D, E, F + the UI half of C. All CSS/JS is inline in this one file by design. |
| `rival/internal/server/server.go` | Fix C only: add `Transport` to `publicSession` and `sessionGroup`; populate in `publicSessions`/`groupSessions`. No other handler change. |
| `rival/internal/server/server_test.go` | Assert `transport` is present and equals the raw CLI id; extend the payload-privacy test to assert `transport` contains no internal model id; assert existing fields unchanged. |

## Tests

**Unit — `server_test.go`.** `transport` present on sessions and groups, raw
CLI value, no leakage of internal model ids through the new field; existing
contract (label fields, pagination, stats) untouched — current tests must pass
unmodified.

**Manual verification matrix (no JS test harness exists — keep it honest):**

- Fix A: open a completed multi-member group → every log card starts at the
  bottom (verdict visible). Open a **running** session → card stays pinned to
  bottom across 3s refreshes. Scroll a card up → refresh does not move it.
  Close/reopen drawer → cards at bottom again.
- Fix B: failed megareview (judge failed, primary exit 0) → overview shows
  Status failed, **no** Exit code/Output rows, Error row names the failing
  member. Failed 1-member plan group → Error row visible. A `-1`-exit session
  → "killed/timeout", not "-1".
- Fix C: solo rows show icon + label in Reviewer and mode in Model — no
  duplicate text. A `retired-model` solo row shows the transport glyph, not
  `·`. Group drawer has one Reviewers row.
- Fix D: spot-compute `--subtle` on panel/bg ≥ 4.5:1 (one-liner in spec
  history; re-run after token change).
- Fix E: 375px viewport → Prompt visible without horizontal scroll; drawer
  still full-width, no overflow; 1440px layout unchanged.
- Fix F: no favicon 404 in console; title shows short id, click copies full.

**Regression.** Full Go suite (`go test ./...` in `rival/`) + `go build`.
Server behaviour other than the additive field must be byte-identical — the
existing pagination/privacy/log-tail tests pin that.

## Failure modes & decisions

| Failure / question | Behaviour |
|---|---|
| User scrolled up in a long log during refresh | `wasAtBottom` check preserves their position; only bottom-pinned users follow the tail. |
| Log tail window slides (256 KiB) while pinned | Accept: pinned users always see the newest chunk; absolute offsets are meaningless across tail slides by definition. |
| 1-member "group" (degraded preflight) | Treated as group for structure, but Error row shown — it is the only place the failure surfaces in the overview. |
| Adding `transport` leaks internal naming | Transport is the CLI id only (`codex`, `claude`, …) — already public via the CLI itself; model ids stay normalized. Privacy test extended. |
| Solo Model cell shows mode — information loss? | No: for solo rows Model duplicated Reviewer bit-for-bit. Docker mode previously surfaced as `/dk` suffix on Reviewer; it now moves to the mode cell, same information. |
| Contrast token change alters look | `#7a8398` is one step toward `--muted`; hierarchy (subtle < muted < text) preserved — verified ratios 4.96 < 6.34 < 15.65. |
| Conflict with sessionview-unification spec | Disjoint function sets in `server.go`; both touch `index.html`. Whichever lands second rebases. |

## Out of scope

TUI parity work; `/api/run` endpoint shape; log/prompt size bounds; any
`internal/session`, `internal/sessionview`, or executor changes; new dashboard
features (filtering, search, kill-from-web, SSE) — file follow-ups if wanted.

## Rollout

- **P1 — drawer correctness (index.html only):** Fix A (stick-to-bottom), Fix B
  (honest group meta, sentinel mapping, error row), F3 (judge naming), F4
  (short id), F6 (title span). One coherent drawer batch; verified with the
  manual matrix against real history.
- **P2 — API + columns:** Fix C (server field + tests, then UI dedup + icon
  map). Sequence **after or before** sessionview-unification as a whole — not
  interleaved (see interaction note).
- **P3 — visual:** Fix D (contrast + sizes), Fix E (mobile columns), F1, F2,
  F5. Pure CSS/JS; ends with the contrast spot-check and a desktop+mobile
  console-error-free pass.

Each phase is independently shippable and leaves the dashboard working; P1 has
the highest user-visible payoff (log scroll) for the least risk.
