# TournamentStudio Web UI Watch Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Build Plan 4c — a live, read-only Watch tab (ranked schedule/standings, driven by the existing WebSocket hub) and a top-level Plugins browser with upload/delete/reload management.

**Architecture:** Two independent feature slices sharing no code. Watch: a new `GET /standings` endpoint that reuses the existing `internal/ranking.Rank()` primitive, plus a frontend page and WebSocket hook that invalidate a TanStack Query on any broadcast message. Plugins: the `plugin.Engine` becomes hot-swappable behind an `atomic.Pointer`, with new upload/delete endpoints that validate a `.lua` file in isolation before persisting and reloading, plus a frontend catalog/management page.

**Tech Stack:** Go 1.25, `net/http` ServeMux, `github.com/coder/websocket` (already a dependency), React 19 + TypeScript + Vite + TanStack Query 5 + react-i18next (already established), Playwright e2e.

**Spec:** `docs/superpowers/specs/2026-08-28-tournamentstudio-web-ui-watch-design.md`

## Global Constraints

- Every mutation's error state renders via `role="alert"`, matching every existing form in this codebase (`TournamentCreatePage`, `TeamImportPage`, `TeamsTab`, 4b's screens).
- No client-side re-validation of business rules the backend already validates — surface API errors as-is.
- All new backend routes register through the existing `authenticated`/`organizerOnly` middleware aliases already defined in `internal/server/server.go`'s `routes()` — no new middleware.
- Component/unit tests use Vitest + React Testing Library with `api` mocked and `react-i18next` mocked to an identity translator (`t: (key) => key`), matching every existing `*.test.tsx` in `frontend/src/pages/`.
- Go tests use the existing `internal/server` test helpers (`newTestServer`, `loginAs`, `createTestTournament`, `createTestRound`, `submitHeatResultsForRound`, `mapLabels`) wherever a test needs a running server — do not reinvent tournament/round/result setup.

---

### Task 1: `GET /api/tournaments/{id}/standings` — ranked results endpoint

**Files:**
- Create: `internal/server/handlers_standings.go`
- Create: `internal/server/handlers_standings_test.go`
- Modify: `internal/server/server.go` (register the new route)

**Interfaces:**
- Consumes: `s.rounds.ListRounds(tournamentID int64) ([]round.PrePhaseRound, error)`, `s.rounds.ListGroups(roundID int64) ([]round.Group, error)`, `s.schedule.ListDivisionsForRound(roundID int64) ([]schedule.Division, error)`, `s.schedule.ListResultsForRound(roundID int64) ([]schedule.HeatResult, error)` — all pre-existing. `ranking.Rank(results []ranking.TeamResult) []ranking.TeamResult` — pre-existing, from `internal/ranking`.
- Produces: `GET /api/tournaments/{id}/standings`, any authenticated role, returning:
  ```json
  {"rounds": [{"id": 1, "round_number": 1, "status": "closed",
    "standings": [{"group_id": 1, "division_id": null, "division_name": null,
      "ranked_teams": [{"rank": 1, "team_id": "3", "time_seconds": 124.11, "status": ""}]}]}]}
  ```
  Later tasks (Task 5) consume this shape directly as `StandingsResponse` on the frontend.

- [ ] **Step 1: Write the failing tests**

Create `internal/server/handlers_standings_test.go`:

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

func TestGetStandingsRanksGroupByTime(t *testing.T) {
	s := newTestServer(t)
	token := loginAs(t, s, "organizer1", "pw", auth.RoleOrganizer)
	tournamentID := createTestTournament(t, s, token)
	roundID, ids := createTestRound(t, s, token, tournamentID, [][]string{
		{"A1", "A2", "A3"},
	})

	submitHeatResultsForRound(t, s, token, tournamentID, roundID, ids,
		"A1", map[string]any{"time_seconds": 130.5},
		"A2", map[string]any{"time_seconds": 124.11},
		"A3", map[string]any{"status": "DNF"},
	)

	req := httptest.NewRequest(http.MethodGet, fmt.Sprintf("/api/tournaments/%d/standings", tournamentID), nil)
	req.Header.Set("Authorization", "Bearer "+token)
	rec := httptest.NewRecorder()
	s.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}

	var resp struct {
		Rounds []struct {
			ID          int64  `json:"id"`
			RoundNumber int    `json:"round_number"`
			Status      string `json:"status"`
			Standings   []struct {
				GroupID      *int64  `json:"group_id"`
				DivisionID   *int64  `json:"division_id"`
				DivisionName *string `json:"division_name"`
				RankedTeams  []struct {
					Rank        int      `json:"rank"`
					TeamID      string   `json:"team_id"`
					TimeSeconds *float64 `json:"time_seconds"`
					Status      string   `json:"status"`
				} `json:"ranked_teams"`
			} `json:"standings"`
		} `json:"rounds"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode standings response: %v", err)
	}

	if len(resp.Rounds) != 1 {
		t.Fatalf("expected 1 round, got %d", len(resp.Rounds))
	}
	rd := resp.Rounds[0]
	if rd.ID != roundID || rd.RoundNumber != 1 {
		t.Fatalf("unexpected round: %+v", rd)
	}
	if len(rd.Standings) != 1 {
		t.Fatalf("expected 1 standings entry (one group), got %d", len(rd.Standings))
	}
	entry := rd.Standings[0]
	if entry.GroupID == nil || entry.DivisionID != nil {
		t.Fatalf("expected a group entry with nil division_id, got %+v", entry)
	}
	if len(entry.RankedTeams) != 3 {
		t.Fatalf("expected 3 ranked teams, got %d", len(entry.RankedTeams))
	}
	wantOrder := mapLabels(ids, "A2", "A1", "A3")
	for i, teamID := range wantOrder {
		if entry.RankedTeams[i].TeamID != teamID {
			t.Fatalf("position %d: expected team %s, got %s", i, teamID, entry.RankedTeams[i].TeamID)
		}
		if entry.RankedTeams[i].Rank != i+1 {
			t.Fatalf("position %d: expected rank %d, got %d", i, i+1, entry.RankedTeams[i].Rank)
		}
	}
	if entry.RankedTeams[2].Status != "DNF" {
		t.Fatalf("expected last-place team to have status DNF, got %q", entry.RankedTeams[2].Status)
	}
}

func TestGetStandingsOmitsTeamsWithoutResults(t *testing.T) {
	s := newTestServer(t)
	token := loginAs(t, s, "organizer1", "pw", auth.RoleOrganizer)
	tournamentID := createTestTournament(t, s, token)
	roundID, ids := createTestRound(t, s, token, tournamentID, [][]string{
		{"A1", "A2"},
	})

	// Only A1 has a submitted result; A2 hasn't raced yet and must not
	// appear in ranked_teams at all (never sorted to the bottom
	// indistinguishable from a real DNF/DSQ/DNS).
	submitHeatResultsForRound(t, s, token, tournamentID, roundID, ids,
		"A1", map[string]any{"time_seconds": 100.0},
	)

	req := httptest.NewRequest(http.MethodGet, fmt.Sprintf("/api/tournaments/%d/standings", tournamentID), nil)
	req.Header.Set("Authorization", "Bearer "+token)
	rec := httptest.NewRecorder()
	s.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}

	var resp struct {
		Rounds []struct {
			Standings []struct {
				RankedTeams []struct {
					TeamID string `json:"team_id"`
				} `json:"ranked_teams"`
			} `json:"standings"`
		} `json:"rounds"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode standings response: %v", err)
	}

	rankedTeams := resp.Rounds[0].Standings[0].RankedTeams
	if len(rankedTeams) != 1 {
		t.Fatalf("expected exactly 1 ranked team (only A1 has a result), got %d", len(rankedTeams))
	}
	if rankedTeams[0].TeamID != ids["A1"] {
		t.Fatalf("expected the ranked team to be A1, got %s", rankedTeams[0].TeamID)
	}
}

func TestGetStandingsUsesDivisionsAfterCut(t *testing.T) {
	s := newTestServer(t)
	token := loginAs(t, s, "organizer1", "pw", auth.RoleOrganizer)
	tournamentID := createTestTournament(t, s, token)
	roundID, ids := createTestRound(t, s, token, tournamentID, [][]string{
		{"t1", "t2", "t3", "t4"},
	})

	submitHeatResultsForRound(t, s, token, tournamentID, roundID, ids,
		"t1", map[string]any{"time_seconds": 100.0},
		"t2", map[string]any{"time_seconds": 110.0},
		"t3", map[string]any{"time_seconds": 200.0},
		"t4", map[string]any{"time_seconds": 210.0},
	)

	divisionsBody := `{"cuts": [{"name": "Gold Final", "size": 2}, {"name": "Silver Final", "size": 2}]}`
	divisionsReq := httptest.NewRequest(http.MethodPost, fmt.Sprintf("/api/tournaments/%d/rounds/%d/divisions", tournamentID, roundID), stringsReader(divisionsBody))
	divisionsReq.Header.Set("Authorization", "Bearer "+token)
	divisionsRec := httptest.NewRecorder()
	s.ServeHTTP(divisionsRec, divisionsReq)
	if divisionsRec.Code != http.StatusOK {
		t.Fatalf("compute divisions: expected 200, got %d: %s", divisionsRec.Code, divisionsRec.Body.String())
	}

	req := httptest.NewRequest(http.MethodGet, fmt.Sprintf("/api/tournaments/%d/standings", tournamentID), nil)
	req.Header.Set("Authorization", "Bearer "+token)
	rec := httptest.NewRecorder()
	s.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}

	var resp struct {
		Rounds []struct {
			Standings []struct {
				GroupID      *int64  `json:"group_id"`
				DivisionID   *int64  `json:"division_id"`
				DivisionName *string `json:"division_name"`
			} `json:"standings"`
		} `json:"rounds"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode standings response: %v", err)
	}

	if len(resp.Rounds[0].Standings) != 2 {
		t.Fatalf("expected 2 division entries, got %d", len(resp.Rounds[0].Standings))
	}
	for _, entry := range resp.Rounds[0].Standings {
		if entry.GroupID != nil {
			t.Fatalf("expected nil group_id once divisions exist, got %v", *entry.GroupID)
		}
		if entry.DivisionID == nil || entry.DivisionName == nil {
			t.Fatalf("expected division_id and division_name to be set, got %+v", entry)
		}
	}
}
```

`stringsReader` doesn't exist yet in this package — add it as a tiny local helper at the bottom of the same file (other test files in this package build request bodies with `bytes.NewReader(body)` after `json.Marshal`, but this test uses a literal JSON string for readability, so it needs `strings.NewReader` wrapped to return an `io.Reader`):

```go
func stringsReader(s string) *strings.Reader {
	return strings.NewReader(s)
}
```

Add `"strings"` to the import block.

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./internal/server/... -run TestGetStandings -v`
Expected: FAIL — `s.handleGetStandings` / route `/standings` does not exist (404s, or a compile error if you added the route registration line already — add it only in Step 3).

- [ ] **Step 3: Implement the handler**

Create `internal/server/handlers_standings.go`:

```go
package server

import (
	"encoding/json"
	"net/http"
	"strconv"

	"tournamentstudio/internal/ranking"
	"tournamentstudio/internal/schedule"
)

type rankedTeamResponse struct {
	Rank        int      `json:"rank"`
	TeamID      string   `json:"team_id"`
	TimeSeconds *float64 `json:"time_seconds"`
	Status      string   `json:"status"`
}

type standingsEntryResponse struct {
	GroupID      *int64                `json:"group_id"`
	DivisionID   *int64                `json:"division_id"`
	DivisionName *string               `json:"division_name"`
	RankedTeams  []rankedTeamResponse  `json:"ranked_teams"`
}

type standingsRoundResponse struct {
	ID          int64                     `json:"id"`
	RoundNumber int                       `json:"round_number"`
	Status      string                    `json:"status"`
	Standings   []standingsEntryResponse  `json:"standings"`
}

// handleGetStandings returns every round's groups -- or, once a round has
// divisions, its divisions instead -- with teams ranked fastest-first via
// ranking.Rank(). Unlike handleComputeDivisions (which only ever runs on a
// closed round, so it can safely treat "no result yet" as the worst
// possible outcome), this endpoint must also render correctly for an open,
// in-progress round: a team with no entry in that round's results is
// omitted from ranked_teams entirely, not sorted to the bottom
// indistinguishable from a real DNF/DSQ/DNS.
func (s *Server) handleGetStandings(w http.ResponseWriter, r *http.Request) {
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

	resp := make([]standingsRoundResponse, 0, len(rounds))
	for _, rd := range rounds {
		results, err := s.schedule.ListResultsForRound(rd.ID)
		if err != nil {
			http.Error(w, "could not list results", http.StatusInternalServerError)
			return
		}
		resultsByTeam := make(map[string]schedule.HeatResult, len(results))
		for _, res := range results {
			resultsByTeam[res.TeamID] = res
		}

		divisions, err := s.schedule.ListDivisionsForRound(rd.ID)
		if err != nil {
			http.Error(w, "could not list divisions", http.StatusInternalServerError)
			return
		}

		var standings []standingsEntryResponse
		if len(divisions) > 0 {
			for _, d := range divisions {
				name := d.Name
				standings = append(standings, buildStandingsEntry(nil, &d.ID, &name, d.TeamIDs, resultsByTeam))
			}
		} else {
			groups, err := s.rounds.ListGroups(rd.ID)
			if err != nil {
				http.Error(w, "could not list groups", http.StatusInternalServerError)
				return
			}
			for _, g := range groups {
				standings = append(standings, buildStandingsEntry(&g.ID, nil, nil, g.TeamIDs, resultsByTeam))
			}
		}
		if standings == nil {
			standings = []standingsEntryResponse{}
		}

		resp = append(resp, standingsRoundResponse{
			ID:          rd.ID,
			RoundNumber: rd.RoundNumber,
			Status:      string(rd.Status),
			Standings:   standings,
		})
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]any{"rounds": resp})
}

func buildStandingsEntry(groupID, divisionID *int64, divisionName *string, teamIDs []string, resultsByTeam map[string]schedule.HeatResult) standingsEntryResponse {
	var teamResults []ranking.TeamResult
	for _, teamID := range teamIDs {
		res, ok := resultsByTeam[teamID]
		if !ok {
			continue
		}
		teamResults = append(teamResults, ranking.TeamResult{
			TeamID:      teamID,
			TimeSeconds: res.TimeSeconds,
			Status:      ranking.Status(res.Status),
		})
	}
	ranked := ranking.Rank(teamResults)
	rankedTeams := make([]rankedTeamResponse, len(ranked))
	for i, res := range ranked {
		rankedTeams[i] = rankedTeamResponse{
			Rank:        i + 1,
			TeamID:      res.TeamID,
			TimeSeconds: res.TimeSeconds,
			Status:      string(res.Status),
		}
	}
	return standingsEntryResponse{
		GroupID:      groupID,
		DivisionID:   divisionID,
		DivisionName: divisionName,
		RankedTeams:  rankedTeams,
	}
}
```

Register the route in `internal/server/server.go`, inside `routes()`, immediately after the existing `GET /api/tournaments/{id}/schedule` line:

```go
	s.mux.Handle("GET /api/tournaments/{id}/standings", authenticated(http.HandlerFunc(s.handleGetStandings)))
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./internal/server/... -run TestGetStandings -v`
Expected: PASS (3 tests)

- [ ] **Step 5: Run the full Go suite**

Run: `go build ./... && go test ./... -count=1`
Expected: all packages ok

- [ ] **Step 6: Commit**

```bash
git add internal/server/handlers_standings.go internal/server/handlers_standings_test.go internal/server/server.go
git commit -m "feat: add ranked standings endpoint"
```

---

### Task 2: Plugin `Source` metadata + isolated `Validate()`

**Files:**
- Modify: `internal/plugin/types.go`
- Modify: `internal/plugin/engine.go`
- Create: `internal/plugin/validate.go`
- Create: `internal/plugin/validate_test.go`
- Modify: `internal/plugin/engine_test.go` (add one test)

**Interfaces:**
- Consumes: nothing from earlier tasks.
- Produces: `SportPlugin.Source string` and `TournamentTypePlugin.Source string` (`"bundled"` or the external filename), and `plugin.Validate(name string, source []byte) error`. Task 3 uses both.

- [ ] **Step 1: Write the failing tests**

Append to `internal/plugin/engine_test.go` (after the existing tests, using the file's existing `writeTestPlugin`/`findSport` helpers):

```go
func TestLoadSetsSourceForBundledAndExternalPlugins(t *testing.T) {
	dir := t.TempDir()
	writeTestPlugin(t, dir, "external-sport.lua", `
return {
  id = "external-sport",
  display_name = "External Sport",
  compatible_tournament_types = {"test-format"},
}
`)

	e, err := Load(dir)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}

	bundled := findSport(e, "dragonboat")
	if bundled == nil {
		t.Fatal("expected bundled dragonboat sport plugin to be loaded")
	}
	if bundled.Source != "bundled" {
		t.Fatalf("expected bundled plugin's Source to be %q, got %q", "bundled", bundled.Source)
	}

	external := findSport(e, "external-sport")
	if external == nil {
		t.Fatal("expected external-sport plugin to be loaded")
	}
	if external.Source != "external-sport.lua" {
		t.Fatalf("expected external plugin's Source to be its filename, got %q", external.Source)
	}
}
```

Create `internal/plugin/validate_test.go`:

```go
package plugin

import "testing"

func TestValidateAcceptsWellFormedSportPlugin(t *testing.T) {
	source := []byte(`
return {
  id = "test-sport",
  display_name = "Test Sport",
  compatible_tournament_types = {"single-elim"},
}
`)
	if err := Validate("test-sport.lua", source); err != nil {
		t.Fatalf("expected valid source to pass, got: %v", err)
	}
}

func TestValidateAcceptsWellFormedTournamentTypePlugin(t *testing.T) {
	source := []byte(`
return {
  id = "test-type",
  compatible_sports = {"test-sport"},
  next_round_groups = function(ranked_groups) return {} end,
  division_cuts = function(ranked_teams, cuts) return {} end,
}
`)
	if err := Validate("test-type.lua", source); err != nil {
		t.Fatalf("expected valid source to pass, got: %v", err)
	}
}

func TestValidateRejectsMalformedLua(t *testing.T) {
	if err := Validate("broken.lua", []byte("this is not valid lua {{{")); err == nil {
		t.Fatal("expected an error for malformed Lua source")
	}
}

func TestValidateRejectsMissingID(t *testing.T) {
	source := []byte(`return { compatible_tournament_types = {"single-elim"} }`)
	if err := Validate("no-id.lua", source); err == nil {
		t.Fatal("expected an error for a plugin table with no id")
	}
}

func TestValidateRejectsUnrecognizedShape(t *testing.T) {
	source := []byte(`return { id = "mystery" }`)
	if err := Validate("mystery.lua", source); err == nil {
		t.Fatal("expected an error for a plugin table with neither known field")
	}
}

func TestValidateDoesNotRegisterOnAnyLiveEngine(t *testing.T) {
	dir := t.TempDir()
	e, err := Load(dir)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}

	source := []byte(`
return {
  id = "not-registered",
  display_name = "Not Registered",
  compatible_tournament_types = {"single-elim"},
}
`)
	if err := Validate("not-registered.lua", source); err != nil {
		t.Fatalf("expected valid source to pass, got: %v", err)
	}

	if findSport(e, "not-registered") != nil {
		t.Fatal("Validate must not register the plugin on any pre-existing Engine")
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./internal/plugin/... -run 'TestLoadSetsSource|TestValidate' -v`
Expected: FAIL — compile errors (`Source` field and `Validate` function don't exist yet)

- [ ] **Step 3: Add the `Source` field**

In `internal/plugin/types.go`, change:

```go
type SportPlugin struct {
	ID                        string
	DisplayName               string
	CompatibleTournamentTypes []string
	RosterFields              []RosterField
}
```
to:
```go
type SportPlugin struct {
	ID                        string
	DisplayName               string
	CompatibleTournamentTypes []string
	RosterFields              []RosterField
	Source                    string
}
```

and change:
```go
type TournamentTypePlugin struct {
	ID               string
	CompatibleSports []string

	state             *lua.LState
	mu                sync.Mutex
	nextRoundGroupsFn *lua.LFunction
	divisionCutsFn    *lua.LFunction
}
```
to:
```go
type TournamentTypePlugin struct {
	ID               string
	CompatibleSports []string
	Source           string

	state             *lua.LState
	mu                sync.Mutex
	nextRoundGroupsFn *lua.LFunction
	divisionCutsFn    *lua.LFunction
}
```

- [ ] **Step 4: Set `Source` in `loadSource`, threaded from `Load`**

In `internal/plugin/engine.go`, change the `loadSource` signature and its two registration branches. Replace:

```go
func (e *Engine) loadSource(name string, source []byte) error {
```
with:
```go
func (e *Engine) loadSource(name string, source []byte, pluginSource string) error {
```

Replace:
```go
	switch {
	case tbl.RawGetString("compatible_tournament_types") != lua.LNil:
		sp := parseSportPlugin(tbl, id)
		e.sports[sp.ID] = sp
		L.Close()
		return nil
	case tbl.RawGetString("compatible_sports") != lua.LNil:
		ttp, err := parseTournamentTypePlugin(tbl, id, L)
		if err != nil {
			L.Close()
			return fmt.Errorf("%s: %w", name, err)
		}
		e.tournamentTypes[ttp.ID] = ttp
		return nil
	default:
		L.Close()
		return fmt.Errorf("%s: plugin table must have either 'compatible_tournament_types' or 'compatible_sports'", name)
	}
```
with:
```go
	switch {
	case tbl.RawGetString("compatible_tournament_types") != lua.LNil:
		sp := parseSportPlugin(tbl, id)
		sp.Source = pluginSource
		e.sports[sp.ID] = sp
		L.Close()
		return nil
	case tbl.RawGetString("compatible_sports") != lua.LNil:
		ttp, err := parseTournamentTypePlugin(tbl, id, L)
		if err != nil {
			L.Close()
			return fmt.Errorf("%s: %w", name, err)
		}
		ttp.Source = pluginSource
		e.tournamentTypes[ttp.ID] = ttp
		return nil
	default:
		L.Close()
		return fmt.Errorf("%s: plugin table must have either 'compatible_tournament_types' or 'compatible_sports'", name)
	}
```

Update both call sites in `Load`. Replace:
```go
		if err := e.loadSource(entry.Name(), source); err != nil {
			return nil, fmt.Errorf("bundled plugin %s: %w", entry.Name(), err)
		}
```
with:
```go
		if err := e.loadSource(entry.Name(), source, "bundled"); err != nil {
			return nil, fmt.Errorf("bundled plugin %s: %w", entry.Name(), err)
		}
```

and replace:
```go
		if err := e.loadSource(entry.Name(), source); err != nil {
			fmt.Fprintf(os.Stderr, "plugin: skipping %s: %v\n", entry.Name(), err)
		}
```
with:
```go
		if err := e.loadSource(entry.Name(), source, entry.Name()); err != nil {
			fmt.Fprintf(os.Stderr, "plugin: skipping %s: %v\n", entry.Name(), err)
		}
```

- [ ] **Step 5: Add `Validate`**

Create `internal/plugin/validate.go`:

```go
package plugin

// Validate parses and loads a single plugin source in isolation --
// exercising the exact same sandboxed-exec-and-shape-check loadSource
// runs for every bundled and external file during Load -- without
// registering the result on any live Engine. Load's own external-file
// scan intentionally logs and skips a bad file rather than failing
// (correct for a background startup scan); Validate instead returns the
// error, for callers like an interactive plugin upload that need to
// tell the caller exactly what's wrong before persisting anything.
func Validate(name string, source []byte) error {
	scratch := &Engine{
		sports:          make(map[string]*SportPlugin),
		tournamentTypes: make(map[string]*TournamentTypePlugin),
	}
	if err := scratch.loadSource(name, source, name); err != nil {
		return err
	}
	scratch.Close()
	return nil
}
```

- [ ] **Step 6: Run tests to verify they pass**

Run: `go test ./internal/plugin/... -v`
Expected: PASS (all tests, including the pre-existing suite)

- [ ] **Step 7: Run the full Go suite**

Run: `go build ./... && go test ./... -count=1`
Expected: all packages ok

- [ ] **Step 8: Commit**

```bash
git add internal/plugin/types.go internal/plugin/engine.go internal/plugin/validate.go internal/plugin/validate_test.go internal/plugin/engine_test.go
git commit -m "feat: add plugin Source metadata and isolated Validate"
```

---

### Task 3: Runtime plugin reload — upload/delete endpoints

**Files:**
- Modify: `internal/server/server.go`
- Modify: `internal/server/round_common.go`
- Modify: `internal/server/handlers_plugins.go`
- Modify: `internal/server/server_test.go`
- Modify: `internal/server/auth_test.go`
- Modify: `internal/server/import_test.go`
- Modify: `cmd/tournamentstudio/main.go`
- Create: `internal/server/handlers_plugins_manage.go`
- Create: `internal/server/handlers_plugins_manage_test.go`

**Interfaces:**
- Consumes: `plugin.Validate(name string, source []byte) error` and `SportPlugin.Source`/`TournamentTypePlugin.Source` from Task 2. `plugin.Load(externalDir string) (*Engine, error)` — pre-existing.
- Produces: `POST /api/plugins` (multipart, field `file`; organizer-only) and `DELETE /api/plugins/{filename}` (organizer-only). `GET /api/plugins`'s existing response gains a `source` field per entry. `server.New`'s signature gains a `pluginsDir string` parameter (5th positional arg, after `plugins *plugin.Engine`) — every caller of `New` across the codebase must be updated in this same task.

- [ ] **Step 1: Update `Server` to hold a swappable Engine**

In `internal/server/server.go`, add `"sync/atomic"` to the imports:

```go
import (
	"encoding/json"
	"io/fs"
	"net/http"
	"sync/atomic"

	"tournamentstudio/internal/auth"
	"tournamentstudio/internal/i18n"
	"tournamentstudio/internal/plugin"
	"tournamentstudio/internal/round"
	"tournamentstudio/internal/schedule"
	"tournamentstudio/internal/store"
	"tournamentstudio/internal/team"
	"tournamentstudio/internal/tournament"
)
```

Change the `Server` struct's `plugins` field and add `pluginsDir`. Replace:

```go
type Server struct {
	mux         *http.ServeMux
	users       *auth.Repo
	sessions    *auth.SessionRepo
	tournaments *tournament.Repo
	teams       *team.Repo
	plugins     *plugin.Engine
	rounds      *round.Repo
	hub         *broadcastHub
	schedule    *schedule.Repo
	i18n        *i18n.Catalog
	webFS       fs.FS
}
```
with:
```go
type Server struct {
	mux         *http.ServeMux
	users       *auth.Repo
	sessions    *auth.SessionRepo
	tournaments *tournament.Repo
	teams       *team.Repo
	plugins     *atomic.Pointer[plugin.Engine]
	pluginsDir  string
	rounds      *round.Repo
	hub         *broadcastHub
	schedule    *schedule.Repo
	i18n        *i18n.Catalog
	webFS       fs.FS
}
```

Change `New`. Replace:
```go
func New(s *store.Store, plugins *plugin.Engine, catalog *i18n.Catalog, webFS fs.FS) *Server {
	srv := &Server{
		mux:         http.NewServeMux(),
		users:       auth.NewRepo(s),
		sessions:    auth.NewSessionRepo(s),
		tournaments: tournament.NewRepo(s),
		teams:       team.NewRepo(s),
		plugins:     plugins,
		rounds:      round.NewRepo(s),
		hub:         newBroadcastHub(),
		schedule:    schedule.NewRepo(s),
		i18n:        catalog,
		webFS:       webFS,
	}
	srv.routes()
	return srv
}
```
with:
```go
func New(s *store.Store, plugins *plugin.Engine, pluginsDir string, catalog *i18n.Catalog, webFS fs.FS) *Server {
	pluginsPtr := &atomic.Pointer[plugin.Engine]{}
	pluginsPtr.Store(plugins)
	srv := &Server{
		mux:         http.NewServeMux(),
		users:       auth.NewRepo(s),
		sessions:    auth.NewSessionRepo(s),
		tournaments: tournament.NewRepo(s),
		teams:       team.NewRepo(s),
		plugins:     pluginsPtr,
		pluginsDir:  pluginsDir,
		rounds:      round.NewRepo(s),
		hub:         newBroadcastHub(),
		schedule:    schedule.NewRepo(s),
		i18n:        catalog,
		webFS:       webFS,
	}
	srv.routes()
	return srv
}
```

Add the two new routes in `routes()`, immediately after the existing `GET /api/plugins` line:

```go
	s.mux.Handle("GET /api/plugins", authenticated(http.HandlerFunc(s.handlePlugins)))
	s.mux.Handle("POST /api/plugins", organizerOnly(http.HandlerFunc(s.handleUploadPlugin)))
	s.mux.Handle("DELETE /api/plugins/{filename}", organizerOnly(http.HandlerFunc(s.handleDeletePlugin)))
```

- [ ] **Step 2: Update the two existing read call sites**

In `internal/server/round_common.go`, replace:
```go
	for _, ttp := range s.plugins.TournamentTypes() {
```
with:
```go
	for _, ttp := range s.plugins.Load().TournamentTypes() {
```

In `internal/server/handlers_plugins.go`, add `Source` to both response types and update both loops. Replace the whole file's content with:

```go
package server

import (
	"encoding/json"
	"net/http"

	"tournamentstudio/internal/plugin"
)

type pluginSportResponse struct {
	ID                        string               `json:"id"`
	DisplayName               string               `json:"display_name"`
	CompatibleTournamentTypes []string             `json:"compatible_tournament_types"`
	RosterFields              []plugin.RosterField `json:"roster_fields"`
	Source                    string               `json:"source"`
}

type pluginTournamentTypeResponse struct {
	ID               string   `json:"id"`
	CompatibleSports []string `json:"compatible_sports"`
	Source           string   `json:"source"`
}

func (s *Server) handlePlugins(w http.ResponseWriter, r *http.Request) {
	engine := s.plugins.Load()

	sports := make([]pluginSportResponse, 0)
	for _, sp := range engine.Sports() {
		sports = append(sports, pluginSportResponse{
			ID:                        sp.ID,
			DisplayName:               sp.DisplayName,
			CompatibleTournamentTypes: sp.CompatibleTournamentTypes,
			RosterFields:              sp.RosterFields,
			Source:                    sp.Source,
		})
	}

	tournamentTypes := make([]pluginTournamentTypeResponse, 0)
	for _, ttp := range engine.TournamentTypes() {
		tournamentTypes = append(tournamentTypes, pluginTournamentTypeResponse{
			ID:               ttp.ID,
			CompatibleSports: ttp.CompatibleSports,
			Source:           ttp.Source,
		})
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]any{
		"sports":           sports,
		"tournament_types": tournamentTypes,
	})
}
```

- [ ] **Step 3: Write the upload/delete handler tests (they will fail to compile first)**

Create `internal/server/handlers_plugins_manage_test.go`:

```go
package server

import (
	"bytes"
	"encoding/json"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"testing"

	"tournamentstudio/internal/auth"
	"tournamentstudio/internal/ranking"
)

func newPluginUploadRequest(t *testing.T, token, filename string, content []byte) *http.Request {
	t.Helper()
	var buf bytes.Buffer
	w := multipart.NewWriter(&buf)
	part, err := w.CreateFormFile("file", filename)
	if err != nil {
		t.Fatalf("create form file: %v", err)
	}
	if _, err := part.Write(content); err != nil {
		t.Fatalf("write form file: %v", err)
	}
	if err := w.Close(); err != nil {
		t.Fatalf("close multipart writer: %v", err)
	}
	req := httptest.NewRequest(http.MethodPost, "/api/plugins", &buf)
	req.Header.Set("Content-Type", w.FormDataContentType())
	req.Header.Set("Authorization", "Bearer "+token)
	return req
}

func getPluginsCatalog(t *testing.T, s *Server, token string) struct {
	Sports []struct {
		ID     string `json:"id"`
		Source string `json:"source"`
	} `json:"sports"`
} {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, "/api/plugins", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	rec := httptest.NewRecorder()
	s.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("GET /api/plugins: expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	var resp struct {
		Sports []struct {
			ID     string `json:"id"`
			Source string `json:"source"`
		} `json:"sports"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode plugins response: %v", err)
	}
	return resp
}

const validSportPluginLua = `
return {
  id = "extra-sport",
  display_name = "Extra Sport",
  compatible_tournament_types = {"timed-heats-reseeding"},
}
`

func TestUploadPluginAddsToCatalog(t *testing.T) {
	s := newTestServer(t)
	token := loginAs(t, s, "organizer1", "pw", auth.RoleOrganizer)

	req := newPluginUploadRequest(t, token, "extra-sport.lua", []byte(validSportPluginLua))
	rec := httptest.NewRecorder()
	s.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}

	catalog := getPluginsCatalog(t, s, token)
	var found bool
	for _, sp := range catalog.Sports {
		if sp.ID == "extra-sport" {
			found = true
			if sp.Source != "extra-sport.lua" {
				t.Fatalf("expected source to be the filename, got %q", sp.Source)
			}
		}
	}
	if !found {
		t.Fatal("expected extra-sport to appear in the catalog after upload")
	}
}

func TestUploadPluginRejectsInvalidLua(t *testing.T) {
	s := newTestServer(t)
	token := loginAs(t, s, "organizer1", "pw", auth.RoleOrganizer)

	req := newPluginUploadRequest(t, token, "broken.lua", []byte("not valid lua {{{"))
	rec := httptest.NewRecorder()
	s.ServeHTTP(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d: %s", rec.Code, rec.Body.String())
	}

	catalog := getPluginsCatalog(t, s, token)
	for _, sp := range catalog.Sports {
		if sp.ID == "broken" {
			t.Fatal("a rejected upload must not appear in the catalog")
		}
	}
}

func TestUploadPluginForbiddenForNonOrganizer(t *testing.T) {
	s := newTestServer(t)
	loginAs(t, s, "organizer1", "pw", auth.RoleOrganizer)
	spectatorToken := loginAs(t, s, "spectator1", "pw", auth.RoleSpectator)

	req := newPluginUploadRequest(t, spectatorToken, "extra-sport.lua", []byte(validSportPluginLua))
	rec := httptest.NewRecorder()
	s.ServeHTTP(rec, req)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("expected 403, got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestDeletePluginRemovesExternalPlugin(t *testing.T) {
	s := newTestServer(t)
	token := loginAs(t, s, "organizer1", "pw", auth.RoleOrganizer)

	uploadReq := newPluginUploadRequest(t, token, "extra-sport.lua", []byte(validSportPluginLua))
	uploadRec := httptest.NewRecorder()
	s.ServeHTTP(uploadRec, uploadReq)
	if uploadRec.Code != http.StatusOK {
		t.Fatalf("upload: expected 200, got %d: %s", uploadRec.Code, uploadRec.Body.String())
	}

	delReq := httptest.NewRequest(http.MethodDelete, "/api/plugins/extra-sport.lua", nil)
	delReq.Header.Set("Authorization", "Bearer "+token)
	delRec := httptest.NewRecorder()
	s.ServeHTTP(delRec, delReq)
	if delRec.Code != http.StatusNoContent {
		t.Fatalf("delete: expected 204, got %d: %s", delRec.Code, delRec.Body.String())
	}

	catalog := getPluginsCatalog(t, s, token)
	for _, sp := range catalog.Sports {
		if sp.ID == "extra-sport" {
			t.Fatal("expected extra-sport to be gone from the catalog after delete")
		}
	}
}

func TestDeletePluginBundledReturnsNotFound(t *testing.T) {
	s := newTestServer(t)
	token := loginAs(t, s, "organizer1", "pw", auth.RoleOrganizer)

	req := httptest.NewRequest(http.MethodDelete, "/api/plugins/dragonboat.lua", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	rec := httptest.NewRecorder()
	s.ServeHTTP(rec, req)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("expected 404 for a bundled plugin's filename, got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestDeletePluginForbiddenForNonOrganizer(t *testing.T) {
	s := newTestServer(t)
	organizerToken := loginAs(t, s, "organizer1", "pw", auth.RoleOrganizer)
	timeEntryToken := loginAs(t, s, "timeentry1", "pw", auth.RoleTimeEntry)

	uploadReq := newPluginUploadRequest(t, organizerToken, "extra-sport.lua", []byte(validSportPluginLua))
	uploadRec := httptest.NewRecorder()
	s.ServeHTTP(uploadRec, uploadReq)
	if uploadRec.Code != http.StatusOK {
		t.Fatalf("upload: expected 200, got %d: %s", uploadRec.Code, uploadRec.Body.String())
	}

	req := httptest.NewRequest(http.MethodDelete, "/api/plugins/extra-sport.lua", nil)
	req.Header.Set("Authorization", "Bearer "+timeEntryToken)
	rec := httptest.NewRecorder()
	s.ServeHTTP(rec, req)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("expected 403, got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestUploadPluginRejectsPathTraversalFilename(t *testing.T) {
	s := newTestServer(t)
	token := loginAs(t, s, "organizer1", "pw", auth.RoleOrganizer)

	req := newPluginUploadRequest(t, token, "../escaped.lua", []byte(validSportPluginLua))
	rec := httptest.NewRecorder()
	s.ServeHTTP(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for a path-traversal filename, got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestReloadDoesNotInvalidateInFlightTournamentTypePlugin(t *testing.T) {
	s := newTestServer(t)
	token := loginAs(t, s, "organizer1", "pw", auth.RoleOrganizer)

	ttp := s.findTournamentType("timed-heats-reseeding")
	if ttp == nil {
		t.Fatal("expected bundled timed-heats-reseeding tournament type plugin")
	}

	// Trigger a reload out from under the reference just obtained.
	uploadReq := newPluginUploadRequest(t, token, "extra-sport.lua", []byte(validSportPluginLua))
	uploadRec := httptest.NewRecorder()
	s.ServeHTTP(uploadRec, uploadReq)
	if uploadRec.Code != http.StatusOK {
		t.Fatalf("upload: expected 200, got %d: %s", uploadRec.Code, uploadRec.Body.String())
	}

	// The reference obtained before the reload must still work -- its Lua
	// state was never closed out from under this call.
	timeA, timeB := 100.0, 200.0
	if _, err := ttp.NextRoundGroups([][]ranking.TeamResult{
		{{TeamID: "1", TimeSeconds: &timeA}, {TeamID: "2", TimeSeconds: &timeB}},
	}); err != nil {
		t.Fatalf("expected the pre-reload plugin reference to keep working, got: %v", err)
	}
}
```

- [ ] **Step 4: Run tests to verify they fail**

Run: `go build ./... 2>&1 | head -50`
Expected: compile errors — `handleUploadPlugin`, `handleDeletePlugin` undefined, `New` call sites have the wrong argument count (this is expected; fixed in Step 5 and Step 6)

- [ ] **Step 5: Implement the upload/delete handlers**

Create `internal/server/handlers_plugins_manage.go`:

```go
package server

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"tournamentstudio/internal/plugin"
)

// maxPluginUploadBytes bounds a single upload -- plugin sources are small,
// hand-written Lua files; the bundled ones are well under 1 KiB.
const maxPluginUploadBytes = 1 << 20 // 1 MiB

var errInvalidPluginFilename = errors.New("filename must be a bare name ending in .lua")

func (s *Server) handleUploadPlugin(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, maxPluginUploadBytes)
	if err := r.ParseMultipartForm(maxPluginUploadBytes); err != nil {
		http.Error(w, "invalid multipart upload", http.StatusBadRequest)
		return
	}
	file, header, err := r.FormFile("file")
	if err != nil {
		http.Error(w, "missing file field", http.StatusBadRequest)
		return
	}
	defer file.Close()

	filename, err := sanitizePluginFilename(header.Filename)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	source, err := io.ReadAll(file)
	if err != nil {
		http.Error(w, "could not read upload", http.StatusBadRequest)
		return
	}

	if err := plugin.Validate(filename, source); err != nil {
		http.Error(w, "invalid plugin: "+err.Error(), http.StatusBadRequest)
		return
	}

	if err := os.MkdirAll(s.pluginsDir, 0o755); err != nil {
		http.Error(w, "could not prepare plugins directory", http.StatusInternalServerError)
		return
	}
	if err := os.WriteFile(filepath.Join(s.pluginsDir, filename), source, 0o644); err != nil {
		http.Error(w, "could not save plugin", http.StatusInternalServerError)
		return
	}

	if err := s.reloadPlugins(); err != nil {
		http.Error(w, "plugin saved but reload failed: "+err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"filename": filename})
}

func (s *Server) handleDeletePlugin(w http.ResponseWriter, r *http.Request) {
	filename, err := sanitizePluginFilename(r.PathValue("filename"))
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	// Bundled plugins are embedded, never written to pluginsDir, so this
	// check alone gives the right 404 for both "unknown filename" and
	// "that's a built-in plugin's name" without needing to consult the
	// live Engine at all.
	path := filepath.Join(s.pluginsDir, filename)
	if _, err := os.Stat(path); err != nil {
		http.Error(w, "plugin not found", http.StatusNotFound)
		return
	}
	if err := os.Remove(path); err != nil {
		http.Error(w, "could not delete plugin", http.StatusInternalServerError)
		return
	}

	if err := s.reloadPlugins(); err != nil {
		http.Error(w, "plugin deleted but reload failed: "+err.Error(), http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

// reloadPlugins rebuilds the Engine from scratch (bundled plugins plus
// everything currently in s.pluginsDir) and atomically swaps it in. The
// superseded Engine is deliberately never closed: an in-flight request may
// still hold a *plugin.TournamentTypePlugin obtained from it, and closing
// that plugin's Lua state out from under an in-progress call would race.
// gopher-lua's LState is pure Go with no external OS resource, so the old
// Engine is simply left for garbage collection once nothing references it.
func (s *Server) reloadPlugins() error {
	engine, err := plugin.Load(s.pluginsDir)
	if err != nil {
		return err
	}
	s.plugins.Store(engine)
	return nil
}

// sanitizePluginFilename rejects anything but a bare "name.lua" -- no path
// separators, no "..", so an upload or delete request can never escape
// s.pluginsDir.
func sanitizePluginFilename(name string) (string, error) {
	if name == "" {
		return "", errInvalidPluginFilename
	}
	if filepath.Base(name) != name {
		return "", errInvalidPluginFilename
	}
	if !strings.HasSuffix(name, ".lua") {
		return "", errInvalidPluginFilename
	}
	return name, nil
}
```

- [ ] **Step 6: Fix every `server.New` call site**

In `internal/server/server_test.go`, in `newTestServerWithWebFS`, replace:
```go
	engine, err := plugin.Load(t.TempDir())
	if err != nil {
		t.Fatalf("load plugins: %v", err)
	}
	t.Cleanup(engine.Close)

	catalog, err := i18n.Load(t.TempDir())
	if err != nil {
		t.Fatalf("load i18n: %v", err)
	}

	return New(s, engine, catalog, webFS)
```
with:
```go
	pluginsDir := t.TempDir()
	engine, err := plugin.Load(pluginsDir)
	if err != nil {
		t.Fatalf("load plugins: %v", err)
	}
	t.Cleanup(engine.Close)

	catalog, err := i18n.Load(t.TempDir())
	if err != nil {
		t.Fatalf("load i18n: %v", err)
	}

	return New(s, engine, pluginsDir, catalog, webFS)
```

In `internal/server/auth_test.go`, replace:
```go
	engine, err := plugin.Load(t.TempDir())
	if err != nil {
		t.Fatalf("load plugins: %v", err)
	}
	t.Cleanup(engine.Close)
	catalog, err := i18n.Load(t.TempDir())
	if err != nil {
		t.Fatalf("load i18n: %v", err)
	}
	s := New(st, engine, catalog, fstest.MapFS{"index.html": &fstest.MapFile{Data: []byte("<html></html>")}})
```
with:
```go
	pluginsDir := t.TempDir()
	engine, err := plugin.Load(pluginsDir)
	if err != nil {
		t.Fatalf("load plugins: %v", err)
	}
	t.Cleanup(engine.Close)
	catalog, err := i18n.Load(t.TempDir())
	if err != nil {
		t.Fatalf("load i18n: %v", err)
	}
	s := New(st, engine, pluginsDir, catalog, fstest.MapFS{"index.html": &fstest.MapFile{Data: []byte("<html></html>")}})
```

In `internal/server/import_test.go`, replace:
```go
	engine, err := plugin.Load(t.TempDir())
	if err != nil {
		t.Fatalf("load plugins: %v", err)
	}
	t.Cleanup(engine.Close)

	catalog, err := i18n.Load(t.TempDir())
	if err != nil {
		t.Fatalf("load i18n: %v", err)
	}

	s := New(st, engine, catalog, fstest.MapFS{"index.html": &fstest.MapFile{Data: []byte("<html></html>")}})
```
with:
```go
	pluginsDir := t.TempDir()
	engine, err := plugin.Load(pluginsDir)
	if err != nil {
		t.Fatalf("load plugins: %v", err)
	}
	t.Cleanup(engine.Close)

	catalog, err := i18n.Load(t.TempDir())
	if err != nil {
		t.Fatalf("load i18n: %v", err)
	}

	s := New(st, engine, pluginsDir, catalog, fstest.MapFS{"index.html": &fstest.MapFile{Data: []byte("<html></html>")}})
```

In `cmd/tournamentstudio/main.go`, replace:
```go
	s := server.New(st, engine, catalog, frontendFS)
```
with:
```go
	s := server.New(st, engine, pluginsDir, catalog, frontendFS)
```

- [ ] **Step 7: Run tests to verify they pass**

Run: `go build ./... && go test ./... -count=1`
Expected: all packages ok

- [ ] **Step 8: Commit**

```bash
git add internal/server/server.go internal/server/round_common.go internal/server/handlers_plugins.go internal/server/handlers_plugins_manage.go internal/server/handlers_plugins_manage_test.go internal/server/server_test.go internal/server/auth_test.go internal/server/import_test.go cmd/tournamentstudio/main.go
git commit -m "feat: add plugin upload/delete with runtime engine reload"
```

---

### Task 4: `useTournamentSocket` — WebSocket client hook

**Files:**
- Create: `frontend/src/hooks/useTournamentSocket.ts`
- Create: `frontend/src/hooks/useTournamentSocket.test.ts`

**Interfaces:**
- Consumes: `getToken()` from `frontend/src/api/client.ts` (pre-existing).
- Produces: `useTournamentSocket(tournamentId: string | undefined): { connectionLost: boolean }`. Opens a WebSocket to `/api/tournaments/{id}/ws?token=...` on mount, closes on unmount, invalidates the `['standings', tournamentId]` query on any message. Task 5 (`WatchPage`) consumes this hook directly.

- [ ] **Step 1: Write the failing test**

Create `frontend/src/hooks/useTournamentSocket.test.ts`:

```ts
import { describe, expect, it, vi, beforeEach, afterEach } from 'vitest'
import { renderHook, waitFor } from '@testing-library/react'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import type { ReactNode } from 'react'
import { useTournamentSocket } from './useTournamentSocket'

vi.mock('../api/client', () => ({ getToken: () => 'test-token' }))

class FakeWebSocket {
  static instances: FakeWebSocket[] = []
  onopen: (() => void) | null = null
  onmessage: (() => void) | null = null
  onclose: (() => void) | null = null
  url: string
  closed = false

  constructor(url: string) {
    this.url = url
    FakeWebSocket.instances.push(this)
  }

  close() {
    this.closed = true
  }
}

describe('useTournamentSocket', () => {
  let queryClient: QueryClient
  let originalWebSocket: typeof WebSocket

  beforeEach(() => {
    vi.useFakeTimers()
    FakeWebSocket.instances = []
    originalWebSocket = globalThis.WebSocket
    // @ts-expect-error -- test double, not a full WebSocket implementation
    globalThis.WebSocket = FakeWebSocket
    queryClient = new QueryClient()
    vi.spyOn(queryClient, 'invalidateQueries')
  })

  afterEach(() => {
    globalThis.WebSocket = originalWebSocket
    vi.useRealTimers()
  })

  function wrapper({ children }: { children: ReactNode }) {
    return <QueryClientProvider client={queryClient}>{children}</QueryClientProvider>
  }

  it('opens a socket to the tournament endpoint with the auth token', () => {
    renderHook(() => useTournamentSocket('42'), { wrapper })

    expect(FakeWebSocket.instances).toHaveLength(1)
    expect(FakeWebSocket.instances[0].url).toContain('/api/tournaments/42/ws?token=test-token')
  })

  it('invalidates the standings query on any message', () => {
    renderHook(() => useTournamentSocket('42'), { wrapper })

    FakeWebSocket.instances[0].onmessage?.()

    expect(queryClient.invalidateQueries).toHaveBeenCalledWith({ queryKey: ['standings', '42'] })
  })

  it('reconnects with backoff and sets connectionLost after repeated failures', async () => {
    const { result } = renderHook(() => useTournamentSocket('42'), { wrapper })

    expect(result.current.connectionLost).toBe(false)

    for (let i = 0; i < 3; i++) {
      const current = FakeWebSocket.instances[FakeWebSocket.instances.length - 1]
      current.onclose?.()
      await vi.runOnlyPendingTimersAsync()
    }

    expect(result.current.connectionLost).toBe(true)
    expect(FakeWebSocket.instances.length).toBeGreaterThan(1)
  })

  it('closes the socket on unmount', () => {
    const { unmount } = renderHook(() => useTournamentSocket('42'), { wrapper })
    const socket = FakeWebSocket.instances[0]

    unmount()

    expect(socket.closed).toBe(true)
  })
})
```

- [ ] **Step 2: Run the test to verify it fails**

Run: `cd frontend && npm test -- --run useTournamentSocket`
Expected: FAIL — `./useTournamentSocket` module does not exist

- [ ] **Step 3: Implement the hook**

Create `frontend/src/hooks/useTournamentSocket.ts`:

```ts
import { useEffect, useRef, useState } from 'react'
import { useQueryClient } from '@tanstack/react-query'
import { getToken } from '../api/client'

const INITIAL_RECONNECT_DELAY_MS = 1000
const MAX_RECONNECT_DELAY_MS = 30000
const RECONNECT_FAILURES_BEFORE_BANNER = 3

export function useTournamentSocket(tournamentId: string | undefined): { connectionLost: boolean } {
  const queryClient = useQueryClient()
  const [connectionLost, setConnectionLost] = useState(false)

  const failureCountRef = useRef(0)
  const reconnectTimeoutRef = useRef<ReturnType<typeof setTimeout> | null>(null)

  useEffect(() => {
    if (!tournamentId) return

    let socket: WebSocket | null = null
    let cancelled = false

    function connect() {
      if (cancelled) return
      const token = getToken()
      const protocol = window.location.protocol === 'https:' ? 'wss:' : 'ws:'
      socket = new WebSocket(`${protocol}//${window.location.host}/api/tournaments/${tournamentId}/ws?token=${token}`)

      socket.onopen = () => {
        failureCountRef.current = 0
        setConnectionLost(false)
      }
      socket.onmessage = () => {
        void queryClient.invalidateQueries({ queryKey: ['standings', tournamentId] })
      }
      socket.onclose = () => {
        if (cancelled) return
        failureCountRef.current += 1
        if (failureCountRef.current >= RECONNECT_FAILURES_BEFORE_BANNER) {
          setConnectionLost(true)
        }
        const delay = Math.min(
          INITIAL_RECONNECT_DELAY_MS * 2 ** (failureCountRef.current - 1),
          MAX_RECONNECT_DELAY_MS,
        )
        reconnectTimeoutRef.current = setTimeout(connect, delay)
      }
    }

    connect()

    return () => {
      cancelled = true
      if (reconnectTimeoutRef.current) clearTimeout(reconnectTimeoutRef.current)
      socket?.close()
    }
  }, [tournamentId, queryClient])

  return { connectionLost }
}
```

- [ ] **Step 4: Run the test to verify it passes**

Run: `cd frontend && npm test -- --run useTournamentSocket`
Expected: PASS (4 tests)

- [ ] **Step 5: Commit**

```bash
git add frontend/src/hooks/useTournamentSocket.ts frontend/src/hooks/useTournamentSocket.test.ts
git commit -m "feat: add WebSocket client hook for live standings"
```

---

### Task 5: `WatchPage` — live standings tab

**Files:**
- Modify: `frontend/src/api/types.ts`
- Create: `frontend/src/pages/WatchPage.tsx`
- Create: `frontend/src/pages/WatchPage.test.tsx`
- Modify: `frontend/src/App.tsx`
- Modify: `frontend/src/pages/TournamentDetailPage.tsx`
- Modify: `internal/i18n/bundles/en.json`
- Modify: `internal/i18n/bundles/de.json`

**Interfaces:**
- Consumes: `GET /api/tournaments/{id}/standings` (Task 1's shape), `useTournamentSocket` (Task 4).
- Produces: route `/tournaments/:id/watch`, component `WatchPage`. Nothing later depends on this task.

- [ ] **Step 1: Add the standings types**

In `frontend/src/api/types.ts`, add at the end of the file:

```ts
export interface RankedTeam {
  rank: number
  team_id: string
  time_seconds: number | null
  status: string
}

export interface StandingsEntry {
  group_id: number | null
  division_id: number | null
  division_name: string | null
  ranked_teams: RankedTeam[]
}

export interface StandingsRound {
  id: number
  round_number: number
  status: string
  standings: StandingsEntry[]
}

export interface StandingsResponse {
  rounds: StandingsRound[]
}
```

- [ ] **Step 2: Add i18n keys**

In `internal/i18n/bundles/en.json`, add (keeping the file's existing alphabetical-ish grouping by prefix is not required — just add these keys anywhere in the top-level object):

```json
  "watch_connection_lost": "Live updates unavailable — showing last loaded data",
  "watch_rank": "Rank",
  "watch_team": "Team",
  "watch_time_or_status": "Time / Status",
```

In `internal/i18n/bundles/de.json`, add:

```json
  "watch_connection_lost": "Live-Updates nicht verfügbar — zeige zuletzt geladene Daten",
  "watch_rank": "Platz",
  "watch_team": "Team",
  "watch_time_or_status": "Zeit / Status",
```

(Both files are flat JSON objects of `"key": "value"` pairs — add these as new top-level entries, keeping the JSON valid, e.g. adding a trailing comma after the preceding entry.)

- [ ] **Step 3: Write the failing test**

Create `frontend/src/pages/WatchPage.test.tsx`:

```tsx
import { describe, expect, it, vi } from 'vitest'
import { render, screen } from '@testing-library/react'
import { MemoryRouter, Route, Routes } from 'react-router-dom'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { WatchPage } from './WatchPage'
import * as client from '../api/client'
import type { StandingsResponse, Team } from '../api/types'

vi.mock('react-i18next', () => ({
  useTranslation: () => ({ t: (key: string, opts?: Record<string, unknown>) => (opts ? `${key} ${JSON.stringify(opts)}` : key) }),
}))
vi.mock('../api/client', async () => {
  const actual = await vi.importActual<typeof import('../api/client')>('../api/client')
  return { ...actual, api: { ...actual.api, get: vi.fn() } }
})
vi.mock('../hooks/useTournamentSocket', () => ({
  useTournamentSocket: () => ({ connectionLost: false }),
}))

const standings: StandingsResponse = {
  rounds: [
    {
      id: 1,
      round_number: 1,
      status: 'open',
      standings: [
        {
          group_id: 10,
          division_id: null,
          division_name: null,
          ranked_teams: [
            { rank: 1, team_id: '1', time_seconds: 100.5, status: '' },
            { rank: 2, team_id: '2', time_seconds: null, status: 'DNF' },
          ],
        },
      ],
    },
  ],
}
const teams: Team[] = [
  { id: 1, tournament_id: 1, name: 'Team One', club: 'Club A', extra_fields: {} },
  { id: 2, tournament_id: 1, name: 'Team Two', club: 'Club B', extra_fields: {} },
]

function renderWatchPage() {
  return render(
    <QueryClientProvider client={new QueryClient()}>
      <MemoryRouter initialEntries={['/tournaments/1/watch']}>
        <Routes>
          <Route path="/tournaments/:id/watch" element={<WatchPage />} />
        </Routes>
      </MemoryRouter>
    </QueryClientProvider>,
  )
}

describe('WatchPage', () => {
  it('renders ranked teams by name, in rank order, with time or status', async () => {
    vi.mocked(client.api.get).mockImplementation((path: string) => {
      if (path.includes('/standings')) return Promise.resolve(standings)
      if (path.includes('/teams')) return Promise.resolve(teams)
      return Promise.reject(new Error(`unexpected path ${path}`))
    })

    renderWatchPage()

    expect(await screen.findByText('Team One')).toBeInTheDocument()
    expect(screen.getByText('Team Two')).toBeInTheDocument()
    expect(screen.getByText('DNF')).toBeInTheDocument()

    const rows = screen.getAllByRole('row')
    // rows[0] is the header row; data rows follow in rank order.
    expect(rows[1]).toHaveTextContent('Team One')
    expect(rows[2]).toHaveTextContent('Team Two')
  })
})
```

- [ ] **Step 4: Run the test to verify it fails**

Run: `cd frontend && npm test -- --run WatchPage`
Expected: FAIL — `./WatchPage` module does not exist

- [ ] **Step 5: Implement `WatchPage`**

Create `frontend/src/pages/WatchPage.tsx`:

```tsx
import { useParams } from 'react-router-dom'
import { useTranslation } from 'react-i18next'
import { useQuery } from '@tanstack/react-query'
import { api } from '../api/client'
import type { StandingsResponse, Team } from '../api/types'
import { useTournamentSocket } from '../hooks/useTournamentSocket'

function teamName(teamId: string, teams: Team[]): string {
  return teams.find((team) => String(team.id) === teamId)?.name ?? teamId
}

export function WatchPage() {
  const { t } = useTranslation()
  const { id } = useParams<{ id: string }>()
  const { connectionLost } = useTournamentSocket(id)

  const { data: standingsData } = useQuery({
    queryKey: ['standings', id],
    queryFn: () => api.get<StandingsResponse>(`/api/tournaments/${id}/standings`),
    enabled: !!id,
  })
  const rounds = standingsData?.rounds ?? []

  const { data: teamsData } = useQuery({
    queryKey: ['teams', id],
    queryFn: () => api.get<Team[]>(`/api/tournaments/${id}/teams`),
    enabled: !!id,
  })
  const teams = teamsData ?? []

  return (
    <div className="mx-auto max-w-3xl p-8">
      {connectionLost && (
        <p role="status" className="mb-4 rounded bg-yellow-50 p-2 text-sm text-yellow-800">
          {t('watch_connection_lost')}
        </p>
      )}
      {[...rounds].reverse().map((round) => (
        <section key={round.id} className="mb-8">
          <h2 className="mb-4 text-lg font-semibold">
            {t('schedule_round_history_entry', { number: round.round_number, status: round.status })}
          </h2>
          {round.standings.map((entry, index) => (
            <table key={entry.group_id ?? entry.division_id} className="mb-4 w-full text-sm">
              <caption className="mb-2 text-left font-medium">
                {entry.division_name ?? t('schedule_round_create_group_label', { number: index + 1 })}
              </caption>
              <thead>
                <tr className="border-b text-left text-gray-500">
                  <th className="py-1">{t('watch_rank')}</th>
                  <th className="py-1">{t('watch_team')}</th>
                  <th className="py-1">{t('watch_time_or_status')}</th>
                </tr>
              </thead>
              <tbody>
                {entry.ranked_teams.map((rt) => (
                  <tr key={rt.team_id}>
                    <td className="py-1">{rt.rank}</td>
                    <td className="py-1">{teamName(rt.team_id, teams)}</td>
                    <td className="py-1">{rt.status || `${rt.time_seconds}s`}</td>
                  </tr>
                ))}
              </tbody>
            </table>
          ))}
        </section>
      ))}
    </div>
  )
}
```

- [ ] **Step 6: Wire the route and tab**

In `frontend/src/App.tsx`, add the import:
```tsx
import { WatchPage } from './pages/WatchPage'
```
and add the route as a sibling to `schedule`:
```tsx
                  <Route path="schedule" element={<SchedulePage />} />
                  <Route path="watch" element={<WatchPage />} />
```

In `frontend/src/pages/TournamentDetailPage.tsx`, replace the disabled placeholder:
```tsx
        <span className="cursor-not-allowed px-4 py-2 text-sm text-gray-300" title={t('tab_coming_soon')}>
          {t('tab_standings')}
        </span>
```
with:
```tsx
        <NavLink to="watch" className={tabLinkClass}>
          {t('tab_standings')}
        </NavLink>
```

- [ ] **Step 7: Run the test to verify it passes**

Run: `cd frontend && npm test -- --run WatchPage`
Expected: PASS

- [ ] **Step 8: Run the full frontend suite and type check**

Run: `cd frontend && npm test -- --run && npx tsc -b`
Expected: all tests pass, 0 type errors

- [ ] **Step 9: Commit**

```bash
git add frontend/src/api/types.ts frontend/src/pages/WatchPage.tsx frontend/src/pages/WatchPage.test.tsx frontend/src/App.tsx frontend/src/pages/TournamentDetailPage.tsx internal/i18n/bundles/en.json internal/i18n/bundles/de.json
git commit -m "feat: add live Watch tab with ranked standings"
```

---

### Task 6: `PluginsPage` — catalog + upload/delete

**Files:**
- Modify: `frontend/src/api/types.ts`
- Modify: `frontend/src/api/client.ts`
- Create: `frontend/src/pages/PluginsPage.tsx`
- Create: `frontend/src/pages/PluginsPage.test.tsx`
- Modify: `frontend/src/App.tsx`
- Modify: `frontend/src/components/AppShell.tsx`
- Modify: `internal/i18n/bundles/en.json`
- Modify: `internal/i18n/bundles/de.json`

**Interfaces:**
- Consumes: `GET /api/plugins` (now with `source`), `POST /api/plugins`, `DELETE /api/plugins/{filename}` (Task 3).
- Produces: route `/plugins`, component `PluginsPage`. Nothing later depends on this task.

- [ ] **Step 1: Add `source` to the existing plugin types, and add `api.delete`**

In `frontend/src/api/types.ts`, replace:
```ts
export interface SportPlugin {
  id: string
  display_name: string
  compatible_tournament_types: string[]
  roster_fields: RosterField[] | null
}

export interface TournamentTypePlugin {
  id: string
  compatible_sports: string[]
}
```
with:
```ts
export interface SportPlugin {
  id: string
  display_name: string
  compatible_tournament_types: string[]
  roster_fields: RosterField[] | null
  source: string
}

export interface TournamentTypePlugin {
  id: string
  compatible_sports: string[]
  source: string
}
```

In `frontend/src/api/client.ts`, add a `delete` verb to the `api` object. Replace:
```ts
export const api = {
  get: <T>(path: string): Promise<T> => request<T>(path),
  post: <T>(path: string, body?: unknown): Promise<T> =>
    request<T>(path, { method: 'POST', body: body !== undefined ? JSON.stringify(body) : undefined }),
  patch: <T>(path: string, body?: unknown): Promise<T> =>
    request<T>(path, { method: 'PATCH', body: body !== undefined ? JSON.stringify(body) : undefined }),
  postForm: <T>(path: string, form: FormData): Promise<T> =>
    request<T>(path, { method: 'POST', body: form }),
}
```
with:
```ts
export const api = {
  get: <T>(path: string): Promise<T> => request<T>(path),
  post: <T>(path: string, body?: unknown): Promise<T> =>
    request<T>(path, { method: 'POST', body: body !== undefined ? JSON.stringify(body) : undefined }),
  patch: <T>(path: string, body?: unknown): Promise<T> =>
    request<T>(path, { method: 'PATCH', body: body !== undefined ? JSON.stringify(body) : undefined }),
  postForm: <T>(path: string, form: FormData): Promise<T> =>
    request<T>(path, { method: 'POST', body: form }),
  delete: <T>(path: string): Promise<T> => request<T>(path, { method: 'DELETE' }),
}
```

- [ ] **Step 2: Add i18n keys**

In `internal/i18n/bundles/en.json`, add:

```json
  "nav_plugins": "Plugins",
  "plugins_title": "Plugins",
  "plugins_sports_title": "Sport plugins",
  "plugins_tournament_types_title": "Tournament type plugins",
  "plugins_builtin_badge": "Built-in",
  "plugins_delete": "Delete",
  "plugins_upload_title": "Upload a plugin",
  "plugins_upload_file_label": "Choose a .lua file",
  "plugins_upload_submit": "Upload",
```

In `internal/i18n/bundles/de.json`, add:

```json
  "nav_plugins": "Plugins",
  "plugins_title": "Plugins",
  "plugins_sports_title": "Sport-Plugins",
  "plugins_tournament_types_title": "Turnierart-Plugins",
  "plugins_builtin_badge": "Eingebaut",
  "plugins_delete": "Löschen",
  "plugins_upload_title": "Plugin hochladen",
  "plugins_upload_file_label": "Lua-Datei auswählen",
  "plugins_upload_submit": "Hochladen",
```

- [ ] **Step 3: Write the failing test**

Create `frontend/src/pages/PluginsPage.test.tsx`:

```tsx
import { describe, expect, it, vi } from 'vitest'
import { render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { MemoryRouter, Route, Routes } from 'react-router-dom'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { PluginsPage } from './PluginsPage'
import * as client from '../api/client'
import { useAuth } from '../auth/AuthContext'
import type { PluginsResponse } from '../api/types'

vi.mock('react-i18next', () => ({
  useTranslation: () => ({ t: (key: string) => key }),
}))
vi.mock('../api/client', async () => {
  const actual = await vi.importActual<typeof import('../api/client')>('../api/client')
  return { ...actual, api: { ...actual.api, get: vi.fn(), postForm: vi.fn(), delete: vi.fn() } }
})
vi.mock('../auth/AuthContext', () => ({ useAuth: vi.fn() }))

const catalog: PluginsResponse = {
  sports: [
    { id: 'dragonboat', display_name: 'Dragonboat', compatible_tournament_types: [], roster_fields: [], source: 'bundled' },
    { id: 'extra-sport', display_name: 'Extra Sport', compatible_tournament_types: [], roster_fields: [], source: 'extra-sport.lua' },
  ],
  tournament_types: [
    { id: 'timed-heats-reseeding', compatible_sports: [], source: 'bundled' },
  ],
}

function renderPluginsPage() {
  return render(
    <QueryClientProvider client={new QueryClient()}>
      <MemoryRouter initialEntries={['/plugins']}>
        <Routes>
          <Route path="/plugins" element={<PluginsPage />} />
        </Routes>
      </MemoryRouter>
    </QueryClientProvider>,
  )
}

describe('PluginsPage', () => {
  it('shows a Built-in badge for bundled plugins and no delete button', async () => {
    vi.mocked(useAuth).mockReturnValue({ role: 'organizer', token: 'x', login: vi.fn(), logout: vi.fn() })
    vi.mocked(client.api.get).mockResolvedValue(catalog)

    renderPluginsPage()

    expect(await screen.findByText('Dragonboat')).toBeInTheDocument()
    const bundledRow = screen.getByText('Dragonboat').closest('li')
    expect(bundledRow).toHaveTextContent('plugins_builtin_badge')
    expect(bundledRow?.querySelector('button')).toBeNull()
  })

  it('shows a delete button for an external plugin when organizer, and calls DELETE with its filename', async () => {
    vi.mocked(useAuth).mockReturnValue({ role: 'organizer', token: 'x', login: vi.fn(), logout: vi.fn() })
    vi.mocked(client.api.get).mockResolvedValue(catalog)
    vi.mocked(client.api.delete).mockResolvedValue(undefined)

    renderPluginsPage()

    const externalRow = (await screen.findByText('Extra Sport')).closest('li')
    const deleteButton = externalRow?.querySelector('button')
    expect(deleteButton).not.toBeNull()
    await userEvent.click(deleteButton as HTMLButtonElement)

    await waitFor(() => expect(client.api.delete).toHaveBeenCalledWith('/api/plugins/extra-sport.lua'))
  })

  it('hides the upload form and delete buttons for a non-organizer', async () => {
    vi.mocked(useAuth).mockReturnValue({ role: 'spectator', token: 'x', login: vi.fn(), logout: vi.fn() })
    vi.mocked(client.api.get).mockResolvedValue(catalog)

    renderPluginsPage()

    await screen.findByText('Dragonboat')
    expect(screen.queryByText('plugins_upload_submit')).toBeNull()
    expect(screen.queryAllByRole('button')).toHaveLength(0)
  })

  it('shows the upload error message returned by the backend', async () => {
    vi.mocked(useAuth).mockReturnValue({ role: 'organizer', token: 'x', login: vi.fn(), logout: vi.fn() })
    vi.mocked(client.api.get).mockResolvedValue(catalog)
    vi.mocked(client.api.postForm).mockRejectedValue(new Error('invalid plugin: load broken.lua: parse error'))

    renderPluginsPage()

    await screen.findByText('Dragonboat')
    const file = new File(['not lua'], 'broken.lua', { type: 'text/plain' })
    const input = screen.getByLabelText('plugins_upload_file_label')
    await userEvent.upload(input, file)
    await userEvent.click(screen.getByText('plugins_upload_submit'))

    expect(await screen.findByRole('alert')).toHaveTextContent('invalid plugin: load broken.lua: parse error')
  })
})
```

- [ ] **Step 4: Run the test to verify it fails**

Run: `cd frontend && npm test -- --run PluginsPage`
Expected: FAIL — `./PluginsPage` module does not exist

- [ ] **Step 5: Implement `PluginsPage`**

Create `frontend/src/pages/PluginsPage.tsx`:

```tsx
import { useState, type ChangeEvent } from 'react'
import { useTranslation } from 'react-i18next'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { api } from '../api/client'
import { useAuth } from '../auth/AuthContext'
import type { PluginsResponse } from '../api/types'

export function PluginsPage() {
  const { t } = useTranslation()
  const { role } = useAuth()
  const queryClient = useQueryClient()
  const [file, setFile] = useState<File | null>(null)
  const isOrganizer = role === 'organizer'

  const { data } = useQuery({
    queryKey: ['plugins'],
    queryFn: () => api.get<PluginsResponse>('/api/plugins'),
  })
  const sports = data?.sports ?? []
  const tournamentTypes = data?.tournament_types ?? []

  const uploadMutation = useMutation({
    mutationFn: () => {
      const form = new FormData()
      form.append('file', file as File)
      return api.postForm<{ filename: string }>('/api/plugins', form)
    },
    onSuccess: () => {
      setFile(null)
      void queryClient.invalidateQueries({ queryKey: ['plugins'] })
    },
  })

  const deleteMutation = useMutation({
    mutationFn: (filename: string) => api.delete(`/api/plugins/${encodeURIComponent(filename)}`),
    onSuccess: () => {
      void queryClient.invalidateQueries({ queryKey: ['plugins'] })
    },
  })

  function handleFileChange(e: ChangeEvent<HTMLInputElement>) {
    setFile(e.target.files?.[0] ?? null)
  }

  return (
    <div className="mx-auto max-w-3xl p-8">
      <h1 className="mb-6 text-xl font-bold">{t('plugins_title')}</h1>

      <section className="mb-8">
        <h2 className="mb-2 text-lg font-semibold">{t('plugins_sports_title')}</h2>
        <ul className="space-y-2">
          {sports.map((sp) => (
            <li key={sp.id} className="flex items-center justify-between rounded border p-3">
              <span className="font-medium">{sp.display_name}</span>
              {sp.source === 'bundled' ? (
                <span className="text-xs text-gray-400">{t('plugins_builtin_badge')}</span>
              ) : (
                isOrganizer && (
                  <button onClick={() => deleteMutation.mutate(sp.source)} className="text-xs text-red-600">
                    {t('plugins_delete')}
                  </button>
                )
              )}
            </li>
          ))}
        </ul>
      </section>

      <section className="mb-8">
        <h2 className="mb-2 text-lg font-semibold">{t('plugins_tournament_types_title')}</h2>
        <ul className="space-y-2">
          {tournamentTypes.map((ttp) => (
            <li key={ttp.id} className="flex items-center justify-between rounded border p-3">
              <span className="font-medium">{ttp.id}</span>
              {ttp.source === 'bundled' ? (
                <span className="text-xs text-gray-400">{t('plugins_builtin_badge')}</span>
              ) : (
                isOrganizer && (
                  <button onClick={() => deleteMutation.mutate(ttp.source)} className="text-xs text-red-600">
                    {t('plugins_delete')}
                  </button>
                )
              )}
            </li>
          ))}
        </ul>
      </section>

      {isOrganizer && (
        <section>
          <h2 className="mb-2 text-lg font-semibold">{t('plugins_upload_title')}</h2>
          <div className="space-y-2">
            <input type="file" accept=".lua" aria-label={t('plugins_upload_file_label')} onChange={handleFileChange} />
            <div>
              <button
                onClick={() => file && uploadMutation.mutate()}
                disabled={!file || uploadMutation.isPending}
                className="rounded bg-blue-600 px-4 py-2 text-sm text-white disabled:opacity-50"
              >
                {t('plugins_upload_submit')}
              </button>
            </div>
            {uploadMutation.isError && (
              <p role="alert" className="text-sm text-red-600">
                {(uploadMutation.error as Error).message}
              </p>
            )}
          </div>
        </section>
      )}
    </div>
  )
}
```

- [ ] **Step 6: Wire the route and nav link**

In `frontend/src/App.tsx`, add the import:
```tsx
import { PluginsPage } from './pages/PluginsPage'
```
and add a top-level route as a sibling of `/tournaments`, inside the `AppShell` route:
```tsx
                <Route path="/tournaments" element={<TournamentListPage />} />
                <Route path="/plugins" element={<PluginsPage />} />
```

In `frontend/src/components/AppShell.tsx`, add a nav link. Replace:
```tsx
        <Link to="/tournaments" className="font-bold">
          TournamentStudio
        </Link>
        <div className="flex items-center gap-4 text-sm">
```
with:
```tsx
        <Link to="/tournaments" className="font-bold">
          TournamentStudio
        </Link>
        <div className="flex items-center gap-4 text-sm">
          <Link to="/plugins">{t('nav_plugins')}</Link>
```

- [ ] **Step 7: Run the test to verify it passes**

Run: `cd frontend && npm test -- --run PluginsPage`
Expected: PASS

- [ ] **Step 8: Run the full frontend suite and type check**

Run: `cd frontend && npm test -- --run && npx tsc -b`
Expected: all tests pass (including the updated `AppShell.test.tsx`, which still passes unmodified since it doesn't assert on the full nav contents), 0 type errors

- [ ] **Step 9: Commit**

```bash
git add frontend/src/api/types.ts frontend/src/api/client.ts frontend/src/pages/PluginsPage.tsx frontend/src/pages/PluginsPage.test.tsx frontend/src/App.tsx frontend/src/components/AppShell.tsx internal/i18n/bundles/en.json internal/i18n/bundles/de.json
git commit -m "feat: add Plugins browser with upload/delete management"
```

---

### Task 7: End-to-end tests

**Files:**
- Create: `frontend/e2e/watch-live-updates.spec.ts`
- Create: `frontend/e2e/plugin-upload.spec.ts`
- Create: `frontend/e2e/fixtures/extra-sport.lua`
- Modify: `frontend/e2e/run-server.sh`

**Interfaces:**
- Consumes: everything from Tasks 1-6, plus the existing e2e helper patterns in `frontend/e2e/round-lifecycle.spec.ts` and `frontend/e2e/setup-flow.spec.ts`.
- Produces: nothing further downstream — this is the last task.

- [ ] **Step 1: Isolate the e2e plugins directory**

Replace `frontend/e2e/run-server.sh`:

```sh
#!/bin/sh
set -e
cd "$(dirname "$0")/../.."
rm -f /tmp/tournamentstudio-e2e.db
rm -rf /tmp/tournamentstudio-e2e-plugins
export TOURNAMENTSTUDIO_DB=/tmp/tournamentstudio-e2e.db
export TOURNAMENTSTUDIO_PLUGINS=/tmp/tournamentstudio-e2e-plugins
export TOURNAMENTSTUDIO_ADMIN_USER=organizer1
export TOURNAMENTSTUDIO_ADMIN_PASSWORD=e2e-test-password
go run ./cmd/tournamentstudio
```

- [ ] **Step 2: Write the Watch live-update e2e test**

Create `frontend/e2e/watch-live-updates.spec.ts`:

```ts
import { test, expect } from '@playwright/test'
import path from 'node:path'
import { fileURLToPath } from 'node:url'

const __dirname = path.dirname(fileURLToPath(import.meta.url))

test('watch page reflects a result submitted elsewhere, without a manual reload', async ({ page }) => {
  await page.goto('/login')
  await page.getByLabel('Username').fill('organizer1')
  await page.getByLabel('Password').fill('e2e-test-password')
  await page.getByRole('button', { name: 'Sign In' }).click()
  await expect(page).toHaveURL(/\/tournaments$/)

  await page.getByRole('link', { name: 'Create Tournament' }).click()
  await page.getByLabel('Name', { exact: true }).fill('Watch Test Cup')
  await page.getByLabel('Sport').selectOption({ label: 'Dragonboat' })
  await page.getByLabel('Tournament Type').selectOption({ index: 1 })
  await page.getByRole('button', { name: 'Create' }).click()

  await expect(page).toHaveURL(/\/tournaments\/\d+\/teams$/)
  const tournamentIdMatch = page.url().match(/\/tournaments\/(\d+)\//)
  if (!tournamentIdMatch) throw new Error(`could not extract tournament id from URL: ${page.url()}`)
  const tournamentId = tournamentIdMatch[1]

  await page.getByRole('link', { name: 'Import from file' }).click()
  const fixturePath = path.join(__dirname, 'fixtures', 'teams.csv')
  await page.getByLabel('Choose a CSV or XLSX file').setInputFiles(fixturePath)
  await page.getByRole('button', { name: 'Upload' }).click()
  await expect(page.getByText(/4 team\(s\) imported\./)).toBeVisible()

  await page.getByRole('link', { name: 'Rounds & Schedule' }).click()
  await page.getByLabel(/^Name$/).fill('Lane 1')
  await page.getByLabel(/heat interval/i).fill('60')
  await page.getByRole('button', { name: 'Add Course' }).click()
  await expect(page.getByText(/Lane 1/)).toBeVisible()

  await page.getByLabel(/number of groups/i).fill('2')
  await page.getByRole('button', { name: 'Shuffle into groups' }).click()
  await page.getByRole('button', { name: 'Create Round 1' }).click()
  await expect(page.getByText("Schedule this round's groups")).toBeVisible()

  const courseSelects = page.locator('select[aria-label*="Course —"]')
  await expect(courseSelects).toHaveCount(2)
  for (let i = 0; i < 2; i++) {
    await courseSelects.nth(i).selectOption({ label: 'Lane 1' })
  }
  await page.getByRole('button', { name: 'Schedule' }).click()
  await expect(page.getByRole('heading', { name: 'Heats' })).toBeVisible()

  // Grab one heat's id directly from the backend so the result can be
  // submitted via the API (not the UI) -- proving the Watch page picks up
  // a change made by someone else, not just its own writes.
  const token = await page.evaluate(() => localStorage.getItem('ts_token'))
  const scheduleRes = await page.request.get(`/api/tournaments/${tournamentId}/schedule`, {
    headers: { Authorization: `Bearer ${token}` },
  })
  expect(scheduleRes.ok()).toBe(true)
  const scheduleBody = (await scheduleRes.json()) as { heats: { id: number; group_id: number | null }[] }
  const heat = scheduleBody.heats.find((h) => h.group_id !== null)
  if (!heat) throw new Error('expected at least one scheduled group heat')

  const roundsRes = await page.request.get(`/api/tournaments/${tournamentId}/rounds`, {
    headers: { Authorization: `Bearer ${token}` },
  })
  const roundsBody = (await roundsRes.json()) as { rounds: { groups: { id: number; team_ids: string[] }[] }[] }
  const group = roundsBody.rounds[0].groups.find((g) => g.id === heat.group_id)
  if (!group) throw new Error('expected the heat\'s group to exist')
  const [firstTeamId] = group.team_ids

  // Now navigate to the Watch tab and leave it untouched. Nobody has a
  // result yet, so the round renders with no ranked-team rows at all
  // (unranked teams are omitted entirely -- see the standings endpoint's
  // design) -- confirm the page loaded via its round heading only.
  await page.getByRole('link', { name: 'Live Standings' }).click()
  await expect(page).toHaveURL(/\/watch$/)
  await expect(page.getByText('Round 1 — open')).toBeVisible()
  await expect(page.getByText('123.45s')).toHaveCount(0)

  // Submit a result for that heat via the raw API -- simulating a second
  // operator elsewhere, not this page's own action.
  const submitRes = await page.request.post(`/api/tournaments/${tournamentId}/heats/${heat.id}/results`, {
    headers: { Authorization: `Bearer ${token}`, 'Content-Type': 'application/json' },
    data: { [firstTeamId]: { time_seconds: 123.45 } },
  })
  expect(submitRes.ok()).toBe(true)

  // The Watch page must reflect this without any reload or click here.
  await expect(page.getByText('123.45s')).toBeVisible({ timeout: 10_000 })
})
```

- [ ] **Step 3: Write the plugin upload e2e test**

Create `frontend/e2e/fixtures/extra-sport.lua`:

```lua
return {
  id = "e2e-extra-sport",
  display_name = "E2E Extra Sport",
  compatible_tournament_types = {"timed-heats-reseeding"},
}
```

Create `frontend/e2e/plugin-upload.spec.ts`:

```ts
import { test, expect } from '@playwright/test'
import path from 'node:path'
import { fileURLToPath } from 'node:url'

const __dirname = path.dirname(fileURLToPath(import.meta.url))

test('organizer uploads a plugin and it becomes selectable when creating a tournament', async ({ page }) => {
  await page.goto('/login')
  await page.getByLabel('Username').fill('organizer1')
  await page.getByLabel('Password').fill('e2e-test-password')
  await page.getByRole('button', { name: 'Sign In' }).click()
  await expect(page).toHaveURL(/\/tournaments$/)

  await page.getByRole('link', { name: 'Plugins' }).click()
  await expect(page).toHaveURL(/\/plugins$/)
  await expect(page.getByText('Dragonboat')).toBeVisible()

  const fixturePath = path.join(__dirname, 'fixtures', 'extra-sport.lua')
  await page.getByLabel('Choose a .lua file').setInputFiles(fixturePath)
  await page.getByRole('button', { name: 'Upload' }).click()
  await expect(page.getByText('E2E Extra Sport')).toBeVisible()

  await page.getByRole('link', { name: 'Create Tournament' }).click()
  await expect(page.getByLabel('Sport')).toContainText('E2E Extra Sport')
})
```

- [ ] **Step 4: Run both new specs**

Playwright's `webServer` config (`frontend/playwright.config.ts`) already starts and stops `e2e/run-server.sh` automatically around the test run — build the embedded frontend first so `go run`'s server has real assets to serve, then run just the two new specs:

```bash
cd frontend
npm run build
npm run test:e2e -- e2e/watch-live-updates.spec.ts e2e/plugin-upload.spec.ts
```
Expected: both specs pass

- [ ] **Step 5: Run the full e2e suite to confirm no regressions**

```bash
cd frontend
npm run test:e2e
```
Expected: all specs pass (`setup-flow.spec.ts`, `round-lifecycle.spec.ts`, `watch-live-updates.spec.ts`, `plugin-upload.spec.ts`)

- [ ] **Step 6: Commit**

```bash
git add frontend/e2e/watch-live-updates.spec.ts frontend/e2e/plugin-upload.spec.ts frontend/e2e/fixtures/extra-sport.lua frontend/e2e/run-server.sh
git commit -m "test: add end-to-end coverage for live Watch updates and plugin upload"
```
