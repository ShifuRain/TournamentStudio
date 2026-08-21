# TournamentStudio Plugin Engine & Dragonboat Logic Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Build the Lua plugin engine (sandboxed, pure-Go), the `dragonboat` sport plugin and `timed-heats-reseeding` tournament-type plugin, and the `PrePhaseRound`/`Group` data model + endpoints that let an organizer run a full pre-phase: create round 1, submit results, compute the next reseeded round, and compute final divisions.

**Architecture:** `internal/ranking` is a pure-Go, dependency-free package implementing the generic time/DNF/DSQ/DNS sort every timed tournament-type wants. `internal/plugin` embeds two default `.lua` files (`bundled/dragonboat.lua`, `bundled/timed-heats-reseeding.lua`) via `go:embed`, merges them with any `.lua` files found in an external directory (same pattern as the Foundation plan's i18n loader), and exposes typed Go methods (`NextRoundGroups`, `DivisionCuts`) that marshal Go structs to Lua tables, call the plugin's function via `gopher-lua`, and marshal the result back — no JSON at that boundary. `internal/round` is a new domain package (mirrors `internal/tournament`/`internal/team`'s shape) holding `PrePhaseRound`, `Group`, and per-round results. `internal/server` orchestrates all of this in HTTP handlers, exactly as it already orchestrates `internal/importer` + `internal/team` in the Foundation plan.

**Tech Stack:** Go (continuing on the Foundation plan's module), `github.com/yuin/gopher-lua` (pure-Go Lua VM, no CGo).

**Spec:** `docs/superpowers/specs/2026-08-21-tournament-studio-design.md` (as amended to Lua) and `docs/superpowers/specs/2026-08-21-tournamentstudio-plugin-engine-design.md` (this plan's detailed design — read both; the second one has the exact algorithm and host-interface contract this plan implements).

## Global Constraints

- Pure-Go dependencies only — `gopher-lua` has no CGo, matching the Foundation plan's SQLite driver choice.
- A plugin runs in its own sandboxed `*lua.LState`: no `os`, `io`, `require`, `load`, or `dofile` ever registered into it.
- One broken/malformed `.lua` file must never prevent other plugins from loading or crash the server (same defensive posture as the Foundation plan's i18n loader).
- Role enforcement stays server-side on every write endpoint: round/results/next/divisions creation is organizer-only except submitting results, which is organizer **or** time-entry.
- `team_id` is a string everywhere in the plugin/ranking/round layers (the decimal string form of a Foundation-plan `Team.ID`), per the design spec — never re-litigate this as an int64 partway through.
- JSON request/response field names are snake_case with explicit struct tags, matching the convention the Foundation plan's final review established — do not ship untagged PascalCase response structs.
- This plan does **not** build `Course`, `Heat`, real result-entry UI, WebSocket broadcast, or a `Division` persistence entity — those are Plan 3/4. The `POST .../divisions` endpoint in this plan computes and returns divisions without persisting them (see Task 11).

---

## File Structure

```
internal/ranking/rank.go                          generic time/status sort, zero dependencies
internal/ranking/rank_test.go

internal/plugin/types.go                           RosterField, SportPlugin, TournamentTypePlugin struct
internal/plugin/engine.go                           Engine, Load, Sports/TournamentTypes/Close, loadSource
internal/plugin/parse.go                            parseSportPlugin, parseTournamentTypePlugin
internal/plugin/engine_test.go
internal/plugin/tournamenttype.go                    NextRoundGroups (Go<->Lua conversion)
internal/plugin/tournamenttype_test.go
internal/plugin/division.go                          Cut, Division, DivisionCuts
internal/plugin/division_test.go
internal/plugin/dragonboat_test.go
internal/plugin/bundled/timed-heats-reseeding.lua     the reseeding + division-cut algorithm
internal/plugin/bundled/dragonboat.lua                declarative sport metadata

internal/round/model.go                             PrePhaseRound, Group, Result
internal/round/repo.go                               SQLite-backed repo for all three
internal/round/repo_test.go
internal/store/migrations/0006_pre_phase_rounds.sql
internal/store/migrations/0007_groups.sql
internal/store/migrations/0008_round_results.sql

internal/server/handlers_plugins.go                  GET /api/plugins
internal/server/handlers_round.go                    POST /api/tournaments/{id}/rounds
internal/server/handlers_round_results.go             POST .../rounds/{round_id}/results
internal/server/handlers_round_next.go                POST .../rounds/{round_id}/next
internal/server/handlers_round_divisions.go            POST .../rounds/{round_id}/divisions
internal/server/*_test.go                             new test files per handler, same pattern as Foundation plan

cmd/tournamentstudio/main.go                         modified: load plugin engine, pass to server.New
```

---

### Task 1: Generic ranking package

**Files:**
- Create: `internal/ranking/rank.go`
- Create: `internal/ranking/rank_test.go`

**Interfaces:**
- Produces: `ranking.Status` (`StatusDNF`, `StatusDSQ`, `StatusDNS`), `ranking.TeamResult{TeamID string, TimeSeconds *float64, Status Status}`, `ranking.Rank(results []TeamResult) []TeamResult` (returns a new, sorted slice; does not mutate its input).

- [ ] **Step 1: Write the failing test**

Create `internal/ranking/rank_test.go`:

```go
package ranking

import "testing"

func f(v float64) *float64 { return &v }

func TestRankOrdersByTimeAscending(t *testing.T) {
	input := []TeamResult{
		{TeamID: "b", TimeSeconds: f(130.5)},
		{TeamID: "a", TimeSeconds: f(124.11)},
		{TeamID: "c", TimeSeconds: f(126.87)},
	}
	got := Rank(input)
	want := []string{"a", "c", "b"}
	for i, w := range want {
		if got[i].TeamID != w {
			t.Fatalf("position %d: expected %s, got %s", i, w, got[i].TeamID)
		}
	}
}

func TestRankPlacesStatusesAfterTimedTeamsInOrder(t *testing.T) {
	input := []TeamResult{
		{TeamID: "dns-team", Status: StatusDNS},
		{TeamID: "timed", TimeSeconds: f(120)},
		{TeamID: "dsq-team", Status: StatusDSQ},
		{TeamID: "dnf-team", Status: StatusDNF},
	}
	got := Rank(input)
	want := []string{"timed", "dnf-team", "dsq-team", "dns-team"}
	for i, w := range want {
		if got[i].TeamID != w {
			t.Fatalf("position %d: expected %s, got %s", i, w, got[i].TeamID)
		}
	}
}

func TestRankIsStableForTies(t *testing.T) {
	input := []TeamResult{
		{TeamID: "first", TimeSeconds: f(100)},
		{TeamID: "second", TimeSeconds: f(100)},
	}
	got := Rank(input)
	if got[0].TeamID != "first" || got[1].TeamID != "second" {
		t.Fatalf("expected stable order first,second; got %s,%s", got[0].TeamID, got[1].TeamID)
	}
}

func TestRankDoesNotMutateInput(t *testing.T) {
	input := []TeamResult{
		{TeamID: "b", TimeSeconds: f(2)},
		{TeamID: "a", TimeSeconds: f(1)},
	}
	_ = Rank(input)
	if input[0].TeamID != "b" {
		t.Fatalf("expected input left unmodified, got %s at position 0", input[0].TeamID)
	}
}
```

- [ ] **Step 2: Run the test to verify it fails**

Run: `go test ./internal/ranking/...`
Expected: FAIL — package does not exist yet.

- [ ] **Step 3: Implement**

Create `internal/ranking/rank.go`:

```go
package ranking

import "sort"

type Status string

const (
	StatusDNF Status = "DNF"
	StatusDSQ Status = "DSQ"
	StatusDNS Status = "DNS"
)

type TeamResult struct {
	TeamID      string
	TimeSeconds *float64
	Status      Status
}

func statusOrder(s Status) int {
	switch s {
	case StatusDNF:
		return 1
	case StatusDSQ:
		return 2
	case StatusDNS:
		return 3
	default:
		return 0
	}
}

// Rank returns a new slice sorted fastest-first: teams with a recorded
// time sort ascending by that time; teams with a status instead of a
// time sort after every timed team, in the order DNF, DSQ, DNS. Ties
// keep their relative input order. The input slice is not modified.
func Rank(results []TeamResult) []TeamResult {
	ranked := make([]TeamResult, len(results))
	copy(ranked, results)

	sort.SliceStable(ranked, func(i, j int) bool {
		a, b := ranked[i], ranked[j]
		aHasTime := a.TimeSeconds != nil
		bHasTime := b.TimeSeconds != nil

		if aHasTime && bHasTime {
			return *a.TimeSeconds < *b.TimeSeconds
		}
		if aHasTime != bHasTime {
			return aHasTime
		}
		return statusOrder(a.Status) < statusOrder(b.Status)
	})

	return ranked
}
```

- [ ] **Step 4: Run the test to verify it passes**

Run: `go test ./internal/ranking/...`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add internal/ranking
git commit -m "feat: add generic time/status ranking"
```

---

### Task 2: Plugin engine skeleton — loading, parsing, sandboxing

**Files:**
- Create: `internal/plugin/types.go`
- Create: `internal/plugin/engine.go`
- Create: `internal/plugin/parse.go`
- Create: `internal/plugin/engine_test.go`

**Interfaces:**
- Produces: `plugin.RosterField{Key, Label string, Required bool}`, `plugin.SportPlugin{ID, DisplayName string, CompatibleTournamentTypes []string, RosterFields []RosterField}`, `plugin.TournamentTypePlugin{ID string, CompatibleSports []string}` (unexported `state`/`mu`/`nextRoundGroupsFn`/`divisionCutsFn` fields used by Task 3/4), `plugin.Load(externalDir string) (*Engine, error)`, `(*Engine).Sports() []*SportPlugin`, `(*Engine).TournamentTypes() []*TournamentTypePlugin`, `(*Engine).Close()`.

- [ ] **Step 1: Add the gopher-lua dependency**

```bash
go get github.com/yuin/gopher-lua
go mod tidy
```

Confirm `go.mod` lists it as a direct dependency (no `// indirect`) — this exact class of mistake happened once already in this project; don't skip verifying it.

- [ ] **Step 2: Write the failing tests**

Create `internal/plugin/engine_test.go`:

```go
package plugin

import (
	"os"
	"path/filepath"
	"testing"
)

func writeTestPlugin(t *testing.T, dir, filename, content string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(dir, filename), []byte(content), 0o644); err != nil {
		t.Fatalf("write test plugin: %v", err)
	}
}

func findSport(e *Engine, id string) *SportPlugin {
	for _, sp := range e.Sports() {
		if sp.ID == id {
			return sp
		}
	}
	return nil
}

func findTournamentType(e *Engine, id string) *TournamentTypePlugin {
	for _, ttp := range e.TournamentTypes() {
		if ttp.ID == id {
			return ttp
		}
	}
	return nil
}

func TestLoadRegistersSportPlugin(t *testing.T) {
	dir := t.TempDir()
	writeTestPlugin(t, dir, "sport.lua", `
return {
  id = "test-sport",
  display_name = "Test Sport",
  compatible_tournament_types = {"test-format"},
  roster_fields = {
    {key = "boat_class", label = "Boat class", required = false},
  },
}
`)

	e, err := Load(dir)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	defer e.Close()

	sp := findSport(e, "test-sport")
	if sp == nil {
		t.Fatalf("expected test-sport to register")
	}
	if sp.DisplayName != "Test Sport" {
		t.Fatalf("expected display name Test Sport, got %s", sp.DisplayName)
	}
	if len(sp.CompatibleTournamentTypes) != 1 || sp.CompatibleTournamentTypes[0] != "test-format" {
		t.Fatalf("unexpected compatible tournament types: %v", sp.CompatibleTournamentTypes)
	}
	if len(sp.RosterFields) != 1 || sp.RosterFields[0].Key != "boat_class" {
		t.Fatalf("unexpected roster fields: %v", sp.RosterFields)
	}
}

func TestLoadRegistersTournamentTypePlugin(t *testing.T) {
	dir := t.TempDir()
	writeTestPlugin(t, dir, "format.lua", `
return {
  id = "test-format",
  compatible_sports = {"test-sport"},
  next_round_groups = function(groups) return {} end,
  division_cuts = function(ranked, cuts) return {} end,
}
`)

	e, err := Load(dir)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	defer e.Close()

	ttp := findTournamentType(e, "test-format")
	if ttp == nil {
		t.Fatalf("expected test-format to register")
	}
	if len(ttp.CompatibleSports) != 1 || ttp.CompatibleSports[0] != "test-sport" {
		t.Fatalf("unexpected compatible sports: %v", ttp.CompatibleSports)
	}
}

func TestLoadSkipsMalformedPluginWithoutFailing(t *testing.T) {
	dir := t.TempDir()
	writeTestPlugin(t, dir, "broken.lua", `this is not valid lua {{{`)
	writeTestPlugin(t, dir, "good.lua", `
return {
  id = "good-sport",
  compatible_tournament_types = {},
}
`)

	e, err := Load(dir)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	defer e.Close()

	if findSport(e, "good-sport") == nil {
		t.Fatalf("expected good-sport to still register despite broken.lua")
	}
}

func TestLoadTournamentTypeRequiresFunctions(t *testing.T) {
	dir := t.TempDir()
	writeTestPlugin(t, dir, "incomplete.lua", `
return {
  id = "incomplete-format",
  compatible_sports = {"test-sport"},
  next_round_groups = function(groups) return {} end,
}
`)

	e, err := Load(dir)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	defer e.Close()

	if findTournamentType(e, "incomplete-format") != nil {
		t.Fatalf("expected incomplete-format to be skipped (missing division_cuts)")
	}
}

func TestLoadNonexistentExternalDirDoesNotError(t *testing.T) {
	e, err := Load(filepath.Join(t.TempDir(), "does-not-exist"))
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	defer e.Close()
}
```

- [ ] **Step 3: Run the tests to verify they fail**

Run: `go test ./internal/plugin/...`
Expected: FAIL — package does not exist yet.

- [ ] **Step 4: Implement the types**

Create `internal/plugin/types.go`:

```go
package plugin

import (
	"sync"

	lua "github.com/yuin/gopher-lua"
)

type RosterField struct {
	Key      string
	Label    string
	Required bool
}

type SportPlugin struct {
	ID                        string
	DisplayName               string
	CompatibleTournamentTypes []string
	RosterFields              []RosterField
}

type TournamentTypePlugin struct {
	ID               string
	CompatibleSports []string

	state             *lua.LState
	mu                sync.Mutex
	nextRoundGroupsFn *lua.LFunction
	divisionCutsFn    *lua.LFunction
}
```

- [ ] **Step 5: Implement parsing**

Create `internal/plugin/parse.go`:

```go
package plugin

import (
	"fmt"

	lua "github.com/yuin/gopher-lua"
)

func parseSportPlugin(tbl *lua.LTable, id string) *SportPlugin {
	sp := &SportPlugin{ID: id}

	if dn, ok := tbl.RawGetString("display_name").(lua.LString); ok {
		sp.DisplayName = string(dn)
	}

	if ctt, ok := tbl.RawGetString("compatible_tournament_types").(*lua.LTable); ok {
		ctt.ForEach(func(_, v lua.LValue) {
			if s, ok := v.(lua.LString); ok {
				sp.CompatibleTournamentTypes = append(sp.CompatibleTournamentTypes, string(s))
			}
		})
	}

	if rf, ok := tbl.RawGetString("roster_fields").(*lua.LTable); ok {
		rf.ForEach(func(_, v lua.LValue) {
			fieldTbl, ok := v.(*lua.LTable)
			if !ok {
				return
			}
			field := RosterField{}
			if key, ok := fieldTbl.RawGetString("key").(lua.LString); ok {
				field.Key = string(key)
			}
			if label, ok := fieldTbl.RawGetString("label").(lua.LString); ok {
				field.Label = string(label)
			}
			if req, ok := fieldTbl.RawGetString("required").(lua.LBool); ok {
				field.Required = bool(req)
			}
			sp.RosterFields = append(sp.RosterFields, field)
		})
	}

	return sp
}

func parseTournamentTypePlugin(tbl *lua.LTable, id string, L *lua.LState) (*TournamentTypePlugin, error) {
	ttp := &TournamentTypePlugin{ID: id, state: L}

	if cs, ok := tbl.RawGetString("compatible_sports").(*lua.LTable); ok {
		cs.ForEach(func(_, v lua.LValue) {
			if s, ok := v.(lua.LString); ok {
				ttp.CompatibleSports = append(ttp.CompatibleSports, string(s))
			}
		})
	}

	nextFn, ok := tbl.RawGetString("next_round_groups").(*lua.LFunction)
	if !ok {
		return nil, fmt.Errorf("must define next_round_groups")
	}
	ttp.nextRoundGroupsFn = nextFn

	divFn, ok := tbl.RawGetString("division_cuts").(*lua.LFunction)
	if !ok {
		return nil, fmt.Errorf("must define division_cuts")
	}
	ttp.divisionCutsFn = divFn

	return ttp, nil
}
```

- [ ] **Step 6: Implement the engine**

Create `internal/plugin/engine.go`:

```go
package plugin

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	lua "github.com/yuin/gopher-lua"
)

type Engine struct {
	sports          map[string]*SportPlugin
	tournamentTypes map[string]*TournamentTypePlugin
}

// Load scans externalDir for *.lua files and registers each as either a
// sport plugin or a tournament-type plugin. A missing externalDir is not
// an error (empty engine). A malformed or invalid individual file is
// logged and skipped, never fatal to the rest of the load.
func Load(externalDir string) (*Engine, error) {
	e := &Engine{
		sports:          make(map[string]*SportPlugin),
		tournamentTypes: make(map[string]*TournamentTypePlugin),
	}

	entries, err := os.ReadDir(externalDir)
	if os.IsNotExist(err) {
		return e, nil
	}
	if err != nil {
		return nil, err
	}

	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".lua") {
			continue
		}
		source, err := os.ReadFile(filepath.Join(externalDir, entry.Name()))
		if err != nil {
			fmt.Fprintf(os.Stderr, "plugin: skipping %s: %v\n", entry.Name(), err)
			continue
		}
		if err := e.loadSource(entry.Name(), source); err != nil {
			fmt.Fprintf(os.Stderr, "plugin: skipping %s: %v\n", entry.Name(), err)
		}
	}

	return e, nil
}

func (e *Engine) loadSource(name string, source []byte) error {
	L := lua.NewState()

	if err := L.DoString(string(source)); err != nil {
		L.Close()
		return fmt.Errorf("load %s: %w", name, err)
	}

	ret := L.Get(-1)
	L.Pop(1)

	tbl, ok := ret.(*lua.LTable)
	if !ok {
		L.Close()
		return fmt.Errorf("%s: plugin file must return a table", name)
	}

	idVal, ok := tbl.RawGetString("id").(lua.LString)
	if !ok || string(idVal) == "" {
		L.Close()
		return fmt.Errorf("%s: plugin table must have a non-empty string 'id' field", name)
	}
	id := string(idVal)

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
}

func (e *Engine) Sports() []*SportPlugin {
	result := make([]*SportPlugin, 0, len(e.sports))
	for _, sp := range e.sports {
		result = append(result, sp)
	}
	return result
}

func (e *Engine) TournamentTypes() []*TournamentTypePlugin {
	result := make([]*TournamentTypePlugin, 0, len(e.tournamentTypes))
	for _, ttp := range e.tournamentTypes {
		result = append(result, ttp)
	}
	return result
}

// Close releases the Lua VM held by every loaded tournament-type plugin.
// Sport plugins are pure metadata and their VM is already closed by the
// time Load returns.
func (e *Engine) Close() {
	for _, ttp := range e.tournamentTypes {
		if ttp.state != nil {
			ttp.state.Close()
		}
	}
}
```

- [ ] **Step 7: Run the tests to verify they pass**

Run: `go test ./internal/plugin/...`
Expected: PASS

- [ ] **Step 8: Commit**

```bash
git add go.mod go.sum internal/plugin
git commit -m "feat: add Lua plugin engine (loading, parsing, sandboxing)"
```

---

### Task 3: `NextRoundGroups` — Go↔Lua conversion and the reseeding algorithm

**Files:**
- Create: `internal/plugin/bundled/timed-heats-reseeding.lua`
- Create: `internal/plugin/tournamenttype.go`
- Create: `internal/plugin/tournamenttype_test.go`

**Interfaces:**
- Consumes: `ranking.TeamResult` (Task 1), `TournamentTypePlugin` (Task 2).
- Produces: `(*TournamentTypePlugin).NextRoundGroups(groups [][]ranking.TeamResult) ([][]string, error)`.

- [ ] **Step 1: Write the failing test**

Create `internal/plugin/tournamenttype_test.go`:

```go
package plugin

import (
	"testing"

	"tournamentstudio/internal/ranking"
)

func loadTimedHeatsReseeding(t *testing.T) *TournamentTypePlugin {
	t.Helper()
	// bundled/ is not embedded until Task 6 — until then, point Load at
	// the real bundled directory directly, same as any external dir.
	e, err := Load("bundled")
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	t.Cleanup(e.Close)

	ttp := findTournamentType(e, "timed-heats-reseeding")
	if ttp == nil {
		t.Fatalf("timed-heats-reseeding plugin not found")
	}
	return ttp
}

func assertSameMembers(t *testing.T, got, want []string) {
	t.Helper()
	if len(got) != len(want) {
		t.Fatalf("expected %v, got %v", want, got)
	}
	seen := make(map[string]bool, len(got))
	for _, g := range got {
		seen[g] = true
	}
	for _, w := range want {
		if !seen[w] {
			t.Fatalf("expected %v to contain %s, got %v", want, w, got)
		}
	}
}

func TestNextRoundGroupsCrossSwapHalves(t *testing.T) {
	ttp := loadTimedHeatsReseeding(t)

	groups := [][]ranking.TeamResult{
		{
			{TeamID: "A1", TimeSeconds: f(120)},
			{TeamID: "A2", TimeSeconds: f(121)},
			{TeamID: "A3", TimeSeconds: f(122)},
			{TeamID: "A4", TimeSeconds: f(123)},
		},
		{
			{TeamID: "B1", TimeSeconds: f(120)},
			{TeamID: "B2", TimeSeconds: f(121)},
			{TeamID: "B3", TimeSeconds: f(122)},
			{TeamID: "B4", TimeSeconds: f(123)},
		},
	}

	got, err := ttp.NextRoundGroups(groups)
	if err != nil {
		t.Fatalf("NextRoundGroups: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("expected 2 new groups, got %d", len(got))
	}

	assertSameMembers(t, got[0], []string{"A1", "A2", "B3", "B4"})
	assertSameMembers(t, got[1], []string{"A3", "A4", "B1", "B2"})
}

func TestNextRoundGroupsThreeGroupsConservesAllTeams(t *testing.T) {
	ttp := loadTimedHeatsReseeding(t)

	groups := [][]ranking.TeamResult{
		{{TeamID: "A1", TimeSeconds: f(1)}, {TeamID: "A2", TimeSeconds: f(2)}, {TeamID: "A3", TimeSeconds: f(3)}},
		{{TeamID: "B1", TimeSeconds: f(1)}, {TeamID: "B2", TimeSeconds: f(2)}, {TeamID: "B3", TimeSeconds: f(3)}},
		{{TeamID: "C1", TimeSeconds: f(1)}, {TeamID: "C2", TimeSeconds: f(2)}, {TeamID: "C3", TimeSeconds: f(3)}},
	}

	got, err := ttp.NextRoundGroups(groups)
	if err != nil {
		t.Fatalf("NextRoundGroups: %v", err)
	}
	if len(got) != 3 {
		t.Fatalf("expected 3 new groups, got %d", len(got))
	}

	all := make(map[string]bool)
	for _, group := range got {
		for _, id := range group {
			if all[id] {
				t.Fatalf("team %s appeared in more than one new group", id)
			}
			all[id] = true
		}
	}
	for _, id := range []string{"A1", "A2", "A3", "B1", "B2", "B3", "C1", "C2", "C3"} {
		if !all[id] {
			t.Fatalf("team %s missing from reseeded groups", id)
		}
	}
}

func TestNextRoundGroupsPlacesNonFinishersInSlowestTier(t *testing.T) {
	ttp := loadTimedHeatsReseeding(t)

	groups := [][]ranking.TeamResult{
		{
			{TeamID: "A1", TimeSeconds: f(120)},
			{TeamID: "A2", TimeSeconds: f(121)},
			{TeamID: "A-dnf", Status: ranking.StatusDNF},
			{TeamID: "A-dns", Status: ranking.StatusDNS},
		},
		{
			{TeamID: "B1", TimeSeconds: f(120)},
			{TeamID: "B2", TimeSeconds: f(121)},
			{TeamID: "B3", TimeSeconds: f(122)},
			{TeamID: "B4", TimeSeconds: f(123)},
		},
	}

	got, err := ttp.NextRoundGroups(groups)
	if err != nil {
		t.Fatalf("NextRoundGroups: %v", err)
	}

	assertSameMembers(t, got[1], []string{"A-dnf", "A-dns", "B1", "B2"})
}
```

- [ ] **Step 2: Run the test to verify it fails**

Run: `go test ./internal/plugin/...`
Expected: FAIL — `bundled` directory / `timed-heats-reseeding` plugin does not exist yet, and `NextRoundGroups` is undefined.

- [ ] **Step 3: Write the Lua plugin's algorithm**

Create `internal/plugin/bundled/timed-heats-reseeding.lua`:

```lua
local function tiers_for_group(group, k)
  local n = #group
  local tier_size = math.floor(n / k)
  local remainder = n % k
  local tiers = {}
  local idx = 1
  for j = 0, k - 1 do
    tiers[j] = {}
    local size = tier_size
    if j == k - 1 then
      size = tier_size + remainder
    end
    for _ = 1, size do
      table.insert(tiers[j], group[idx])
      idx = idx + 1
    end
  end
  return tiers
end

local function next_round_groups(groups)
  local k = #groups
  local tiers_by_group = {}
  for i = 1, k do
    tiers_by_group[i] = tiers_for_group(groups[i], k)
  end

  local new_groups = {}
  for n = 0, k - 1 do
    local new_group = {}
    for i = 1, k do
      local tier_index = (n + (i - 1)) % k
      for _, entry in ipairs(tiers_by_group[i][tier_index]) do
        table.insert(new_group, entry.team_id)
      end
    end
    new_groups[n + 1] = new_group
  end

  return new_groups
end

local function division_cuts(ranked_teams, cuts)
  -- Replaced with the real implementation in the next task.
  return {}
end

return {
  id = "timed-heats-reseeding",
  compatible_sports = {"dragonboat"},
  next_round_groups = next_round_groups,
  division_cuts = division_cuts,
}
```

- [ ] **Step 4: Implement the Go↔Lua conversion**

Create `internal/plugin/tournamenttype.go`:

```go
package plugin

import (
	"fmt"

	lua "github.com/yuin/gopher-lua"

	"tournamentstudio/internal/ranking"
)

// NextRoundGroups calls the plugin's next_round_groups(groups) function.
// groups[i] must already be sorted fastest-first (see ranking.Rank) —
// this method does not re-sort.
func (t *TournamentTypePlugin) NextRoundGroups(groups [][]ranking.TeamResult) ([][]string, error) {
	t.mu.Lock()
	defer t.mu.Unlock()

	L := t.state

	groupsTbl := L.NewTable()
	for _, group := range groups {
		groupTbl := L.NewTable()
		for _, r := range group {
			entry := L.NewTable()
			entry.RawSetString("team_id", lua.LString(r.TeamID))
			if r.TimeSeconds != nil {
				entry.RawSetString("time_seconds", lua.LNumber(*r.TimeSeconds))
			} else {
				entry.RawSetString("time_seconds", lua.LNil)
			}
			if r.Status != "" {
				entry.RawSetString("status", lua.LString(r.Status))
			} else {
				entry.RawSetString("status", lua.LNil)
			}
			groupTbl.Append(entry)
		}
		groupsTbl.Append(groupTbl)
	}

	if err := L.CallByParam(lua.P{
		Fn:      t.nextRoundGroupsFn,
		NRet:    1,
		Protect: true,
	}, groupsTbl); err != nil {
		return nil, fmt.Errorf("next_round_groups: %w", err)
	}

	ret := L.Get(-1)
	L.Pop(1)

	retTbl, ok := ret.(*lua.LTable)
	if !ok {
		return nil, fmt.Errorf("next_round_groups must return a table")
	}

	var result [][]string
	n := retTbl.Len()
	for i := 1; i <= n; i++ {
		groupTbl, ok := retTbl.RawGetInt(i).(*lua.LTable)
		if !ok {
			return nil, fmt.Errorf("next_round_groups result group %d is not a table", i)
		}
		var teamIDs []string
		m := groupTbl.Len()
		for j := 1; j <= m; j++ {
			idStr, ok := groupTbl.RawGetInt(j).(lua.LString)
			if !ok {
				return nil, fmt.Errorf("next_round_groups result group %d entry %d is not a string", i, j)
			}
			teamIDs = append(teamIDs, string(idStr))
		}
		result = append(result, teamIDs)
	}

	return result, nil
}
```

- [ ] **Step 5: Run the tests to verify they pass**

Run: `go test ./internal/plugin/...`
Expected: PASS

- [ ] **Step 6: Commit**

```bash
git add internal/plugin
git commit -m "feat: implement next_round_groups reseeding algorithm"
```

---

### Task 4: `DivisionCuts`

**Files:**
- Modify: `internal/plugin/bundled/timed-heats-reseeding.lua` (replace the `division_cuts` stub)
- Create: `internal/plugin/division.go`
- Create: `internal/plugin/division_test.go`

**Interfaces:**
- Produces: `plugin.Cut{Name string, Size int}`, `plugin.Division{Name string, TeamIDs []string}`, `(*TournamentTypePlugin).DivisionCuts(rankedTeamIDs []string, cuts []Cut) ([]Division, error)`.

- [ ] **Step 1: Write the failing tests**

Create `internal/plugin/division_test.go`:

```go
package plugin

import "testing"

func TestDivisionCutsFillsInOrder(t *testing.T) {
	ttp := loadTimedHeatsReseeding(t)

	ranked := []string{"t1", "t2", "t3", "t4", "t5", "t6"}
	cuts := []Cut{
		{Name: "Gold Final", Size: 3},
		{Name: "Silver Final", Size: 3},
	}

	got, err := ttp.DivisionCuts(ranked, cuts)
	if err != nil {
		t.Fatalf("DivisionCuts: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("expected 2 divisions, got %d", len(got))
	}
	if got[0].Name != "Gold Final" || len(got[0].TeamIDs) != 3 || got[0].TeamIDs[0] != "t1" {
		t.Fatalf("unexpected first division: %+v", got[0])
	}
	if got[1].Name != "Silver Final" || len(got[1].TeamIDs) != 3 || got[1].TeamIDs[0] != "t4" {
		t.Fatalf("unexpected second division: %+v", got[1])
	}
}

func TestDivisionCutsAddsImplicitFinalForRemainder(t *testing.T) {
	ttp := loadTimedHeatsReseeding(t)

	ranked := []string{"t1", "t2", "t3", "t4", "t5"}
	cuts := []Cut{
		{Name: "Gold Final", Size: 3},
	}

	got, err := ttp.DivisionCuts(ranked, cuts)
	if err != nil {
		t.Fatalf("DivisionCuts: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("expected 2 divisions (including implicit), got %d", len(got))
	}
	if got[1].Name != "Final" || len(got[1].TeamIDs) != 2 {
		t.Fatalf("unexpected implicit division: %+v", got[1])
	}
}

func TestDivisionCutsTruncatesOverflow(t *testing.T) {
	ttp := loadTimedHeatsReseeding(t)

	ranked := []string{"t1", "t2", "t3"}
	cuts := []Cut{
		{Name: "Gold Final", Size: 10},
	}

	got, err := ttp.DivisionCuts(ranked, cuts)
	if err != nil {
		t.Fatalf("DivisionCuts: %v", err)
	}
	if len(got) != 1 || len(got[0].TeamIDs) != 3 {
		t.Fatalf("expected single division truncated to 3 teams, got %+v", got)
	}
}
```

- [ ] **Step 2: Run the tests to verify they fail**

Run: `go test ./internal/plugin/...`
Expected: FAIL — `Cut`/`Division`/`DivisionCuts` undefined.

- [ ] **Step 3: Replace the `division_cuts` stub with the real algorithm**

In `internal/plugin/bundled/timed-heats-reseeding.lua`, replace:

```lua
local function division_cuts(ranked_teams, cuts)
  -- Replaced with the real implementation in the next task.
  return {}
end
```

with:

```lua
local function division_cuts(ranked_teams, cuts)
  local divisions = {}
  local idx = 1
  local total = #ranked_teams
  local used_names = {}

  for _, cut in ipairs(cuts) do
    if idx > total then
      break
    end
    local size = cut.size
    local remaining = total - idx + 1
    if size > remaining then
      size = remaining
    end
    local team_ids = {}
    for _ = 1, size do
      table.insert(team_ids, ranked_teams[idx])
      idx = idx + 1
    end
    table.insert(divisions, {name = cut.name, team_ids = team_ids})
    used_names[cut.name] = true
  end

  if idx <= total then
    local name = "Final"
    local suffix = 1
    while used_names[name] do
      suffix = suffix + 1
      name = "Final " .. suffix
    end
    local team_ids = {}
    for i = idx, total do
      table.insert(team_ids, ranked_teams[i])
    end
    table.insert(divisions, {name = name, team_ids = team_ids})
  end

  return divisions
end
```

- [ ] **Step 4: Implement the Go side**

Create `internal/plugin/division.go`:

```go
package plugin

import (
	"fmt"

	lua "github.com/yuin/gopher-lua"
)

type Cut struct {
	Name string `json:"name"`
	Size int    `json:"size"`
}

type Division struct {
	Name    string   `json:"name"`
	TeamIDs []string `json:"team_ids"`
}

// DivisionCuts calls the plugin's division_cuts(ranked_teams, cuts)
// function. rankedTeamIDs must already be in final rank order (fastest
// first) — this method does not sort.
func (t *TournamentTypePlugin) DivisionCuts(rankedTeamIDs []string, cuts []Cut) ([]Division, error) {
	t.mu.Lock()
	defer t.mu.Unlock()

	L := t.state

	rankedTbl := L.NewTable()
	for _, id := range rankedTeamIDs {
		rankedTbl.Append(lua.LString(id))
	}

	cutsTbl := L.NewTable()
	for _, c := range cuts {
		cutTbl := L.NewTable()
		cutTbl.RawSetString("name", lua.LString(c.Name))
		cutTbl.RawSetString("size", lua.LNumber(c.Size))
		cutsTbl.Append(cutTbl)
	}

	if err := L.CallByParam(lua.P{
		Fn:      t.divisionCutsFn,
		NRet:    1,
		Protect: true,
	}, rankedTbl, cutsTbl); err != nil {
		return nil, fmt.Errorf("division_cuts: %w", err)
	}

	ret := L.Get(-1)
	L.Pop(1)

	retTbl, ok := ret.(*lua.LTable)
	if !ok {
		return nil, fmt.Errorf("division_cuts must return a table")
	}

	var divisions []Division
	n := retTbl.Len()
	for i := 1; i <= n; i++ {
		divTbl, ok := retTbl.RawGetInt(i).(*lua.LTable)
		if !ok {
			return nil, fmt.Errorf("division_cuts result entry %d is not a table", i)
		}
		name, _ := divTbl.RawGetString("name").(lua.LString)
		var teamIDs []string
		if teamIDsTbl, ok := divTbl.RawGetString("team_ids").(*lua.LTable); ok {
			m := teamIDsTbl.Len()
			for j := 1; j <= m; j++ {
				if idStr, ok := teamIDsTbl.RawGetInt(j).(lua.LString); ok {
					teamIDs = append(teamIDs, string(idStr))
				}
			}
		}
		divisions = append(divisions, Division{Name: string(name), TeamIDs: teamIDs})
	}

	return divisions, nil
}
```

- [ ] **Step 5: Run the tests to verify they pass**

Run: `go test ./internal/plugin/...`
Expected: PASS

- [ ] **Step 6: Commit**

```bash
git add internal/plugin
git commit -m "feat: implement division_cuts algorithm"
```

---

### Task 5: `dragonboat` sport plugin

**Files:**
- Create: `internal/plugin/bundled/dragonboat.lua`
- Create: `internal/plugin/dragonboat_test.go`

**Interfaces:**
- Consumes: `Load`, `findSport` (Task 2).

- [ ] **Step 1: Write the failing test**

Create `internal/plugin/dragonboat_test.go`:

```go
package plugin

import "testing"

func TestBundledDragonboatPluginRegisters(t *testing.T) {
	e, err := Load("bundled")
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	t.Cleanup(e.Close)

	found := findSport(e, "dragonboat")
	if found == nil {
		t.Fatalf("expected dragonboat sport plugin to register")
	}
	if found.DisplayName != "Dragonboat" {
		t.Fatalf("expected display name Dragonboat, got %s", found.DisplayName)
	}
	if len(found.CompatibleTournamentTypes) != 1 || found.CompatibleTournamentTypes[0] != "timed-heats-reseeding" {
		t.Fatalf("unexpected compatible tournament types: %v", found.CompatibleTournamentTypes)
	}
	if len(found.RosterFields) != 1 || found.RosterFields[0].Key != "boat_class" {
		t.Fatalf("unexpected roster fields: %v", found.RosterFields)
	}
}
```

- [ ] **Step 2: Run the test to verify it fails**

Run: `go test ./internal/plugin/...`
Expected: FAIL — `dragonboat` plugin file doesn't exist yet, so it never registers.

- [ ] **Step 3: Write the plugin**

Create `internal/plugin/bundled/dragonboat.lua`:

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

- [ ] **Step 4: Run the test to verify it passes**

Run: `go test ./internal/plugin/...`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add internal/plugin
git commit -m "feat: add dragonboat sport plugin"
```

---

### Task 6: Embed the bundled plugins into the binary

**Files:**
- Modify: `internal/plugin/engine.go`
- Modify: `internal/plugin/tournamenttype_test.go` (simplify `loadTimedHeatsReseeding`)
- Modify: `internal/plugin/dragonboat_test.go`

**Interfaces:**
- `Load(externalDir string) (*Engine, error)` signature is unchanged — its *behavior* changes: it now always loads the two bundled plugins (compiled into the binary) in addition to whatever is in `externalDir`. An external file with the same `id` as a bundled one silently replaces it (last-loaded-wins on the map, matching the Foundation plan's i18n external-override behavior).

- [ ] **Step 1: Embed the bundled plugins and load them first**

Replace the top of `internal/plugin/engine.go` (imports and the `Load` function) with:

```go
package plugin

import (
	"embed"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	lua "github.com/yuin/gopher-lua"
)

//go:embed bundled/*.lua
var bundledPlugins embed.FS

type Engine struct {
	sports          map[string]*SportPlugin
	tournamentTypes map[string]*TournamentTypePlugin
}

// Load registers the two plugins built into the binary (bundled/*.lua,
// embedded at compile time), then scans externalDir for additional or
// overriding *.lua files. A missing externalDir is not an error. A
// malformed or invalid external file is logged and skipped; a malformed
// bundled file is a hard error (it shipped broken, which is our bug, not
// a plugin author's).
func Load(externalDir string) (*Engine, error) {
	e := &Engine{
		sports:          make(map[string]*SportPlugin),
		tournamentTypes: make(map[string]*TournamentTypePlugin),
	}

	bundledEntries, err := bundledPlugins.ReadDir("bundled")
	if err != nil {
		return nil, fmt.Errorf("read bundled plugins: %w", err)
	}
	for _, entry := range bundledEntries {
		source, err := bundledPlugins.ReadFile("bundled/" + entry.Name())
		if err != nil {
			return nil, fmt.Errorf("read bundled plugin %s: %w", entry.Name(), err)
		}
		if err := e.loadSource(entry.Name(), source); err != nil {
			return nil, fmt.Errorf("bundled plugin %s: %w", entry.Name(), err)
		}
	}

	entries, err := os.ReadDir(externalDir)
	if os.IsNotExist(err) {
		return e, nil
	}
	if err != nil {
		return nil, err
	}

	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".lua") {
			continue
		}
		source, err := os.ReadFile(filepath.Join(externalDir, entry.Name()))
		if err != nil {
			fmt.Fprintf(os.Stderr, "plugin: skipping %s: %v\n", entry.Name(), err)
			continue
		}
		if err := e.loadSource(entry.Name(), source); err != nil {
			fmt.Fprintf(os.Stderr, "plugin: skipping %s: %v\n", entry.Name(), err)
		}
	}

	return e, nil
}
```

The rest of `internal/plugin/engine.go` (`loadSource`, `Sports`, `TournamentTypes`, `Close`) is unchanged — leave it exactly as Task 2 left it.

- [ ] **Step 2: Simplify the test helper now that bundled plugins always load**

In `internal/plugin/tournamenttype_test.go`, replace `loadTimedHeatsReseeding` with:

```go
func loadTimedHeatsReseeding(t *testing.T) *TournamentTypePlugin {
	t.Helper()
	e, err := Load(t.TempDir())
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	t.Cleanup(e.Close)

	ttp := findTournamentType(e, "timed-heats-reseeding")
	if ttp == nil {
		t.Fatalf("timed-heats-reseeding plugin not found")
	}
	return ttp
}
```

- [ ] **Step 3: Simplify the dragonboat test the same way**

In `internal/plugin/dragonboat_test.go`, change:

```go
	e, err := Load("bundled")
```

to:

```go
	e, err := Load(t.TempDir())
```

- [ ] **Step 4: Write the failing test for external-override behavior**

Add to `internal/plugin/engine_test.go`:

```go
func TestExternalPluginOverridesBundledOnSameID(t *testing.T) {
	dir := t.TempDir()
	writeTestPlugin(t, dir, "dragonboat.lua", `
return {
  id = "dragonboat",
  display_name = "Overridden Dragonboat",
  compatible_tournament_types = {"timed-heats-reseeding"},
}
`)

	e, err := Load(dir)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	defer e.Close()

	sp := findSport(e, "dragonboat")
	if sp == nil {
		t.Fatalf("expected dragonboat to still be registered")
	}
	if sp.DisplayName != "Overridden Dragonboat" {
		t.Fatalf("expected external plugin to override bundled one, got display name %q", sp.DisplayName)
	}
}
```

- [ ] **Step 5: Run the full package test suite**

Run: `go test ./internal/plugin/... -v`
Expected: PASS — all tests from Tasks 2-5 plus the new override test.

- [ ] **Step 6: Commit**

```bash
git add internal/plugin
git commit -m "feat: embed bundled plugins, merge with external plugins/ dir"
```

---

### Task 7: `GET /api/plugins` endpoint

**Files:**
- Create: `internal/server/handlers_plugins.go`
- Create: `internal/server/plugins_test.go`
- Modify: `internal/server/server.go` (add `plugins` field, new `New()` signature)
- Modify: `internal/server/server_test.go` (`newTestServer` constructs a plugin engine)
- Modify: `cmd/tournamentstudio/main.go` (load plugins, pass to `server.New`)

**Interfaces:**
- Consumes: `plugin.Load`, `(*Engine).Sports()`, `(*Engine).TournamentTypes()` (Tasks 2, 6).
- Produces: `server.New(s *store.Store, plugins *plugin.Engine) *Server` (breaking change from the Foundation plan's `New(s *store.Store)` — same kind of intentional evolution as that plan's own Task 4).

- [ ] **Step 1: Update the server constructor and routes**

Modify `internal/server/server.go` — add the field, thread it through `New`, and add the route. The `Server` struct and `New` function become:

```go
type Server struct {
	mux         *http.ServeMux
	users       *auth.Repo
	sessions    *auth.SessionRepo
	tournaments *tournament.Repo
	teams       *team.Repo
	plugins     *plugin.Engine
}

func New(s *store.Store, plugins *plugin.Engine) *Server {
	srv := &Server{
		mux:         http.NewServeMux(),
		users:       auth.NewRepo(s),
		sessions:    auth.NewSessionRepo(s),
		tournaments: tournament.NewRepo(s),
		teams:       team.NewRepo(s),
		plugins:     plugins,
	}
	srv.routes()
	return srv
}
```

Add the import `"tournamentstudio/internal/plugin"`.

Add this line inside the existing `routes()` function body, alongside the other `authenticated`-wrapped routes:

```go
	s.mux.Handle("GET /api/plugins", authenticated(http.HandlerFunc(s.handlePlugins)))
```

- [ ] **Step 2: Write the handler**

Create `internal/server/handlers_plugins.go`:

```go
package server

import (
	"encoding/json"
	"net/http"
)

type pluginSportResponse struct {
	ID                        string   `json:"id"`
	DisplayName               string   `json:"display_name"`
	CompatibleTournamentTypes []string `json:"compatible_tournament_types"`
}

type pluginTournamentTypeResponse struct {
	ID               string   `json:"id"`
	CompatibleSports []string `json:"compatible_sports"`
}

func (s *Server) handlePlugins(w http.ResponseWriter, r *http.Request) {
	sports := make([]pluginSportResponse, 0)
	for _, sp := range s.plugins.Sports() {
		sports = append(sports, pluginSportResponse{
			ID:                        sp.ID,
			DisplayName:               sp.DisplayName,
			CompatibleTournamentTypes: sp.CompatibleTournamentTypes,
		})
	}

	tournamentTypes := make([]pluginTournamentTypeResponse, 0)
	for _, ttp := range s.plugins.TournamentTypes() {
		tournamentTypes = append(tournamentTypes, pluginTournamentTypeResponse{
			ID:               ttp.ID,
			CompatibleSports: ttp.CompatibleSports,
		})
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]any{
		"sports":           sports,
		"tournament_types": tournamentTypes,
	})
}
```

- [ ] **Step 3: Update the shared test server helper**

Modify `internal/server/server_test.go` — add the import `"tournamentstudio/internal/plugin"`, and change `newTestServer` to:

```go
func newTestServer(t *testing.T) *Server {
	t.Helper()
	s, err := store.Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	t.Cleanup(func() { s.DB.Close() })

	engine, err := plugin.Load(t.TempDir())
	if err != nil {
		t.Fatalf("load plugins: %v", err)
	}
	t.Cleanup(engine.Close)

	return New(s, engine)
}
```

- [ ] **Step 4: Write the failing test**

Create `internal/server/plugins_test.go`:

```go
package server

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"tournamentstudio/internal/auth"
)

func TestGetPluginsListsBundledPlugins(t *testing.T) {
	s := newTestServer(t)
	token := loginAs(t, s, "organizer1", "pw", auth.RoleOrganizer)

	req := httptest.NewRequest(http.MethodGet, "/api/plugins", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	rec := httptest.NewRecorder()
	s.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}

	var resp struct {
		Sports []struct {
			ID string `json:"id"`
		} `json:"sports"`
		TournamentTypes []struct {
			ID string `json:"id"`
		} `json:"tournament_types"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}

	foundSport := false
	for _, sp := range resp.Sports {
		if sp.ID == "dragonboat" {
			foundSport = true
		}
	}
	if !foundSport {
		t.Fatalf("expected dragonboat in sports list, got %v", resp.Sports)
	}

	foundType := false
	for _, tt := range resp.TournamentTypes {
		if tt.ID == "timed-heats-reseeding" {
			foundType = true
		}
	}
	if !foundType {
		t.Fatalf("expected timed-heats-reseeding in tournament_types list, got %v", resp.TournamentTypes)
	}
}
```

- [ ] **Step 5: Run the tests to verify they pass**

Run: `go test ./internal/server/...`
Expected: PASS

- [ ] **Step 6: Wire the plugin engine into `main.go`**

Modify `cmd/tournamentstudio/main.go`: add the import `"tournamentstudio/internal/plugin"`. After the existing `bootstrapAdmin` call and before constructing the server, add:

```go
	pluginsDir := os.Getenv("TOURNAMENTSTUDIO_PLUGINS")
	if pluginsDir == "" {
		pluginsDir = "plugins"
	}
	engine, err := plugin.Load(pluginsDir)
	if err != nil {
		log.Fatalf("load plugins: %v", err)
	}
	defer engine.Close()
```

Then change the server construction line from `server.New(st)` to `server.New(st, engine)`. Leave `bootstrapAdmin` and every other part of `main.go` exactly as they are.

Run: `go build ./...`
Expected: builds with no errors.

- [ ] **Step 7: Commit**

```bash
git add internal/server cmd/tournamentstudio/main.go
git commit -m "feat: add GET /api/plugins, wire plugin engine into the server"
```

---

### Task 8: `PrePhaseRound` + `Group` domain and the create-round endpoint

**Files:**
- Create: `internal/round/model.go`
- Create: `internal/round/repo.go`
- Create: `internal/round/repo_test.go`
- Create: `internal/store/migrations/0006_pre_phase_rounds.sql`
- Create: `internal/store/migrations/0007_groups.sql`
- Create: `internal/server/handlers_round.go`
- Create: `internal/server/round_test.go`
- Modify: `internal/server/server.go` (add `rounds` field, wire the create-round route)

**Interfaces:**
- Consumes: `store.Store` (Foundation plan), `(*Server).requireRole` (Foundation plan).
- Produces: `round.Status` (`StatusOpen`, `StatusClosed`), `round.PrePhaseRound{ID, TournamentID int64, RoundNumber int, Status Status}`, `round.Group{ID, RoundID int64, TeamIDs []string}`, `round.NewRepo(s *store.Store) *Repo`, `(*Repo).CreateRound(tournamentID int64, roundNumber int, groups [][]string) (*PrePhaseRound, []Group, error)`, `(*Repo).GetRound(id int64) (*PrePhaseRound, error)`, `(*Repo).ListGroups(roundID int64) ([]Group, error)`, `(*Repo).SetStatus(roundID int64, status Status) error`, `round.ErrNotFound`.

- [ ] **Step 1: Add the migrations**

Create `internal/store/migrations/0006_pre_phase_rounds.sql`:

```sql
CREATE TABLE pre_phase_rounds (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    tournament_id INTEGER NOT NULL REFERENCES tournaments(id),
    round_number INTEGER NOT NULL,
    status TEXT NOT NULL DEFAULT 'open'
);
```

Create `internal/store/migrations/0007_groups.sql`:

```sql
CREATE TABLE groups (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    round_id INTEGER NOT NULL REFERENCES pre_phase_rounds(id),
    team_ids TEXT NOT NULL
);
```

- [ ] **Step 2: Write the failing repo test**

Create `internal/round/repo_test.go`:

```go
package round

import (
	"path/filepath"
	"testing"

	"tournamentstudio/internal/store"
)

func newTestStore(t *testing.T) *store.Store {
	t.Helper()
	s, err := store.Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	t.Cleanup(func() { s.DB.Close() })
	return s
}

func seedTournament(t *testing.T, s *store.Store) int64 {
	t.Helper()
	res, err := s.DB.Exec(`INSERT INTO tournaments (name, sport_plugin_id, tournament_type_id, language, status) VALUES ('Test', 'dragonboat', 'timed-heats-reseeding', 'en', 'draft')`)
	if err != nil {
		t.Fatalf("seed tournament: %v", err)
	}
	id, err := res.LastInsertId()
	if err != nil {
		t.Fatalf("seed tournament id: %v", err)
	}
	return id
}

func TestCreateRoundWithGroups(t *testing.T) {
	s := newTestStore(t)
	tournamentID := seedTournament(t, s)
	repo := NewRepo(s)

	pr, groups, err := repo.CreateRound(tournamentID, 1, [][]string{{"t1", "t2"}, {"t3", "t4"}})
	if err != nil {
		t.Fatalf("CreateRound: %v", err)
	}
	if pr.ID == 0 {
		t.Fatalf("expected non-zero round ID")
	}
	if pr.Status != StatusOpen {
		t.Fatalf("expected status open, got %s", pr.Status)
	}
	if len(groups) != 2 {
		t.Fatalf("expected 2 groups, got %d", len(groups))
	}

	fetched, err := repo.GetRound(pr.ID)
	if err != nil {
		t.Fatalf("GetRound: %v", err)
	}
	if fetched.RoundNumber != 1 {
		t.Fatalf("expected round number 1, got %d", fetched.RoundNumber)
	}

	listed, err := repo.ListGroups(pr.ID)
	if err != nil {
		t.Fatalf("ListGroups: %v", err)
	}
	if len(listed) != 2 || listed[0].TeamIDs[0] != "t1" {
		t.Fatalf("unexpected groups: %+v", listed)
	}
}

func TestSetStatus(t *testing.T) {
	s := newTestStore(t)
	tournamentID := seedTournament(t, s)
	repo := NewRepo(s)

	pr, _, err := repo.CreateRound(tournamentID, 1, [][]string{{"t1"}})
	if err != nil {
		t.Fatalf("CreateRound: %v", err)
	}

	if err := repo.SetStatus(pr.ID, StatusClosed); err != nil {
		t.Fatalf("SetStatus: %v", err)
	}

	fetched, err := repo.GetRound(pr.ID)
	if err != nil {
		t.Fatalf("GetRound: %v", err)
	}
	if fetched.Status != StatusClosed {
		t.Fatalf("expected status closed, got %s", fetched.Status)
	}
}

func TestGetRoundNotFound(t *testing.T) {
	repo := NewRepo(newTestStore(t))
	if _, err := repo.GetRound(999); err != ErrNotFound {
		t.Fatalf("expected ErrNotFound, got %v", err)
	}
}
```

- [ ] **Step 3: Run the test to verify it fails**

Run: `go test ./internal/round/...`
Expected: FAIL — package does not exist yet.

- [ ] **Step 4: Implement the model and repository**

Create `internal/round/model.go`:

```go
package round

type Status string

const (
	StatusOpen   Status = "open"
	StatusClosed Status = "closed"
)

type PrePhaseRound struct {
	ID           int64
	TournamentID int64
	RoundNumber  int
	Status       Status
}

type Group struct {
	ID      int64
	RoundID int64
	TeamIDs []string
}
```

Create `internal/round/repo.go`:

```go
package round

import (
	"database/sql"
	"encoding/json"
	"errors"

	"tournamentstudio/internal/store"
)

var ErrNotFound = errors.New("round not found")

type Repo struct {
	db *sql.DB
}

func NewRepo(s *store.Store) *Repo {
	return &Repo{db: s.DB}
}

func (r *Repo) CreateRound(tournamentID int64, roundNumber int, groups [][]string) (*PrePhaseRound, []Group, error) {
	tx, err := r.db.Begin()
	if err != nil {
		return nil, nil, err
	}

	res, err := tx.Exec(
		`INSERT INTO pre_phase_rounds (tournament_id, round_number, status) VALUES (?, ?, ?)`,
		tournamentID, roundNumber, string(StatusOpen),
	)
	if err != nil {
		tx.Rollback()
		return nil, nil, err
	}
	roundID, err := res.LastInsertId()
	if err != nil {
		tx.Rollback()
		return nil, nil, err
	}

	createdGroups := make([]Group, 0, len(groups))
	for _, teamIDs := range groups {
		teamIDsJSON, err := json.Marshal(teamIDs)
		if err != nil {
			tx.Rollback()
			return nil, nil, err
		}
		gres, err := tx.Exec(`INSERT INTO groups (round_id, team_ids) VALUES (?, ?)`, roundID, string(teamIDsJSON))
		if err != nil {
			tx.Rollback()
			return nil, nil, err
		}
		groupID, err := gres.LastInsertId()
		if err != nil {
			tx.Rollback()
			return nil, nil, err
		}
		createdGroups = append(createdGroups, Group{ID: groupID, RoundID: roundID, TeamIDs: teamIDs})
	}

	if err := tx.Commit(); err != nil {
		return nil, nil, err
	}

	return &PrePhaseRound{ID: roundID, TournamentID: tournamentID, RoundNumber: roundNumber, Status: StatusOpen}, createdGroups, nil
}

func (r *Repo) GetRound(id int64) (*PrePhaseRound, error) {
	row := r.db.QueryRow(`SELECT id, tournament_id, round_number, status FROM pre_phase_rounds WHERE id = ?`, id)
	var pr PrePhaseRound
	var status string
	if err := row.Scan(&pr.ID, &pr.TournamentID, &pr.RoundNumber, &status); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	pr.Status = Status(status)
	return &pr, nil
}

func (r *Repo) ListGroups(roundID int64) ([]Group, error) {
	rows, err := r.db.Query(`SELECT id, round_id, team_ids FROM groups WHERE round_id = ? ORDER BY id`, roundID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var groups []Group
	for rows.Next() {
		var g Group
		var teamIDsJSON string
		if err := rows.Scan(&g.ID, &g.RoundID, &teamIDsJSON); err != nil {
			return nil, err
		}
		if err := json.Unmarshal([]byte(teamIDsJSON), &g.TeamIDs); err != nil {
			return nil, err
		}
		groups = append(groups, g)
	}
	return groups, rows.Err()
}

func (r *Repo) SetStatus(roundID int64, status Status) error {
	_, err := r.db.Exec(`UPDATE pre_phase_rounds SET status = ? WHERE id = ?`, string(status), roundID)
	return err
}
```

- [ ] **Step 5: Run the test to verify it passes**

Run: `go test ./internal/round/...`
Expected: PASS

- [ ] **Step 6: Wire the server to the round repo**

Modify `internal/server/server.go` — add the field, construct it, register the route:

```go
type Server struct {
	mux         *http.ServeMux
	users       *auth.Repo
	sessions    *auth.SessionRepo
	tournaments *tournament.Repo
	teams       *team.Repo
	plugins     *plugin.Engine
	rounds      *round.Repo
}

func New(s *store.Store, plugins *plugin.Engine) *Server {
	srv := &Server{
		mux:         http.NewServeMux(),
		users:       auth.NewRepo(s),
		sessions:    auth.NewSessionRepo(s),
		tournaments: tournament.NewRepo(s),
		teams:       team.NewRepo(s),
		plugins:     plugins,
		rounds:      round.NewRepo(s),
	}
	srv.routes()
	return srv
}
```

Add the import `"tournamentstudio/internal/round"`.

Add this line inside `routes()`, alongside the other `organizerOnly`-wrapped routes:

```go
	s.mux.Handle("POST /api/tournaments/{id}/rounds", organizerOnly(http.HandlerFunc(s.handleCreateRound)))
```

- [ ] **Step 7: Write the handler**

Create `internal/server/handlers_round.go`:

```go
package server

import (
	"encoding/json"
	"net/http"
	"strconv"

	"tournamentstudio/internal/round"
)

type createRoundRequest struct {
	RoundNumber int        `json:"round_number"`
	Groups      [][]string `json:"groups"`
}

type groupResponse struct {
	ID      int64    `json:"id"`
	TeamIDs []string `json:"team_ids"`
}

type roundResponse struct {
	ID          int64           `json:"id"`
	RoundNumber int             `json:"round_number"`
	Status      string          `json:"status"`
	Groups      []groupResponse `json:"groups"`
}

func roundToResponse(pr *round.PrePhaseRound, groups []round.Group) roundResponse {
	gr := make([]groupResponse, 0, len(groups))
	for _, g := range groups {
		gr = append(gr, groupResponse{ID: g.ID, TeamIDs: g.TeamIDs})
	}
	return roundResponse{
		ID:          pr.ID,
		RoundNumber: pr.RoundNumber,
		Status:      string(pr.Status),
		Groups:      gr,
	}
}

func (s *Server) handleCreateRound(w http.ResponseWriter, r *http.Request) {
	tournamentID, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		http.Error(w, "invalid tournament id", http.StatusBadRequest)
		return
	}

	var req createRoundRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}
	if req.RoundNumber < 1 {
		http.Error(w, "round_number must be at least 1", http.StatusBadRequest)
		return
	}
	if len(req.Groups) == 0 {
		http.Error(w, "at least one group is required", http.StatusBadRequest)
		return
	}

	pr, groups, err := s.rounds.CreateRound(tournamentID, req.RoundNumber, req.Groups)
	if err != nil {
		http.Error(w, "could not create round", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(roundToResponse(pr, groups))
}
```

- [ ] **Step 8: Write the failing HTTP test**

Create `internal/server/round_test.go`:

```go
package server

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"tournamentstudio/internal/auth"
)

func TestCreateRound(t *testing.T) {
	s := newTestServer(t)
	token := loginAs(t, s, "organizer1", "pw", auth.RoleOrganizer)
	tournamentID := createTestTournament(t, s, token)

	body, _ := json.Marshal(map[string]any{
		"round_number": 1,
		"groups":       [][]string{{"t1", "t2"}, {"t3", "t4"}},
	})
	req := httptest.NewRequest(http.MethodPost, fmt.Sprintf("/api/tournaments/%d/rounds", tournamentID), bytes.NewReader(body))
	req.Header.Set("Authorization", "Bearer "+token)
	rec := httptest.NewRecorder()
	s.ServeHTTP(rec, req)

	if rec.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", rec.Code, rec.Body.String())
	}

	var resp struct {
		ID     int64  `json:"id"`
		Status string `json:"status"`
		Groups []struct {
			TeamIDs []string `json:"team_ids"`
		} `json:"groups"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if resp.Status != "open" {
		t.Fatalf("expected status open, got %s", resp.Status)
	}
	if len(resp.Groups) != 2 {
		t.Fatalf("expected 2 groups, got %d", len(resp.Groups))
	}
}

func TestCreateRoundForbiddenForSpectator(t *testing.T) {
	s := newTestServer(t)
	organizerToken := loginAs(t, s, "organizer1", "pw", auth.RoleOrganizer)
	tournamentID := createTestTournament(t, s, organizerToken)
	spectatorToken := loginAs(t, s, "spectator1", "pw", auth.RoleSpectator)

	body, _ := json.Marshal(map[string]any{"round_number": 1, "groups": [][]string{{"t1"}}})
	req := httptest.NewRequest(http.MethodPost, fmt.Sprintf("/api/tournaments/%d/rounds", tournamentID), bytes.NewReader(body))
	req.Header.Set("Authorization", "Bearer "+spectatorToken)
	rec := httptest.NewRecorder()
	s.ServeHTTP(rec, req)

	if rec.Code != http.StatusForbidden {
		t.Fatalf("expected 403, got %d", rec.Code)
	}
}
```

- [ ] **Step 9: Run the tests to verify they pass**

Run: `go test ./internal/round/... ./internal/server/...`
Expected: PASS

- [ ] **Step 10: Commit**

```bash
git add internal/round internal/server internal/store/migrations/0006_pre_phase_rounds.sql internal/store/migrations/0007_groups.sql
git commit -m "feat: add PrePhaseRound/Group domain and create-round endpoint"
```

---

### Task 9: Round results and the submit-results endpoint

**Files:**
- Create: `internal/store/migrations/0008_round_results.sql`
- Modify: `internal/round/model.go` (add `Result`)
- Modify: `internal/round/repo.go` (add `SubmitResult`, `ListResults`)
- Modify: `internal/round/repo_test.go` (add new test functions to the existing file)
- Create: `internal/server/handlers_round_results.go`
- Create: `internal/server/round_results_test.go`
- Modify: `internal/server/server.go` (register the results route with a new `resultsWriter` role wrapper)

**Interfaces:**
- Consumes: `round.Repo`, `round.ErrNotFound` (Task 8).
- Produces: `round.Result{TeamID string, TimeSeconds *float64, Status string}`, `(*Repo).SubmitResult(roundID int64, res Result) error`, `(*Repo).ListResults(roundID int64) ([]Result, error)`.

- [ ] **Step 1: Add the migration**

Create `internal/store/migrations/0008_round_results.sql`:

```sql
CREATE TABLE round_results (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    round_id INTEGER NOT NULL REFERENCES pre_phase_rounds(id),
    team_id TEXT NOT NULL,
    time_seconds REAL,
    status TEXT,
    UNIQUE(round_id, team_id)
);
```

- [ ] **Step 2: Write the failing repo tests**

Add to `internal/round/repo_test.go`:

```go
func TestSubmitAndListResults(t *testing.T) {
	s := newTestStore(t)
	tournamentID := seedTournament(t, s)
	repo := NewRepo(s)

	pr, _, err := repo.CreateRound(tournamentID, 1, [][]string{{"t1", "t2"}})
	if err != nil {
		t.Fatalf("CreateRound: %v", err)
	}

	timeVal := 124.11
	if err := repo.SubmitResult(pr.ID, Result{TeamID: "t1", TimeSeconds: &timeVal}); err != nil {
		t.Fatalf("SubmitResult t1: %v", err)
	}
	if err := repo.SubmitResult(pr.ID, Result{TeamID: "t2", Status: "DNF"}); err != nil {
		t.Fatalf("SubmitResult t2: %v", err)
	}

	results, err := repo.ListResults(pr.ID)
	if err != nil {
		t.Fatalf("ListResults: %v", err)
	}
	if len(results) != 2 {
		t.Fatalf("expected 2 results, got %d", len(results))
	}

	byTeam := make(map[string]Result, len(results))
	for _, res := range results {
		byTeam[res.TeamID] = res
	}
	if byTeam["t1"].TimeSeconds == nil || *byTeam["t1"].TimeSeconds != 124.11 {
		t.Fatalf("unexpected t1 result: %+v", byTeam["t1"])
	}
	if byTeam["t2"].Status != "DNF" {
		t.Fatalf("unexpected t2 result: %+v", byTeam["t2"])
	}
}

func TestSubmitResultUpsertsOnResubmission(t *testing.T) {
	s := newTestStore(t)
	tournamentID := seedTournament(t, s)
	repo := NewRepo(s)

	pr, _, err := repo.CreateRound(tournamentID, 1, [][]string{{"t1"}})
	if err != nil {
		t.Fatalf("CreateRound: %v", err)
	}

	firstTime := 130.0
	if err := repo.SubmitResult(pr.ID, Result{TeamID: "t1", TimeSeconds: &firstTime}); err != nil {
		t.Fatalf("SubmitResult first: %v", err)
	}
	correctedTime := 124.11
	if err := repo.SubmitResult(pr.ID, Result{TeamID: "t1", TimeSeconds: &correctedTime}); err != nil {
		t.Fatalf("SubmitResult correction: %v", err)
	}

	results, err := repo.ListResults(pr.ID)
	if err != nil {
		t.Fatalf("ListResults: %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("expected 1 result after correction (not 2), got %d", len(results))
	}
	if *results[0].TimeSeconds != 124.11 {
		t.Fatalf("expected corrected time 124.11, got %v", *results[0].TimeSeconds)
	}
}
```

- [ ] **Step 3: Run the tests to verify they fail**

Run: `go test ./internal/round/...`
Expected: FAIL — `Result`/`SubmitResult`/`ListResults` undefined.

- [ ] **Step 4: Implement**

Add to `internal/round/model.go`:

```go
type Result struct {
	TeamID      string
	TimeSeconds *float64
	Status      string
}
```

Add to `internal/round/repo.go` (needs the additional import `nullIfEmpty` is a local helper, no new package imports required beyond what's already there):

```go
func (r *Repo) SubmitResult(roundID int64, res Result) error {
	_, err := r.db.Exec(
		`INSERT INTO round_results (round_id, team_id, time_seconds, status) VALUES (?, ?, ?, ?)
		 ON CONFLICT(round_id, team_id) DO UPDATE SET time_seconds = excluded.time_seconds, status = excluded.status`,
		roundID, res.TeamID, res.TimeSeconds, nullIfEmpty(res.Status),
	)
	return err
}

func (r *Repo) ListResults(roundID int64) ([]Result, error) {
	rows, err := r.db.Query(`SELECT team_id, time_seconds, status FROM round_results WHERE round_id = ?`, roundID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var results []Result
	for rows.Next() {
		var res Result
		var timeSeconds sql.NullFloat64
		var status sql.NullString
		if err := rows.Scan(&res.TeamID, &timeSeconds, &status); err != nil {
			return nil, err
		}
		if timeSeconds.Valid {
			v := timeSeconds.Float64
			res.TimeSeconds = &v
		}
		if status.Valid {
			res.Status = status.String
		}
		results = append(results, res)
	}
	return results, rows.Err()
}

func nullIfEmpty(s string) any {
	if s == "" {
		return nil
	}
	return s
}
```

- [ ] **Step 5: Run the tests to verify they pass**

Run: `go test ./internal/round/...`
Expected: PASS

- [ ] **Step 6: Register the results route**

Add this line inside `routes()` in `internal/server/server.go`, and declare the new role wrapper alongside the existing `authenticated`/`organizerOnly` locals:

```go
	resultsWriter := s.requireRole(auth.RoleOrganizer, auth.RoleTimeEntry)
	s.mux.Handle("POST /api/tournaments/{id}/rounds/{round_id}/results", resultsWriter(http.HandlerFunc(s.handleSubmitResults)))
```

- [ ] **Step 7: Write the handler**

Create `internal/server/handlers_round_results.go`:

```go
package server

import (
	"encoding/json"
	"net/http"
	"strconv"

	"tournamentstudio/internal/round"
)

type submitResultsRequest map[string]struct {
	TimeSeconds *float64 `json:"time_seconds"`
	Status      string   `json:"status"`
}

func (s *Server) handleSubmitResults(w http.ResponseWriter, r *http.Request) {
	tournamentID, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		http.Error(w, "invalid tournament id", http.StatusBadRequest)
		return
	}
	roundID, err := strconv.ParseInt(r.PathValue("round_id"), 10, 64)
	if err != nil {
		http.Error(w, "invalid round id", http.StatusBadRequest)
		return
	}

	pr, err := s.rounds.GetRound(roundID)
	if err != nil {
		if err == round.ErrNotFound {
			http.Error(w, "round not found", http.StatusNotFound)
			return
		}
		http.Error(w, "could not get round", http.StatusInternalServerError)
		return
	}
	if pr.TournamentID != tournamentID {
		http.Error(w, "round not found", http.StatusNotFound)
		return
	}

	var req submitResultsRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}

	for teamID, entry := range req {
		if entry.TimeSeconds == nil && entry.Status == "" {
			http.Error(w, "each result must have either time_seconds or status", http.StatusBadRequest)
			return
		}
		if err := s.rounds.SubmitResult(roundID, round.Result{
			TeamID:      teamID,
			TimeSeconds: entry.TimeSeconds,
			Status:      entry.Status,
		}); err != nil {
			http.Error(w, "could not save result", http.StatusInternalServerError)
			return
		}
	}

	groups, err := s.rounds.ListGroups(roundID)
	if err != nil {
		http.Error(w, "could not list groups", http.StatusInternalServerError)
		return
	}
	expected := 0
	for _, g := range groups {
		expected += len(g.TeamIDs)
	}

	results, err := s.rounds.ListResults(roundID)
	if err != nil {
		http.Error(w, "could not list results", http.StatusInternalServerError)
		return
	}

	if expected > 0 && len(results) >= expected {
		if err := s.rounds.SetStatus(roundID, round.StatusClosed); err != nil {
			http.Error(w, "could not close round", http.StatusInternalServerError)
			return
		}
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]int{"results_recorded": len(results)})
}
```

- [ ] **Step 8: Write the failing HTTP tests**

Create `internal/server/round_results_test.go`:

```go
package server

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"tournamentstudio/internal/auth"
	"tournamentstudio/internal/round"
)

func createTestRound(t *testing.T, s *Server, token string, tournamentID int64, groups [][]string) int64 {
	t.Helper()
	body, _ := json.Marshal(map[string]any{"round_number": 1, "groups": groups})
	req := httptest.NewRequest(http.MethodPost, fmt.Sprintf("/api/tournaments/%d/rounds", tournamentID), bytes.NewReader(body))
	req.Header.Set("Authorization", "Bearer "+token)
	rec := httptest.NewRecorder()
	s.ServeHTTP(rec, req)
	var created struct{ ID int64 `json:"id"` }
	if err := json.Unmarshal(rec.Body.Bytes(), &created); err != nil {
		t.Fatalf("decode created round: %v", err)
	}
	return created.ID
}

func TestSubmitResultsClosesRoundWhenComplete(t *testing.T) {
	s := newTestServer(t)
	token := loginAs(t, s, "organizer1", "pw", auth.RoleOrganizer)
	tournamentID := createTestTournament(t, s, token)
	roundID := createTestRound(t, s, token, tournamentID, [][]string{{"t1", "t2"}})

	partialBody, _ := json.Marshal(map[string]any{"t1": map[string]any{"time_seconds": 124.5}})
	partialReq := httptest.NewRequest(http.MethodPost, fmt.Sprintf("/api/tournaments/%d/rounds/%d/results", tournamentID, roundID), bytes.NewReader(partialBody))
	partialReq.Header.Set("Authorization", "Bearer "+token)
	partialRec := httptest.NewRecorder()
	s.ServeHTTP(partialRec, partialReq)
	if partialRec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", partialRec.Code, partialRec.Body.String())
	}

	pr, err := s.rounds.GetRound(roundID)
	if err != nil {
		t.Fatalf("GetRound: %v", err)
	}
	if pr.Status != round.StatusOpen {
		t.Fatalf("expected round still open after partial submission, got %s", pr.Status)
	}

	remainingBody, _ := json.Marshal(map[string]any{"t2": map[string]any{"status": "DNF"}})
	remainingReq := httptest.NewRequest(http.MethodPost, fmt.Sprintf("/api/tournaments/%d/rounds/%d/results", tournamentID, roundID), bytes.NewReader(remainingBody))
	remainingReq.Header.Set("Authorization", "Bearer "+token)
	remainingRec := httptest.NewRecorder()
	s.ServeHTTP(remainingRec, remainingReq)
	if remainingRec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", remainingRec.Code, remainingRec.Body.String())
	}

	pr2, err := s.rounds.GetRound(roundID)
	if err != nil {
		t.Fatalf("GetRound: %v", err)
	}
	if pr2.Status != round.StatusClosed {
		t.Fatalf("expected round closed after all results submitted, got %s", pr2.Status)
	}
}

func TestSubmitResultsAllowsTimeEntryRole(t *testing.T) {
	s := newTestServer(t)
	organizerToken := loginAs(t, s, "organizer1", "pw", auth.RoleOrganizer)
	tournamentID := createTestTournament(t, s, organizerToken)
	roundID := createTestRound(t, s, organizerToken, tournamentID, [][]string{{"t1"}})

	timeEntryToken := loginAs(t, s, "timekeeper1", "pw", auth.RoleTimeEntry)
	body, _ := json.Marshal(map[string]any{"t1": map[string]any{"time_seconds": 100.0}})
	req := httptest.NewRequest(http.MethodPost, fmt.Sprintf("/api/tournaments/%d/rounds/%d/results", tournamentID, roundID), bytes.NewReader(body))
	req.Header.Set("Authorization", "Bearer "+timeEntryToken)
	rec := httptest.NewRecorder()
	s.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200 for time_entry role, got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestSubmitResultsForbiddenForSpectator(t *testing.T) {
	s := newTestServer(t)
	organizerToken := loginAs(t, s, "organizer1", "pw", auth.RoleOrganizer)
	tournamentID := createTestTournament(t, s, organizerToken)
	roundID := createTestRound(t, s, organizerToken, tournamentID, [][]string{{"t1"}})

	spectatorToken := loginAs(t, s, "spectator1", "pw", auth.RoleSpectator)
	body, _ := json.Marshal(map[string]any{"t1": map[string]any{"time_seconds": 100.0}})
	req := httptest.NewRequest(http.MethodPost, fmt.Sprintf("/api/tournaments/%d/rounds/%d/results", tournamentID, roundID), bytes.NewReader(body))
	req.Header.Set("Authorization", "Bearer "+spectatorToken)
	rec := httptest.NewRecorder()
	s.ServeHTTP(rec, req)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("expected 403, got %d", rec.Code)
	}
}
```

- [ ] **Step 9: Run the tests to verify they pass**

Run: `go test ./internal/round/... ./internal/server/...`
Expected: PASS

- [ ] **Step 10: Commit**

```bash
git add internal/round internal/server internal/store/migrations/0008_round_results.sql
git commit -m "feat: add round results and submit-results endpoint"
```

---

### Task 10: Next-round endpoint (reseeding orchestration)

**Files:**
- Create: `internal/server/handlers_round_next.go`
- Create: `internal/server/round_next_test.go`
- Modify: `internal/server/server.go` (register the route)

**Interfaces:**
- Consumes: `ranking.Rank`, `ranking.TeamResult` (Task 1); `(*plugin.TournamentTypePlugin).NextRoundGroups` (Task 3); `round.Repo`, `round.Result`, `round.ErrNotFound`, `round.StatusClosed` (Tasks 8, 9); `roundToResponse` (Task 8).

- [ ] **Step 1: Register the route**

Add this line inside `routes()` in `internal/server/server.go`:

```go
	s.mux.Handle("POST /api/tournaments/{id}/rounds/{round_id}/next", organizerOnly(http.HandlerFunc(s.handleNextRound)))
```

- [ ] **Step 2: Write the handler**

Create `internal/server/handlers_round_next.go`:

```go
package server

import (
	"encoding/json"
	"net/http"
	"strconv"

	"tournamentstudio/internal/plugin"
	"tournamentstudio/internal/ranking"
	"tournamentstudio/internal/round"
)

func (s *Server) findTournamentType(id string) *plugin.TournamentTypePlugin {
	for _, ttp := range s.plugins.TournamentTypes() {
		if ttp.ID == id {
			return ttp
		}
	}
	return nil
}

func (s *Server) handleNextRound(w http.ResponseWriter, r *http.Request) {
	tournamentID, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		http.Error(w, "invalid tournament id", http.StatusBadRequest)
		return
	}
	roundID, err := strconv.ParseInt(r.PathValue("round_id"), 10, 64)
	if err != nil {
		http.Error(w, "invalid round id", http.StatusBadRequest)
		return
	}

	pr, err := s.rounds.GetRound(roundID)
	if err != nil {
		if err == round.ErrNotFound {
			http.Error(w, "round not found", http.StatusNotFound)
			return
		}
		http.Error(w, "could not get round", http.StatusInternalServerError)
		return
	}
	if pr.TournamentID != tournamentID {
		http.Error(w, "round not found", http.StatusNotFound)
		return
	}
	if pr.Status != round.StatusClosed {
		http.Error(w, "round must be closed before computing the next round", http.StatusConflict)
		return
	}

	tour, err := s.tournaments.Get(tournamentID)
	if err != nil {
		http.Error(w, "could not get tournament", http.StatusInternalServerError)
		return
	}
	ttp := s.findTournamentType(tour.TournamentTypeID)
	if ttp == nil {
		http.Error(w, "tournament type plugin not found", http.StatusInternalServerError)
		return
	}

	groups, err := s.rounds.ListGroups(roundID)
	if err != nil {
		http.Error(w, "could not list groups", http.StatusInternalServerError)
		return
	}
	results, err := s.rounds.ListResults(roundID)
	if err != nil {
		http.Error(w, "could not list results", http.StatusInternalServerError)
		return
	}
	resultsByTeam := make(map[string]round.Result, len(results))
	for _, res := range results {
		resultsByTeam[res.TeamID] = res
	}

	rankedGroups := make([][]ranking.TeamResult, 0, len(groups))
	for _, g := range groups {
		unranked := make([]ranking.TeamResult, 0, len(g.TeamIDs))
		for _, teamID := range g.TeamIDs {
			res := resultsByTeam[teamID]
			unranked = append(unranked, ranking.TeamResult{
				TeamID:      teamID,
				TimeSeconds: res.TimeSeconds,
				Status:      ranking.Status(res.Status),
			})
		}
		rankedGroups = append(rankedGroups, ranking.Rank(unranked))
	}

	nextGroupTeamIDs, err := ttp.NextRoundGroups(rankedGroups)
	if err != nil {
		http.Error(w, "plugin error computing next round: "+err.Error(), http.StatusInternalServerError)
		return
	}

	nextPR, nextGroups, err := s.rounds.CreateRound(tournamentID, pr.RoundNumber+1, nextGroupTeamIDs)
	if err != nil {
		http.Error(w, "could not create next round", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(roundToResponse(nextPR, nextGroups))
}
```

- [ ] **Step 3: Write the failing tests**

Create `internal/server/round_next_test.go`:

```go
package server

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"tournamentstudio/internal/auth"
)

func TestNextRoundComputesReseededGroups(t *testing.T) {
	s := newTestServer(t)
	token := loginAs(t, s, "organizer1", "pw", auth.RoleOrganizer)
	tournamentID := createTestTournament(t, s, token)
	roundID := createTestRound(t, s, token, tournamentID, [][]string{
		{"A1", "A2", "A3", "A4"},
		{"B1", "B2", "B3", "B4"},
	})

	resultsBody, _ := json.Marshal(map[string]any{
		"A1": map[string]any{"time_seconds": 120.0},
		"A2": map[string]any{"time_seconds": 121.0},
		"A3": map[string]any{"time_seconds": 122.0},
		"A4": map[string]any{"time_seconds": 123.0},
		"B1": map[string]any{"time_seconds": 120.0},
		"B2": map[string]any{"time_seconds": 121.0},
		"B3": map[string]any{"time_seconds": 122.0},
		"B4": map[string]any{"time_seconds": 123.0},
	})
	resultsReq := httptest.NewRequest(http.MethodPost, fmt.Sprintf("/api/tournaments/%d/rounds/%d/results", tournamentID, roundID), bytes.NewReader(resultsBody))
	resultsReq.Header.Set("Authorization", "Bearer "+token)
	resultsRec := httptest.NewRecorder()
	s.ServeHTTP(resultsRec, resultsReq)
	if resultsRec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", resultsRec.Code, resultsRec.Body.String())
	}

	nextReq := httptest.NewRequest(http.MethodPost, fmt.Sprintf("/api/tournaments/%d/rounds/%d/next", tournamentID, roundID), nil)
	nextReq.Header.Set("Authorization", "Bearer "+token)
	nextRec := httptest.NewRecorder()
	s.ServeHTTP(nextRec, nextReq)
	if nextRec.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", nextRec.Code, nextRec.Body.String())
	}

	var next struct {
		RoundNumber int `json:"round_number"`
		Groups      []struct {
			TeamIDs []string `json:"team_ids"`
		} `json:"groups"`
	}
	if err := json.Unmarshal(nextRec.Body.Bytes(), &next); err != nil {
		t.Fatalf("decode next round response: %v", err)
	}
	if next.RoundNumber != 2 {
		t.Fatalf("expected round number 2, got %d", next.RoundNumber)
	}
	if len(next.Groups) != 2 {
		t.Fatalf("expected 2 groups, got %d", len(next.Groups))
	}
}

func TestNextRoundRejectsOpenRound(t *testing.T) {
	s := newTestServer(t)
	token := loginAs(t, s, "organizer1", "pw", auth.RoleOrganizer)
	tournamentID := createTestTournament(t, s, token)
	roundID := createTestRound(t, s, token, tournamentID, [][]string{{"t1", "t2"}})

	nextReq := httptest.NewRequest(http.MethodPost, fmt.Sprintf("/api/tournaments/%d/rounds/%d/next", tournamentID, roundID), nil)
	nextReq.Header.Set("Authorization", "Bearer "+token)
	nextRec := httptest.NewRecorder()
	s.ServeHTTP(nextRec, nextReq)
	if nextRec.Code != http.StatusConflict {
		t.Fatalf("expected 409, got %d", nextRec.Code)
	}
}
```

- [ ] **Step 4: Run the tests to verify they pass**

Run: `go test ./internal/server/...`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add internal/server
git commit -m "feat: add next-round endpoint (reseeding orchestration)"
```

---

### Task 11: Divisions endpoint

**Files:**
- Create: `internal/server/handlers_round_divisions.go`
- Create: `internal/server/round_divisions_test.go`
- Modify: `internal/server/server.go` (register the route)

**Interfaces:**
- Consumes: `ranking.Rank` (Task 1); `plugin.Cut`, `(*plugin.TournamentTypePlugin).DivisionCuts` (Task 4); `(*Server).findTournamentType` (Task 10); `round.Repo`, `round.ErrNotFound`, `round.StatusClosed` (Tasks 8, 9).

- [ ] **Step 1: Register the route**

Add this line inside `routes()` in `internal/server/server.go`:

```go
	s.mux.Handle("POST /api/tournaments/{id}/rounds/{round_id}/divisions", organizerOnly(http.HandlerFunc(s.handleComputeDivisions)))
```

- [ ] **Step 2: Write the handler**

Create `internal/server/handlers_round_divisions.go`:

```go
package server

import (
	"encoding/json"
	"net/http"
	"strconv"

	"tournamentstudio/internal/plugin"
	"tournamentstudio/internal/ranking"
	"tournamentstudio/internal/round"
)

type computeDivisionsRequest struct {
	Cuts []plugin.Cut `json:"cuts"`
}

func (s *Server) handleComputeDivisions(w http.ResponseWriter, r *http.Request) {
	tournamentID, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		http.Error(w, "invalid tournament id", http.StatusBadRequest)
		return
	}
	roundID, err := strconv.ParseInt(r.PathValue("round_id"), 10, 64)
	if err != nil {
		http.Error(w, "invalid round id", http.StatusBadRequest)
		return
	}

	var req computeDivisionsRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}

	pr, err := s.rounds.GetRound(roundID)
	if err != nil {
		if err == round.ErrNotFound {
			http.Error(w, "round not found", http.StatusNotFound)
			return
		}
		http.Error(w, "could not get round", http.StatusInternalServerError)
		return
	}
	if pr.TournamentID != tournamentID {
		http.Error(w, "round not found", http.StatusNotFound)
		return
	}
	if pr.Status != round.StatusClosed {
		http.Error(w, "round must be closed before computing divisions", http.StatusConflict)
		return
	}

	tour, err := s.tournaments.Get(tournamentID)
	if err != nil {
		http.Error(w, "could not get tournament", http.StatusInternalServerError)
		return
	}
	ttp := s.findTournamentType(tour.TournamentTypeID)
	if ttp == nil {
		http.Error(w, "tournament type plugin not found", http.StatusInternalServerError)
		return
	}

	groups, err := s.rounds.ListGroups(roundID)
	if err != nil {
		http.Error(w, "could not list groups", http.StatusInternalServerError)
		return
	}
	results, err := s.rounds.ListResults(roundID)
	if err != nil {
		http.Error(w, "could not list results", http.StatusInternalServerError)
		return
	}
	resultsByTeam := make(map[string]round.Result, len(results))
	for _, res := range results {
		resultsByTeam[res.TeamID] = res
	}

	var allResults []ranking.TeamResult
	for _, g := range groups {
		for _, teamID := range g.TeamIDs {
			res := resultsByTeam[teamID]
			allResults = append(allResults, ranking.TeamResult{
				TeamID:      teamID,
				TimeSeconds: res.TimeSeconds,
				Status:      ranking.Status(res.Status),
			})
		}
	}
	ranked := ranking.Rank(allResults)
	rankedIDs := make([]string, len(ranked))
	for i, res := range ranked {
		rankedIDs[i] = res.TeamID
	}

	divisions, err := ttp.DivisionCuts(rankedIDs, req.Cuts)
	if err != nil {
		http.Error(w, "plugin error computing divisions: "+err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]any{"divisions": divisions})
}
```

- [ ] **Step 3: Write the failing test**

Create `internal/server/round_divisions_test.go`:

```go
package server

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"tournamentstudio/internal/auth"
)

func TestComputeDivisionsSplitsRankedTeams(t *testing.T) {
	s := newTestServer(t)
	token := loginAs(t, s, "organizer1", "pw", auth.RoleOrganizer)
	tournamentID := createTestTournament(t, s, token)
	roundID := createTestRound(t, s, token, tournamentID, [][]string{{"t1", "t2", "t3", "t4"}})

	resultsBody, _ := json.Marshal(map[string]any{
		"t1": map[string]any{"time_seconds": 120.0},
		"t2": map[string]any{"time_seconds": 121.0},
		"t3": map[string]any{"time_seconds": 122.0},
		"t4": map[string]any{"time_seconds": 123.0},
	})
	resultsReq := httptest.NewRequest(http.MethodPost, fmt.Sprintf("/api/tournaments/%d/rounds/%d/results", tournamentID, roundID), bytes.NewReader(resultsBody))
	resultsReq.Header.Set("Authorization", "Bearer "+token)
	resultsRec := httptest.NewRecorder()
	s.ServeHTTP(resultsRec, resultsReq)

	divisionsBody, _ := json.Marshal(map[string]any{
		"cuts": []map[string]any{
			{"name": "Gold Final", "size": 2},
		},
	})
	divisionsReq := httptest.NewRequest(http.MethodPost, fmt.Sprintf("/api/tournaments/%d/rounds/%d/divisions", tournamentID, roundID), bytes.NewReader(divisionsBody))
	divisionsReq.Header.Set("Authorization", "Bearer "+token)
	divisionsRec := httptest.NewRecorder()
	s.ServeHTTP(divisionsRec, divisionsReq)
	if divisionsRec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", divisionsRec.Code, divisionsRec.Body.String())
	}

	var resp struct {
		Divisions []struct {
			Name    string   `json:"name"`
			TeamIDs []string `json:"team_ids"`
		} `json:"divisions"`
	}
	if err := json.Unmarshal(divisionsRec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if len(resp.Divisions) != 2 {
		t.Fatalf("expected 2 divisions (Gold Final + implicit Final), got %d", len(resp.Divisions))
	}
	if resp.Divisions[0].Name != "Gold Final" || len(resp.Divisions[0].TeamIDs) != 2 {
		t.Fatalf("unexpected first division: %+v", resp.Divisions[0])
	}
	if resp.Divisions[0].TeamIDs[0] != "t1" || resp.Divisions[0].TeamIDs[1] != "t2" {
		t.Fatalf("expected Gold Final to be the fastest two teams, got %v", resp.Divisions[0].TeamIDs)
	}
}
```

- [ ] **Step 4: Run the full test suite**

Run: `go test ./...`
Expected: PASS, all packages — this is the last task, confirm everything from Tasks 1-11 still passes together.

- [ ] **Step 5: Commit**

```bash
git add internal/server
git commit -m "feat: add divisions endpoint"
```

---

## Self-Review Notes

- **Spec coverage:** Lua plugin engine with sandboxing and per-plugin isolation (Task 2) · `dragonboat`/`timed-heats-reseeding` bundled plugins embedded and merged with an external dir (Tasks 5, 6) · the cross-swap-halves reseeding algorithm generalized to *K* groups via cyclic tier rotation, golden-tested against the confirmed 2-group example (Task 3) · division cuts with implicit-remainder and overflow handling (Task 4) · `GET /api/plugins` for compatibility discovery (Task 7) · `PrePhaseRound`/`Group`/results persistence and the full create → submit results → next/divisions flow (Tasks 8-11).
- **Deferred to Plan 3, not forgotten:** `Course`/`Heat`, delay offsets, real result-entry UI, WebSocket broadcast, and `Division` as a persisted entity (§7 of the design spec explains why the divisions endpoint doesn't persist anything yet).
- **Type consistency checked:** `team_id` is a string end-to-end across `ranking.TeamResult`, `plugin`'s Lua conversion, `round.Group.TeamIDs`, and `round.Result.TeamID` — no int64 sneaks in anywhere. `ranking.Status` and `round.Result.Status` are deliberately different types (the former a constrained enum used inside the algorithm, the latter a free string column) — the handler layer (`handlers_round_next.go`, `handlers_round_divisions.go`) is the single place that converts between them (`ranking.Status(res.Status)`), so a future new status value only needs a change there and in `ranking.statusOrder`. `plugin.Cut`/`plugin.Division` carry JSON tags from the moment they're defined (Task 4) so Task 11's request/response handling needed no later retrofit.

---

**Plan complete and saved to `docs/superpowers/plans/2026-08-21-tournamentstudio-plugin-engine.md`.** Two execution options:

**1. Subagent-Driven (recommended)** — I dispatch a fresh subagent per task, review between tasks, fast iteration.

**2. Inline Execution** — Execute tasks in this session using executing-plans, batch execution with checkpoints.

**Which approach?**
