# TournamentStudio — Scheduling & Live Results Design

Date: 2026-08-22
Status: Approved for planning
Builds on: `2026-08-21-tournament-studio-design.md` (Foundation) and
`2026-08-21-tournamentstudio-plugin-engine-design.md` (Plan 2, plugin
engine + reseeding/division-cut algorithms + round/group persistence)

## 1. Overview

This is the third implementation plan for TournamentStudio ("Plan 3"):
the physical scheduling layer (`Course`/`Heat`), real per-heat result
entry with DNF/DSQ/DNS, `Division` as a persisted entity with its own
scheduled heats, and a WebSocket live-broadcast layer for result and
schedule changes. It builds directly on Plan 2's `PrePhaseRound`/`Group`
model and Lua plugin engine, and is itself a prerequisite for Plan 4 (the
browser-based live schedule/standings UI and result-entry screens).

**Scope boundary with Plan 4:** this plan is backend/API only, matching
the pattern established by the Foundation and Plugin Engine plans — every
capability here is testable via HTTP and WebSocket clients, no browser
involved. Plan 4 builds the actual UI screens (result entry, live
schedule/standings display, the Plugins browser) against the API surface
this plan exposes. The spec language that originally read "real
result-entry UI" for this plan refers to real per-heat result *data* (as
opposed to Plan 2's lightweight whole-round time-map), not a rendered
screen.

## 2. Goals / Non-Goals

**Goals**
- `Course` entities: a named schedule lane with a heat interval and a
  live-adjustable delay offset.
- `Heat`: the actual scheduled race for one round-group or one division,
  assigned to a course with an auto-sequenced (and manually overridable)
  planned start time.
- Real per-heat, per-team results (time or DNF/DSQ/DNS), replacing Plan
  2's whole-round lightweight result map.
- `Division` becomes a persisted entity, scheduled the same way a
  round's groups are.
- A WebSocket layer broadcasting result submissions and course
  delay-offset changes to every client connected to a tournament.
- One consolidated read endpoint (`GET /schedule`) giving a full computed
  view (courses, heats, effective times, current results) — the surface
  Plan 4's live view will consume.

**Non-Goals (this plan)**
- Any browser UI — Plan 4.
- Splitting a group/division across multiple heats (lane-capacity
  modeling) — Group:Heat and Division:Heat both stay strictly 1:1 in v1.
  Revisit only if a real tournament format needs it.
- Tie-breaking rules beyond the platform spec's default (fastest single
  recorded time wins) — unchanged, not configurable in v1.
- Broadcasting round/division-computation events over WebSocket — only
  result submission and delay-offset changes broadcast in v1; a client
  that needs to notice a newly computed round can re-fetch
  `GET /schedule` or poll the existing round endpoints.
- Any multi-process/distributed broadcast mechanism (Redis pub/sub,
  etc.) — the fan-out is in-process memory, matching the single-binary,
  offline-first architecture.

## 3. Data Model

```
Course {
  ID, TournamentID, Name,
  HeatIntervalSeconds int,     // spacing between consecutive heats on this course
  DelayOffsetSeconds  int,     // live-adjustable, default 0
}

Heat {
  ID, RoundID int64,  // RoundID is denormalized: the group's or division's
                       // round, always set regardless of which of the two
                       // fields below is set — simplifies the round-closure
                       // cascade check and the GET /schedule query to a
                       // single WHERE round_id = ?, instead of joining
                       // through groups/divisions on every read
  GroupID *int64, DivisionID *int64,  // exactly one is set
  CourseID int64,
  PlannedStart time.Time,
  Status string,  // "scheduled" | "closed"
}

HeatResult {
  HeatID int64, TeamID string,
  TimeSeconds *float64, Status string,  // "" | "DNF" | "DSQ" | "DNS"
}

Division {
  ID, TournamentID, RoundID int64,  // the closed round it was cut from
  Name string,
  TeamIDs []string,  // stored as a JSON TEXT column, same pattern as Group.TeamIDs
}
```

- **Group:Heat and Division:Heat are both strictly 1:1.** A `Heat`
  belongs to exactly one `Group` (pre-phase) or exactly one `Division`
  (finals), never both, never split across multiple heats.
- **Effective/displayed start time** is never stored — it's always
  `Heat.PlannedStart + Course.DelayOffsetSeconds`, computed at read
  time. Nudging a course's offset instantly changes every one of that
  course's future heats' displayed time with zero row writes.
- `HeatResult` replaces `round.Result` from Plan 2 — same shape
  (`TeamID`, `TimeSeconds`, `Status` with the same DNF/DSQ/DNS
  validation Plan 2's final review added), now keyed by `HeatID` instead
  of `RoundID`.
- **A round's ranking input is unchanged in shape**: aggregate each of
  the round's heats' `HeatResult`s by team, feed into `ranking.Rank`
  exactly as Plan 2 already does. The plugin contract
  (`NextRoundGroups`, `DivisionCuts`) does not change — only where the
  results underneath come from changes.
- A round auto-closes once every one of its heats is closed; a heat
  closes once its `HeatResult`s cover every team in its group/division
  (same auto-close logic Plan 2 built, moved one level down, reusing the
  team-conservation-style validation Plan 2's final review added).
  `Division.TeamIDs` is what makes this possible for a division's heat —
  without it, a division's heat would have no team list to check
  completeness against, exactly the role `Group.TeamIDs` already plays
  for a round's heat.
- `round.Group`, `round.PrePhaseRound`, and the entire Plan 2 plugin
  engine are untouched by this plan.

## 4. Endpoints

**Courses** (tournament-scoped, organizer-only writes):
```
POST   /api/tournaments/{id}/courses
GET    /api/tournaments/{id}/courses
PATCH  /api/tournaments/{id}/courses/{course_id}
```
`PATCH` accepts any of `name`, `heat_interval_seconds`,
`delay_offset_seconds`. A request that changes `delay_offset_seconds`
triggers a `delay_offset_changed` WebSocket broadcast (§6) after a
successful write; a request that only changes other fields does not
broadcast.

**Scheduling** (organizer-only) — creates `Heat` rows for an
already-created round's groups, or for already-persisted divisions:
```
POST /api/tournaments/{id}/rounds/{round_id}/schedule
  body: {assignments: [{group_id, course_id}, ...]}

POST /api/tournaments/{id}/divisions/schedule
  body: {assignments: [{division_id, course_id}, ...]}
```
Both auto-sequence `PlannedStart` per course: the first heat scheduled
on a course starts at a base time (now, or an explicit
`start_at` the request may supply), each subsequent heat assigned to
that same course in the same call is `HeatIntervalSeconds` later.

Validation, same all-or-nothing discipline as Plan 2's
`handleCreateRound`/`handleNextRound` (validate the whole request before
writing anything, transactional creation): every `course_id` must exist
for this tournament, and every `group_id`/`division_id` must be
unscheduled (no existing `Heat` row) and must belong to this tournament.
The two endpoints differ in *completeness* checking, because a round is
a named, URL-scoped set and a division-schedule request is not: `POST
.../rounds/{round_id}/schedule` additionally requires `assignments` to
cover every group of that specific round exactly once (partial
scheduling of a round is rejected — same spirit as the round's other
all-or-nothing endpoints). `POST .../divisions/schedule` has no such
outer set to complete against — it schedules exactly the `division_id`s
listed in the request, nothing more, nothing less, so a partial batch
(scheduling some of a round's divisions now, the rest later) is valid.

```
PATCH /api/tournaments/{id}/heats/{heat_id}
  body: {planned_start}
```
Manual override of one heat's `PlannedStart`, organizer-only.

**Results** (organizer or time-entry — same `resultsWriter` role pair
Plan 2 established), replaces Plan 2's round-level results endpoint:
```
POST /api/tournaments/{id}/heats/{heat_id}/results
  body: {team_id: {time_seconds} | {status}, ...}
```
Same validate-entire-body-then-write-in-one-transaction discipline and
status validation (`DNF`/`DSQ`/`DNS` only) Plan 2's final review
established, moved from round-scope to heat-scope. A successful write
triggers a `result_submitted` WebSocket broadcast (§6). Once every team
in the heat's group/division has a result, the heat closes; once every
heat in a round is closed, the round closes (as today).

**Divisions** — now persists directly instead of previewing:
```
POST /api/tournaments/{id}/rounds/{round_id}/divisions
```
Unchanged request shape from Plan 2 (`cuts`). Now computes via the
plugin **and** creates the `Division` rows in the same call — matching
how `POST .../next` already both computes reseeding and creates the next
round atomically. No separate preview/confirm step.

**Schedule/standings read** (any authenticated role) — the one new read
endpoint, the backend surface Plan 4's live view consumes:
```
GET /api/tournaments/{id}/schedule
```
Returns every heat (round and division alike) with its course, planned
start, computed effective start, status, and current results, in one
response — no client-side joining across multiple endpoints required.

**WebSocket** (any authenticated role):
```
GET /api/tournaments/{id}/ws?token=<session-token>
```
See §6.

## 5. End-to-End Flow

Unchanged from the platform spec's Core Flow, now fully wired:
create round (Plan 2, unchanged) → **schedule it (new)** → **submit
results per heat (moved from round-level)** → round auto-closes once
every heat closes → `/next` (Plan 2, unchanged) or `/divisions`
(**now persists**) → **schedule the divisions (new)** → **submit
division results (same heat-results endpoint)** → division heats close
→ final standings readable via `GET /schedule`.

## 6. WebSocket Live Broadcast

- **Library:** `github.com/coder/websocket` — pure Go, no CGo, actively
  maintained (the actively-developed successor to the now-deprecated
  `nhooyr.io/websocket`, same author, identical API, confirmed by
  downloading and diffing both package docs), matching the project's
  existing pure-Go dependency constraint (the SQLite driver and Lua
  runtime are both pure Go). Requires Go >= 1.23; this project's `go.mod`
  already pins 1.25.
- **Connection:** `GET /api/tournaments/{id}/ws?token=<session-token>`.
  The token is a query parameter rather than an `Authorization` header
  because browsers cannot set custom headers on a WebSocket handshake —
  Plan 4's client will need this. The token is validated exactly like
  the `Authorization: Bearer` header everywhere else in the API; any
  authenticated role (Organizer, Time Entry, Spectator) may connect.
  Connections are scoped per-tournament: a client only receives events
  for the tournament it connected to.
- **Events broadcast (v1 — only these two):**
  ```json
  {"type": "result_submitted", "heat_id": 42, "results": [{"team_id":"7","time_seconds":121.3,"status":""}]}
  {"type": "delay_offset_changed", "course_id": 3, "delay_offset_seconds": 900}
  ```
  Round/division-computation events are explicitly not broadcast in v1
  (see Non-Goals) — a client notices a new round/division by re-fetching
  `GET /schedule` or the existing round endpoints.
- **Fan-out:** in-process only — a `map[tournamentID]map[*Conn]chan
  Message` guarded by a mutex; one write-pump goroutine per connection
  reading its own buffered channel. No external pub/sub — this is a
  single-process binary, matching the offline-first architecture. A
  slow/stalled client's channel filling up causes that client's message
  to be dropped (and logged), never blocks the broadcaster, other
  clients, or the HTTP request that triggered the broadcast. The feed is
  a convenience layer, not the source of truth: `GET /schedule` always
  has the authoritative current state for a client that needs to catch
  up after a dropped message or a reconnect.
- **Error handling:** a connection that fails token validation closes
  immediately with the standard WebSocket policy-violation close code.
  A write-side failure (client disconnected, buffer full) only affects
  that one connection.

## 7. Testing Approach

Same pattern as Plans 1 and 2 throughout: real SQLite, HTTP tests via
`s.ServeHTTP` with real login, reusing existing test helpers
(`newTestServer`, `loginAs`, `createTestTournament`, the team/round
helpers Plan 2 built).

- **Course/Heat/scheduling**: repo tests against real SQLite for the
  auto-sequencing math (given N groups assigned to 2 courses with
  different intervals, correct `PlannedStart`s result); HTTP tests for
  the schedule endpoints' all-or-nothing validation (unknown course ID,
  incomplete assignment list, etc.), mirroring Plan 2's
  `handleCreateRound`/`handleNextRound` validation test patterns.
- **Heat results**: HTTP tests for the moved validate-then-write,
  upsert/correction, and auto-close-cascades-to-round behavior — same
  shapes Plan 2's `round_results_test.go` already covers, retargeted to
  heat scope, plus a new test confirming a round closes only once every
  one of its heats has closed.
- **Division persistence**: HTTP test confirming `POST .../divisions`
  creates real `Division` rows now (not just a computed response), and
  that a subsequent `POST .../divisions/schedule` can schedule them.
- **WebSocket**: `httptest.NewServer` + `nhooyr.io/websocket`'s dial
  client. At minimum one test driving two connected clients on the same
  tournament, confirming a result submitted by one is visible to the
  other without polling (the exact scenario the platform spec's testing
  section calls out), plus a delay-offset-change broadcast test, plus a
  test confirming a client connected to a *different* tournament
  receives neither.

## 8. Open Questions / Deferred

- Lane-capacity/group-splitting (Group:Heat or Division:Heat as 1:many)
  — deferred, no current requirement, revisit if a real tournament
  format needs it (§2).
- Broadcasting round/division-computation events over WebSocket —
  deferred; not required by the platform spec, and `GET /schedule`
  covers the same need via polling (§2, §6).
- Reconnect/resume semantics for a WebSocket client that missed
  messages while disconnected — not specified beyond "re-fetch
  `GET /schedule`"; a more granular catch-up protocol (e.g. a
  last-seen-sequence-number resume) is left for Plan 4 if the UI needs
  it.
