# Sessionview Unification Implementation Plan v1.0

**Date:** 2026-07-23
**Status:** reviewed
**Spec:** ./spec.md

**Goal:** Extract one in-process `internal/sessionview` package (cache + grouping logic) so the TUI and web dashboard read the same data through the same code, eliminating eight duplicated functions and a live data-loss bug.

**Architecture:** New leaf package `internal/sessionview` sits between `internal/session` (file parsing) and the two consumers. It owns the session cache (moved from `server/session_cache.go`) and one copy of each grouping function, all over `[]*session.Session`, returning a neutral `Bucket`. Each consumer keeps a thin local adapter (`Bucket → displayItem` for the TUI, `Bucket → sessionGroup` for the server) plus its own presentation (Unicode icons / sanitized DTOs).

**Tech Stack:** Go stdlib, `bubbletea`/`lipgloss` (TUI, unchanged), `net/http` (server, unchanged), stdlib `testing` (table-driven).

> For agentic workers: use superpowers:subagent-driven-development to implement task-by-task. Checkbox syntax for tracking.

## Spec drift folded in

The spec lists **seven** duplicated functions; the tree has **eight** —
`groupEffort` is duplicated too (`server/server.go:439`,
`dashboard/session_list.go:141`), identical pattern. It is included below. The
spec's "seven" should read "eight"; this is recorded here rather than silently,
per the plan smell-check. No other spec deviation.

## File map

**Create**
- `rival/internal/sessionview/cache.go` — cache moved from server (exported).
- `rival/internal/sessionview/group.go` — `Bucket`, `Group`, eight grouping fns.
- `rival/internal/sessionview/cache_test.go`
- `rival/internal/sessionview/group_test.go`

**Modify**
- `rival/internal/server/server.go` — delete 8 local copies + `groupSessions`; call `sessionview` via adapter.
- `rival/internal/server/server_test.go` — migrate call sites; values unchanged.
- `rival/internal/server/templates/index.html` — render API `elapsed` for running/queued groups.
- `rival/internal/dashboard/session_list.go` — delete 8 local copies + `groupIsPlan`/`groupKindLabel`; wrappers delegate.
- `rival/internal/dashboard/model.go` — `groupSessions` → adapter; add `hydrated`; `x` handler re-reads before mutating.
- `rival/internal/dashboard/detail_view.go` — migrate `groupIsPlan`/`groupKindLabel` call sites; render from `hydrated`.
- `rival/internal/dashboard/session_list_test.go` — group-elapsed expectations → wall-clock span.
- `rival/internal/dashboard/watcher.go` — `session.LoadAll()` → shared `sessionview.Cache.Load()`.

**Delete**
- `rival/internal/server/session_cache.go` (in Task 6, after server switches).

**Out of scope**
- `internal/session` exported API; `groupModelRank`/`SortGroupMembers`; TUI-over-HTTP; retired-provider cleanup.

## Locked interfaces

```go
// package sessionview

type Bucket struct {
    GroupID  string             // "" for a solo session
    Sessions []*session.Session // SortGroupMembers order; sessionview never mutates
}

func Group(sessions []*session.Session) []Bucket
func Status(sessions []*session.Session) string       // running>queued>failed>completed
func CLIs(sessions []*session.Session) string         // dedup, first-seen, "+" join
func Models(sessions []*session.Session) string       // dedup, first-seen, " + " join
func Effort(sessions []*session.Session) string       // primary member's effort
func Elapsed(sessions []*session.Session) string       // wall-clock span (server semantics)
func Kind(sessions []*session.Session) string          // "plan" | "megareview"
func EngineLabel(s *session.Session) string            // == config.EngineLabel(s.CLI, s.Model)

type Cache struct { /* unexported fields */ }
func New(dir string) *Cache
func (c *Cache) Load() ([]*session.Session, uint64)    // sessions, revision
func (c *Cache) Get(id string) *session.Session
```

Consumer adapters (kept local, NOT in sessionview):
- TUI: `func toDisplayItem(b sessionview.Bucket) displayItem { return displayItem{Sessions: b.Sessions} }`
- Server: existing `sessionGroup` construction, fed by `sessionview.Group` + the shared fns.

Behaviour parity anchors (unchanged from today):
- `displayItem.IsGroup()`: `len>1 || (len==1 && Sessions[0].GroupID != "")`.
- `displayItem.Primary()`: `Sessions[0]`.
- `Group` bucket order: first-appearance of `GroupID`; solo sessions keyed `solo:<ID>`.

---

## Self-test sanity check (before Task 1)

- [ ] `cd rival && go build ./... && go test ./... 2>&1 | tail -20` — baseline green. If red, STOP: the tree is dirty from other work; resolve before starting.

---

## Task 1 — sessionview package skeleton + Bucket/Group (TDD)

**Files:** Create `rival/internal/sessionview/group.go`, `rival/internal/sessionview/group_test.go`

- [ ] Write `group_test.go` first — `TestGroup`:
  - solo session (`GroupID==""`) → one bucket, `GroupID==""`, one member.
  - two sessions same `GroupID` → one bucket, both members, `SortGroupMembers` order.
  - bucket order follows first appearance of GroupID in input.
  - input slice/elements not mutated (compare a deep copy after the call).
  - `go test ./internal/sessionview/ -run TestGroup` → FAIL (no impl).
- [ ] Implement `Bucket` + `Group` in `group.go`, porting `dashboard/model.go:43` logic (map by GroupID, `solo:<ID>` for standalone, `SortGroupMembers` each, first-appearance order). Return `[]Bucket`.
- [ ] `go test ./internal/sessionview/ -run TestGroup` → PASS.
- [ ] `go build ./...` green.
- [ ] Commit on clean review: "feat(sessionview): Bucket + Group".

## Task 2 — grouping functions (TDD)

**Files:** Modify `rival/internal/sessionview/group.go`, `rival/internal/sessionview/group_test.go`

- [ ] Extend `group_test.go`:
  - `TestStatus`: mixed group with a running member → "running"; queued+failed+completed (no running) → "queued"; failed+completed → "failed"; all completed → "completed".
  - `TestKind`: any member `Mode=="plan"` → "plan"; else "megareview".
  - `TestCLIs`/`TestModels`: dedup preserving first-seen order; separators `"+"` and `" + "` respectively.
  - `TestEffort`: returns `Sessions[0].Effort`.
  - `TestElapsed` (canonical, wall-clock span): **sequential** two members (start T, 4min; end; start T+4min, 3min) → ~7min; **overlapping** → span; **running** member → extends to now (assert ≥ its start delta); **queued** member → counts from `QueuedAt`. Use fixed `StartTime`/`EndTime`/`QueuedAt`; for running/now cases assert a range, not equality.
  - all new tests FAIL first.
- [ ] Implement `Status`, `CLIs`, `Models`, `Effort`, `Kind`, `EngineLabel` porting the **server** bodies (`server/server.go:408,430,439,453,458,471`), and `Elapsed` porting the **server** wall-clock body (`server/server.go:525`). `EngineLabel` = `config.EngineLabel(s.CLI, s.Model)`.
- [ ] `go test ./internal/sessionview/` → PASS.
- [ ] Type-consistency: every fn signature matches the Locked interfaces block.
- [ ] Commit: "feat(sessionview): grouping functions (server-canonical elapsed)".

## Task 3 — cache move (copy) + tests (TDD)

**Files:** Create `rival/internal/sessionview/cache.go`, `rival/internal/sessionview/cache_test.go`

- [ ] Write `cache_test.go` first (this coverage does not exist today, Sol #7): initial load returns all sessions newest-first; unchanged second `Load` does not bump revision; a rewritten file (new mtime/size) bumps revision and reparses; a deleted file drops from results and bumps revision; a parse-failure file is skipped, others load; **absent dir → nil sessions** (Sol #8, matches existing `os.ErrNotExist` branch); `Get(id)` returns the cached record or nil. Use `t.TempDir()` + hand-written JSON files. FAIL first.
- [ ] **Copy** (not move) `server/session_cache.go` → `cache.go`, package `sessionview`, exporting `Cache`, `New`, `Load`, `Get`; keep `cachedSession`/`cachedSessionValues` unexported. Behaviour byte-identical. Server copy stays until Task 6 (Sol #9).
- [ ] `go test ./internal/sessionview/` → PASS. `go build ./...` green.
- [ ] Commit: "feat(sessionview): cache with full test coverage".

## Task 4 — server uses sessionview grouping (TDD-guarded)

**Files:** Modify `rival/internal/server/server.go`, `rival/internal/server/server_test.go`

- [ ] Run existing `go test ./internal/server/` → PASS (baseline; values are canonical).
- [ ] In `server.go`, replace bodies of `groupStatus/groupKind/groupEffort/groupCLIs/groupModels/groupElapsed/groupEngineLabel` call sites with `sessionview.*`; replace local `groupSessions` with `sessionview.Group` + a `bucketToGroup(sessionview.Bucket) sessionGroup` adapter that builds `sessionGroup` (calling `publicSessions`, `sessionview.Status/Models/…`). Delete the now-unused local grouping funcs.
- [ ] Migrate any `server_test.go` calls to deleted helpers onto `sessionview.*` or the adapter. **Expected values unchanged.**
- [ ] `go test ./internal/server/` → PASS. `go build ./...` green.
- [ ] Commit: "refactor(server): group via sessionview".

## Task 5 — server cache → sessionview.Cache; delete server copy

**Files:** Modify `rival/internal/server/server.go`; Delete `rival/internal/server/session_cache.go`

- [ ] Replace `newSessionCache(...)`/`cache.load()`/`cache.get()` with `sessionview.New(...)`/`.Load()`/`.Get()`.
- [ ] `git rm rival/internal/server/session_cache.go`.
- [ ] `go build ./... && go test ./internal/server/` → PASS.
- [ ] Commit: "refactor(server): use sessionview.Cache, drop local copy".

## Task 6 — browser renders API elapsed for active groups (Sol #6)

**Files:** Modify `rival/internal/server/templates/index.html`

- [ ] Locate the JS that recomputes elapsed for running/queued groups from the primary member (bypassing `group.elapsed`).
- [ ] Replace with rendering `group.elapsed` (the API value) for all group states.
- [ ] Manual: start a multi-member running group, load the dashboard, confirm the row's elapsed matches `GET /api/sessions` `elapsed` (not a primary-only recompute).
- [ ] Commit: "fix(dashboard-web): render API elapsed for active groups".

## Task 7 — TUI grouping via sessionview (TDD)

**Files:** Modify `rival/internal/dashboard/session_list.go`, `rival/internal/dashboard/detail_view.go`, `rival/internal/dashboard/model.go`, `rival/internal/dashboard/session_list_test.go`

- [ ] Update `session_list_test.go` group-elapsed expectations to wall-clock span (intentional change, Sol #2); adjust any `cliLabel`/kind cases that referenced removed helpers. Run → FAIL where elapsed changed.
- [ ] Add `toDisplayItem(sessionview.Bucket) displayItem`; replace `groupSessions` in `model.go` with `sessionview.Group` mapped through it.
- [ ] Replace `session_list.go` `groupStatus/groupCLIs/groupModels/groupEffort/groupElapsed/groupKindLabel` bodies with one-line delegations to `sessionview.*(item.Sessions)`. Delete `groupIsPlan`; `groupIcon` calls `sessionview.Kind(item.Sessions)=="plan"`.
- [ ] Migrate `detail_view.go` `groupIsPlan`/`groupKindLabel` call sites to `sessionview.Kind`.
- [ ] `go test ./internal/dashboard/` → PASS. `go build ./...` green.
- [ ] Commit: "refactor(dashboard-tui): group via sessionview".

## Task 8 — TUI read path + hydration + mutation safety (TDD, the risk phase)

**Files:** Modify `rival/internal/dashboard/model.go`, `rival/internal/dashboard/watcher.go`, `rival/internal/dashboard/detail_view.go`, tests colocated

- [ ] **Mutation-safety test first (Sol #1, critical).** Write `TestKillReloadsBeforeFail` (colocated in dashboard): create a session JSON with a long prompt on disk; build a `displayItem` from a **summary** (`Prompt==""`, via `LoadSummaryFile`); invoke the kill/`Fail` path; re-read the file from disk and assert `Prompt` and all persisted fields survived. One solo case, one grouped case where a non-primary member is "killed". FAIL first (current code Saves the prompt-less summary).
- [ ] Fix the `x` handler in `model.go`: before `Fail`, `full, err := session.Load(s.ID)`; on success mutate/`Fail` `full`, else skip that member (do not Fail a summary). Applies to solo and every grouped member.
- [ ] **Hydration test (Sol #4):** entering detail view populates `hydrated` for every bucket member; a subsequent `SessionEvent` does NOT clear `hydrated`; the detail renderer prefers `hydrated[id]`, falls back to summary on miss; a `session.Load` error falls back to preview without caching. FAIL first.
- [ ] Add `hydrated map[string]*session.Session` to `Model`; populate on entering detail view; preserve across `SessionEvent`; invalidate on detail exit and per-ID on terminal-status re-read; wire `detail_view.go` to prefer it.
- [ ] Switch `watcher.go` both `session.LoadAll()` calls to a package-level `sessionview.Cache.Load()`; the model's items are summaries thereafter (which is why hydration + mutation-safety above are prerequisites, done first in this task).
- [ ] `go test ./internal/dashboard/` → PASS. `go build ./...` green.
- [ ] Commit: "feat(dashboard-tui): shared cache read path with hydration + mutation safety".

## Task 9 — full verification + manual parity

**Files:** none (verification only)

- [ ] `cd rival && make test` (lint + suite + build) → green.
- [ ] Leftover grep: `grep -rn "^func group" rival/internal/dashboard rival/internal/server` → only `groupIcon` (dashboard) + `bucketToGroup`/DTO helpers (server); no duplicated pure grouping fns.
- [ ] Manual cross-UI parity: mixed session dir (running, queued, failed, completed, one megareview group, one plan group). TUI rows vs `GET /api/sessions` agree on status, models, CLIs, kind, elapsed. Rendered browser row for a running multi-member group matches (Sol #6).
- [ ] Manual TUI detail: long-prompt session shows full prompt + log pane + PID/log-path/account, and stays correct across a live refresh during an active run.
- [ ] Manual mutation safety: in the TUI, `x` a queued session that has a long stored prompt; confirm the on-disk JSON keeps its prompt after the kill.

## Type-consistency check

- `Bucket`/`Group`/`Status`/`CLIs`/`Models`/`Effort`/`Elapsed`/`Kind`/`EngineLabel` names identical in Tasks 1-2 (definition), 4 (server use), 7 (TUI use).
- `toDisplayItem` (Task 7) and `bucketToGroup` (Task 4) are the only adapters; names used consistently.
- `hydrated` field name identical in Task 8 test and impl.
- Cache methods `Load`/`Get`/`New` identical across Tasks 3, 5, 8.
