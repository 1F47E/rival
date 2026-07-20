# Unify TUI and web dashboard on one in-process data layer

**Date:** 2026-07-20
**Scope:** /Users/kass/dev/claude-codex-plugin (Go module in `rival/`)
**Status:** pending review
**Revision:** 2 — rewritten after Sol review (3/10, 1 crit + 4 high + 4 med, all addressed below) and re-verified against the working tree as of the in-flight 71-file runtime-removal refactor. All line citations re-checked; they will drift again if that refactor lands before implementation, so treat them as landmarks and re-grep by symbol name.

## Problem(s)

1. **Seven grouping functions exist twice.** `groupSessions`, `groupStatus`,
   `groupCLIs`, `groupModels`, `groupElapsed`, `groupKind`/`groupKindLabel`, and
   `groupEngineLabel` each have one copy in the TUI and one in the server.
   `groupStatus`, `groupCLIs`, and `groupModels` are identical logic differing
   only in signature (`*displayItem` vs `[]*session.Session`).
   `dashboard/session_list.go:196,210,223,243` + `dashboard/model.go:43` vs
   `server/server.go:351,408,458,471,525`.

2. **Two pairs have already diverged.**
   (a) `groupKind` (`server/server.go:430`) answers "is this a plan group" with an
   inline `Mode == "plan"` loop; the TUI routes the same question through
   `groupIsPlan` (`dashboard/session_list.go:156`) feeding `groupKindLabel:183`.
   (b) **`groupElapsed` computes different numbers.** The server takes a
   wall-clock span from earliest start/queue time to latest end
   (`server/server.go:525-547`); the TUI takes the maximum member duration
   (`dashboard/session_list.go:243`). For two sequential 4-minute and 3-minute
   members the server reports ~7 minutes and the TUI reports 4. The two UIs
   already disagree on screen today.

3. **The two UIs read the session store differently.** The server uses
   `sessionCache` over `session.LoadAllSummaries` (bounded 64 KiB edge reads,
   mtime+size invalidation, revision counter — `server/session_cache.go:37`,
   `server/server.go:135,194,224`). The TUI calls `session.LoadAll()` — a full
   parse of every session file — on startup and again on every fsnotify event
   (`dashboard/watcher.go:38,55`). "Same data" is a coincidence of two code
   paths, not a guarantee.

## Goals

1. One copy of each grouping function, consumed by both UIs. (P2, P3)
2. One canonical `Elapsed` definition, with the resulting behaviour change to
   one UI made explicit rather than described as behaviour-preserving. (P2)
3. One cache, so both UIs read the store through the same code. (P4)
4. Preserve TUI detail view exactly: full prompt, log pane, metadata. (P4)
5. **No session data loss**: cached summaries must never reach a `Save`. (P4)
6. Keep the HTTP API a thin sanitizing wrapper — no public-DTO widening.

## Non-goals

- Making the TUI an HTTP client (rejected: needs a running server, adds a
  round-trip per refresh, and forces PIDs/log paths/prompts — deliberately
  stripped by `publicSessions` — back onto the network).
- Changing the on-disk session format or the shape of `/api/sessions`.
- Touching `internal/session`'s exported API.
- Unifying presentation: TUI icons and server DTOs stay separate.
- Any interaction with the in-flight retired-provider cleanup.

## Sessionview layer

```
                    ~/.rival/sessions/*.json
                              |
        internal/session   (LoadAll, LoadSummaryFile, Load, SortGroupMembers)
                              |
        internal/sessionview                      <-- NEW, single copy
          Cache: Load() ([]*session.Session, uint64) / Get(id)
          Group([]*session.Session) []Bucket
          Status / CLIs / Models / Elapsed / Kind / EngineLabel  (over []*session.Session)
                    |                              |
        internal/dashboard (TUI)          internal/server (HTTP)
          adapter: Bucket -> displayItem    adapter: Bucket -> sessionGroup
          + groupIcon (Unicode)             + publicSessions / publicLogData
          + session.Load(id) hydration      + loadPromptResponse
```

**Return contract (Sol #3).** Shared functions cannot return either consumer's
private type, so `Group` returns a neutral bucket:

```go
type Bucket struct {
    GroupID  string             // "" for a solo session
    Sessions []*session.Session // SortGroupMembers order; never mutated by sessionview
}
func Group(sessions []*session.Session) []Bucket
```

Contract, pinned by tests: buckets are newest-first by their primary session's
`StartTime` (today's order in both consumers); members are ordered by
`session.SortGroupMembers`; a session with an empty `GroupID` yields a
single-member bucket whose `GroupID` is `""`; `Sessions[0]` is the primary;
sessionview never writes to the slice or its elements. Each consumer keeps a
local adapter — `Bucket → displayItem` in the TUI, `Bucket → sessionGroup`
(with DTO conversion) in the server.

**Canonical `Elapsed` (Sol #2).** Adopt the **server's wall-clock span**:
earliest start-or-queue time to latest end (running/queued members extend to
now). It is the more useful group answer — "how long has this run taken" —
and it is already the one covered by tests. Consequence: **the TUI's group
elapsed changes** for sequential members (4min+3min now shows ~7min, not 4min).
This is an intentional, user-visible behaviour change, not a refactor artifact.
`dashboard/session_list_test.go` expectations for group elapsed must be updated;
solo-session elapsed (`formatElapsed`) is untouched.

**Cache ownership.** `server/session_cache.go` moves to sessionview; it has zero
web-specific code (imports only stdlib + `internal/session`). Exported as
`Cache`, `New(dir)`, `Load()`, `Get(id)`.

**Prompt hydration lifecycle (Sol #4).** A one-shot `session.Load` on Enter is
insufficient: every `SessionEvent` replaces the model's items with fresh
summaries, including events fired by log writes during an active run, so the
prompt would silently revert to its preview. Instead the TUI model keeps
explicit hydration state:

```go
hydrated map[string]*session.Session // session ID -> full record
```

- Populated on entering detail view for **every member** of the selected bucket.
- A `SessionEvent` refresh replaces list items but **must not** clear `hydrated`;
  the detail renderer prefers `hydrated[id]` and falls back to the summary.
- Invalidated when detail view is exited, and per-ID when that session's status
  reaches a terminal state and is re-read.
- On `session.Load` error, fall back to the summary (renders `PromptPreview`,
  today's behaviour) and do not cache the failure.

**Mutation safety (Sol #1 — the critical one).** `LoadSummaryFile` clears
`Prompt` (`session/summary.go:61`). The TUI's `x` handler calls `s.Fail(...)`
directly on `item.Sessions` elements (`dashboard/model.go:245,249`), and `Fail`
→ `Save` serializes the **whole** struct — so killing a cache-backed session
would overwrite its stored JSON with a prompt-less copy, permanently destroying
the prompt. Rule, enforced by test: **cached summaries are read-only.** Any
mutation path must re-read the complete record with `session.Load(id)`
immediately before mutating, and mutate that object. This covers the `x` handler
for solo *and* grouped sessions, plus any future TUI mutation.

## File-level changes

| File | Change |
|---|---|
| `rival/internal/sessionview/cache.go` | **New.** Copy of `server/session_cache.go`, identifiers exported. Server copy deleted in P2, not P1 (Sol #9). |
| `rival/internal/sessionview/group.go` | **New.** `Bucket`, `Group`, `Status`, `CLIs`, `Models`, `Elapsed` (server semantics), `Kind`, `EngineLabel` — all over `[]*session.Session`. |
| `rival/internal/sessionview/group_test.go` | **New.** Pins the contract above. |
| `rival/internal/sessionview/cache_test.go` | **New** (Sol #7): initial load, mtime+size invalidation, unchanged-load stability, parse failure, deletion, unreadable dir, revision increments, ordering, returned-pointer aliasing. |
| `rival/internal/server/session_cache.go` | **Deleted in P2** (after the server switches over). |
| `rival/internal/server/server.go` | Delete the seven local copies; call `sessionview.*` + a local `Bucket → sessionGroup` adapter. DTO layer untouched. |
| `rival/internal/server/server_test.go` | Migrate direct calls to local helpers onto `sessionview`/the adapter (Sol #5). Expected values unchanged — server behaviour is the canonical one. |
| `rival/internal/server/templates/index.html` | Sol #6: for running/queued groups the browser recomputes elapsed from the primary member only, bypassing `group.elapsed`. Render the API value instead so the browser matches the shared logic. |
| `rival/internal/dashboard/watcher.go` | Both `session.LoadAll()` calls → shared `sessionview.Cache.Load()`. |
| `rival/internal/dashboard/session_list.go` | Delete the six local grouping funcs + `groupIsPlan` + `groupKindLabel`; wrappers delegate to `sessionview`. `groupIcon` keeps its `*displayItem` signature, calls `sessionview.Kind`. |
| `rival/internal/dashboard/detail_view.go` | **Was missing from revision 1** (Sol #5): it calls `groupIsPlan`/`groupKindLabel`. Migrate those call sites and render from `hydrated` when present. |
| `rival/internal/dashboard/model.go` | Local `groupSessions` → `sessionview.Group` + adapter; add `hydrated` state; **`x` handler re-reads via `session.Load(id)` before `Fail`**. |
| `rival/internal/dashboard/session_list_test.go` | Update group-elapsed expectations to wall-clock span (intentional change). |

## Tests

**Unit — `sessionview/group_test.go`.** Status tier (running > queued > failed >
completed) on a mixed group; `Kind` = `plan` iff any member `Mode == "plan"`;
`CLIs`/`Models` dedup preserving first-seen order with `+` and ` + `;
`Elapsed` over four explicit cases — **sequential** (4min then 3min → ~7min),
**overlapping**, **running** (extends to now), **queued** (counts from
`QueuedAt`); `Group` bucket ordering, member ordering, solo-session bucket,
primary selection, and that inputs are not mutated.

**Unit — `sessionview/cache_test.go`.** As listed in the file table (Sol #7);
this coverage does not exist today, so it is new work, not a move.

**Unit — mutation safety (Sol #1, the critical regression test).** With a
session whose JSON holds a long prompt: load it through the cache (summary,
`Prompt == ""`), run the kill path, then re-read the file from disk and assert
the prompt and every other persisted field survived. One solo case, one grouped
case where a non-primary member is killed.

**Unit — hydration (Sol #4).** Entering detail view hydrates all bucket members;
a subsequent `SessionEvent` refresh does not revert the rendered prompt to the
preview; a failed `session.Load` falls back to the preview without caching.

**Regression.** `server_test.go` values unchanged (migrated call sites only);
`dashboard/session_list_test.go` group-elapsed values intentionally updated;
`detail_view_test.go` must pass.

**Manual — cross-UI parity.** Mixed session dir (running, queued, failed,
completed, one megareview group, one plan group): TUI rows vs `GET /api/sessions`
must agree on status, models, CLIs, kind, and elapsed — **and** the rendered
browser rows must match for a running multi-member group (Sol #6; API-only
comparison cannot catch the browser-side recompute).

**Manual — TUI detail view.** Long-prompt session: full prompt, log pane, and
PID/log-path/account metadata render as today, and stay correct across a live
refresh during an active run.

## Failure modes & decisions

| Failure / question | Behaviour |
|---|---|
| Killing a cache-backed session erases its prompt (Sol #1) | Cached summaries are read-only. Mutation paths `session.Load(id)` first and mutate the full record. Regression-tested solo + grouped. |
| TUI vs server elapsed disagree (Sol #2) | One definition: the server's wall-clock span. TUI group elapsed changes intentionally; documented and test-updated. |
| Shared `Group` cannot return a consumer type (Sol #3) | Returns neutral `[]Bucket`; each consumer keeps a local adapter. |
| Hydrated prompt lost on live refresh (Sol #4) | `hydrated` map survives `SessionEvent`; renderer prefers it, falls back to summary. |
| Deleting helpers breaks callers/tests (Sol #5) | Every call site enumerated, including `detail_view.go`; tests migrated in the same phase as the deletion. |
| Browser bypasses shared elapsed (Sol #6) | `index.html` renders the API value for running/queued groups; verified against rendered rows. |
| Session dir absent (Sol #8) | Existing code returns **nil** on `os.ErrNotExist` (empty UI) and the last known values only on other read errors. Preserved verbatim and pinned by test — revision 1 described this incorrectly. |
| P1 green build (Sol #9) | P1 **copies** the cache into sessionview; the server copy is deleted in P2 when its consumer switches. No phase leaves a dangling reference. |
| Cache/fsnotify disagreement | Existing semantics: mtime+size mismatch reparses, missing file dropped next `Load`. No new logic. |
| Post-change UI divergence | Structurally prevented for shared functions; remaining differences live only in per-UI presentation by design. |

## Out of scope

- TUI-over-HTTP; `groupModelRank`/`SortGroupMembers` rework; performance work
  beyond the incidental cache win; the k3/opencode work in `f48c956`…`391ed8f`;
  the in-flight retired-provider cleanup.

## Rollout

- **P1** — Create `internal/sessionview`: **copy** the cache in (server keeps its
  copy for now), add `Bucket`/`Group`/the six functions, plus `group_test.go`
  and `cache_test.go`. No consumer changes. Green build proves the package in
  isolation (Sol #9).
- **P2** — Migrate `internal/server` onto sessionview; delete
  `server/session_cache.go` and the seven local copies; migrate `server_test.go`
  call sites; fix the `index.html` elapsed recompute (Sol #6).
- **P3** — Migrate `internal/dashboard` grouping: `session_list.go`,
  `detail_view.go`, `model.go` adapters onto sessionview; update group-elapsed
  test expectations. TUI still on `session.LoadAll()`.
- **P4** — Migrate the TUI read path: watcher onto the shared cache, add
  `hydrated` detail state, and **make the `x` handler re-read before mutating**.
  Gated on the mutation-safety and hydration tests plus manual detail-view and
  parity verification.

**Precondition:** the working tree currently holds a 71-file uncommitted
retired-provider cleanup touching every file above. Per the exec preconditions,
that must be committed or stashed before implementation starts, and the
citations here re-grepped by symbol name.
