# Rival server dashboard remediation

Status: completed for v3.23.0.

## Baseline findings

The previous dashboard did not degrade gracefully as session history grew:

- `GET /api/sessions` loaded every session file and serialized the full `Session`
  object, including the full prompt. On the development machine, 1,736 session
  records produce a 309 MB response and take about 2.2 seconds before browser
  parsing or rendering begins.
- The browser rebuilt the complete table after every poll. Active runs polled
  every two seconds, so a large history can keep both the server and browser
  continuously busy.
- The detail panel was rendered after the full table. Clicking a row near the top
  opens details below thousands of rows, which looks like the click did nothing.
- Detail log requests read and returned the entire log. Existing logs reached
  40–62 MB, enough to make the detail view stall or exhaust browser memory.
- The page depended on Tailwind's runtime CDN compiler. That added a network
  dependency and startup work to a dashboard served from localhost.
- The web logo was a shortened, malformed copy of Rival's canonical terminal
  banner, and its tight line height makes it even less legible.
- The list API exposed internal fields that the page did not need, including
  prompt bodies, log paths, and process metadata.

## Implemented

1. Introduce a metadata cache for the web server. Refresh only session JSON files
   whose size or modification time changed, and avoid retaining full prompts in
   the list cache. Large legacy session files are summarized from bounded file
   edges so their embedded prompts are never loaded into memory.
2. Return a dedicated public summary DTO from the list API. Default to the 100
   newest grouped runs, support a bounded `limit`, and include pagination
   metadata while keeping aggregate status counts over all sessions.
3. Add a bounded prompt-detail endpoint and change log delivery to a UTF-8-safe
   256 KiB tail with an explicit truncation header. Keep runtime/model
   normalization on returned log text.
4. Replace the Tailwind CDN page with self-contained HTML, CSS, and JavaScript.
   Use the canonical logo, a responsive session table, clear loading/error/empty
   states, and accessible keyboard-selectable rows.
5. Put details in a fixed side drawer so they open immediately in the viewport.
   Load member logs in parallel, cancel stale requests when selection changes,
   show member-specific failures, and refresh live logs without rebuilding the
   drawer.
6. Added direct run lookup so session links still work after a run moves beyond
   the newest page, plus explicit feedback for invalid or deleted runs.
7. Added server tests for summary loading, pagination and payload privacy,
   direct run lookup, prompt bounds, log tailing, group wall time, and the
   dashboard's key interaction hooks.

## Validation

- `go test ./...`, `go build ./...`, inline JavaScript parsing, and
  `git diff --check` pass.
- A real Playwright/Chromium run exercised completed, failed, running, queued,
  mixed-group, empty-log, missing-log, and queued-to-completed transitions.
- The same run covered current, older-than-page-100, and invalid session links;
  Escape/close, focus containment, drawer scroll reset, and a 390×844 mobile
  viewport.
- The page has no third-party runtime requests.
