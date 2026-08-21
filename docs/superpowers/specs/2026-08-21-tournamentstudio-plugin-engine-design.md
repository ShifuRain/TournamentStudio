# TournamentStudio — Plugin Engine & Dragonboat Logic Design

Date: 2026-08-21
Status: Approved for planning
Supersedes: nothing — extends `2026-08-21-tournament-studio-design.md` (as
amended to Lua) with the implementation-level detail needed to build it.

## 1. Overview

This spec covers the second implementation plan for TournamentStudio: the
Lua plugin engine, the `dragonboat` sport plugin, the
`timed-heats-reseeding` tournament-type plugin, and the `PrePhaseRound`/
`Group` data model that plugin computes over. It builds directly on the
Foundation plan (Go server, SQLite store, roles, i18n, Tournament/Team
domain) and is itself a prerequisite for Plan 3 (courses, heats,
scheduling, live result entry) and Plan 4 (live schedule/standings
display).

**Scope boundary with Plan 3:** this plan owns the *grouping* side of the
spec's data model — `PrePhaseRound` and `Group`, and the plugin
computations that operate on them (reseeding, division cuts). It does
**not** build `Course` or `Heat` — the physical scheduling layer — or real
result-entry UI. Results are supplied to this plan's endpoints as a
lightweight `team_id → time | status` map, with no course or clock
involved. Plan 3 later attaches real `Heat`/`Course` records underneath
each `Group`, and real result entry feeds the same computations this plan
builds — the plugin contract does not change when that happens.

## 2. Goals / Non-Goals

**Goals**
- A Lua plugin engine: load `.lua` files from a `plugins/` folder at
  startup, isolate each in its own sandboxed VM, expose a stable
  Go↔Lua data contract, and fail one broken plugin without affecting
  others or crashing the server.
- The `dragonboat` sport plugin (purely declarative) and the
  `timed-heats-reseeding` tournament-type plugin (the reseeding and
  division-cut algorithms) as the first real plugins proving the engine.
- `PrePhaseRound`/`Group` persistence and the endpoints to create a
  round, submit a round's results, and compute the next round's groups or
  the final divisions via the loaded plugin.

**Non-Goals (this plan)**
- `Course`/`Heat` and any scheduling/delay-offset logic — Plan 3.
- Real result-entry UI, DNF/DSQ/DNS entry, WebSocket live broadcast —
  Plan 3.
- A plugin marketplace/browser UI screen — the backend "list installed
  plugins with declared compatibility" endpoint is in scope; the frontend
  screen is Plan 4.
- Any tournament-type plugin other than `timed-heats-reseeding`, any sport
  plugin other than `dragonboat` — deferred indefinitely, same as the
  Foundation plan's non-goals.
- Making the ranking rule (time ascending, DNF/DSQ/DNS last) itself
  pluggable — every timed tournament-type wants the same rule; it's
  generic host-side logic, not plugin logic, in v1.

## 3. Plugin Architecture

- On startup, the server scans a `plugins/` folder for `*.lua` files
  (mirrors the Foundation plan's `languages/` folder pattern for i18n).
  Each file is loaded into its own `*lua.LState` (via `gopher-lua`, pure
  Go, no CGo — same build story as the SQLite driver) and executed once
  to obtain its returned table.
- **Registration:** a plugin table with a `compatible_tournament_types`
  field registers as a **sport plugin**; a table with a
  `compatible_sports` field registers as a **tournament-type plugin**.
  Both require an `id` string field. A file that returns neither, or a
  file that fails to load/execute (syntax error, runtime error at load
  time), is skipped with a logged warning — it does not prevent other
  plugins from loading or crash the server.
- **Isolation:** each plugin's `*lua.LState` is long-lived (one per
  loaded plugin, reused across calls) but guarded by a mutex (gopher-lua
  states are not goroutine-safe) and a per-call instruction budget (via
  gopher-lua's `SetContext`/hook mechanism) so a runaway or malicious
  script can't hang a request indefinitely. Dangerous stdlib (`os`, `io`,
  `require`, `load`, `dofile`) is never registered into these VMs — only
  `string`, `table`, `math`, and the base library minus file/process
  access.
- **Compatibility discovery:** `GET /api/plugins` (any authenticated
  role) returns the list of loaded sport and tournament-type plugins with
  their declared IDs, display names, and compatibility lists — this is
  what a future "Plugins" screen (Plan 4) and the tournament-creation flow
  read to filter valid sport+tournament-type pairs.

## 4. The `dragonboat` Sport Plugin

Purely declarative — no functions. Returns a table:

```lua
return {
  id = "dragonboat",
  display_name = "Dragonboat",
  compatible_tournament_types = {"timed-heats-reseeding"},
  roster_fields = {
    {key = "boat_class", label = "Boat class", required = false},
  },
}
```

`roster_fields` extends the Foundation plan's `team.Team.ExtraFields`
concept — declares which extra fields this sport expects on a team, for
future UI use (Plan 4). Not enforced server-side in this plan (Team
validation stays exactly as Plan 1 built it — this is metadata only).

## 5. The `timed-heats-reseeding` Tournament-Type Plugin

Returns a table with `id`, `compatible_sports = {"dragonboat"}`, and two
functions:

### `next_round_groups(groups)`

**Input:** `groups` — a Lua array of arrays. Each sub-array represents
one current group's teams, **already sorted fastest-first by the host**
(DNF/DSQ/DNS teams sorted after all finishers, in the order the host
determines — see §6). Each entry is `{team_id = <string>, time_seconds =
<number|nil>, status = <string|nil>}`.

**Output:** a new Lua array of arrays of `team_id` strings — the next
round's groups, in the same group count as the input.

**Algorithm — cyclic tier rotation** (generalizes cross-swap-halves to
*K* groups):

Let there be *K* input groups. For each input group *i* (0-indexed) with
*n_i* teams already ranked 1..*n_i* (rank 1 = fastest):

1. Split group *i* into *K* tiers by rank. `tier_size = floor(n_i / K)`,
   `remainder = n_i mod K`.
   - Tier `j` for `j = 0 .. K-2` gets exactly `tier_size` teams: ranks
     `[j*tier_size + 1, (j+1)*tier_size]`.
   - Tier `K-1` (the slowest tier) absorbs the remainder: it gets
     `tier_size + remainder` teams — all remaining ranks through `n_i`.
     (This is also where DNF/DSQ/DNS teams land, since they sort last.)
2. New group `N` (0-indexed, `N = 0 .. K-1`) is the union, over
   `i = 0 .. K-1`, of old group `i`'s tier `((N + i) mod K)`.

This is exactly the confirmed cross-swap-halves example when *K*=2 (tier 0
= fast half, tier 1 = slow half; new group 0 = G1 tier 0 + G2 tier 1; new
group 1 = G1 tier 1 + G2 tier 0). Team count is conserved (every team
appears in exactly one tier of exactly one old group, and every tier is
placed into exactly one new group). New group sizes may differ slightly
between groups if input group sizes/remainders differ — this is expected
and not an error.

**Assumption:** the number of groups *K* is constant across all pre-phase
rounds of a tournament — chosen once when the organizer sets up round 1
(per the Foundation-adjacent spec's "configurable number of pre-phase
rounds," group *membership* changes each round via reseeding, but group
*count* does not, in v1). If this assumption needs to change later, it's
a host-side round-setup decision, not a plugin contract change.

### `division_cuts(ranked_teams, cuts)`

**Input:** `ranked_teams` — a flat Lua array of `team_id` strings in
final rank order (fastest first, DNF/DSQ/DNS last), already computed by
the host from the final pre-phase round's results. `cuts` — a Lua array
of `{name = <string>, size = <number>}`, organizer-supplied via the
triggering API call (see §7), in the order divisions should be filled.

**Output:** a Lua array of `{name = <string>, team_ids = {<string>, ...}}`.

**Algorithm:** fill each named cut in order, consuming that many teams
off the front of `ranked_teams`. If `sum(cuts[*].size) < #ranked_teams`,
the remaining teams (in rank order) form one implicit final division
named `"Final"` (or the next unused ordinal name if `"Final"` collides
with an organizer-supplied name). If `sum(cuts[*].size) >
#ranked_teams`, the last cut that would overflow is truncated to however
many teams remain (never an error — an organizer misconfiguring sizes
shouldn't 500).

## 6. Generic Host-Side Ranking

Not plugin logic — shared Go code used to build the sorted input to both
plugin functions above, and reused by Plan 3/4 for standings display:

- Sort by `time_seconds` ascending among teams with a recorded time.
- Any team with a status (DNF/DSQ/DNS) instead of a time sorts after all
  timed teams, in a fixed order: DNF, then DSQ, then DNS (matches how
  most regattas read a results sheet — a team that started and didn't
  finish outranks one that was disqualified, which outranks one that
  never started). Ties within the same status keep their prior stable
  order (Go's sort is stable; input order is team-ID order).

## 7. Data Model & Endpoints

- `PrePhaseRound{ID, TournamentID, RoundNumber, Status}` — `Status` is
  `"open"` (accepting results) or `"closed"` (results submitted, next
  round or division cuts computed).
- `Group{ID, RoundID, TeamIDs []string}` — stored as a JSON array column
  (same pattern as `Team.ExtraFields` in Plan 1), since group membership
  has no independent identity beyond "which teams, in which group, this
  round."
- `POST /api/tournaments/{id}/rounds` (organizer) — creates round 1;
  body supplies the initial groups (`groups: [[team_id, ...], ...]`) —
  manual assignment is just the caller choosing the grouping; "randomize"
  is a client-side or future convenience that still POSTs explicit
  groups, not a server-side random endpoint in this plan (YAGNI — the
  server doesn't need to own randomization).
- `POST /api/tournaments/{id}/rounds/{round_id}/results` (organizer or
  time-entry) — body is `{team_id: {time_seconds} | {status}, ...}` for
  every team across the round's groups. Closes the round (`Status =
  "closed"`) once every team has a recorded result.
- `POST /api/tournaments/{id}/rounds/{round_id}/next` (organizer) — only
  valid on a closed round; calls the tournament-type plugin's
  `next_round_groups` with the closed round's groups+results (ranked
  per §6), creates and returns the new `PrePhaseRound` + `Group`s.
- `POST /api/tournaments/{id}/rounds/{round_id}/divisions` (organizer) —
  only valid on a closed round; body supplies `cuts` (see §5); calls
  `division_cuts` with the round's final ranking, returns the resulting
  divisions. (Division persistence/entity design is intentionally left
  to Plan 3, where divisions get their own heats — this endpoint returns
  the computed split without yet persisting a `Division` row, since
  nothing consumes that persistence until Plan 3 exists. Recording this
  explicitly so it isn't mistaken for an oversight.)
- `GET /api/plugins` (any authenticated role) — see §3.

## 8. Testing Approach

- **Plugin logic**, direct: load `timed-heats-reseeding.lua` into a bare
  `*lua.LState` in a test, hand-build Lua tables matching §5's input
  shape (including the confirmed 2-group/4-team cross-swap example as a
  golden test), call `next_round_groups`/`division_cuts`, assert on the
  returned tables. Covers uneven group sizes, DNF/DSQ/DNS placement, and
  `division_cuts`' overflow/underflow handling from §5.
- **Plugin logic**, through the host boundary: a smaller set of tests
  going through the actual Go engine code that builds Lua tables from Go
  structs and reads results back, to catch marshaling bugs the direct
  test can't see.
- **Plugin loading**: a test directory with one valid `dragonboat.lua`,
  one valid `timed-heats-reseeding.lua`, and one deliberately malformed
  `.lua` file (syntax error) — asserts the two valid plugins still load
  and register correctly, and the malformed one is skipped with no
  crash. Same defensive posture Plan 1 required for malformed i18n
  language files.
- **Round/Group repo and HTTP handlers**: same pattern as every domain
  package in Plan 1 — repo tests against real SQLite, HTTP tests through
  `s.ServeHTTP` with real login, reusing the `loginAs`/
  `createTestTournament` helpers already in the `server` test package.

## 9. Open Questions / Deferred

- Whether group *count* should ever change mid-tournament (§5's
  assumption) — no current requirement for it; revisit if a real
  tournament format needs it.
- `Division` as a persisted entity with its own heats — explicitly
  deferred to Plan 3 (§7).
- Server-side "randomize round 1 groups" convenience endpoint — deferred,
  YAGNI (§7).
