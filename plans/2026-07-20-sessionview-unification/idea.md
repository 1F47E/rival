# Idea — unify TUI + web dashboard data layer

**Date:** 2026-07-20
**Status:** done

The user asked whether the TUI and the web dashboard share a data connector, to
guarantee identical data and avoid duplicate code. Investigation says no: they
share only `internal/session` (file parsing), and everything above it is
duplicated — seven grouping functions written twice, one of which has already
drifted.

Why now: the just-landed dashboard rebuild (`3dc2b8e`) introduced a session
cache with bounded reads for the web path only. That widened the gap — the two
UIs now *read differently*, not just render differently, so "same data" is no
longer guaranteed by construction.

First-guess scope: extract a new `internal/sessionview` package holding the
cache plus one copy of each grouping function; point both consumers at it.
Explicitly rejected by the user: making the TUI an HTTP client of the dashboard
API (needs a running server, adds round-trips, and would force widening the
sanitized public DTO with PIDs/log paths/prompts).
