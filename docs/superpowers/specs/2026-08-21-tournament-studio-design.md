# TournamentStudio — v1 Design

Date: 2026-08-21
Status: Approved for planning

## 1. Overview

TournamentStudio is a free/open-source, offline-first application that lets
sports clubs organize and run tournaments. It is modular along two axes —
**sport** and **tournament type** — via a WASM plugin system, and is
multi-lingual (English + German built in, more via drop-in translation
files).

This spec covers the **first vertical slice**: the core platform plus a
Dragonboat sport plugin and a "Timed Heats with Reseeding + Multi-Division
Finals" tournament-type plugin. Everything needed to run a real dragonboat
regatta end-to-end — from tournament creation through team import,
qualification heats, reseeding, final divisions, and a live/printable
schedule — is in scope. Additional sports, tournament types (including any
knockout/elimination format), and a plugin marketplace are explicitly future
work.

## 2. Goals / Non-Goals

**Goals**
- Runs fully offline, on all major desktop OSes, as a single binary.
- Works for a single organizer on one laptop, *or* scales to multiple
  devices on a club's local WiFi (timekeepers, organizer, spectator
  screens) — same binary, no separate deployment mode.
- Sport- and tournament-type-specific logic is pluggable (WASM), not
  hardcoded in core.
- Attendees can always see an up-to-date schedule/standings view, live and
  printable.
- Non-developers can customize meaningful things (language, plugins,
  roles) by editing/dropping files, documented in the README.

**Non-Goals (v1)**
- No knockout/elimination tournament-type (deferred).
- No sports beyond Dragonboat (deferred).
- No plugin marketplace/download UI — plugins are installed by dropping a
  `.wasm` file into a folder.
- No internet/cloud sync or hosted accounts.
- No native mobile apps (the browser-based UI covers phones/tablets).

## 3. Architecture

- **Single Go binary**, cross-compiled per OS/arch, no external runtime
  dependency.
  - Embedded HTTP server (REST + WebSocket for live updates) serving a
    web-based SPA UI.
  - Embedded SQLite (pure-Go driver, no CGo) as the single-file datastore —
    trivial to back up (copy one file) and fully offline.
  - Embedded WASM runtime (pure Go, e.g. `wazero`) hosting sport and
    tournament-type plugins.
- **Deployment is a non-decision at the code level**: running on one
  laptop and opening `localhost` in a browser, versus running on one
  machine and having other devices join via its LAN IP, is the exact same
  server — no separate single-device/multi-device code paths.
- **Roles & access**: local accounts (no external auth provider) with one
  of three roles — **Organizer** (full control), **Time Entry** (can only
  submit/edit race results), **Spectator** (read-only). Enforced
  server-side on every write endpoint, not just hidden in the UI.
- **Error isolation**: a plugin (sport or tournament-type) runs sandboxed
  in its WASM instance; a plugin panic/trap is caught at the host boundary
  and surfaced as an error to the organizer without crashing the server or
  affecting other tournaments/plugins running in the same process.
- **Concurrency**: result submissions are last-write-wins per
  team-per-heat, broadcast to all connected clients over WebSocket so the
  schedule/standings view and other Time Entry devices update immediately.
  A default tie-breaking rule (fastest single recorded time wins) is
  documented and adjustable later; not configurable in v1.

## 4. Plugin System

Two independent plugin axes, both implemented as WASM modules loaded from
a `plugins/` folder at startup:

- **Sport plugins** (e.g. `dragonboat`) declare: an ID, display
  terminology (translated via the i18n layer), sport-specific team/roster
  fields (e.g. boat class, crew size), and a list of compatible
  tournament-type plugin IDs.
- **Tournament-type plugins** (e.g. `timed-heats-reseeding`) declare: an
  ID, a list of compatible sport plugin IDs, and implement the
  competition-structure logic (grouping, reseeding, division cuts) against
  a generic host interface — the plugin never sees "dragonboat" directly,
  only teams, recorded results, and round/division structure.
- Compatibility is declared by ID on **both** sides and is browsable
  in-app via a "Plugins" screen listing installed plugins and what they
  declare support for — this is how an organizer discovers valid
  sport + tournament-type pairings when creating a tournament.
- **Language plugins** are *not* WASM — they're JSON translation-key
  bundles dropped into a `languages/` folder. No code execution, so a
  non-programmer can contribute or edit a translation.

## 5. Data Model

- `Tournament` — name, sport plugin ID, tournament-type plugin ID, status,
  language.
- `Team` — name, club, roster fields (core fields + sport-plugin-declared
  extra fields); created via import or manual entry against the identical
  schema/validation.
- `Course` — a physical race track that runs heats sequentially; holds a
  **running delay offset** (e.g. "Course A: +15 min") that the organizer
  updates live.
- `PrePhaseRound` → `Group`s → `Heat`s.
  - Round 1 groups: organizer chooses manual assignment or randomize.
  - Round N>1 groups: computed by the tournament-type plugin from round
    N-1 results, applying the reseeding rule (cross-group: slowest
    qualifiers from one group paired against fastest of another, so no
    heat is uniformly fast or slow). Organizer reviews/confirms before
    it's published to the schedule.
  - Number of pre-phase rounds is configurable per tournament.
- `Heat` — belongs to a group (pre-phase) or division (finals); assigned
  to a `Course`; has a planned start timestamp (auto-sequenced by course +
  a configurable heat interval, manually overridable); the *displayed/
  effective* time is planned time + that course's current delay offset;
  lane assignments; one `Result` per team.
- `Result` — either a time, or a status: DNF / DSQ / DNS. Statuses always
  rank worse than any finishing time and are excluded cleanly from
  reseeding/ranking math.
- **Final phase (no elimination)**: once pre-phase rounds complete, the
  tournament-type plugin computes a ranked list from final pre-phase
  results and splits it into multiple **Divisions** (e.g. Gold/Silver/
  Bronze Final) per organizer-configured cut lines. Each division is
  structured like a pre-phase group — its teams race together (one or
  more heats) and are ranked by that result. This is the terminal stage;
  there is no elimination and every team keeps racing until their
  division's final heat(s).

## 6. Core Flow

1. Organizer creates a tournament, picking a sport plugin + a compatible
   tournament-type plugin (UI filters to valid pairs using the declared
   compatibility IDs).
2. Teams are added via CSV/XLSX import (upload → column-mapping step →
   validation preview with errors highlighted → commit) or manual entry
   through a form using the identical schema.
3. Organizer configures the pre-phase: number of rounds, round-1 grouping
   (manual or random), courses, heat interval → a schedule is generated
   with planned per-course timestamps.
4. Time Entry role records results live per heat; the schedule/standings
   view and all connected devices update immediately over WebSocket.
   Organizer can nudge a course's delay offset at any time, live-shifting
   all of that course's downstream displayed times.
5. Once a round's heats are all resolved, the tournament-type plugin
   proposes the next round's reseeded groups; organizer confirms; the next
   round's heats + schedule are generated.
6. After the final pre-phase round, the plugin proposes division cuts;
   organizer confirms; division heats are scheduled like any other round.
7. Division results produce final standings per division — shown on the
   always-visible schedule/standings view, which remains the source of
   truth for attendees throughout the whole event (not just at the end).

## 7. Display & Print

The always-visible, role-agnostic **schedule/standings view** replaces
the "bracket tree" concept from the original ask (there is no bracket in
this tournament type). It shows, filterable by course/round/division:
upcoming heats with live-adjusted times, lane assignments, and current
standings. A "Print" action generates a clean print-CSS/PDF snapshot for
posting on a physical board.

## 8. Internationalization

All UI strings and plugin-declared strings route through a
translation-key layer. English and German bundles ship in core. A club
adds a language by dropping a JSON file into `languages/` — no rebuild,
no code.

## 9. Customization for Non-Developers

Documented in the README as copy-a-file operations, not build steps:
- Add/edit a language: drop/edit a JSON file in `languages/`.
- Install a community plugin: drop a `.wasm` file into `plugins/`; it
  appears on the in-app "Plugins" screen.
- Assign device roles: create a local account per role from the
  Organizer's admin screen.
- Back up/restore data: copy the single SQLite file.

## 10. Testing Approach

- **Tournament-type plugin logic** (reseeding, division cuts) is the
  highest-value surface for unit tests, since it's pure computation over
  teams/results — testable independently of the server or WASM host via
  a native (non-WASM) build target used only in tests, plus a smaller set
  of tests running the actual compiled `.wasm` through the host interface
  to catch host-boundary issues.
- **Import/validation** gets table-driven tests over malformed/edge-case
  CSV and XLSX fixtures (missing fields, encoding issues, empty rows).
- **Scheduling/delay math** (planned time + offset → effective time,
  cascading to a course's remaining heats) gets unit tests independent of
  the UI.
- **Role enforcement** gets integration tests hitting each write endpoint
  as each role, asserting Spectator/Time Entry are correctly blocked from
  out-of-scope actions.
- **Multi-device live update** gets at least one integration test driving
  two WebSocket clients against one server instance, confirming a result
  submitted by one is visible to the other without refresh.
- UI/browser-level testing is manual for v1 (no automated end-to-end
  suite planned in this spec).

## 11. Open Questions / Deferred

- Tie-breaking beyond the default (fastest single time) — deferred,
  flagged as adjustable later.
- Multi-course support is in scope for v1 (confirmed), but cross-course
  scheduling optimization (e.g. auto-balancing heats across courses) is
  not — course assignment is organizer-driven.
- Plugin marketplace/discovery beyond the local `plugins/` folder —
  future work.
