# TournamentStudio — Web UI Foundation & Setup Design

Date: 2026-08-23
Status: Approved for planning
Builds on: `2026-08-21-tournament-studio-design.md` (Foundation),
`2026-08-21-tournamentstudio-plugin-engine-design.md` (Plan 2),
`2026-08-22-tournamentstudio-scheduling-live-results-design.md` (Plan 3)

## 1. Overview

This is "Plan 4a" — the first of three sub-projects decomposing the
originally-scoped "Plan 4" (the full web UI), split by usage phase since
the complete UI surface (setup, operational run, live spectator view) is
too large for one spec:

- **4a — Foundation + Setup** (this plan): frontend scaffolding, auth,
  tournament creation, team management. Every backend capability from
  the Foundation plan gets a UI.
- **4b — Run** (future): courses, round scheduling, heat results entry,
  next-round/division triggers. Every backend capability from Plans 2
  and 3's operational endpoints gets a UI.
- **4c — Watch** (future): the live schedule/standings display
  (WebSocket-driven), the Plugins browser screen.

Everything built so far (Plans 1-3) is a JSON/WebSocket API with no
frontend at all — this is the first UI code in the project.

## 2. Goals / Non-Goals

**Goals**
- A React + Vite + TypeScript SPA, built to static assets and embedded
  into the Go binary — no separate frontend server at runtime,
  preserving the project's single-binary, offline-first architecture.
- Login, tournament creation (with sport/type compatibility filtering),
  and team management (manual add + CSV/XLSX import), fully functional
  end to end against the real backend.
- A translation-key layer (`react-i18next`) wired in from the first
  screen, backed by a new `GET /api/i18n/{lang}` endpoint exposing the
  already-built `internal/i18n` catalog.
- The navigational shell (app layout, tournament detail tabs) that 4b
  and 4c will fill in, so this plan's screens sit inside a structure
  that doesn't need reshaping later.

**Non-Goals (this plan)**
- Courses, round scheduling, heat results entry, next-round/division
  triggers — Plan 4b.
- Live schedule/standings display, WebSocket client, Plugins browser
  screen — Plan 4c.
- A preview-before-commit import flow (column mapping, editable
  validation preview) — the real `POST /teams/import` endpoint commits
  immediately and returns `{imported, problems}` after the fact; the UI
  matches that real one-shot behavior rather than the platform spec's
  original (aspirational, never-built) multi-step wizard wording. A real
  preview-before-commit flow would require a new backend endpoint and is
  explicitly deferred, not silently dropped — flagged here so it isn't
  mistaken for an oversight.
- A single `go build` pipeline that also runs `npm run build`
  automatically — documented as a manual two-step process for now
  (`npm run build` in `frontend/`, then `go build`).
- Session token refresh/expiry — matches the backend's current
  permanent-token session model.
- CI configuration — tests are written to be CI-runnable, but no CI
  pipeline exists yet in this project.

## 3. Architecture & Build Pipeline

- New `frontend/` directory at the repo root: a standalone React 18 +
  Vite + TypeScript project, Tailwind CSS for styling, React Router for
  client-side routing, TanStack Query for API data fetching/caching.
- `vite build` outputs to `frontend/dist/`. A new `internal/webui`
  package embeds it via `//go:embed dist/*` (mirroring the existing
  `internal/i18n`/`internal/plugin` embed pattern). `server.go` gets a
  catch-all handler, registered after every `/api/...`, `/healthz`, and
  the WebSocket path: any unmatched request serves the matching embedded
  static asset if one exists at that path, else falls back to
  `dist/index.html` (SPA client-side routing survives a hard refresh on
  any route).
- `frontend/dist/` is git-ignored (build output); the committed
  `frontend/` tree is source only.

## 4. Auth & API Client

- `frontend/src/api/client.ts`: a single typed wrapper around `fetch`
  that reads the session token from `localStorage` (key `ts_token`),
  attaches `Authorization: Bearer <token>` to every request, and
  centralizes error handling — any 401 response clears the stored token
  and redirects to `/login`.
- `POST /login` → `POST /api/login` → store `{token, role}` in
  `localStorage` → redirect to `/tournaments`. Logout: `POST
  /api/logout`, clear storage, redirect to `/login`.
- The stored `role` drives which nav items/actions render (UX
  convenience only — the backend's `requireRole` middleware remains the
  actual enforcement boundary, unchanged).
- No token refresh/expiry logic — matches the backend's current
  permanent-session model.

## 5. i18n Wiring

- **Backend**: `internal/i18n.Catalog` gets two additive exports —
  `Languages() []string` and `Strings(lang string) map[string]string`
  (that language's full flat map, with English's keys merged
  underneath as fallback so a partially-translated language never shows
  a raw key). `main.go` calls `i18n.Load(languagesDir)` (mirroring
  `plugin.Load`'s existing wiring) and passes the catalog into
  `server.New`. New endpoint `GET /api/i18n/{lang}` (no auth required —
  the login screen itself needs translated labels) returns that
  language's map as flat JSON.
- **Frontend**: `react-i18next` with a custom backend loader fetching
  `GET /api/i18n/{lang}` once per language, cached in memory for the
  session. Every string in every 4a screen goes through `t('key')` —
  no hardcoded English strings.
- A UI language switcher in the app shell, persisted to `localStorage`,
  defaulting to `navigator.language` if it matches an available
  language, else English. This is independent of a `Tournament`'s own
  `language` field (metadata about the tournament's primary audience,
  not the logged-in user's UI preference).
- Every task in the implementation plan that adds a screen adds that
  screen's keys to both `internal/i18n/bundles/en.json` and `de.json` in
  the same commit — no screen ships with missing German strings.

## 6. Screens

- **`/login`** — username/password form → `POST /api/login`.
- **App shell** — top nav (tournament list, language switcher, logged-in
  username + role, logout), wraps every authenticated route, gates nav
  items by role.
- **`/tournaments`** — list (`GET /api/tournaments`), each row links to
  its detail page; "Create Tournament" button (Organizer only).
- **`/tournaments/new`** — create form: name, language, sport plugin,
  tournament type. Sport/type dropdowns come from `GET /api/plugins`,
  filtered to compatible pairs only (per the platform spec: "UI filters
  to valid pairs using declared compatibility IDs"). → `POST
  /api/tournaments` → redirect to the new tournament's detail page.
- **`/tournaments/:id`** — detail page with a tab strip: **Teams**
  (functional in 4a), **Rounds/Schedule** and **Live Standings**
  (visible-but-disabled placeholders, so 4b/4c fill in an existing slot
  rather than reshaping navigation).
- **Teams tab** — list (`GET .../teams`); manual "Add Team" form (`POST
  .../teams` — name, club, plus the sport plugin's declared
  `roster_fields` as extra fields); import flow (file picker → `POST
  .../teams/import` → results screen: N imported, a list of any per-row
  problems — one-shot, matching the real backend behavior per §2's
  Non-Goals).

  `GET /api/plugins`'s response currently omits `roster_fields`
  entirely (`pluginSportResponse` only carries `id`/`display_name`/
  `compatible_tournament_types`), even though the Foundation spec
  explicitly earmarked `roster_fields` "for future UI use (Plan 4)."
  This plan adds a small, additive backend change: `plugin.RosterField`
  gets `json` tags (`key`/`label`/`required`) directly on the domain
  struct — the same pattern `plugin.Cut`/`plugin.Division` already
  establish, reusing the domain type in the response rather than a
  parallel DTO — and `pluginSportResponse` gets a
  `RosterFields []plugin.RosterField` field populated from
  `plugin.SportPlugin.RosterFields`. Found while grounding this design
  against the real endpoint, not assumed from the spec's aspirational
  wording.

## 7. Testing Approach

- **Component/unit tests**: Vitest + React Testing Library for every
  screen and shared component (forms, tables, the API client's
  error/redirect handling), `fetch` mocked.
- **End-to-end tests**: Playwright, against a real built Go binary (real
  SQLite, real migrations) serving the real built frontend — no mocks,
  matching this project's established "test through the real stack"
  philosophy. At minimum: login → create tournament → add a team
  manually → import a CSV with one deliberately bad row → see it listed
  with its problem. This is the first true end-to-end (real browser)
  test in the project — every prior test stopped at the HTTP/WS layer.
- Both suites run non-interactively (`npm test`, `npx playwright test
  --reporter=line`), CI-runnable even though no CI pipeline exists yet.

## 8. Open Questions / Deferred

- Preview-before-commit import flow (§2) — would need a new backend
  endpoint; deferred, not dropped.
- Single-command build pipeline (`go build` also running `npm run
  build`) — manual two-step process for now.
- Token refresh/expiry — out of scope, matches current backend session
  model.
- 4b's exact task breakdown (courses/scheduling/results UI) and 4c's
  (live standings/WebSocket/Plugins browser) — each gets its own
  brainstorm → spec → plan cycle when its turn comes.
