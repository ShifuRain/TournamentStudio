# TournamentStudio — Web UI Watch Design

Date: 2026-08-28
Status: Approved for planning
Builds on: `2026-08-21-tournament-studio-design.md` (Foundation),
`2026-08-21-tournamentstudio-plugin-engine-design.md` (Plan 2),
`2026-08-22-tournamentstudio-scheduling-live-results-design.md` (Plan 3),
`2026-08-23-tournamentstudio-web-ui-foundation-design.md` (Plan 4a),
`2026-08-24-tournamentstudio-web-ui-run-design.md` (Plan 4b)

## 1. Overview

This is "Plan 4c" — the third and final sub-project decomposing the
originally-scoped "Plan 4" (the full web UI), split by usage phase:

- **4a — Foundation + Setup** (done, merged): frontend scaffolding, auth,
  tournament creation, team management.
- **4b — Run** (done, merged): courses, round/group creation, heat
  scheduling, results entry, next-round/division triggers.
- **4c — Watch** (this plan): a live, read-only schedule/standings display
  driven by the existing WebSocket broadcast hub, and a Plugins browser
  with upload/reload management.

4a left a disabled "Standings" tab placeholder on the tournament detail
page specifically for this plan. This plan fills that slot, and adds a
second, independent top-level screen for plugin management.

This plan bundles two pieces that are functionally unrelated but were
both deferred to "4c" by the original decomposition and share no code —
they're built and reviewed as separate task groups within one plan.

## 2. Goals / Non-Goals

**Goals**

- A `watch` screen under `/tournaments/:id/watch`: every round's groups
  or (once cut) divisions, teams ranked fastest-first within each, with
  DNF/DSQ/DNS sorted after every timed team — computed server-side by
  reusing `internal/ranking.Rank()`, the same function already trusted
  for reseeding and division cuts. Visible to every authenticated role,
  read-only, no editing controls.
- Live updates: a WebSocket client that invalidates the Watch screen's
  queries on any broadcast message, so results submitted by an operator
  elsewhere appear without a manual reload. This is the first UI
  consumer of the WebSocket hub built in Plan 3 — 4b explicitly deferred
  it.
- A `plugins` screen (top-level, not tournament-scoped, since the plugin
  `Engine` is one process-wide instance): a read-only catalog of loaded
  sport and tournament-type plugins for every role, plus organizer-only
  upload of new external `.lua` plugin files and deletion of previously
  uploaded ones, both triggering a live reload of the running Engine.

**Non-Goals (this plan)**

- Any change to the ranking algorithm itself, or a plugin-provided
  scoring hook — `ranking.Rank()`'s time-ascending, DNF/DSQ/DNS-ordered
  behavior is reused exactly as-is.
- Cross-round aggregate standings (a single combined ranking spanning
  multiple rounds). The tournament is elimination-style — each round has
  a different, shrinking team pool — so standings are scoped per round,
  per group/division, matching the existing data model.
- Enabling/disabling already-loaded plugins, or any other plugin
  management action beyond upload and delete of external files. Bundled
  plugins (embedded in the binary) can never be deleted or disabled.
- Editing anything from the Watch screen. It is strictly read-only —
  all mutation controls remain on 4b's `schedule` screen.
- Reconciling `time_entry`/`spectator` navigation between `schedule` and
  `watch` beyond adding the new tab — 4b's screen keeps its existing
  role gating unchanged.

## 3. Backend Addition: `GET /api/tournaments/{id}/standings`

**The gap:** nothing today returns *ranked* results. `GET
.../schedule` (Plan 3) returns every heat with its raw, unordered
results — correct for 4b's organizer-facing results entry, but the
Watch screen needs "who's currently in the lead," which requires the
same ranking computation `handleComputeDivisions` and `handleNextRound`
already run internally but never expose.

**The fix:** a new read endpoint, any authenticated role:

```
GET /api/tournaments/{id}/standings
```

For every round in the tournament (ordered by `round_number`): the
round's id, number, and status, plus one entry per group — or, once
`ListDivisionsForRound` returns any, one entry per division instead —
each holding its teams ranked via `ranking.Rank()`:

```json
{
  "rounds": [
    {
      "id": 1, "round_number": 1, "status": "closed",
      "standings": [
        {
          "group_id": 1, "division_id": null, "division_name": null,
          "ranked_teams": [
            {"rank": 1, "team_id": "3", "time_seconds": 124.11, "status": ""},
            {"rank": 2, "team_id": "1", "time_seconds": null, "status": "DNF"}
          ]
        }
      ]
    }
  ]
}
```

`group_id`/`division_id` are mutually exclusive per entry (mirroring
`Heat`'s own `GroupID`/`DivisionID` nullable pair); `division_name`
carries the plugin-assigned name (e.g. `"A"`) when present. Ranking
uses whatever results exist so far — a round need not be closed for its
standings to render; partial results rank normally, with
not-yet-submitted teams simply absent from `ranked_teams` (unranked-but-
scheduled is a display concern, not a ranking one).

Implementation reuses `round_common.go`'s existing pattern
(`s.rounds.ListGroups` / `s.schedule.ListDivisionsForRound`,
`s.schedule.ListResultsForRound` for the team→result lookup) without its
`loadClosedRoundContext` closed-round requirement — a new, unexported
helper assembles the same `resultsByTeam` map for an open round. No
changes to `internal/ranking` or `internal/plugin`.

One deliberate divergence from `handleComputeDivisions`'s existing
pattern: that handler builds its `ranking.TeamResult` slice from *every*
team in the group, so a team with no entry in `resultsByTeam` becomes a
zero-value result (`TimeSeconds: nil, Status: ""`) that
`ranking.Rank`'s unrecognized-status branch sorts after every real
DNF/DSQ/DNS — correct there, since that handler only ever runs on a
*closed* round where every team's fate is already decided. This
endpoint must also render correctly for an *open*, in-progress round, so
its ranking input includes only teams with an actual entry in
`resultsByTeam`; a team that simply hasn't raced yet is omitted from
`ranked_teams` entirely rather than sorted to the bottom indistinguishable
from a DSQ.

## 4. Backend Addition: Plugin Upload, Delete, and Runtime Reload

**The gap:** `plugin.Load(externalDir)` runs exactly once, in
`cmd/tournamentstudio/main.go`, at process startup. There is no way to
add a plugin without restarting the process, and no way to discover
*which* loaded plugins came from the embedded bundle versus the external
directory.

**The fix, in `internal/plugin`:**

- `SportPlugin` and `TournamentTypePlugin` each gain a `Source string`
  field: `"bundled"` for the two embedded plugins, or the external
  filename for anything scanned from `externalDir`. Set in `Load`'s two
  existing loop passes (no new loop needed).
- A new exported `Validate(name string, source []byte) error`: runs the
  same sandboxed-exec-and-shape-check `loadSource` already performs, but
  against a throwaway `&Engine{sports: map[...], tournamentTypes:
  map[...]}` so it never touches a live Engine. Returns the same error
  `loadSource` would have logged-and-skipped for a bad external file —
  today's silent-skip behavior is correct for a background startup scan
  but wrong for an interactive upload, which needs to tell the organizer
  exactly what failed.

**The fix, in `internal/server`:**

- `Server.plugins` changes from `*plugin.Engine` to
  `*atomic.Pointer[plugin.Engine]`. Every existing read site
  (`s.plugins.Sports()`, `s.plugins.TournamentTypes()`,
  `findTournamentType`) adds a `.Load()` call to dereference the current
  Engine first. `server.New` gains a `pluginsDir string` parameter,
  threaded from `main.go`'s already-computed `pluginsDir`.
- A reload is: validate the new/removed file → mutate `pluginsDir` on
  disk → `plugin.Load(pluginsDir)` to build a fresh Engine → `Store()` it
  on the atomic pointer. The **superseded Engine is never explicitly
  closed.** An in-flight request may still hold a `*TournamentTypePlugin`
  obtained from it (mid-`DivisionCuts` or `NextRoundGroups` call);
  closing that plugin's Lua state out from under an in-progress call
  would race. Since `gopher-lua`'s `LState` is pure Go with no external
  OS resource, the old Engine is simply left for garbage collection once
  the last reference to it drops — safe, and appropriate for an action
  this infrequent (an admin uploading a plugin, not a hot path).
- New endpoints, organizer-only (`requireRole` with the existing
  `auth.RoleOrganizer`):
  ```
  POST   /api/plugins             multipart, field "file" — validate, write, reload
  DELETE /api/plugins/{filename}  external files only — 404 for bundled or missing
  ```
  Both reject any filename that isn't a bare basename ending in `.lua`
  (no `/`, no `..`) before touching the filesystem. A validation failure
  on upload returns 400 with `Validate`'s error message and leaves both
  the filesystem and the live Engine untouched — a bad upload cannot
  affect any tournament already running. Re-uploading an existing
  filename overwrites it; a new file whose plugin `id` collides with an
  existing one wins by last-loaded, the same override behavior `Load`
  already documents for external-over-bundled today.
- `GET /api/plugins`'s existing response gains the `source` field on
  each entry; otherwise unchanged.

## 5. Screen Layout

**Watch tab** — `/tournaments/:id/watch`, sibling to `teams` and
`schedule` under `TournamentDetailPage`'s tab strip. The disabled
"Standings" placeholder `NavLink` becomes real, matching `schedule`'s
existing pattern. Renders one section per round (most recent first),
each round showing its status and one ranked table per group/division:
position, team name (joined against the same `['teams', id]` query
`SchedulePage` already uses), time or status badge. No forms, no
buttons — every authenticated role sees the identical view.

A `useTournamentSocket(tournamentId)` hook opens
`wss://…/api/tournaments/{id}/ws?token=<token>` (token from the same
source `api/client.ts` already uses for the `Authorization` header) the
moment `WatchPage` mounts, and closes it on unmount. On any message it
calls `queryClient.invalidateQueries` for `['standings', id]` — not
branching on the message's `type`, so a future broadcast event never
requires updating this hook. Reconnects with capped exponential backoff
on drop; after repeated failed reconnects, a small banner reads "live
updates unavailable — showing last loaded data" without blocking the
already-rendered content. Only `WatchPage` uses this hook — `SchedulePage`
is unchanged, still refetch-on-own-action per 4b's explicit choice.

**Plugins screen** — `/plugins`, a new top-level route alongside
`/tournaments` (its own `AppShell` nav entry — not nested under any
tournament, since plugins are process-global). Two sections:

1. **Sport plugins** and **tournament-type plugins**, each listed with
   `id`, `display_name`/compatibility fields, and a "Built-in" badge when
   `source === "bundled"`.
2. Organizer-only: a file upload control (`<input type="file"
   accept=".lua">`, submitted via the multipart `POST /api/plugins`) and
   a delete button per external (non-bundled) entry. A failed upload
   shows `Validate`'s error message via the same `role="alert"` pattern
   used everywhere else.

## 6. Error Handling

- `GET /standings` failures follow the same pattern as every other list
  endpoint in this codebase: internal errors return 500; an empty list
  (no rounds yet) is valid, not an error.
- The WebSocket hook's reconnect/banner behavior is described in §5 —
  a dropped socket degrades to stale-but-present data, never a blank
  screen.
- Plugin upload/delete errors render via `role="alert"`, matching
  `TournamentCreatePage`/`TeamImportPage`/`TeamsTab`'s existing
  convention. A validation failure never touches the live plugin set, so
  the catalog and any running tournament remain exactly as they were
  before the failed attempt.

## 7. Testing Approach

- **Backend unit/table tests**: the standings handler, covering a
  pre-cut round (grouped) and a post-cut round (divisions), including
  mixed timed/DNF/DSQ/DNS results, in the style of
  `internal/ranking/rank_test.go`'s fixtures. `plugin.Validate` gets
  direct tests (valid source; malformed Lua; missing `id`; wrong table
  shape). Upload/delete handlers get tests for organizer-only
  enforcement (403 for other roles), bad-file rejection, filename
  sanitization (path traversal attempt), overwrite-by-same-filename, and
  bundled-delete-returns-404. One test exercises a reload occurring while
  a concurrent request still holds a `*TournamentTypePlugin` from the
  superseded Engine, confirming that call completes without racing the
  reload — locking in §4's "never close a superseded engine" decision.
- **Component/unit tests**: Vitest + React Testing Library for
  `WatchPage` (ranked-table rendering from a mocked `standings`
  response), `useTournamentSocket` (mocked WebSocket — asserts
  `invalidateQueries` fires on message receipt and the backoff banner
  appears after repeated failures), and `PluginsPage` (catalog
  rendering, organizer-only visibility of upload/delete controls, the
  validation-error display path) — `api` mocked, matching every prior
  screen's established pattern.
- **End-to-end tests**: two new Playwright scenarios. One drives 4b's
  existing round/results flow, then asserts the Watch page reflects a
  submitted result without a manual reload — the one point that actually
  proves the WebSocket → invalidation → refetch chain works end to end.
  The other uploads a small fixture `.lua` plugin file through the
  Plugins screen and confirms it appears in the catalog and becomes
  selectable during tournament creation. Both run against the real Go
  binary + real SQLite + real built frontend, no mocks, non-interactively.

## 8. Open Questions / Deferred

None — this is the last sub-project in the original Plan 4 decomposition.
Any further web UI work (e.g. a public/unauthenticated read-only view,
should that ever be wanted) would be a new, separately-brainstormed plan.
