# Web UI Run (Plan 4b) Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Fill the "Rounds & Schedule" tab (left disabled by Plan 4a) with a
full operational UI covering courses, round/group creation, heat
scheduling, results entry, and next-round/division triggers — every
backend capability Plans 2 and 3 already built.

**Architecture:** One route, `/tournaments/:id/schedule`, rendering a
single `SchedulePage` that hydrates from three read endpoints
(`GET .../rounds` — new, `GET .../courses`, `GET .../schedule`) via
TanStack Query, and composes five focused section components (Courses,
Round Create, Scheduling, Heats/Results, Round Actions/History). Every
mutation invalidates the shared query keys the sections read from — no
WebSocket, no push updates, matching 4a's established pattern exactly.

**Tech Stack:** Go 1.25 / `net/http` / `modernc.org/sqlite` (backend);
React 19 / Vite / TypeScript strict / Tailwind CSS 4 / TanStack Query 5 /
react-i18next (frontend) — all already wired up by Plan 4a, no new
dependencies.

**Spec:** `docs/superpowers/specs/2026-08-24-tournamentstudio-web-ui-run-design.md`

## Global Constraints

- No new frontend dependencies (no drag-and-drop library — see spec §5).
- No WebSocket client in this plan — every screen refetches via TanStack
  Query invalidation after its own writes only (spec §2 Non-Goals).
- Every mutation's error state renders via the existing `role="alert"`
  pattern (`<p role="alert" className="text-sm text-red-600">{t('...')}</p>`)
  — no client-side re-validation of rules the backend already validates
  all-or-nothing.
- i18n keys are flat snake_case, added to **both**
  `internal/i18n/bundles/en.json` and `internal/i18n/bundles/de.json` in
  the same commit, parity verified after every task that adds keys.
- Organizer-only controls (course edit, round create, scheduling, heat
  start override, next-round/divisions) render only when
  `useAuth().role === 'organizer'`. Results entry is available to
  `organizer` and `time_entry`. `spectator` sees a read-only view (no
  forms/buttons render, only data).
- Frontend tests: Vitest + React Testing Library, `api` mocked, matching
  every existing `*.test.tsx` in `frontend/src/pages/`.
- Backend tests: real SQLite via `newTestServer(t)` / `newTestStore(t)`,
  HTTP-level via `httptest`, matching every existing `*_test.go` in
  `internal/server/` and `internal/round/`.
- TypeScript strict mode must stay clean (`npx tsc -b`), Vitest suite
  must stay green (`npm test -- --run`) after every task.

---

### Task 1: Backend — `GET /api/tournaments/{id}/rounds`

**Files:**
- Modify: `internal/round/repo.go` (add `ListRounds`)
- Modify: `internal/round/repo_test.go` (test `ListRounds`)
- Modify: `internal/server/handlers_round.go` (extend `roundResponse`/`roundToResponse`, add `handleListRounds`)
- Modify: `internal/server/handlers_round_next.go:95` (update `roundToResponse` call site)
- Modify: `internal/server/server.go` (register route)
- Create: `internal/server/round_list_test.go`

**Interfaces:**
- Consumes: `round.PrePhaseRound{ID, TournamentID, RoundNumber, Status}`,
  `round.Group{ID, RoundID, TeamIDs}`, `round.Repo.ListGroups(roundID int64) ([]round.Group, error)`,
  `schedule.Division{ID, TournamentID, RoundID, Name, TeamIDs}`,
  `schedule.Repo.ListDivisionsForRound(roundID int64) ([]schedule.Division, error)`
  (all already exist), and the existing `divisionResponse{ID, Name, TeamIDs}`
  type already defined in `internal/server/handlers_round_divisions.go`.
- Produces: `round.Repo.ListRounds(tournamentID int64) ([]round.PrePhaseRound, error)`;
  `roundToResponse(pr *round.PrePhaseRound, groups []round.Group, divisions []schedule.Division) roundResponse`
  (signature now takes a third argument — every existing call site must
  be updated); HTTP route `GET /api/tournaments/{id}/rounds` (any
  authenticated role) returning `{"rounds": [roundResponse, ...]}`, each
  `roundResponse` now carrying a `"divisions"` field. Frontend Task 2
  consumes this response shape directly.

- [ ] **Step 1: Write the failing repo test**

Add to `internal/round/repo_test.go`:

```go
func TestListRoundsOrdersByRoundNumber(t *testing.T) {
	s := newTestStore(t)
	tournamentID := seedTournament(t, s)
	repo := NewRepo(s)

	if _, _, err := repo.CreateRound(tournamentID, 2, [][]string{{"t3", "t4"}}); err != nil {
		t.Fatalf("CreateRound 2: %v", err)
	}
	if _, _, err := repo.CreateRound(tournamentID, 1, [][]string{{"t1", "t2"}}); err != nil {
		t.Fatalf("CreateRound 1: %v", err)
	}

	rounds, err := repo.ListRounds(tournamentID)
	if err != nil {
		t.Fatalf("ListRounds: %v", err)
	}
	if len(rounds) != 2 {
		t.Fatalf("expected 2 rounds, got %d", len(rounds))
	}
	if rounds[0].RoundNumber != 1 || rounds[1].RoundNumber != 2 {
		t.Fatalf("expected rounds ordered 1, 2 regardless of creation order; got %d, %d", rounds[0].RoundNumber, rounds[1].RoundNumber)
	}
}

func TestListRoundsEmptyForTournamentWithNoRounds(t *testing.T) {
	s := newTestStore(t)
	tournamentID := seedTournament(t, s)
	repo := NewRepo(s)

	rounds, err := repo.ListRounds(tournamentID)
	if err != nil {
		t.Fatalf("ListRounds: %v", err)
	}
	if len(rounds) != 0 {
		t.Fatalf("expected 0 rounds, got %d", len(rounds))
	}
}
```

- [ ] **Step 2: Run the tests to verify they fail**

Run: `go test ./internal/round/... -run TestListRounds -v`
Expected: FAIL with `repo.ListRounds undefined (type *Repo has no field or method ListRounds)`

- [ ] **Step 3: Implement `ListRounds`**

Add to `internal/round/repo.go`, after `RoundExists`:

```go
// ListRounds returns every round for the tournament, ordered by round
// number, for callers that need to discover a tournament's round
// history rather than already knowing a specific round ID (e.g. the web
// UI's Schedule screen after a page refresh, or in a fresh browser tab).
func (r *Repo) ListRounds(tournamentID int64) ([]PrePhaseRound, error) {
	rows, err := r.db.Query(
		`SELECT id, tournament_id, round_number, status FROM pre_phase_rounds WHERE tournament_id = ? ORDER BY round_number`,
		tournamentID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var rounds []PrePhaseRound
	for rows.Next() {
		var pr PrePhaseRound
		var status string
		if err := rows.Scan(&pr.ID, &pr.TournamentID, &pr.RoundNumber, &status); err != nil {
			return nil, err
		}
		pr.Status = Status(status)
		rounds = append(rounds, pr)
	}
	return rounds, rows.Err()
}
```

- [ ] **Step 4: Run the tests to verify they pass**

Run: `go test ./internal/round/... -run TestListRounds -v`
Expected: PASS (both tests)

- [ ] **Step 5: Extend `roundResponse`/`roundToResponse` with divisions**

In `internal/server/handlers_round.go`, add the `schedule` import and
replace the `roundResponse` struct and `roundToResponse` function:

```go
import (
	"encoding/json"
	"net/http"
	"strconv"

	"tournamentstudio/internal/round"
	"tournamentstudio/internal/schedule"
	"tournamentstudio/internal/tournament"
)

type groupResponse struct {
	ID      int64    `json:"id"`
	TeamIDs []string `json:"team_ids"`
}

type roundResponse struct {
	ID          int64              `json:"id"`
	RoundNumber int                `json:"round_number"`
	Status      string             `json:"status"`
	Groups      []groupResponse    `json:"groups"`
	Divisions   []divisionResponse `json:"divisions"`
}

func roundToResponse(pr *round.PrePhaseRound, groups []round.Group, divisions []schedule.Division) roundResponse {
	gr := make([]groupResponse, 0, len(groups))
	for _, g := range groups {
		gr = append(gr, groupResponse{ID: g.ID, TeamIDs: g.TeamIDs})
	}
	dr := make([]divisionResponse, 0, len(divisions))
	for _, d := range divisions {
		dr = append(dr, divisionResponse{ID: d.ID, Name: d.Name, TeamIDs: d.TeamIDs})
	}
	return roundResponse{
		ID:          pr.ID,
		RoundNumber: pr.RoundNumber,
		Status:      string(pr.Status),
		Groups:      gr,
		Divisions:   dr,
	}
}
```

Update `handleCreateRound`'s existing call site (currently
`json.NewEncoder(w).Encode(roundToResponse(pr, groups))`) to:

```go
	json.NewEncoder(w).Encode(roundToResponse(pr, groups, nil))
```

(A freshly created round 1 never has divisions yet.)

- [ ] **Step 6: Update the other existing call site**

In `internal/server/handlers_round_next.go:95`, change:

```go
	json.NewEncoder(w).Encode(roundToResponse(nextPR, nextGroups))
```

to:

```go
	json.NewEncoder(w).Encode(roundToResponse(nextPR, nextGroups, nil))
```

(A round just created via `/next` also never has divisions yet.)

- [ ] **Step 7: Run the existing round tests to confirm nothing broke**

Run: `go test ./internal/server/... -run 'Round|Division' -v`
Expected: PASS (all existing round/division tests still pass with the
extended response shape — `TestComputeDivisionsSplitsRankedTeams` and
friends don't assert on the exact top-level fields of `roundResponse`,
only on `POST .../divisions`'s own response, which is untouched)

- [ ] **Step 8: Add the `handleListRounds` handler**

Append to `internal/server/handlers_round.go`:

```go
func (s *Server) handleListRounds(w http.ResponseWriter, r *http.Request) {
	tournamentID, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		http.Error(w, "invalid tournament id", http.StatusBadRequest)
		return
	}

	rounds, err := s.rounds.ListRounds(tournamentID)
	if err != nil {
		http.Error(w, "could not list rounds", http.StatusInternalServerError)
		return
	}

	resp := make([]roundResponse, 0, len(rounds))
	for i := range rounds {
		pr := &rounds[i]
		groups, err := s.rounds.ListGroups(pr.ID)
		if err != nil {
			http.Error(w, "could not list groups", http.StatusInternalServerError)
			return
		}
		divisions, err := s.schedule.ListDivisionsForRound(pr.ID)
		if err != nil {
			http.Error(w, "could not list divisions", http.StatusInternalServerError)
			return
		}
		resp = append(resp, roundToResponse(pr, groups, divisions))
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]any{"rounds": resp})
}
```

- [ ] **Step 9: Register the route**

In `internal/server/server.go`, add after the existing
`GET /api/tournaments/{id}` registration:

```go
	s.mux.Handle("GET /api/tournaments/{id}/rounds", authenticated(http.HandlerFunc(s.handleListRounds)))
```

- [ ] **Step 10: Write the failing HTTP-level tests**

Create `internal/server/round_list_test.go`:

```go
package server

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"tournamentstudio/internal/auth"
)

func TestListRoundsHTTPReturnsGroupsAndEmptyDivisions(t *testing.T) {
	s := newTestServer(t)
	token := loginAs(t, s, "organizer1", "pw", auth.RoleOrganizer)
	tournamentID := createTestTournament(t, s, token)
	roundID, _ := createTestRound(t, s, token, tournamentID, [][]string{{"t1", "t2"}, {"t3", "t4"}})

	req := httptest.NewRequest(http.MethodGet, fmt.Sprintf("/api/tournaments/%d/rounds", tournamentID), nil)
	req.Header.Set("Authorization", "Bearer "+token)
	rec := httptest.NewRecorder()
	s.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}

	var resp struct {
		Rounds []struct {
			ID        int64 `json:"id"`
			Groups    []struct {
				ID      int64    `json:"id"`
				TeamIDs []string `json:"team_ids"`
			} `json:"groups"`
			Divisions []struct {
				Name string `json:"name"`
			} `json:"divisions"`
		} `json:"rounds"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(resp.Rounds) != 1 {
		t.Fatalf("expected 1 round, got %d", len(resp.Rounds))
	}
	if resp.Rounds[0].ID != roundID {
		t.Fatalf("expected round id %d, got %d", roundID, resp.Rounds[0].ID)
	}
	if len(resp.Rounds[0].Groups) != 2 {
		t.Fatalf("expected 2 groups, got %d", len(resp.Rounds[0].Groups))
	}
	if len(resp.Rounds[0].Divisions) != 0 {
		t.Fatalf("expected 0 divisions before any /divisions call, got %d", len(resp.Rounds[0].Divisions))
	}
}

func TestListRoundsHTTPIncludesDivisionsAfterCut(t *testing.T) {
	s := newTestServer(t)
	token := loginAs(t, s, "organizer1", "pw", auth.RoleOrganizer)
	tournamentID := createTestTournament(t, s, token)
	roundID, ids := createTestRound(t, s, token, tournamentID, [][]string{{"t1", "t2"}})

	submitHeatResultsForRound(t, s, token, tournamentID, roundID, ids,
		"t1", map[string]any{"time_seconds": 100.0},
		"t2", map[string]any{"time_seconds": 110.0},
	)

	divisionsBody, _ := json.Marshal(map[string]any{
		"cuts": []map[string]any{{"name": "Gold", "size": 1}},
	})
	divReq := httptest.NewRequest(http.MethodPost, fmt.Sprintf("/api/tournaments/%d/rounds/%d/divisions", tournamentID, roundID), jsonReader(divisionsBody))
	divReq.Header.Set("Authorization", "Bearer "+token)
	divRec := httptest.NewRecorder()
	s.ServeHTTP(divRec, divReq)
	if divRec.Code != http.StatusOK {
		t.Fatalf("expected 200 creating divisions, got %d: %s", divRec.Code, divRec.Body.String())
	}

	req := httptest.NewRequest(http.MethodGet, fmt.Sprintf("/api/tournaments/%d/rounds", tournamentID), nil)
	req.Header.Set("Authorization", "Bearer "+token)
	rec := httptest.NewRecorder()
	s.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}

	var resp struct {
		Rounds []struct {
			Divisions []struct {
				Name string `json:"name"`
			} `json:"divisions"`
		} `json:"rounds"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(resp.Rounds) != 1 || len(resp.Rounds[0].Divisions) != 2 {
		t.Fatalf("expected 1 round with 2 divisions (Gold + implicit Final), got %+v", resp.Rounds)
	}
}

func TestListRoundsHTTPEmptyForTournamentWithNoRounds(t *testing.T) {
	s := newTestServer(t)
	token := loginAs(t, s, "organizer1", "pw", auth.RoleOrganizer)
	tournamentID := createTestTournament(t, s, token)

	req := httptest.NewRequest(http.MethodGet, fmt.Sprintf("/api/tournaments/%d/rounds", tournamentID), nil)
	req.Header.Set("Authorization", "Bearer "+token)
	rec := httptest.NewRecorder()
	s.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}

	var resp struct {
		Rounds []any `json:"rounds"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(resp.Rounds) != 0 {
		t.Fatalf("expected 0 rounds, got %d", len(resp.Rounds))
	}
}
```

`jsonReader` doesn't exist yet in this package — add it as a tiny local
helper right above `TestListRoundsHTTPIncludesDivisionsAfterCut` in the
same file (every other test file constructs its request bodies with
`bytes.NewReader`, but this file has no other need for the `bytes`
import; using it directly is simpler than adding an unnecessary helper —
replace the `jsonReader(divisionsBody)` call above with
`bytes.NewReader(divisionsBody)` and add `"bytes"` to the import block
instead of introducing a new helper function).

- [ ] **Step 11: Run the tests to verify they fail, then pass**

Run: `go test ./internal/server/... -run TestListRoundsHTTP -v`
Expected first: FAIL (`s.handleListRounds` / route not found — actually
since Steps 8-9 already added the handler and route in this same task,
this file should compile and the tests should already pass on first
run; if you're following strict TDD, temporarily comment out the route
registration from Step 9, confirm a 404, then restore it and confirm
200)
Expected after: PASS (all three tests)

- [ ] **Step 12: Run the full Go suite**

Run: `go build ./... && go test ./... -count=1`
Expected: all packages pass, 0 failures

- [ ] **Step 13: Commit**

```bash
git add internal/round/repo.go internal/round/repo_test.go \
  internal/server/handlers_round.go internal/server/handlers_round_next.go \
  internal/server/server.go internal/server/round_list_test.go
git commit -m "feat: add GET /api/tournaments/{id}/rounds endpoint"
```

---

### Task 2: Frontend — types, route wiring, and the Courses section

**Files:**
- Modify: `frontend/src/api/types.ts` (add `Course`, `Group`, `DivisionInfo`, `Round`)
- Modify: `frontend/src/App.tsx` (add `schedule` route)
- Modify: `frontend/src/pages/TournamentDetailPage.tsx` (make the "Rounds & Schedule" tab a real link)
- Modify: `frontend/src/pages/TournamentDetailPage.test.tsx` (update for the new link)
- Create: `frontend/src/pages/SchedulePage.tsx`
- Create: `frontend/src/pages/SchedulePage.test.tsx`
- Create: `frontend/src/pages/ScheduleCourses.tsx`
- Create: `frontend/src/pages/ScheduleCourses.test.tsx`
- Modify: `internal/i18n/bundles/en.json`, `internal/i18n/bundles/de.json`

**Interfaces:**
- Consumes: `api.get`/`api.post`/`api.patch` from `../api/client` (Task
  1's endpoint response shape); `useAuth()` from `../auth/AuthContext`
  (returns `{role: Role | null, ...}`).
- Produces: `Course{id, tournament_id, name, heat_interval_seconds, delay_offset_seconds}`,
  `Group{id, team_ids}`, `DivisionInfo{id, name, team_ids}`,
  `Round{id, round_number, status, groups: Group[], divisions: DivisionInfo[]}`
  in `frontend/src/api/types.ts` — every later task imports these.
  `SchedulePage` component (default export, mounted at
  `/tournaments/:id/schedule`) owning three `useQuery` calls with keys
  `['rounds', id]`, `['courses', id]`, `['schedule', id]` — every later
  task's section component reads data these queries produce and
  invalidates these exact keys after its own mutations. `ScheduleCourses`
  component (named export, no props — owns `useParams`, its own query on
  `['courses', id]`, and its own create/update mutations).

- [ ] **Step 1: Add the new types**

Append to `frontend/src/api/types.ts`:

```ts
export interface Course {
  id: number
  tournament_id: number
  name: string
  heat_interval_seconds: number
  delay_offset_seconds: number
}

export interface Group {
  id: number
  team_ids: string[]
}

export interface DivisionInfo {
  id: number
  name: string
  team_ids: string[]
}

export interface Round {
  id: number
  round_number: number
  status: string
  groups: Group[]
  divisions: DivisionInfo[]
}
```

- [ ] **Step 2: Add i18n keys for the Courses section**

Add to `internal/i18n/bundles/en.json` (before the closing `}`):

```json
  "schedule_courses_title": "Courses",
  "schedule_courses_name": "Name",
  "schedule_courses_interval": "Heat interval (seconds)",
  "schedule_courses_offset": "Delay offset (seconds)",
  "schedule_courses_add_submit": "Add Course",
  "schedule_courses_add_error": "Could not add the course. Please try again.",
  "schedule_courses_update_error": "Could not update the course. Please try again."
```

Add the same keys with German values to `internal/i18n/bundles/de.json`:

```json
  "schedule_courses_title": "Bahnen",
  "schedule_courses_name": "Name",
  "schedule_courses_interval": "Heat-Abstand (Sekunden)",
  "schedule_courses_offset": "Verzögerung (Sekunden)",
  "schedule_courses_add_submit": "Bahn hinzufügen",
  "schedule_courses_add_error": "Die Bahn konnte nicht hinzugefügt werden. Bitte versuchen Sie es erneut.",
  "schedule_courses_update_error": "Die Bahn konnte nicht aktualisiert werden. Bitte versuchen Sie es erneut."
```

- [ ] **Step 3: Write `ScheduleCourses`**

Create `frontend/src/pages/ScheduleCourses.tsx`:

```tsx
import { useState, type FormEvent } from 'react'
import { useParams } from 'react-router-dom'
import { useTranslation } from 'react-i18next'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { api } from '../api/client'
import { useAuth } from '../auth/AuthContext'
import type { Course } from '../api/types'

export function ScheduleCourses() {
  const { t } = useTranslation()
  const { id } = useParams<{ id: string }>()
  const { role } = useAuth()
  const queryClient = useQueryClient()

  const { data } = useQuery({
    queryKey: ['courses', id],
    queryFn: () => api.get<{ courses: Course[] }>(`/api/tournaments/${id}/courses`),
    enabled: !!id,
  })
  const courses = data?.courses ?? []

  const [name, setName] = useState('')
  const [intervalSeconds, setIntervalSeconds] = useState(300)

  const createMutation = useMutation({
    mutationFn: () =>
      api.post<Course>(`/api/tournaments/${id}/courses`, { name, heat_interval_seconds: intervalSeconds }),
    onSuccess: () => {
      void queryClient.invalidateQueries({ queryKey: ['courses', id] })
      setName('')
      setIntervalSeconds(300)
    },
  })

  const updateMutation = useMutation({
    mutationFn: (vars: { courseId: number; delayOffsetSeconds: number }) =>
      api.patch<Course>(`/api/tournaments/${id}/courses/${vars.courseId}`, {
        delay_offset_seconds: vars.delayOffsetSeconds,
      }),
    onSuccess: () => {
      void queryClient.invalidateQueries({ queryKey: ['courses', id] })
    },
  })

  function handleSubmit(e: FormEvent) {
    e.preventDefault()
    createMutation.mutate()
  }

  const canEdit = role === 'organizer'

  return (
    <section className="mb-6 rounded border bg-white p-4">
      <h2 className="mb-3 text-lg font-semibold">{t('schedule_courses_title')}</h2>
      <ul className="mb-4 divide-y">
        {courses.map((course) => (
          <li key={course.id} className="flex items-center justify-between py-2 text-sm">
            <span>
              {course.name} — {course.heat_interval_seconds}s interval
            </span>
            {canEdit && (
              <label className="flex items-center gap-2">
                {t('schedule_courses_offset')}
                <input
                  type="number"
                  defaultValue={course.delay_offset_seconds}
                  onBlur={(e) =>
                    updateMutation.mutate({ courseId: course.id, delayOffsetSeconds: Number(e.target.value) })
                  }
                  className="w-20 rounded border px-2 py-1"
                  aria-label={`${t('schedule_courses_offset')} — ${course.name}`}
                />
              </label>
            )}
          </li>
        ))}
      </ul>
      {updateMutation.isError && (
        <p role="alert" className="mb-2 text-sm text-red-600">
          {t('schedule_courses_update_error')}
        </p>
      )}
      {canEdit && (
        <form onSubmit={handleSubmit} className="flex items-end gap-3">
          <div>
            <label htmlFor="course-name" className="block text-sm font-medium">
              {t('schedule_courses_name')}
            </label>
            <input
              id="course-name"
              value={name}
              onChange={(e) => setName(e.target.value)}
              className="mt-1 rounded border px-3 py-2"
              required
            />
          </div>
          <div>
            <label htmlFor="course-interval" className="block text-sm font-medium">
              {t('schedule_courses_interval')}
            </label>
            <input
              id="course-interval"
              type="number"
              min={1}
              value={intervalSeconds}
              onChange={(e) => setIntervalSeconds(Number(e.target.value))}
              className="mt-1 w-32 rounded border px-3 py-2"
              required
            />
          </div>
          <button
            type="submit"
            disabled={createMutation.isPending}
            className="rounded bg-blue-600 px-4 py-2 text-sm text-white disabled:opacity-50"
          >
            {t('schedule_courses_add_submit')}
          </button>
        </form>
      )}
      {createMutation.isError && (
        <p role="alert" className="mt-2 text-sm text-red-600">
          {t('schedule_courses_add_error')}
        </p>
      )}
    </section>
  )
}
```

- [ ] **Step 4: Write `ScheduleCourses.test.tsx`**

Create `frontend/src/pages/ScheduleCourses.test.tsx`:

```tsx
import { describe, expect, it, vi } from 'vitest'
import { render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { MemoryRouter, Route, Routes } from 'react-router-dom'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { ScheduleCourses } from './ScheduleCourses'
import * as client from '../api/client'

vi.mock('react-i18next', () => ({
  useTranslation: () => ({ t: (key: string) => key }),
}))
vi.mock('../api/client', async () => {
  const actual = await vi.importActual<typeof import('../api/client')>('../api/client')
  return { ...actual, api: { ...actual.api, get: vi.fn(), post: vi.fn(), patch: vi.fn() } }
})

function renderCourses(role: 'organizer' | 'time_entry' | 'spectator' = 'organizer') {
  vi.doMock('../auth/AuthContext', () => ({ useAuth: () => ({ role }) }))
  const queryClient = new QueryClient()
  render(
    <QueryClientProvider client={queryClient}>
      <MemoryRouter initialEntries={['/tournaments/1/schedule']}>
        <Routes>
          <Route path="/tournaments/:id/schedule" element={<ScheduleCourses />} />
        </Routes>
      </MemoryRouter>
    </QueryClientProvider>,
  )
}

describe('ScheduleCourses', () => {
  it('renders courses and submits a new one', async () => {
    vi.mocked(client.api.get).mockResolvedValue({
      courses: [{ id: 1, tournament_id: 1, name: 'Course A', heat_interval_seconds: 300, delay_offset_seconds: 0 }],
    })
    vi.mocked(client.api.post).mockResolvedValue({
      id: 2,
      tournament_id: 1,
      name: 'Course B',
      heat_interval_seconds: 240,
      delay_offset_seconds: 0,
    })

    renderCourses()

    expect(await screen.findByText(/Course A/)).toBeInTheDocument()

    await userEvent.type(screen.getByLabelText('schedule_courses_name'), 'Course B')
    await userEvent.clear(screen.getByLabelText('schedule_courses_interval'))
    await userEvent.type(screen.getByLabelText('schedule_courses_interval'), '240')
    await userEvent.click(screen.getByText('schedule_courses_add_submit'))

    await waitFor(() =>
      expect(client.api.post).toHaveBeenCalledWith('/api/tournaments/1/courses', {
        name: 'Course B',
        heat_interval_seconds: 240,
      }),
    )
  })

  it('shows an error message when adding a course fails', async () => {
    vi.mocked(client.api.get).mockResolvedValue({ courses: [] })
    vi.mocked(client.api.post).mockRejectedValue(new Error('server error'))

    renderCourses()

    await userEvent.type(await screen.findByLabelText('schedule_courses_name'), 'Course B')
    await userEvent.click(screen.getByText('schedule_courses_add_submit'))

    expect(await screen.findByRole('alert')).toHaveTextContent('schedule_courses_add_error')
  })
})
```

Note: this test mocks `useAuth` inline via `vi.doMock` inside
`renderCourses` rather than a top-level `vi.mock`, because the role
needs to vary per test. `vi.doMock` must be called before the module
under test is imported for a given test run; since `ScheduleCourses` is
imported statically at the top of the file, `vi.doMock` here only takes
effect if Vitest's module graph re-evaluates `AuthContext` per test —
which it does NOT by default for a static top-level import. **Fix:**
add a top-level `vi.mock('../auth/AuthContext', () => ({ useAuth: vi.fn() }))`
instead, and set the mock's return value per test with
`vi.mocked(useAuth).mockReturnValue({ role: 'organizer', token: 'x', login: vi.fn(), logout: vi.fn() })`.
Rewrite the test file's mocking this way:

```tsx
import { describe, expect, it, vi } from 'vitest'
import { render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { MemoryRouter, Route, Routes } from 'react-router-dom'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { ScheduleCourses } from './ScheduleCourses'
import * as client from '../api/client'
import { useAuth } from '../auth/AuthContext'

vi.mock('react-i18next', () => ({
  useTranslation: () => ({ t: (key: string) => key }),
}))
vi.mock('../api/client', async () => {
  const actual = await vi.importActual<typeof import('../api/client')>('../api/client')
  return { ...actual, api: { ...actual.api, get: vi.fn(), post: vi.fn(), patch: vi.fn() } }
})
vi.mock('../auth/AuthContext', () => ({ useAuth: vi.fn() }))

function renderCourses() {
  const queryClient = new QueryClient()
  render(
    <QueryClientProvider client={queryClient}>
      <MemoryRouter initialEntries={['/tournaments/1/schedule']}>
        <Routes>
          <Route path="/tournaments/:id/schedule" element={<ScheduleCourses />} />
        </Routes>
      </MemoryRouter>
    </QueryClientProvider>,
  )
}

describe('ScheduleCourses', () => {
  it('renders courses and submits a new one', async () => {
    vi.mocked(useAuth).mockReturnValue({ role: 'organizer', token: 'x', login: vi.fn(), logout: vi.fn() })
    vi.mocked(client.api.get).mockResolvedValue({
      courses: [{ id: 1, tournament_id: 1, name: 'Course A', heat_interval_seconds: 300, delay_offset_seconds: 0 }],
    })
    vi.mocked(client.api.post).mockResolvedValue({
      id: 2,
      tournament_id: 1,
      name: 'Course B',
      heat_interval_seconds: 240,
      delay_offset_seconds: 0,
    })

    renderCourses()

    expect(await screen.findByText(/Course A/)).toBeInTheDocument()

    await userEvent.type(screen.getByLabelText('schedule_courses_name'), 'Course B')
    await userEvent.clear(screen.getByLabelText('schedule_courses_interval'))
    await userEvent.type(screen.getByLabelText('schedule_courses_interval'), '240')
    await userEvent.click(screen.getByText('schedule_courses_add_submit'))

    await waitFor(() =>
      expect(client.api.post).toHaveBeenCalledWith('/api/tournaments/1/courses', {
        name: 'Course B',
        heat_interval_seconds: 240,
      }),
    )
  })

  it('shows an error message when adding a course fails', async () => {
    vi.mocked(useAuth).mockReturnValue({ role: 'organizer', token: 'x', login: vi.fn(), logout: vi.fn() })
    vi.mocked(client.api.get).mockResolvedValue({ courses: [] })
    vi.mocked(client.api.post).mockRejectedValue(new Error('server error'))

    renderCourses()

    await userEvent.type(await screen.findByLabelText('schedule_courses_name'), 'Course B')
    await userEvent.click(screen.getByText('schedule_courses_add_submit'))

    expect(await screen.findByRole('alert')).toHaveTextContent('schedule_courses_add_error')
  })

  it('hides the add-course form for non-organizer roles', async () => {
    vi.mocked(useAuth).mockReturnValue({ role: 'time_entry', token: 'x', login: vi.fn(), logout: vi.fn() })
    vi.mocked(client.api.get).mockResolvedValue({
      courses: [{ id: 1, tournament_id: 1, name: 'Course A', heat_interval_seconds: 300, delay_offset_seconds: 0 }],
    })

    renderCourses()

    expect(await screen.findByText(/Course A/)).toBeInTheDocument()
    expect(screen.queryByLabelText('schedule_courses_name')).not.toBeInTheDocument()
  })
})
```

- [ ] **Step 5: Run the new test to verify it fails, then passes**

Run: `cd frontend && npx vitest run src/pages/ScheduleCourses.test.tsx`
Expected first: FAIL (`ScheduleCourses` module not found — file doesn't
exist as a passing target until Step 3/4 land together; if TDD-ing
strictly, write the test first against a stub `export function
ScheduleCourses() { return null }`, confirm it fails on missing content,
then implement fully)
Expected after: PASS (3 tests)

- [ ] **Step 6: Create `SchedulePage`**

Create `frontend/src/pages/SchedulePage.tsx`:

```tsx
import { useParams } from 'react-router-dom'
import { useQuery } from '@tanstack/react-query'
import { api } from '../api/client'
import type { Round } from '../api/types'
import { ScheduleCourses } from './ScheduleCourses'

export function SchedulePage() {
  const { id } = useParams<{ id: string }>()

  useQuery({
    queryKey: ['rounds', id],
    queryFn: () => api.get<{ rounds: Round[] }>(`/api/tournaments/${id}/rounds`),
    enabled: !!id,
  })

  return (
    <div>
      <ScheduleCourses />
    </div>
  )
}
```

(This task wires only the Courses section in. `['schedule', id]` isn't
queried yet — Task 4 adds it, since determining which groups/divisions
still need scheduling requires cross-referencing against existing
heats; Task 5 then adds the `['courses', id]` query to `SchedulePage`
itself, needed to resolve heat course names for display.)

- [ ] **Step 7: Write `SchedulePage.test.tsx`**

Create `frontend/src/pages/SchedulePage.test.tsx`:

```tsx
import { describe, expect, it, vi } from 'vitest'
import { render, screen } from '@testing-library/react'
import { MemoryRouter, Route, Routes } from 'react-router-dom'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { SchedulePage } from './SchedulePage'
import * as client from '../api/client'
import { useAuth } from '../auth/AuthContext'

vi.mock('react-i18next', () => ({
  useTranslation: () => ({ t: (key: string) => key }),
}))
vi.mock('../api/client', async () => {
  const actual = await vi.importActual<typeof import('../api/client')>('../api/client')
  return { ...actual, api: { ...actual.api, get: vi.fn() } }
})
vi.mock('../auth/AuthContext', () => ({ useAuth: vi.fn() }))

describe('SchedulePage', () => {
  it('renders the Courses section', async () => {
    vi.mocked(useAuth).mockReturnValue({ role: 'organizer', token: 'x', login: vi.fn(), logout: vi.fn() })
    vi.mocked(client.api.get).mockImplementation((path: unknown) => {
      const p = path as string
      if (p === '/api/tournaments/1/rounds') return Promise.resolve({ rounds: [] })
      if (p === '/api/tournaments/1/courses') return Promise.resolve({ courses: [] })
      return Promise.reject(new Error(`unexpected path ${p}`))
    })

    render(
      <QueryClientProvider client={new QueryClient()}>
        <MemoryRouter initialEntries={['/tournaments/1/schedule']}>
          <Routes>
            <Route path="/tournaments/:id/schedule" element={<SchedulePage />} />
          </Routes>
        </MemoryRouter>
      </QueryClientProvider>,
    )

    expect(await screen.findByText('schedule_courses_title')).toBeInTheDocument()
  })
})
```

- [ ] **Step 8: Run the test to verify it passes**

Run: `cd frontend && npx vitest run src/pages/SchedulePage.test.tsx`
Expected: PASS

- [ ] **Step 9: Wire the route into `App.tsx`**

In `frontend/src/App.tsx`, add the import and route:

```tsx
import { SchedulePage } from './pages/SchedulePage'
```

and, inside `<Route path="/tournaments/:id" element={<TournamentDetailPage />}>`, after the `teams/import` route:

```tsx
                  <Route path="schedule" element={<SchedulePage />} />
```

- [ ] **Step 10: Make the tab strip link real**

In `frontend/src/pages/TournamentDetailPage.tsx`, replace:

```tsx
        <span className="cursor-not-allowed px-4 py-2 text-sm text-gray-300" title={t('tab_coming_soon')}>
          {t('tab_schedule')}
        </span>
```

with:

```tsx
        <NavLink to="schedule" className={tabLinkClass}>
          {t('tab_schedule')}
        </NavLink>
```

- [ ] **Step 11: Update `TournamentDetailPage.test.tsx`**

Read `frontend/src/pages/TournamentDetailPage.test.tsx` first to find
any assertion checking for the disabled "coming soon" schedule tab
(e.g. `expect(screen.getByTitle('tab_coming_soon'))` or similar), and
replace it with an assertion that the "Rounds & Schedule" tab is now a
real link:

```tsx
    expect(screen.getByRole('link', { name: 'tab_schedule' })).toBeInTheDocument()
```

- [ ] **Step 12: Run the full frontend suite and typecheck**

Run: `cd frontend && npm test -- --run && npx tsc -b`
Expected: all tests pass, 0 type errors

- [ ] **Step 13: Verify i18n bundle parity**

Run:
```bash
python3 -c "
import json
en = json.load(open('internal/i18n/bundles/en.json'))
de = json.load(open('internal/i18n/bundles/de.json'))
print('en-only:', set(en) - set(de))
print('de-only:', set(de) - set(en))
"
```
Expected: both empty

- [ ] **Step 14: Commit**

```bash
git add frontend/src/api/types.ts frontend/src/App.tsx \
  frontend/src/pages/TournamentDetailPage.tsx frontend/src/pages/TournamentDetailPage.test.tsx \
  frontend/src/pages/SchedulePage.tsx frontend/src/pages/SchedulePage.test.tsx \
  frontend/src/pages/ScheduleCourses.tsx frontend/src/pages/ScheduleCourses.test.tsx \
  internal/i18n/bundles/en.json internal/i18n/bundles/de.json
git commit -m "feat: add Schedule tab route with a Courses section"
```

---

### Task 3: Round 1 creation — auto-split group editor

**Files:**
- Create: `frontend/src/pages/ScheduleRoundCreate.tsx`
- Create: `frontend/src/pages/ScheduleRoundCreate.test.tsx`
- Modify: `frontend/src/pages/SchedulePage.tsx` (render `ScheduleRoundCreate` when no round exists yet)
- Modify: `internal/i18n/bundles/en.json`, `internal/i18n/bundles/de.json`

**Interfaces:**
- Consumes: `Team{id, tournament_id, name, club, extra_fields}` (already
  in `types.ts`), `Round` (Task 2), `useAuth()`.
- Produces: `ScheduleRoundCreate` component (named export, no props —
  owns `useParams`, a `['teams', id]` query — reusing the exact query
  key `TeamsTab.tsx` already populates, so no extra fetch if the Teams
  tab was visited this session — a `['rounds', id]` invalidation on
  success, matching the key `SchedulePage` owns).

- [ ] **Step 1: Add i18n keys**

Add to `internal/i18n/bundles/en.json`:

```json
  "schedule_round_create_title": "Create Round 1",
  "schedule_round_create_group_count": "Number of groups",
  "schedule_round_create_shuffle": "Shuffle into groups",
  "schedule_round_create_group_label": "Group {{number}}",
  "schedule_round_create_move_to": "Move to group",
  "schedule_round_create_submit": "Create Round 1",
  "schedule_round_create_error": "Could not create the round. Please try again."
```

Add to `internal/i18n/bundles/de.json`:

```json
  "schedule_round_create_title": "Runde 1 erstellen",
  "schedule_round_create_group_count": "Anzahl Gruppen",
  "schedule_round_create_shuffle": "In Gruppen aufteilen",
  "schedule_round_create_group_label": "Gruppe {{number}}",
  "schedule_round_create_move_to": "Verschieben nach Gruppe",
  "schedule_round_create_submit": "Runde 1 erstellen",
  "schedule_round_create_error": "Die Runde konnte nicht erstellt werden. Bitte versuchen Sie es erneut."
```

- [ ] **Step 2: Write `ScheduleRoundCreate`**

Create `frontend/src/pages/ScheduleRoundCreate.tsx`:

```tsx
import { useState } from 'react'
import { useParams } from 'react-router-dom'
import { useTranslation } from 'react-i18next'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { api } from '../api/client'
import { useAuth } from '../auth/AuthContext'
import type { Round, Team } from '../api/types'

function shuffledGroups(teamIds: string[], groupCount: number): string[][] {
  const shuffled = [...teamIds]
  for (let i = shuffled.length - 1; i > 0; i--) {
    const j = Math.floor(Math.random() * (i + 1))
    ;[shuffled[i], shuffled[j]] = [shuffled[j], shuffled[i]]
  }
  const groups: string[][] = Array.from({ length: groupCount }, () => [])
  shuffled.forEach((teamId, index) => {
    groups[index % groupCount].push(teamId)
  })
  return groups
}

export function ScheduleRoundCreate() {
  const { t } = useTranslation()
  const { id } = useParams<{ id: string }>()
  const { role } = useAuth()
  const queryClient = useQueryClient()

  const { data: teamsData } = useQuery({
    queryKey: ['teams', id],
    queryFn: () => api.get<Team[]>(`/api/tournaments/${id}/teams`),
    enabled: !!id,
  })
  const teams = teamsData ?? []

  const [groupCount, setGroupCount] = useState(2)
  const [groups, setGroups] = useState<string[][] | null>(null)

  function teamName(teamId: string): string {
    return teams.find((team) => String(team.id) === teamId)?.name ?? teamId
  }

  function handleShuffle() {
    setGroups(shuffledGroups(teams.map((team) => String(team.id)), groupCount))
  }

  function moveTeam(teamId: string, toGroupIndex: number) {
    setGroups((prev) => {
      if (!prev) return prev
      const next = prev.map((group) => group.filter((id2) => id2 !== teamId))
      next[toGroupIndex].push(teamId)
      return next
    })
  }

  const createMutation = useMutation({
    mutationFn: () => api.post<Round>(`/api/tournaments/${id}/rounds`, { round_number: 1, groups }),
    onSuccess: () => {
      void queryClient.invalidateQueries({ queryKey: ['rounds', id] })
    },
  })

  if (role !== 'organizer') {
    return null
  }

  return (
    <section className="mb-6 rounded border bg-white p-4">
      <h2 className="mb-3 text-lg font-semibold">{t('schedule_round_create_title')}</h2>
      <div className="mb-3 flex items-end gap-3">
        <div>
          <label htmlFor="group-count" className="block text-sm font-medium">
            {t('schedule_round_create_group_count')}
          </label>
          <input
            id="group-count"
            type="number"
            min={1}
            value={groupCount}
            onChange={(e) => setGroupCount(Number(e.target.value))}
            className="mt-1 w-24 rounded border px-3 py-2"
          />
        </div>
        <button
          type="button"
          onClick={handleShuffle}
          className="rounded border px-4 py-2 text-sm"
          disabled={teams.length === 0}
        >
          {t('schedule_round_create_shuffle')}
        </button>
      </div>

      {groups && (
        <>
          <div className="mb-4 grid grid-cols-2 gap-4">
            {groups.map((group, groupIndex) => (
              <div key={groupIndex} className="rounded border p-3">
                <h3 className="mb-2 text-sm font-medium">
                  {t('schedule_round_create_group_label', { number: groupIndex + 1 })}
                </h3>
                <ul className="space-y-1">
                  {group.map((teamId) => (
                    <li key={teamId} className="flex items-center justify-between text-sm">
                      <span>{teamName(teamId)}</span>
                      <select
                        aria-label={`${t('schedule_round_create_move_to')} — ${teamName(teamId)}`}
                        value={groupIndex}
                        onChange={(e) => moveTeam(teamId, Number(e.target.value))}
                        className="rounded border px-2 py-1 text-xs"
                      >
                        {groups.map((_, targetIndex) => (
                          <option key={targetIndex} value={targetIndex}>
                            {t('schedule_round_create_group_label', { number: targetIndex + 1 })}
                          </option>
                        ))}
                      </select>
                    </li>
                  ))}
                </ul>
              </div>
            ))}
          </div>
          {createMutation.isError && (
            <p role="alert" className="mb-2 text-sm text-red-600">
              {t('schedule_round_create_error')}
            </p>
          )}
          <button
            type="button"
            onClick={() => createMutation.mutate()}
            disabled={createMutation.isPending}
            className="rounded bg-blue-600 px-4 py-2 text-sm text-white disabled:opacity-50"
          >
            {t('schedule_round_create_submit')}
          </button>
        </>
      )}
    </section>
  )
}
```

- [ ] **Step 3: Write `ScheduleRoundCreate.test.tsx`**

Create `frontend/src/pages/ScheduleRoundCreate.test.tsx`:

```tsx
import { describe, expect, it, vi } from 'vitest'
import { render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { MemoryRouter, Route, Routes } from 'react-router-dom'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { ScheduleRoundCreate } from './ScheduleRoundCreate'
import * as client from '../api/client'
import { useAuth } from '../auth/AuthContext'

vi.mock('react-i18next', () => ({
  useTranslation: () => ({ t: (key: string, vars?: Record<string, unknown>) => (vars ? `${key} ${JSON.stringify(vars)}` : key) }),
}))
vi.mock('../api/client', async () => {
  const actual = await vi.importActual<typeof import('../api/client')>('../api/client')
  return { ...actual, api: { ...actual.api, get: vi.fn(), post: vi.fn() } }
})
vi.mock('../auth/AuthContext', () => ({ useAuth: vi.fn() }))

function renderPanel() {
  render(
    <QueryClientProvider client={new QueryClient()}>
      <MemoryRouter initialEntries={['/tournaments/1/schedule']}>
        <Routes>
          <Route path="/tournaments/:id/schedule" element={<ScheduleRoundCreate />} />
        </Routes>
      </MemoryRouter>
    </QueryClientProvider>,
  )
}

describe('ScheduleRoundCreate', () => {
  it('shuffles teams into groups, allows moving a team, and submits', async () => {
    vi.mocked(useAuth).mockReturnValue({ role: 'organizer', token: 'x', login: vi.fn(), logout: vi.fn() })
    vi.mocked(client.api.get).mockResolvedValue([
      { id: 1, tournament_id: 1, name: 'Team A', club: '', extra_fields: {} },
      { id: 2, tournament_id: 1, name: 'Team B', club: '', extra_fields: {} },
    ])
    vi.mocked(client.api.post).mockResolvedValue({ id: 1, round_number: 1, status: 'open', groups: [], divisions: [] })

    renderPanel()

    await userEvent.click(await screen.findByText('schedule_round_create_shuffle'))

    expect(await screen.findByText('Team A')).toBeInTheDocument()
    expect(screen.getByText('Team B')).toBeInTheDocument()

    await userEvent.click(screen.getByText(/schedule_round_create_submit/))

    await waitFor(() =>
      expect(client.api.post).toHaveBeenCalledWith(
        '/api/tournaments/1/rounds',
        expect.objectContaining({ round_number: 1, groups: expect.any(Array) }),
      ),
    )
  })

  it('shows an error message when round creation fails', async () => {
    vi.mocked(useAuth).mockReturnValue({ role: 'organizer', token: 'x', login: vi.fn(), logout: vi.fn() })
    vi.mocked(client.api.get).mockResolvedValue([
      { id: 1, tournament_id: 1, name: 'Team A', club: '', extra_fields: {} },
    ])
    vi.mocked(client.api.post).mockRejectedValue(new Error('server error'))

    renderPanel()

    await userEvent.click(await screen.findByText('schedule_round_create_shuffle'))
    await userEvent.click(screen.getByText(/schedule_round_create_submit/))

    expect(await screen.findByRole('alert')).toHaveTextContent('schedule_round_create_error')
  })

  it('renders nothing for a non-organizer role', () => {
    vi.mocked(useAuth).mockReturnValue({ role: 'time_entry', token: 'x', login: vi.fn(), logout: vi.fn() })
    vi.mocked(client.api.get).mockResolvedValue([])

    const { container } = render(
      <QueryClientProvider client={new QueryClient()}>
        <MemoryRouter initialEntries={['/tournaments/1/schedule']}>
          <Routes>
            <Route path="/tournaments/:id/schedule" element={<ScheduleRoundCreate />} />
          </Routes>
        </MemoryRouter>
      </QueryClientProvider>,
    )

    expect(container).toBeEmptyDOMElement()
  })
})
```

- [ ] **Step 4: Run the test to verify it fails, then passes**

Run: `cd frontend && npx vitest run src/pages/ScheduleRoundCreate.test.tsx`
Expected first: FAIL (module doesn't exist)
Expected after: PASS (3 tests)

- [ ] **Step 5: Wire into `SchedulePage`**

Modify `frontend/src/pages/SchedulePage.tsx` to conditionally render
`ScheduleRoundCreate` only when no round exists yet:

```tsx
import { useParams } from 'react-router-dom'
import { useQuery } from '@tanstack/react-query'
import { api } from '../api/client'
import type { Round } from '../api/types'
import { ScheduleCourses } from './ScheduleCourses'
import { ScheduleRoundCreate } from './ScheduleRoundCreate'

export function SchedulePage() {
  const { id } = useParams<{ id: string }>()

  const { data: roundsData } = useQuery({
    queryKey: ['rounds', id],
    queryFn: () => api.get<{ rounds: Round[] }>(`/api/tournaments/${id}/rounds`),
    enabled: !!id,
  })
  const rounds = roundsData?.rounds ?? []

  return (
    <div>
      <ScheduleCourses />
      {rounds.length === 0 && <ScheduleRoundCreate />}
    </div>
  )
}
```

- [ ] **Step 6: Update `SchedulePage.test.tsx`**

Add a second test to `frontend/src/pages/SchedulePage.test.tsx`
confirming `ScheduleRoundCreate` renders when there are no rounds, and
that it does not render once a round exists:

```tsx
  it('renders the round-create panel only when no round exists yet', async () => {
    vi.mocked(useAuth).mockReturnValue({ role: 'organizer', token: 'x', login: vi.fn(), logout: vi.fn() })
    vi.mocked(client.api.get).mockImplementation((path: unknown) => {
      const p = path as string
      if (p === '/api/tournaments/1/rounds') return Promise.resolve({ rounds: [] })
      if (p === '/api/tournaments/1/courses') return Promise.resolve({ courses: [] })
      if (p === '/api/tournaments/1/teams') return Promise.resolve([])
      return Promise.reject(new Error(`unexpected path ${p}`))
    })

    render(
      <QueryClientProvider client={new QueryClient()}>
        <MemoryRouter initialEntries={['/tournaments/1/schedule']}>
          <Routes>
            <Route path="/tournaments/:id/schedule" element={<SchedulePage />} />
          </Routes>
        </MemoryRouter>
      </QueryClientProvider>,
    )

    expect(await screen.findByText('schedule_round_create_title')).toBeInTheDocument()
  })

  it('hides the round-create panel once a round exists', async () => {
    vi.mocked(useAuth).mockReturnValue({ role: 'organizer', token: 'x', login: vi.fn(), logout: vi.fn() })
    vi.mocked(client.api.get).mockImplementation((path: unknown) => {
      const p = path as string
      if (p === '/api/tournaments/1/rounds')
        return Promise.resolve({ rounds: [{ id: 1, round_number: 1, status: 'open', groups: [], divisions: [] }] })
      if (p === '/api/tournaments/1/courses') return Promise.resolve({ courses: [] })
      return Promise.reject(new Error(`unexpected path ${p}`))
    })

    render(
      <QueryClientProvider client={new QueryClient()}>
        <MemoryRouter initialEntries={['/tournaments/1/schedule']}>
          <Routes>
            <Route path="/tournaments/:id/schedule" element={<SchedulePage />} />
          </Routes>
        </MemoryRouter>
      </QueryClientProvider>,
    )

    expect(await screen.findByText('schedule_courses_title')).toBeInTheDocument()
    expect(screen.queryByText('schedule_round_create_title')).not.toBeInTheDocument()
  })
```

- [ ] **Step 7: Run the full frontend suite and typecheck**

Run: `cd frontend && npm test -- --run && npx tsc -b`
Expected: all tests pass, 0 type errors

- [ ] **Step 8: Verify i18n bundle parity**

Run the same parity check as Task 2 Step 13.
Expected: both empty

- [ ] **Step 9: Commit**

```bash
git add frontend/src/pages/ScheduleRoundCreate.tsx frontend/src/pages/ScheduleRoundCreate.test.tsx \
  frontend/src/pages/SchedulePage.tsx frontend/src/pages/SchedulePage.test.tsx \
  internal/i18n/bundles/en.json internal/i18n/bundles/de.json
git commit -m "feat: add round 1 creation with auto-split group editor"
```

---

### Task 4: Heat scheduling — course assignment for groups and divisions

**Files:**
- Create: `frontend/src/pages/ScheduleAssignments.tsx`
- Create: `frontend/src/pages/ScheduleAssignments.test.tsx`
- Modify: `frontend/src/pages/SchedulePage.tsx` (add the `['schedule', id]` query, render `ScheduleAssignments` for the current round's unscheduled groups/divisions)
- Modify: `internal/i18n/bundles/en.json`, `internal/i18n/bundles/de.json`

**Interfaces:**
- Consumes: `Course`, `Round`, `Group`, `DivisionInfo` (Tasks 1-2),
  `useAuth()`.
- Produces: a new frontend type `Heat{id, round_id, group_id, division_id, course_id, planned_start, effective_start, status, results: HeatResult[]}`
  and `HeatResult{heat_id, team_id, time_seconds, status}` in
  `frontend/src/api/types.ts` — later tasks (5, 6) also consume these.
  `ScheduleAssignments` component (named export) with props
  `{mode: 'group' | 'division'; roundId: number; items: {id: number; label: string}[]}`
  — `SchedulePage` computes `items` by diffing the current round's
  `groups`/`divisions` against `['schedule', id]`'s heats (by
  `group_id`/`division_id`), and passes only the not-yet-scheduled ones.

- [ ] **Step 1: Add the `Heat`/`HeatResult` types**

Append to `frontend/src/api/types.ts`:

```ts
export interface HeatResult {
  heat_id: number
  team_id: string
  time_seconds: number | null
  status: string
}

export interface Heat {
  id: number
  round_id: number
  group_id: number | null
  division_id: number | null
  course_id: number
  planned_start: string
  effective_start: string
  status: string
  results: HeatResult[]
}
```

- [ ] **Step 2: Add i18n keys**

Add to `internal/i18n/bundles/en.json`:

```json
  "schedule_assignments_group_title": "Schedule this round's groups",
  "schedule_assignments_division_title": "Schedule divisions",
  "schedule_assignments_course_label": "Course",
  "schedule_assignments_select_course": "Select a course",
  "schedule_assignments_submit": "Schedule",
  "schedule_assignments_error": "Could not schedule. Please try again."
```

Add to `internal/i18n/bundles/de.json`:

```json
  "schedule_assignments_group_title": "Gruppen dieser Runde einteilen",
  "schedule_assignments_division_title": "Divisionen einteilen",
  "schedule_assignments_course_label": "Bahn",
  "schedule_assignments_select_course": "Bahn auswählen",
  "schedule_assignments_submit": "Einteilen",
  "schedule_assignments_error": "Einteilung fehlgeschlagen. Bitte versuchen Sie es erneut."
```

- [ ] **Step 3: Write `ScheduleAssignments`**

Create `frontend/src/pages/ScheduleAssignments.tsx`:

```tsx
import { useState } from 'react'
import { useParams } from 'react-router-dom'
import { useTranslation } from 'react-i18next'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { api } from '../api/client'
import { useAuth } from '../auth/AuthContext'
import type { Course } from '../api/types'

interface ScheduleAssignmentsProps {
  mode: 'group' | 'division'
  roundId: number
  items: { id: number; label: string }[]
}

export function ScheduleAssignments({ mode, roundId, items }: ScheduleAssignmentsProps) {
  const { t } = useTranslation()
  const { id } = useParams<{ id: string }>()
  const { role } = useAuth()
  const queryClient = useQueryClient()

  const { data: coursesData } = useQuery({
    queryKey: ['courses', id],
    queryFn: () => api.get<{ courses: Course[] }>(`/api/tournaments/${id}/courses`),
    enabled: !!id,
  })
  const courses = coursesData?.courses ?? []

  const [selectedCourse, setSelectedCourse] = useState<Record<number, number>>({})

  const scheduleMutation = useMutation({
    mutationFn: () => {
      const assignments = items.map((item) => ({
        [mode === 'group' ? 'group_id' : 'division_id']: item.id,
        course_id: selectedCourse[item.id],
      }))
      const path =
        mode === 'group'
          ? `/api/tournaments/${id}/rounds/${roundId}/schedule`
          : `/api/tournaments/${id}/divisions/schedule`
      return api.post(path, { assignments })
    },
    onSuccess: () => {
      void queryClient.invalidateQueries({ queryKey: ['schedule', id] })
    },
  })

  if (role !== 'organizer' || items.length === 0) {
    return null
  }

  const allCourseSelected = items.every((item) => selectedCourse[item.id] !== undefined)

  return (
    <section className="mb-6 rounded border bg-white p-4">
      <h2 className="mb-3 text-lg font-semibold">
        {t(mode === 'group' ? 'schedule_assignments_group_title' : 'schedule_assignments_division_title')}
      </h2>
      <ul className="mb-4 space-y-2">
        {items.map((item) => (
          <li key={item.id} className="flex items-center justify-between text-sm">
            <span>{item.label}</span>
            <label className="flex items-center gap-2">
              {t('schedule_assignments_course_label')}
              <select
                aria-label={`${t('schedule_assignments_course_label')} — ${item.label}`}
                value={selectedCourse[item.id] ?? ''}
                onChange={(e) =>
                  setSelectedCourse((prev) => ({ ...prev, [item.id]: Number(e.target.value) }))
                }
                className="rounded border px-2 py-1"
              >
                <option value="" disabled>
                  {t('schedule_assignments_select_course')}
                </option>
                {courses.map((course) => (
                  <option key={course.id} value={course.id}>
                    {course.name}
                  </option>
                ))}
              </select>
            </label>
          </li>
        ))}
      </ul>
      {scheduleMutation.isError && (
        <p role="alert" className="mb-2 text-sm text-red-600">
          {t('schedule_assignments_error')}
        </p>
      )}
      <button
        type="button"
        onClick={() => scheduleMutation.mutate()}
        disabled={!allCourseSelected || scheduleMutation.isPending}
        className="rounded bg-blue-600 px-4 py-2 text-sm text-white disabled:opacity-50"
      >
        {t('schedule_assignments_submit')}
      </button>
    </section>
  )
}
```

- [ ] **Step 4: Write `ScheduleAssignments.test.tsx`**

Create `frontend/src/pages/ScheduleAssignments.test.tsx`:

```tsx
import { describe, expect, it, vi } from 'vitest'
import { render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { MemoryRouter, Route, Routes } from 'react-router-dom'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { ScheduleAssignments } from './ScheduleAssignments'
import * as client from '../api/client'
import { useAuth } from '../auth/AuthContext'

vi.mock('react-i18next', () => ({
  useTranslation: () => ({ t: (key: string) => key }),
}))
vi.mock('../api/client', async () => {
  const actual = await vi.importActual<typeof import('../api/client')>('../api/client')
  return { ...actual, api: { ...actual.api, get: vi.fn(), post: vi.fn() } }
})
vi.mock('../auth/AuthContext', () => ({ useAuth: vi.fn() }))

function renderPanel(mode: 'group' | 'division') {
  render(
    <QueryClientProvider client={new QueryClient()}>
      <MemoryRouter initialEntries={['/tournaments/1/schedule']}>
        <Routes>
          <Route
            path="/tournaments/:id/schedule"
            element={
              <ScheduleAssignments
                mode={mode}
                roundId={5}
                items={[
                  { id: 10, label: 'Group 1' },
                  { id: 11, label: 'Group 2' },
                ]}
              />
            }
          />
        </Routes>
      </MemoryRouter>
    </QueryClientProvider>,
  )
}

describe('ScheduleAssignments', () => {
  it('schedules group heats with the chosen courses', async () => {
    vi.mocked(useAuth).mockReturnValue({ role: 'organizer', token: 'x', login: vi.fn(), logout: vi.fn() })
    vi.mocked(client.api.get).mockResolvedValue({
      courses: [
        { id: 1, tournament_id: 1, name: 'Course A', heat_interval_seconds: 300, delay_offset_seconds: 0 },
        { id: 2, tournament_id: 1, name: 'Course B', heat_interval_seconds: 300, delay_offset_seconds: 0 },
      ],
    })
    vi.mocked(client.api.post).mockResolvedValue({ heats: [] })

    renderPanel('group')

    await userEvent.selectOptions(await screen.findByLabelText(/Course — Group 1/), '1')
    await userEvent.selectOptions(screen.getByLabelText(/Course — Group 2/), '2')
    await userEvent.click(screen.getByText('schedule_assignments_submit'))

    await waitFor(() =>
      expect(client.api.post).toHaveBeenCalledWith('/api/tournaments/1/rounds/5/schedule', {
        assignments: [
          { group_id: 10, course_id: 1 },
          { group_id: 11, course_id: 2 },
        ],
      }),
    )
  })

  it('schedules division heats against the divisions/schedule endpoint', async () => {
    vi.mocked(useAuth).mockReturnValue({ role: 'organizer', token: 'x', login: vi.fn(), logout: vi.fn() })
    vi.mocked(client.api.get).mockResolvedValue({
      courses: [{ id: 1, tournament_id: 1, name: 'Course A', heat_interval_seconds: 300, delay_offset_seconds: 0 }],
    })
    vi.mocked(client.api.post).mockResolvedValue({ heats: [] })

    renderPanel('division')

    await userEvent.selectOptions(await screen.findByLabelText(/Course — Group 1/), '1')
    await userEvent.selectOptions(screen.getByLabelText(/Course — Group 2/), '1')
    await userEvent.click(screen.getByText('schedule_assignments_submit'))

    await waitFor(() =>
      expect(client.api.post).toHaveBeenCalledWith(
        '/api/tournaments/1/divisions/schedule',
        expect.objectContaining({
          assignments: [
            { division_id: 10, course_id: 1 },
            { division_id: 11, course_id: 1 },
          ],
        }),
      ),
    )
  })

  it('renders nothing when there are no items to schedule', () => {
    vi.mocked(useAuth).mockReturnValue({ role: 'organizer', token: 'x', login: vi.fn(), logout: vi.fn() })
    vi.mocked(client.api.get).mockResolvedValue({ courses: [] })

    const { container } = render(
      <QueryClientProvider client={new QueryClient()}>
        <MemoryRouter initialEntries={['/tournaments/1/schedule']}>
          <Routes>
            <Route
              path="/tournaments/:id/schedule"
              element={<ScheduleAssignments mode="group" roundId={5} items={[]} />}
            />
          </Routes>
        </MemoryRouter>
      </QueryClientProvider>,
    )

    expect(container).toBeEmptyDOMElement()
  })
})
```

- [ ] **Step 5: Run the test to verify it fails, then passes**

Run: `cd frontend && npx vitest run src/pages/ScheduleAssignments.test.tsx`
Expected first: FAIL (module doesn't exist)
Expected after: PASS (3 tests)

- [ ] **Step 6: Wire into `SchedulePage`**

Modify `frontend/src/pages/SchedulePage.tsx`:

```tsx
import { useParams } from 'react-router-dom'
import { useQuery } from '@tanstack/react-query'
import { api } from '../api/client'
import type { Heat, Round } from '../api/types'
import { ScheduleCourses } from './ScheduleCourses'
import { ScheduleRoundCreate } from './ScheduleRoundCreate'
import { ScheduleAssignments } from './ScheduleAssignments'

export function SchedulePage() {
  const { id } = useParams<{ id: string }>()

  const { data: roundsData } = useQuery({
    queryKey: ['rounds', id],
    queryFn: () => api.get<{ rounds: Round[] }>(`/api/tournaments/${id}/rounds`),
    enabled: !!id,
  })
  const rounds = roundsData?.rounds ?? []
  const currentRound = rounds[rounds.length - 1]

  const { data: scheduleData } = useQuery({
    queryKey: ['schedule', id],
    queryFn: () => api.get<{ heats: Heat[] }>(`/api/tournaments/${id}/schedule`),
    enabled: !!id,
  })
  const heats = scheduleData?.heats ?? []

  const scheduledGroupIds = new Set(heats.filter((h) => h.group_id !== null).map((h) => h.group_id))
  const scheduledDivisionIds = new Set(heats.filter((h) => h.division_id !== null).map((h) => h.division_id))

  const unscheduledGroups =
    currentRound?.groups
      .filter((g) => !scheduledGroupIds.has(g.id))
      .map((g) => ({ id: g.id, label: `Group ${g.id} (${g.team_ids.length} teams)` })) ?? []
  const unscheduledDivisions =
    currentRound?.divisions
      .filter((d) => !scheduledDivisionIds.has(d.id))
      .map((d) => ({ id: d.id, label: `${d.name} (${d.team_ids.length} teams)` })) ?? []

  return (
    <div>
      <ScheduleCourses />
      {rounds.length === 0 && <ScheduleRoundCreate />}
      {currentRound && unscheduledGroups.length > 0 && (
        <ScheduleAssignments mode="group" roundId={currentRound.id} items={unscheduledGroups} />
      )}
      {currentRound && unscheduledDivisions.length > 0 && (
        <ScheduleAssignments mode="division" roundId={currentRound.id} items={unscheduledDivisions} />
      )}
    </div>
  )
}
```

- [ ] **Step 7: Update `SchedulePage.test.tsx`**

Add a test confirming the group-scheduling section appears when the
current round has groups without heats:

```tsx
  it('renders the group-scheduling panel for a round with unscheduled groups', async () => {
    vi.mocked(useAuth).mockReturnValue({ role: 'organizer', token: 'x', login: vi.fn(), logout: vi.fn() })
    vi.mocked(client.api.get).mockImplementation((path: unknown) => {
      const p = path as string
      if (p === '/api/tournaments/1/rounds')
        return Promise.resolve({
          rounds: [
            {
              id: 1,
              round_number: 1,
              status: 'open',
              groups: [{ id: 10, team_ids: ['1', '2'] }],
              divisions: [],
            },
          ],
        })
      if (p === '/api/tournaments/1/courses') return Promise.resolve({ courses: [] })
      if (p === '/api/tournaments/1/schedule') return Promise.resolve({ heats: [] })
      return Promise.reject(new Error(`unexpected path ${p}`))
    })

    render(
      <QueryClientProvider client={new QueryClient()}>
        <MemoryRouter initialEntries={['/tournaments/1/schedule']}>
          <Routes>
            <Route path="/tournaments/:id/schedule" element={<SchedulePage />} />
          </Routes>
        </MemoryRouter>
      </QueryClientProvider>,
    )

    expect(await screen.findByText('schedule_assignments_group_title')).toBeInTheDocument()
  })
```

- [ ] **Step 8: Run the full frontend suite and typecheck**

Run: `cd frontend && npm test -- --run && npx tsc -b`
Expected: all tests pass, 0 type errors

- [ ] **Step 9: Verify i18n bundle parity**

Run the same parity check as Task 2 Step 13.
Expected: both empty

- [ ] **Step 10: Commit**

```bash
git add frontend/src/api/types.ts frontend/src/pages/ScheduleAssignments.tsx \
  frontend/src/pages/ScheduleAssignments.test.tsx frontend/src/pages/SchedulePage.tsx \
  frontend/src/pages/SchedulePage.test.tsx internal/i18n/bundles/en.json internal/i18n/bundles/de.json
git commit -m "feat: add heat scheduling for round groups and divisions"
```

---

### Task 5: Heat list, results entry, and manual start override

**Files:**
- Create: `frontend/src/pages/ScheduleHeats.tsx`
- Create: `frontend/src/pages/ScheduleHeats.test.tsx`
- Modify: `frontend/src/pages/SchedulePage.tsx` (render `ScheduleHeats` with the current round's heats)
- Modify: `internal/i18n/bundles/en.json`, `internal/i18n/bundles/de.json`

**Interfaces:**
- Consumes: `Heat`, `HeatResult`, `Course`, `Round` (Tasks 1-4),
  `useAuth()`.
- Produces: `ScheduleHeats` component (named export) with props
  `{heats: Heat[]; courses: Course[]; currentRound: Round}`.

- [ ] **Step 1: Add i18n keys**

Add to `internal/i18n/bundles/en.json`:

```json
  "schedule_heats_title": "Heats",
  "schedule_heats_status_scheduled": "Scheduled",
  "schedule_heats_status_closed": "Closed",
  "schedule_heats_start_label": "Start time",
  "schedule_heats_start_override_submit": "Update start time",
  "schedule_heats_start_override_error": "Could not update the start time. Please try again.",
  "schedule_heats_results_time": "Time (seconds)",
  "schedule_heats_results_status": "Status",
  "schedule_heats_results_status_finished": "Finished",
  "schedule_heats_results_submit": "Submit Results",
  "schedule_heats_results_error": "Could not submit results. Please try again."
```

Add to `internal/i18n/bundles/de.json`:

```json
  "schedule_heats_title": "Heats",
  "schedule_heats_status_scheduled": "Geplant",
  "schedule_heats_status_closed": "Abgeschlossen",
  "schedule_heats_start_label": "Startzeit",
  "schedule_heats_start_override_submit": "Startzeit aktualisieren",
  "schedule_heats_start_override_error": "Die Startzeit konnte nicht aktualisiert werden. Bitte versuchen Sie es erneut.",
  "schedule_heats_results_time": "Zeit (Sekunden)",
  "schedule_heats_results_status": "Status",
  "schedule_heats_results_status_finished": "Beendet",
  "schedule_heats_results_submit": "Ergebnisse übermitteln",
  "schedule_heats_results_error": "Ergebnisse konnten nicht übermittelt werden. Bitte versuchen Sie es erneut."
```

- [ ] **Step 2: Write `ScheduleHeats`**

Create `frontend/src/pages/ScheduleHeats.tsx`:

```tsx
import { useState } from 'react'
import { useParams } from 'react-router-dom'
import { useTranslation } from 'react-i18next'
import { useMutation, useQueryClient } from '@tanstack/react-query'
import { api } from '../api/client'
import { useAuth } from '../auth/AuthContext'
import type { Course, Heat, Round } from '../api/types'

interface ScheduleHeatsProps {
  heats: Heat[]
  courses: Course[]
  currentRound: Round
}

type ResultEntry = { timeSeconds: string; status: string }

function teamIdsForHeat(heat: Heat, round: Round): string[] {
  if (heat.group_id !== null) {
    return round.groups.find((g) => g.id === heat.group_id)?.team_ids ?? []
  }
  return round.divisions.find((d) => d.id === heat.division_id)?.team_ids ?? []
}

function HeatRow({ heat, courseName, round }: { heat: Heat; courseName: string; round: Round }) {
  const { t } = useTranslation()
  const { id } = useParams<{ id: string }>()
  const { role } = useAuth()
  const queryClient = useQueryClient()
  const teamIds = teamIdsForHeat(heat, round)

  const [start, setStart] = useState(heat.planned_start)
  const startMutation = useMutation({
    mutationFn: () => api.patch(`/api/tournaments/${id}/heats/${heat.id}`, { planned_start: start }),
    onSuccess: () => {
      void queryClient.invalidateQueries({ queryKey: ['schedule', id] })
    },
  })

  const [entries, setEntries] = useState<Record<string, ResultEntry>>({})
  const resultsMutation = useMutation({
    mutationFn: () => {
      const body: Record<string, { time_seconds?: number; status?: string }> = {}
      for (const teamId of teamIds) {
        const entry = entries[teamId]
        if (!entry) continue
        if (entry.status) {
          body[teamId] = { status: entry.status }
        } else if (entry.timeSeconds) {
          body[teamId] = { time_seconds: Number(entry.timeSeconds) }
        }
      }
      return api.post(`/api/tournaments/${id}/heats/${heat.id}/results`, body)
    },
    onSuccess: () => {
      void queryClient.invalidateQueries({ queryKey: ['schedule', id] })
      void queryClient.invalidateQueries({ queryKey: ['rounds', id] })
    },
  })

  const canEnterResults = role === 'organizer' || role === 'time_entry'
  const isOpen = heat.status !== 'closed'

  return (
    <li className="rounded border p-3">
      <div className="mb-2 flex items-center justify-between text-sm">
        <span>
          {courseName} —{' '}
          {t(heat.status === 'closed' ? 'schedule_heats_status_closed' : 'schedule_heats_status_scheduled')}
        </span>
        {role === 'organizer' && (
          <div className="flex items-center gap-2">
            <label htmlFor={`start-${heat.id}`}>{t('schedule_heats_start_label')}</label>
            <input
              id={`start-${heat.id}`}
              value={start}
              onChange={(e) => setStart(e.target.value)}
              className="rounded border px-2 py-1 text-xs"
            />
            <button
              type="button"
              onClick={() => startMutation.mutate()}
              className="rounded border px-2 py-1 text-xs"
            >
              {t('schedule_heats_start_override_submit')}
            </button>
          </div>
        )}
      </div>
      {startMutation.isError && (
        <p role="alert" className="mb-2 text-xs text-red-600">
          {t('schedule_heats_start_override_error')}
        </p>
      )}

      {isOpen && canEnterResults && (
        <>
          <table className="w-full text-sm">
            <tbody>
              {teamIds.map((teamId) => (
                <tr key={teamId}>
                  <td className="py-1">{teamId}</td>
                  <td>
                    <label htmlFor={`time-${heat.id}-${teamId}`} className="sr-only">
                      {t('schedule_heats_results_time')} — {teamId}
                    </label>
                    <input
                      id={`time-${heat.id}-${teamId}`}
                      placeholder={t('schedule_heats_results_time')}
                      value={entries[teamId]?.timeSeconds ?? ''}
                      onChange={(e) =>
                        setEntries((prev) => ({
                          ...prev,
                          [teamId]: { timeSeconds: e.target.value, status: '' },
                        }))
                      }
                      className="w-24 rounded border px-2 py-1"
                    />
                  </td>
                  <td>
                    <label htmlFor={`status-${heat.id}-${teamId}`} className="sr-only">
                      {t('schedule_heats_results_status')} — {teamId}
                    </label>
                    <select
                      id={`status-${heat.id}-${teamId}`}
                      value={entries[teamId]?.status ?? ''}
                      onChange={(e) =>
                        setEntries((prev) => ({
                          ...prev,
                          [teamId]: { timeSeconds: '', status: e.target.value },
                        }))
                      }
                      className="rounded border px-2 py-1"
                    >
                      <option value="">{t('schedule_heats_results_status_finished')}</option>
                      <option value="DNF">DNF</option>
                      <option value="DSQ">DSQ</option>
                      <option value="DNS">DNS</option>
                    </select>
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
          {resultsMutation.isError && (
            <p role="alert" className="mt-2 text-xs text-red-600">
              {t('schedule_heats_results_error')}
            </p>
          )}
          <button
            type="button"
            onClick={() => resultsMutation.mutate()}
            disabled={resultsMutation.isPending}
            className="mt-2 rounded bg-blue-600 px-4 py-2 text-sm text-white disabled:opacity-50"
          >
            {t('schedule_heats_results_submit')}
          </button>
        </>
      )}
    </li>
  )
}

export function ScheduleHeats({ heats, courses, currentRound }: ScheduleHeatsProps) {
  const { t } = useTranslation()

  if (heats.length === 0) {
    return null
  }

  function courseName(courseId: number): string {
    return courses.find((c) => c.id === courseId)?.name ?? String(courseId)
  }

  return (
    <section className="mb-6 rounded border bg-white p-4">
      <h2 className="mb-3 text-lg font-semibold">{t('schedule_heats_title')}</h2>
      <ul className="space-y-3">
        {heats.map((heat) => (
          <HeatRow key={heat.id} heat={heat} courseName={courseName(heat.course_id)} round={currentRound} />
        ))}
      </ul>
    </section>
  )
}
```

- [ ] **Step 3: Write `ScheduleHeats.test.tsx`**

Create `frontend/src/pages/ScheduleHeats.test.tsx`:

```tsx
import { describe, expect, it, vi } from 'vitest'
import { render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { MemoryRouter, Route, Routes } from 'react-router-dom'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { ScheduleHeats } from './ScheduleHeats'
import * as client from '../api/client'
import { useAuth } from '../auth/AuthContext'
import type { Course, Heat, Round } from '../api/types'

vi.mock('react-i18next', () => ({
  useTranslation: () => ({ t: (key: string) => key }),
}))
vi.mock('../api/client', async () => {
  const actual = await vi.importActual<typeof import('../api/client')>('../api/client')
  return { ...actual, api: { ...actual.api, post: vi.fn(), patch: vi.fn() } }
})
vi.mock('../auth/AuthContext', () => ({ useAuth: vi.fn() }))

const round: Round = {
  id: 1,
  round_number: 1,
  status: 'open',
  groups: [{ id: 10, team_ids: ['1', '2'] }],
  divisions: [],
}
const courses: Course[] = [
  { id: 1, tournament_id: 1, name: 'Course A', heat_interval_seconds: 300, delay_offset_seconds: 0 },
]
const openHeat: Heat = {
  id: 100,
  round_id: 1,
  group_id: 10,
  division_id: null,
  course_id: 1,
  planned_start: '2026-01-01T10:00:00Z',
  effective_start: '2026-01-01T10:00:00Z',
  status: 'scheduled',
  results: [],
}

function renderHeats(heats: Heat[]) {
  render(
    <QueryClientProvider client={new QueryClient()}>
      <MemoryRouter initialEntries={['/tournaments/1/schedule']}>
        <Routes>
          <Route
            path="/tournaments/:id/schedule"
            element={<ScheduleHeats heats={heats} courses={courses} currentRound={round} />}
          />
        </Routes>
      </MemoryRouter>
    </QueryClientProvider>,
  )
}

describe('ScheduleHeats', () => {
  it('submits results for both teams in an open heat', async () => {
    vi.mocked(useAuth).mockReturnValue({ role: 'time_entry', token: 'x', login: vi.fn(), logout: vi.fn() })
    vi.mocked(client.api.post).mockResolvedValue({ results_recorded: 2 })

    renderHeats([openHeat])

    await userEvent.type(screen.getByLabelText('schedule_heats_results_time — 1'), '100.5')
    await userEvent.type(screen.getByLabelText('schedule_heats_results_time — 2'), '105.2')
    await userEvent.click(screen.getByText('schedule_heats_results_submit'))

    await waitFor(() =>
      expect(client.api.post).toHaveBeenCalledWith('/api/tournaments/1/heats/100/results', {
        '1': { time_seconds: 100.5 },
        '2': { time_seconds: 105.2 },
      }),
    )
  })

  it('does not render a results form for a closed heat', () => {
    vi.mocked(useAuth).mockReturnValue({ role: 'organizer', token: 'x', login: vi.fn(), logout: vi.fn() })

    renderHeats([{ ...openHeat, status: 'closed' }])

    expect(screen.queryByText('schedule_heats_results_submit')).not.toBeInTheDocument()
  })

  it('hides results entry for a spectator', () => {
    vi.mocked(useAuth).mockReturnValue({ role: 'spectator', token: 'x', login: vi.fn(), logout: vi.fn() })

    renderHeats([openHeat])

    expect(screen.queryByText('schedule_heats_results_submit')).not.toBeInTheDocument()
  })

  it('renders nothing when there are no heats', () => {
    vi.mocked(useAuth).mockReturnValue({ role: 'organizer', token: 'x', login: vi.fn(), logout: vi.fn() })

    const { container } = renderHeats([])
    expect(container).toBeEmptyDOMElement()
  })
})
```

- [ ] **Step 4: Run the test to verify it fails, then passes**

Run: `cd frontend && npx vitest run src/pages/ScheduleHeats.test.tsx`
Expected first: FAIL (module doesn't exist)
Expected after: PASS (4 tests)

- [ ] **Step 5: Wire into `SchedulePage`**

Modify `frontend/src/pages/SchedulePage.tsx` to pass the current round's
heats to `ScheduleHeats`:

```tsx
import { useParams } from 'react-router-dom'
import { useQuery } from '@tanstack/react-query'
import { api } from '../api/client'
import type { Course, Heat, Round } from '../api/types'
import { ScheduleCourses } from './ScheduleCourses'
import { ScheduleRoundCreate } from './ScheduleRoundCreate'
import { ScheduleAssignments } from './ScheduleAssignments'
import { ScheduleHeats } from './ScheduleHeats'

export function SchedulePage() {
  const { id } = useParams<{ id: string }>()

  const { data: roundsData } = useQuery({
    queryKey: ['rounds', id],
    queryFn: () => api.get<{ rounds: Round[] }>(`/api/tournaments/${id}/rounds`),
    enabled: !!id,
  })
  const rounds = roundsData?.rounds ?? []
  const currentRound = rounds[rounds.length - 1]

  const { data: coursesData } = useQuery({
    queryKey: ['courses', id],
    queryFn: () => api.get<{ courses: Course[] }>(`/api/tournaments/${id}/courses`),
    enabled: !!id,
  })
  const courses = coursesData?.courses ?? []

  const { data: scheduleData } = useQuery({
    queryKey: ['schedule', id],
    queryFn: () => api.get<{ heats: Heat[] }>(`/api/tournaments/${id}/schedule`),
    enabled: !!id,
  })
  const heats = scheduleData?.heats ?? []
  const currentRoundHeats = currentRound ? heats.filter((h) => h.round_id === currentRound.id) : []

  const scheduledGroupIds = new Set(heats.filter((h) => h.group_id !== null).map((h) => h.group_id))
  const scheduledDivisionIds = new Set(heats.filter((h) => h.division_id !== null).map((h) => h.division_id))

  const unscheduledGroups =
    currentRound?.groups
      .filter((g) => !scheduledGroupIds.has(g.id))
      .map((g) => ({ id: g.id, label: `Group ${g.id} (${g.team_ids.length} teams)` })) ?? []
  const unscheduledDivisions =
    currentRound?.divisions
      .filter((d) => !scheduledDivisionIds.has(d.id))
      .map((d) => ({ id: d.id, label: `${d.name} (${d.team_ids.length} teams)` })) ?? []

  return (
    <div>
      <ScheduleCourses />
      {rounds.length === 0 && <ScheduleRoundCreate />}
      {currentRound && unscheduledGroups.length > 0 && (
        <ScheduleAssignments mode="group" roundId={currentRound.id} items={unscheduledGroups} />
      )}
      {currentRound && unscheduledDivisions.length > 0 && (
        <ScheduleAssignments mode="division" roundId={currentRound.id} items={unscheduledDivisions} />
      )}
      {currentRound && (
        <ScheduleHeats heats={currentRoundHeats} courses={courses} currentRound={currentRound} />
      )}
    </div>
  )
}
```

Note: `currentRoundHeats` filters by `h.round_id === currentRound.id`,
which correctly includes both the round's own group-heats AND its
divisions' heats, since a `Division`'s heats carry the same `round_id`
as their parent round (per Plan 3's data model §3: "`RoundID` is
denormalized ... always set regardless of which of the two fields below
is set").

- [ ] **Step 6: Run the full frontend suite and typecheck**

Run: `cd frontend && npm test -- --run && npx tsc -b`
Expected: all tests pass, 0 type errors

- [ ] **Step 7: Verify i18n bundle parity**

Run the same parity check as Task 2 Step 13.
Expected: both empty

- [ ] **Step 8: Commit**

```bash
git add frontend/src/pages/ScheduleHeats.tsx frontend/src/pages/ScheduleHeats.test.tsx \
  frontend/src/pages/SchedulePage.tsx internal/i18n/bundles/en.json internal/i18n/bundles/de.json
git commit -m "feat: add heat list, results entry, and start-time override"
```

---

### Task 6: Round actions (Next Round / Cut Divisions) and round history

**Files:**
- Create: `frontend/src/pages/ScheduleRoundActions.tsx`
- Create: `frontend/src/pages/ScheduleRoundActions.test.tsx`
- Modify: `frontend/src/pages/SchedulePage.tsx` (render `ScheduleRoundActions`)
- Modify: `internal/i18n/bundles/en.json`, `internal/i18n/bundles/de.json`

**Interfaces:**
- Consumes: `Round` (Task 2), `useAuth()`.
- Produces: `ScheduleRoundActions` component (named export) with props
  `{currentRound: Round; allRounds: Round[]}`.

- [ ] **Step 1: Add i18n keys**

Add to `internal/i18n/bundles/en.json`:

```json
  "schedule_round_actions_next": "Create Next Round",
  "schedule_round_actions_next_error": "Could not create the next round. Please try again.",
  "schedule_round_actions_divisions_title": "Cut into Divisions",
  "schedule_round_actions_divisions_name": "Division name",
  "schedule_round_actions_divisions_size": "Size",
  "schedule_round_actions_divisions_add_cut": "Add division",
  "schedule_round_actions_divisions_submit": "Cut Divisions",
  "schedule_round_actions_divisions_error": "Could not cut divisions. Please try again.",
  "schedule_round_history_title": "Round history",
  "schedule_round_history_entry": "Round {{number}} — {{status}}"
```

Add to `internal/i18n/bundles/de.json`:

```json
  "schedule_round_actions_next": "Nächste Runde erstellen",
  "schedule_round_actions_next_error": "Die nächste Runde konnte nicht erstellt werden. Bitte versuchen Sie es erneut.",
  "schedule_round_actions_divisions_title": "In Divisionen einteilen",
  "schedule_round_actions_divisions_name": "Name der Division",
  "schedule_round_actions_divisions_size": "Größe",
  "schedule_round_actions_divisions_add_cut": "Division hinzufügen",
  "schedule_round_actions_divisions_submit": "Divisionen einteilen",
  "schedule_round_actions_divisions_error": "Divisionen konnten nicht eingeteilt werden. Bitte versuchen Sie es erneut.",
  "schedule_round_history_title": "Rundenverlauf",
  "schedule_round_history_entry": "Runde {{number}} — {{status}}"
```

- [ ] **Step 2: Write `ScheduleRoundActions`**

Create `frontend/src/pages/ScheduleRoundActions.tsx`:

```tsx
import { useState } from 'react'
import { useParams } from 'react-router-dom'
import { useTranslation } from 'react-i18next'
import { useMutation, useQueryClient } from '@tanstack/react-query'
import { api } from '../api/client'
import { useAuth } from '../auth/AuthContext'
import type { Round } from '../api/types'

interface ScheduleRoundActionsProps {
  currentRound: Round
  allRounds: Round[]
}

interface Cut {
  name: string
  size: number
}

export function ScheduleRoundActions({ currentRound, allRounds }: ScheduleRoundActionsProps) {
  const { t } = useTranslation()
  const { id } = useParams<{ id: string }>()
  const { role } = useAuth()
  const queryClient = useQueryClient()

  const nextRoundMutation = useMutation({
    mutationFn: () => api.post(`/api/tournaments/${id}/rounds/${currentRound.id}/next`),
    onSuccess: () => {
      void queryClient.invalidateQueries({ queryKey: ['rounds', id] })
    },
  })

  const [cuts, setCuts] = useState<Cut[]>([{ name: '', size: 1 }])
  const divisionsMutation = useMutation({
    mutationFn: () =>
      api.post(`/api/tournaments/${id}/rounds/${currentRound.id}/divisions`, {
        cuts: cuts.filter((c) => c.name.trim() !== ''),
      }),
    onSuccess: () => {
      void queryClient.invalidateQueries({ queryKey: ['rounds', id] })
    },
  })

  if (role !== 'organizer' || currentRound.status !== 'closed') {
    return (
      <RoundHistory allRounds={allRounds} />
    )
  }

  return (
    <>
      <section className="mb-6 rounded border bg-white p-4">
        <div className="mb-4">
          {nextRoundMutation.isError && (
            <p role="alert" className="mb-2 text-sm text-red-600">
              {t('schedule_round_actions_next_error')}
            </p>
          )}
          <button
            type="button"
            onClick={() => nextRoundMutation.mutate()}
            disabled={nextRoundMutation.isPending}
            className="rounded bg-blue-600 px-4 py-2 text-sm text-white disabled:opacity-50"
          >
            {t('schedule_round_actions_next')}
          </button>
        </div>

        <h3 className="mb-2 text-sm font-semibold">{t('schedule_round_actions_divisions_title')}</h3>
        {cuts.map((cut, index) => (
          <div key={index} className="mb-2 flex items-end gap-2">
            <div>
              <label htmlFor={`cut-name-${index}`} className="block text-xs font-medium">
                {t('schedule_round_actions_divisions_name')}
              </label>
              <input
                id={`cut-name-${index}`}
                value={cut.name}
                onChange={(e) =>
                  setCuts((prev) => prev.map((c, i) => (i === index ? { ...c, name: e.target.value } : c)))
                }
                className="rounded border px-2 py-1 text-sm"
              />
            </div>
            <div>
              <label htmlFor={`cut-size-${index}`} className="block text-xs font-medium">
                {t('schedule_round_actions_divisions_size')}
              </label>
              <input
                id={`cut-size-${index}`}
                type="number"
                min={1}
                value={cut.size}
                onChange={(e) =>
                  setCuts((prev) =>
                    prev.map((c, i) => (i === index ? { ...c, size: Number(e.target.value) } : c)),
                  )
                }
                className="w-20 rounded border px-2 py-1 text-sm"
              />
            </div>
          </div>
        ))}
        <button
          type="button"
          onClick={() => setCuts((prev) => [...prev, { name: '', size: 1 }])}
          className="mb-3 rounded border px-3 py-1 text-xs"
        >
          {t('schedule_round_actions_divisions_add_cut')}
        </button>
        {divisionsMutation.isError && (
          <p role="alert" className="mb-2 text-sm text-red-600">
            {t('schedule_round_actions_divisions_error')}
          </p>
        )}
        <div>
          <button
            type="button"
            onClick={() => divisionsMutation.mutate()}
            disabled={divisionsMutation.isPending}
            className="rounded bg-blue-600 px-4 py-2 text-sm text-white disabled:opacity-50"
          >
            {t('schedule_round_actions_divisions_submit')}
          </button>
        </div>
      </section>
      <RoundHistory allRounds={allRounds} />
    </>
  )
}

function RoundHistory({ allRounds }: { allRounds: Round[] }) {
  const { t } = useTranslation()
  const earlierRounds = allRounds.slice(0, -1)

  if (earlierRounds.length === 0) {
    return null
  }

  return (
    <section className="mb-6 rounded border bg-white p-4">
      <h2 className="mb-3 text-lg font-semibold">{t('schedule_round_history_title')}</h2>
      <ul className="space-y-1 text-sm text-gray-600">
        {earlierRounds.map((round) => (
          <li key={round.id}>
            {t('schedule_round_history_entry', { number: round.round_number, status: round.status })}
          </li>
        ))}
      </ul>
    </section>
  )
}
```

- [ ] **Step 3: Write `ScheduleRoundActions.test.tsx`**

Create `frontend/src/pages/ScheduleRoundActions.test.tsx`:

```tsx
import { describe, expect, it, vi } from 'vitest'
import { render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { MemoryRouter, Route, Routes } from 'react-router-dom'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { ScheduleRoundActions } from './ScheduleRoundActions'
import * as client from '../api/client'
import { useAuth } from '../auth/AuthContext'
import type { Round } from '../api/types'

vi.mock('react-i18next', () => ({
  useTranslation: () => ({ t: (key: string, vars?: Record<string, unknown>) => (vars ? `${key} ${JSON.stringify(vars)}` : key) }),
}))
vi.mock('../api/client', async () => {
  const actual = await vi.importActual<typeof import('../api/client')>('../api/client')
  return { ...actual, api: { ...actual.api, post: vi.fn() } }
})
vi.mock('../auth/AuthContext', () => ({ useAuth: vi.fn() }))

const closedRound: Round = { id: 2, round_number: 2, status: 'closed', groups: [], divisions: [] }
const earlierRound: Round = { id: 1, round_number: 1, status: 'closed', groups: [], divisions: [] }

function renderActions(currentRound: Round, allRounds: Round[]) {
  render(
    <QueryClientProvider client={new QueryClient()}>
      <MemoryRouter initialEntries={['/tournaments/1/schedule']}>
        <Routes>
          <Route
            path="/tournaments/:id/schedule"
            element={<ScheduleRoundActions currentRound={currentRound} allRounds={allRounds} />}
          />
        </Routes>
      </MemoryRouter>
    </QueryClientProvider>,
  )
}

describe('ScheduleRoundActions', () => {
  it('creates the next round', async () => {
    vi.mocked(useAuth).mockReturnValue({ role: 'organizer', token: 'x', login: vi.fn(), logout: vi.fn() })
    vi.mocked(client.api.post).mockResolvedValue({ id: 3, round_number: 3, status: 'open', groups: [], divisions: [] })

    renderActions(closedRound, [earlierRound, closedRound])

    await userEvent.click(screen.getByText('schedule_round_actions_next'))

    await waitFor(() =>
      expect(client.api.post).toHaveBeenCalledWith('/api/tournaments/1/rounds/2/next'),
    )
  })

  it('cuts divisions with the entered cuts', async () => {
    vi.mocked(useAuth).mockReturnValue({ role: 'organizer', token: 'x', login: vi.fn(), logout: vi.fn() })
    vi.mocked(client.api.post).mockResolvedValue({ divisions: [] })

    renderActions(closedRound, [earlierRound, closedRound])

    await userEvent.type(screen.getByLabelText('schedule_round_actions_divisions_name'), 'Gold')
    await userEvent.click(screen.getByText('schedule_round_actions_divisions_submit'))

    await waitFor(() =>
      expect(client.api.post).toHaveBeenCalledWith('/api/tournaments/1/rounds/2/divisions', {
        cuts: [{ name: 'Gold', size: 1 }],
      }),
    )
  })

  it('does not render round actions for an open round', () => {
    vi.mocked(useAuth).mockReturnValue({ role: 'organizer', token: 'x', login: vi.fn(), logout: vi.fn() })

    renderActions({ ...closedRound, status: 'open' }, [{ ...closedRound, status: 'open' }])

    expect(screen.queryByText('schedule_round_actions_next')).not.toBeInTheDocument()
  })

  it('renders round history for earlier rounds', () => {
    vi.mocked(useAuth).mockReturnValue({ role: 'time_entry', token: 'x', login: vi.fn(), logout: vi.fn() })

    renderActions(closedRound, [earlierRound, closedRound])

    expect(screen.getByText('schedule_round_history_title')).toBeInTheDocument()
  })
})
```

- [ ] **Step 4: Run the test to verify it fails, then passes**

Run: `cd frontend && npx vitest run src/pages/ScheduleRoundActions.test.tsx`
Expected first: FAIL (module doesn't exist)
Expected after: PASS (4 tests)

- [ ] **Step 5: Wire into `SchedulePage`**

Modify `frontend/src/pages/SchedulePage.tsx`, adding the import and
rendering `ScheduleRoundActions` after `ScheduleHeats`:

```tsx
import { ScheduleRoundActions } from './ScheduleRoundActions'
```

```tsx
      {currentRound && (
        <ScheduleHeats heats={currentRoundHeats} courses={courses} currentRound={currentRound} />
      )}
      {currentRound && <ScheduleRoundActions currentRound={currentRound} allRounds={rounds} />}
    </div>
  )
}
```

- [ ] **Step 6: Run the full frontend suite and typecheck**

Run: `cd frontend && npm test -- --run && npx tsc -b`
Expected: all tests pass, 0 type errors

- [ ] **Step 7: Verify i18n bundle parity**

Run the same parity check as Task 2 Step 13.
Expected: both empty

- [ ] **Step 8: Commit**

```bash
git add frontend/src/pages/ScheduleRoundActions.tsx frontend/src/pages/ScheduleRoundActions.test.tsx \
  frontend/src/pages/SchedulePage.tsx internal/i18n/bundles/en.json internal/i18n/bundles/de.json
git commit -m "feat: add next-round/divisions triggers and round history"
```

---

### Task 7: Playwright e2e test — full round lifecycle

**Files:**
- Create: `frontend/e2e/round-lifecycle.spec.ts`
- Modify: `frontend/e2e/fixtures/teams.csv` — no change needed (reuse Task 12's existing fixture from Plan 4a); confirm it has at least 4 valid rows (needed for two groups of 2)

**Interfaces:**
- Consumes: `frontend/e2e/run-server.sh` (Plan 4a's real-binary test
  harness, unchanged), `frontend/e2e/fixtures/teams.csv` (Plan 4a's
  fixture).
- Produces: nothing consumed by later tasks — this is the plan's final
  verification.

- [ ] **Step 1: Confirm the existing fixture has enough teams**

Run: `cat frontend/e2e/fixtures/teams.csv`
Expected: at least 4 rows with valid data (one deliberately bad row is
fine and expected per Plan 4a's Task 12 — the bad row is simply not
selected into a group in this test). If fewer than 4 valid rows exist,
add rows following the existing CSV's column format until there are at
least 4 valid teams, and note this as a deviation in your report (the
file is shared with Plan 4a's `setup-flow.spec.ts`, so check that test
still passes after any addition).

- [ ] **Step 2: Write the e2e test**

Create `frontend/e2e/round-lifecycle.spec.ts`:

```typescript
import { test, expect } from '@playwright/test'
import path from 'node:path'

test('organizer runs a full round: create, schedule, results, next round', async ({ page }) => {
  await page.goto('/login')
  await page.getByLabel('login_username', { exact: false }).fill('organizer1')
  await page.getByLabel('login_password', { exact: false }).fill('pw')
  await page.getByRole('button', { name: /sign in/i }).click()

  await page.getByRole('link', { name: /create tournament/i }).click()
  await page.getByLabel('Name', { exact: true }).fill('Lifecycle Test Cup')
  await page.getByLabel(/sport/i).selectOption({ index: 1 })
  await page.getByLabel(/tournament type/i).selectOption({ index: 1 })
  await page.getByRole('button', { name: /create/i }).click()

  await expect(page).toHaveURL(/\/tournaments\/\d+\/teams$/)

  await page.getByRole('link', { name: /import from file/i }).click()
  const fileInput = page.locator('input[type="file"]')
  await fileInput.setInputFiles(path.join(__dirname, 'fixtures', 'teams.csv'))
  await page.getByRole('button', { name: /upload/i }).click()
  await expect(page.getByText(/imported/i)).toBeVisible()

  await page.getByRole('link', { name: /rounds & schedule/i }).click()

  // Courses
  await page.getByLabel(/^Name$/).fill('Lane 1')
  await page.getByLabel(/heat interval/i).fill('60')
  await page.getByRole('button', { name: /add course/i }).click()
  await expect(page.getByText(/Lane 1/)).toBeVisible()

  // Round 1
  await page.getByLabel(/number of groups/i).fill('2')
  await page.getByRole('button', { name: /shuffle into groups/i }).click()
  await page.getByRole('button', { name: /^create round 1$/i }).click()
  await expect(page.getByText(/schedule this round's groups/i)).toBeVisible()

  // Schedule both groups onto the one course
  const courseSelects = page.locator('select[aria-label*="Course —"]')
  const selectCount = await courseSelects.count()
  for (let i = 0; i < selectCount; i++) {
    await courseSelects.nth(i).selectOption({ label: 'Lane 1' })
  }
  await page.getByRole('button', { name: /^schedule$/i }).click()
  await expect(page.getByText(/^heats$/i)).toBeVisible()

  // Submit results for every heat
  const timeInputs = page.locator('input[placeholder="Time (seconds)"]')
  const timeCount = await timeInputs.count()
  for (let i = 0; i < timeCount; i++) {
    await timeInputs.nth(i).fill(String(100 + i * 5))
  }
  const submitButtons = page.getByRole('button', { name: /submit results/i })
  const submitCount = await submitButtons.count()
  for (let i = 0; i < submitCount; i++) {
    await submitButtons.nth(0).click()
    await page.waitForTimeout(200)
  }

  // Round should now be closed and offer Next Round
  await expect(page.getByRole('button', { name: /create next round/i })).toBeVisible({ timeout: 10000 })
  await page.getByRole('button', { name: /create next round/i }).click()

  // A new round's groups appear, proving reseeding ran end to end
  await expect(page.getByText(/schedule_round_history_title|round history/i)).toBeVisible()
})
```

- [ ] **Step 3: Run the e2e suite**

Run: `cd frontend && npm run build && npm run test:e2e`
Expected: the new `round-lifecycle.spec.ts` passes alongside the
existing `setup-flow.spec.ts`. If any locator doesn't match (label text,
button name), inspect the actual rendered DOM from the failure output
and adjust the locator to match what Tasks 2-6 actually rendered —
follow the same "reproduce the exact failure, then fix the locator"
discipline Plan 4a's Task 12 established, and document any such
deviation in your report.

- [ ] **Step 4: Run the full test suite one final time**

Run:
```bash
go build ./... && go test ./... -count=1
cd frontend && npm test -- --run && npx tsc -b && npm run test:e2e
```
Expected: everything green.

- [ ] **Step 5: Commit**

```bash
git add frontend/e2e/round-lifecycle.spec.ts
git commit -m "test: add Playwright end-to-end test for the round lifecycle"
```
