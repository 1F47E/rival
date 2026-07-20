# Rival server dashboard remediation plan

## Findings

The current dashboard does not degrade gracefully as session history grows:

- `GET /api/sessions` loads every session file and serializes the full `Session`
  object, including the full prompt. On the development machine, 1,736 session
  records produce a 309 MB response and take about 2.2 seconds before browser
  parsing or rendering begins.
- The browser rebuilds the complete table after every poll. Active runs poll
  every two seconds, so a large history can keep both the server and browser
  continuously busy.
- The detail panel is rendered after the full table. Clicking a row near the top
  opens details below thousands of rows, which looks like the click did nothing.
- Detail log requests read and return the entire log. Existing logs reach
  40–62 MB, enough to make the detail view stall or exhaust browser memory.
- The page depends on Tailwind's runtime CDN compiler. That adds a network
  dependency and startup work to a dashboard served from localhost.
- The web logo is a shortened, malformed copy of Rival's canonical terminal
  banner, and its tight line height makes it even less legible.
- The list API exposes internal fields that the page does not need, including
  prompt bodies, log paths, and process metadata.

## Implementation

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
6. Add server tests for summary loading, pagination and payload privacy, prompt
   bounds, log tailing, and the dashboard's key interaction hooks.

## Validation

- Run the Go test suite and build the `rival` binary.
- Benchmark response size and latency against the same existing session history.
- Capture desktop and mobile screenshots from the real local server.
- Exercise row click, Escape/close, grouped detail logs, load-more behavior, and
  offline rendering without any third-party requests.
