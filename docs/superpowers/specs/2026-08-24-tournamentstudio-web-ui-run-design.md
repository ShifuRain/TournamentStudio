# TournamentStudio — Web UI Run Design

Date: 2026-08-24
Status: Approved for planning
Builds on: `2026-08-21-tournament-studio-design.md` (Foundation),
`2026-08-21-tournamentstudio-plugin-engine-design.md` (Plan 2),
`2026-08-22-tournamentstudio-scheduling-live-results-design.md` (Plan 3),
`2026-08-23-tournamentstudio-web-ui-foundation-design.md` (Plan 4a)

## 1. Overview

This is "Plan 4b" — the second of three sub-projects decomposing the
originally-scoped "Plan 4" (the full web UI), split by usage phase:

- **4a — Foundation + Setup** (done, merged): frontend scaffolding, auth,
  tournament creation, team management.
- **4b — Run** (this plan): courses, round/group creation, heat
  scheduling, results entry, next-round/division triggers. Every
  operational backend capability from Plans 2 and 3 gets a UI.
- **4c — Watch** (future): the live schedule/standings display
  (WebSocket-driven), the Plugins browser screen.

4a left a disabled "Rounds/Schedule" tab placeholder on the tournament
detail page specifically so this plan could fill it in without reshaping
navigation. This plan fills that slot.

## 2. Goals / Non-Goals

**Goals**
- A single `schedule` screen under `/tournaments/:id/schedule` covering
  the full operational lifecycle: course setup, round/group creation,
  heat scheduling, results entry, and next-round/division triggers —
  fully functional end to end against the real backend endpoints Plans
  2 and 3 already built.
- Role-appropriate UI: organizer sees every control; the `time_entry`
  role sees the same screen with only results entry available (course,
  scheduling, and round-progression controls simply don't render for
  that role); `spectator` sees a read-only view.
- A client-side "auto-split into groups" convenience for round 1 — the
  only round ever created via manual group submission (`POST
  .../rounds`); every later round is always plugin-computed reseeding
  via `POST .../rounds/{round_id}/next`, never a fresh manual group
  list. Since the backend intentionally has no server-side randomizer
  for round 1, the organizer picks a group count, the client shuffles
  the current roster into that many groups, and the result is editable
  (teams can be moved between groups) before submitting.

**Non-Goals (this plan)**
- Live schedule/standings display, WebSocket client, Plugins browser
  screen — Plan 4c.
- Push-based live updates between concurrent users of this screen (e.g.
  a second time-entry operator's result appearing without a refresh) —
  out of scope by the same 4a/4c boundary that deferred the WebSocket
  client entirely to 4c. This screen refetches via TanStack Query
  invalidation after its own writes only, matching every 4a screen's
  existing pattern.
- Automatic detection of "this is the final pre-phase round" — the
  backend has no such signal (no stored pre-phase-round-count field).
  The organizer chooses between "Create Next Round" and "Cut Divisions"
  on any closed round; the UI does not try to guess which applies.
- Client-side re-validation of business rules the backend already
  validates all-or-nothing (duplicate/unknown team IDs, non-existent
  course IDs, incomplete round coverage, DNF/DSQ/DNS-only status
  values). Errors surface from the API response, not duplicated
  client-side logic.
- A drag-and-drop library dependency for the group-editing UI — see §5.

## 3. Backend Addition: `GET /api/tournaments/{id}/rounds`

**The gap:** no endpoint today lets a client discover a tournament's
existing rounds. `POST /api/tournaments/{id}/rounds` returns the created
round in its response body, but nothing lets the UI re-fetch that state —
after a page refresh, in a second browser tab, or when an organizer
returns later to schedule the next round. This isn't a nice-to-have gap;
without it the Schedule screen cannot render anything beyond "no round
yet" on first load after any navigation away and back. `round.Repo` has
`GetRound(id)` and `ListGroups(roundID)` (both require already knowing a
round ID) but no `ListRounds(tournamentID)`.

**The fix:** a new read endpoint, in the same additive spirit as 4a's own
`RosterFields` addition to `GET /api/plugins`:

```
GET /api/tournaments/{id}/rounds   (any authenticated role)
```

Returns every round for the tournament, ordered by `round_number`, each
with its groups and — once cut — its divisions, so the Schedule screen
needs exactly one call to answer "what's this tournament's current
round/group/division structure":

```json
{
  "rounds": [
    {
      "id": 1, "round_number": 1, "status": "closed",
      "groups": [{"id": 1, "team_ids": ["1","2","3"]}, ...],
      "divisions": [{"id": 1, "name": "A", "team_ids": ["1","2"]}, ...]
    }
  ]
}
```

Implementation: `round.Repo.ListRounds(tournamentID) ([]PrePhaseRound, error)`
(new method — a straightforward `SELECT ... WHERE tournament_id = ?
ORDER BY round_number`, same pattern as `schedule.Repo.ListCourses`), a
new handler reusing the existing `roundToResponse` helper (extended with
a `Divisions []divisionResponse` field, populated via the already-
existing `schedule.Repo.ListDivisionsForRound(roundID)` — empty until a
round is cut), and `s.mux.Handle("GET /api/tournaments/{id}/rounds",
authenticated(...))` alongside the other list endpoints in `server.go`.
`divisionResponse` mirrors `groupResponse`'s existing shape (`id`,
`name`, `team_ids`).

## 4. Screen Layout

One route, `/tournaments/:id/schedule`, added as a sibling to `teams` and
`teams/import` under `TournamentDetailPage`'s existing tab strip (which
already links here — 4a left it a disabled placeholder specifically for
this). The "Rounds/Schedule" `NavLink` in `TournamentDetailPage.tsx`
becomes a real link, matching the "Teams" tab's existing pattern.

Sections render top to bottom, each showing only what the tournament's
current state calls for:

1. **Courses panel** — collapsible (collapsed by default once at least
   one course exists). List of courses (name, heat interval, delay
   offset) with inline edit (`PATCH`); a small "Add Course" form
   (`POST`). Visible and editable to organizer only; hidden entirely for
   `time_entry`/`spectator` (not part of a time-entry operator's job,
   and read-only course data doesn't help a spectator on this screen —
   4c's live view is where spectators belong).
2. **Current round card** — the latest round's number and status. If no
   round exists yet: an "Auto-split into groups" control — organizer
   picks a group count, the client shuffles the already-loaded team
   roster (from the same `GET .../teams` query 4a's Teams tab already
   uses) into that many groups, rendered as editable group cards. Teams
   move between groups via a simple "move to group N" `<select>` per
   team row (see §5 for why not drag-and-drop) rather than showing raw
   groups. "Create Round 1" submits `POST .../rounds`. Organizer-only.
3. **Scheduling** — for the current round's groups (or, once cut,
   divisions) that don't yet have a `Heat` (cross-referenced against the
   heat list from `GET .../schedule` by `group_id`/`division_id`): one
   row per unscheduled group/division with a course `<select>`
   (populated from the courses panel's data), batch-submitted to `POST
   .../rounds/{round_id}/schedule` or `POST .../divisions/schedule`.
   Organizer-only.
4. **Heat list + results entry** — every heat belonging to the current
   round or its divisions, each showing its course name, effective start
   time, status, and:
   - a manual start-time override (`PATCH .../heats/{id}`) — organizer-only.
   - if the heat is open: a results form, one row per team in that
     heat's group/division, a time input or a DNF/DSQ/DNS `<select>`,
     submitted as one batch to `POST .../heats/{id}/results`. Available
     to organizer and `time_entry`.
5. **Round actions** — once every heat in the round (or every division's
   heat) is closed, two actions appear: "Create Next Round" (`POST
   .../next`) and "Cut Divisions" (`POST .../divisions`, with a small
   repeatable `{name, size}` row form — add/remove rows; the backend
   already tolerates size mismatches gracefully per Plan 3 §5, so no
   client-side sum validation is needed). Organizer-only.
6. **Round history** — earlier rounds listed below the current one
   (round number, status, team count), collapsed, non-interactive —
   just orientation, not a navigation target.

## 5. Group-Editing Interaction (no drag-and-drop library)

Moving a team between groups is a `<select>` per team row ("currently in
Group 2" → change to "Group 3"), not drag-and-drop. This keeps the
screen consistent with the rest of the app (which uses plain form
controls throughout, no drag-and-drop anywhere yet) and avoids a new
dependency for what both spec review and the group sizes involved (a
few dozen teams at most, per the platform's target scale) don't actually
need — a `<select>` is fully keyboard-accessible and requires no new
library, unlike a drag-and-drop implementation.

## 6. Error Handling

Every mutation renders its `isError` state via the same `role="alert"`
pattern 4a's fix-wave made consistent across every existing form
(`TournamentCreatePage`, `TeamImportPage`, `TeamsTab`). Backend validation
errors (unknown/duplicate team IDs, non-existent course IDs, incomplete
round coverage, invalid result status values) surface as-is from the
API response — no duplicated client-side business-rule validation.

## 7. Testing Approach

- **Component/unit tests**: Vitest + React Testing Library per
  screen/section (course panel, group editor, scheduling form, results
  form, round-actions form), `api` mocked — matching 4a's established
  pattern exactly.
- **End-to-end test**: one new Playwright scenario extending the
  `setup-flow.spec.ts` lineage — starting from a tournament with teams
  already imported (reusing 4a's fixture), create round 1, schedule its
  heats, submit results for every heat, trigger "Create Next Round",
  confirm the new round's groups reflect the reseeding — run against the
  real Go binary + real SQLite + real built frontend, no mocks.
- Both suites run non-interactively, matching 4a's CI-ready invocation.

## 8. Open Questions / Deferred

- 4c's exact task breakdown (live standings/WebSocket/Plugins browser)
  gets its own brainstorm → spec → plan cycle when its turn comes.
- Whether `spectator`-role users should see this screen at all, versus
  being redirected straight to 4c's live view once that exists — left as
  a 4c-time decision; this plan treats `spectator` as read-only on this
  screen rather than blocking it entirely, since 4c doesn't exist yet
  and an operator might reasonably want any authenticated user to at
  least see round/heat state in the meantime.
