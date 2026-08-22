# Scheduling & Live Results Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add the physical scheduling layer (Course/Heat), real per-heat DNF/DSQ/DNS results replacing Plan 2's whole-round result map, persisted Division entities with their own scheduled heats, and a WebSocket live-broadcast layer for result and delay-offset changes.

**Architecture:** A new `internal/schedule` package (mirroring `internal/round`'s shape) owns `Course`, `Heat`, `HeatResult`, and `Division`. Heat-scoped results replace Plan 2's round-scoped results endpoint outright (no dual-write, no compatibility shim). An in-process WebSocket hub (no external pub/sub) broadcasts two event types to tournament-scoped connections. Everything stays backend/API only, tested via HTTP and WebSocket clients — same discipline as Plans 1-2.

**Tech Stack:** Go, `net/http` 1.22+ ServeMux, `modernc.org/sqlite` (pure Go, unchanged), `github.com/coder/websocket` (new dependency, pure Go, no CGo).

**Spec:** `docs/superpowers/specs/2026-08-22-tournamentstudio-scheduling-live-results-design.md`

## Global Constraints

- Go >= 1.23 required by `github.com/coder/websocket`; this repo's `go.mod` already pins 1.25 — no version bump needed.
- `github.com/coder/websocket` is the actively-maintained successor to the now-deprecated `nhooyr.io/websocket` (same author, identical API). Do not add `nhooyr.io/websocket` — use `github.com/coder/websocket` exclusively.
- Group:Heat and Division:Heat are both strictly 1:1 — never split a group/division across multiple heats.
- Timestamps are stored as `TEXT` columns, RFC3339 formatted (`time.RFC3339`), parsed/formatted explicitly in Go on every read/write. Never pass a raw `time.Time` to `Exec`/`Scan` — this project has no prior convention for driver-native time handling, and an explicit string keeps behavior identical to how `team_ids`/`extra_fields` are already stored as JSON `TEXT`.
- JSON API convention (established since Plan 1's final review): snake_case field names via explicit struct tags on every request/response type.
- Role enforcement stays server-side on every write endpoint: Organizer-only for courses, scheduling, and heat overrides; Organizer + Time Entry (`resultsWriter`, already declared in `routes()`) for heat results; any authenticated role for reads and the WebSocket connection.
- Every repo write that touches more than one row (batch heat creation, batch result submission) validates the entire request before writing anything, then writes inside one transaction with rollback on any error — the discipline every Plan 2 task after its final review follows (`round.Repo.CreateRound`, `round.Repo.SubmitResults`).
- Migrations are additive-only, matching this codebase's existing convention (every migration through `0009_round_number_unique.sql` only creates tables/indexes, never alters or drops). This plan's migrations follow the same rule.
- `round.PrePhaseRound`, `round.Group`, and the entire Plan 2 plugin engine (`internal/plugin`) are untouched except for one small additive helper (`round.Repo.GetGroup`, added in Task 4) — no existing round/plugin behavior changes.
- Every task that changes the migration count must bump the two `count != N` assertions in `internal/store/store_test.go` — same mechanical pattern established in Plan 2.

---

## Task 1: WebSocket broadcast hub + connection endpoint

**Files:**
- Create: `internal/server/broadcast.go`
- Create: `internal/server/handlers_ws.go`
- Create: `internal/server/ws_test.go`
- Modify: `internal/server/server.go` (add `hub` field, register the WS route)
- Modify: `go.mod`, `go.sum` (add `github.com/coder/websocket`)

**Interfaces:**
- Produces: `newBroadcastHub() *broadcastHub`; `(*broadcastHub).register(tournamentID int64) chan []byte`; `(*broadcastHub).unregister(tournamentID int64, ch chan []byte)`; `(*broadcastHub).broadcast(tournamentID int64, msg []byte)`; `(*Server).hub *broadcastHub` field. Every later task that broadcasts an event (Task 2's course delay-offset change, Task 4's result submission) calls `s.hub.broadcast(tournamentID, msg)` directly — no other task touches `broadcast.go` or `handlers_ws.go` again.

- [ ] **Step 1: Add the dependency**

```bash
go get github.com/coder/websocket
```

Run: `go build ./...`
Expected: succeeds, `go.mod`/`go.sum` now list `github.com/coder/websocket`.

- [ ] **Step 2: Write the hub**

Create `internal/server/broadcast.go`:

```go
package server

import "sync"

// broadcastHub fans out event messages to every WebSocket connection
// registered for a tournament. It's in-process memory only -- no
// external pub/sub -- matching the single-binary, offline-first
// architecture. A registrant that isn't reading fast enough (buffer
// full) has its message dropped rather than blocking the broadcaster,
// other connections, or the HTTP request that triggered the broadcast.
type broadcastHub struct {
	mu    sync.Mutex
	conns map[int64]map[chan []byte]struct{}
}

func newBroadcastHub() *broadcastHub {
	return &broadcastHub{conns: make(map[int64]map[chan []byte]struct{})}
}

func (h *broadcastHub) register(tournamentID int64) chan []byte {
	ch := make(chan []byte, 16)
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.conns[tournamentID] == nil {
		h.conns[tournamentID] = make(map[chan []byte]struct{})
	}
	h.conns[tournamentID][ch] = struct{}{}
	return ch
}

func (h *broadcastHub) unregister(tournamentID int64, ch chan []byte) {
	h.mu.Lock()
	defer h.mu.Unlock()
	delete(h.conns[tournamentID], ch)
}

func (h *broadcastHub) broadcast(tournamentID int64, msg []byte) {
	h.mu.Lock()
	defer h.mu.Unlock()
	for ch := range h.conns[tournamentID] {
		select {
		case ch <- msg:
		default:
		}
	}
}
```

- [ ] **Step 3: Write the connection handler**

Create `internal/server/handlers_ws.go`:

```go
package server

import (
	"net/http"
	"strconv"

	"github.com/coder/websocket"
)

// handleWebSocket upgrades the connection and streams every broadcast
// event for the tournament in the URL until the client disconnects.
// Auth is manual (not through requireRole) because browsers cannot set
// an Authorization header on a WebSocket handshake -- the token travels
// as a query parameter instead, validated the same way session tokens
// are validated everywhere else in this API.
func (s *Server) handleWebSocket(w http.ResponseWriter, r *http.Request) {
	tournamentID, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		http.Error(w, "invalid tournament id", http.StatusBadRequest)
		return
	}

	token := r.URL.Query().Get("token")
	if token == "" {
		http.Error(w, "missing token", http.StatusUnauthorized)
		return
	}
	if _, err := s.sessions.Find(token); err != nil {
		http.Error(w, "invalid session", http.StatusUnauthorized)
		return
	}

	c, err := websocket.Accept(w, r, nil)
	if err != nil {
		return
	}
	defer c.CloseNow()

	// CloseRead spawns a goroutine that discards incoming reads (this
	// connection is server-push only) and handles control frames; its
	// returned context is canceled the moment the client disconnects.
	ctx := c.CloseRead(r.Context())

	ch := s.hub.register(tournamentID)
	defer s.hub.unregister(tournamentID, ch)

	for {
		select {
		case msg := <-ch:
			if err := c.Write(ctx, websocket.MessageText, msg); err != nil {
				return
			}
		case <-ctx.Done():
			return
		}
	}
}
```

- [ ] **Step 4: Wire the hub and route into the server**

Modify `internal/server/server.go` — replace the `Server` struct, `New`, and `routes` with:

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
		hub:         newBroadcastHub(),
	}
	srv.routes()
	return srv
}

func (s *Server) routes() {
	s.mux.HandleFunc("GET /healthz", s.handleHealthz)
	s.mux.HandleFunc("POST /api/login", s.handleLogin)

	authenticated := s.requireRole(auth.RoleOrganizer, auth.RoleTimeEntry, auth.RoleSpectator)
	organizerOnly := s.requireRole(auth.RoleOrganizer)
	resultsWriter := s.requireRole(auth.RoleOrganizer, auth.RoleTimeEntry)

	s.mux.Handle("GET /api/whoami", authenticated(http.HandlerFunc(s.handleWhoAmI)))
	s.mux.Handle("POST /api/logout", authenticated(http.HandlerFunc(s.handleLogout)))
	s.mux.Handle("POST /api/tournaments", organizerOnly(http.HandlerFunc(s.handleCreateTournament)))
	s.mux.Handle("GET /api/tournaments", authenticated(http.HandlerFunc(s.handleListTournaments)))
	s.mux.Handle("GET /api/tournaments/{id}", authenticated(http.HandlerFunc(s.handleGetTournament)))
	s.mux.Handle("POST /api/tournaments/{id}/teams", organizerOnly(http.HandlerFunc(s.handleCreateTeam)))
	s.mux.Handle("GET /api/tournaments/{id}/teams", authenticated(http.HandlerFunc(s.handleListTeams)))
	s.mux.Handle("POST /api/tournaments/{id}/teams/import", organizerOnly(http.HandlerFunc(s.handleImportTeams)))
	s.mux.Handle("POST /api/tournaments/{id}/rounds", organizerOnly(http.HandlerFunc(s.handleCreateRound)))
	s.mux.Handle("POST /api/tournaments/{id}/rounds/{round_id}/results", resultsWriter(http.HandlerFunc(s.handleSubmitResults)))
	s.mux.Handle("POST /api/tournaments/{id}/rounds/{round_id}/next", organizerOnly(http.HandlerFunc(s.handleNextRound)))
	s.mux.Handle("POST /api/tournaments/{id}/rounds/{round_id}/divisions", organizerOnly(http.HandlerFunc(s.handleComputeDivisions)))
	s.mux.Handle("GET /api/plugins", authenticated(http.HandlerFunc(s.handlePlugins)))
	s.mux.HandleFunc("GET /api/tournaments/{id}/ws", s.handleWebSocket)
}
```

(The only changes from the current file: the new `hub` field, `hub: newBroadcastHub(),` in `New`, and the final `s.mux.HandleFunc("GET /api/tournaments/{id}/ws", ...)` line in `routes`. The WS route is registered with plain `HandleFunc`, not wrapped in `authenticated`/`organizerOnly` — its auth check is manual, inside `handleWebSocket`, because it reads the token from a query parameter instead of the `Authorization` header the `requireRole` middleware expects.)

- [ ] **Step 5: Write the tests**

Create `internal/server/ws_test.go`:

```go
package server

import (
	"context"
	"fmt"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/coder/websocket"

	"tournamentstudio/internal/auth"
)

// dialWS opens a WebSocket connection to the given tournament's live feed
// using httpServer's address, authenticating with token as the ?token=
// query parameter the way a real client must.
func dialWS(t *testing.T, httpServer *httptest.Server, tournamentID int64, token string) *websocket.Conn {
	t.Helper()
	url := "ws" + httpServer.URL[len("http"):] + fmt.Sprintf("/api/tournaments/%d/ws?token=%s", tournamentID, token)
	c, _, err := websocket.Dial(context.Background(), url, nil)
	if err != nil {
		t.Fatalf("dial ws: %v", err)
	}
	t.Cleanup(func() { c.CloseNow() })
	return c
}

func TestBroadcastReachesConnectedClientsOnlyForTheirTournament(t *testing.T) {
	s := newTestServer(t)
	token := loginAs(t, s, "organizer1", "pw", auth.RoleOrganizer)
	httpServer := httptest.NewServer(s)
	t.Cleanup(httpServer.Close)

	tournamentA := createTestTournament(t, s, token)
	tournamentB := createTestTournament(t, s, token)

	connA := dialWS(t, httpServer, tournamentA, token)
	connB := dialWS(t, httpServer, tournamentB, token)

	s.hub.broadcast(tournamentA, []byte(`{"type":"test"}`))

	readCtx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	_, msg, err := connA.Read(readCtx)
	if err != nil {
		t.Fatalf("connA should have received the broadcast: %v", err)
	}
	if string(msg) != `{"type":"test"}` {
		t.Fatalf("unexpected message: %s", msg)
	}

	shortCtx, cancel2 := context.WithTimeout(context.Background(), 300*time.Millisecond)
	defer cancel2()
	if _, _, err := connB.Read(shortCtx); err == nil {
		t.Fatalf("connB should NOT have received tournamentA's broadcast")
	}
}

func TestWebSocketRejectsInvalidToken(t *testing.T) {
	s := newTestServer(t)
	_ = loginAs(t, s, "organizer1", "pw", auth.RoleOrganizer)
	httpServer := httptest.NewServer(s)
	t.Cleanup(httpServer.Close)

	tournamentID := int64(1)
	url := "ws" + httpServer.URL[len("http"):] + fmt.Sprintf("/api/tournaments/%d/ws?token=not-a-real-token", tournamentID)
	_, _, err := websocket.Dial(context.Background(), url, nil)
	if err == nil {
		t.Fatalf("expected dial with an invalid token to fail")
	}
}

func TestWebSocketRejectsMissingToken(t *testing.T) {
	s := newTestServer(t)
	httpServer := httptest.NewServer(s)
	t.Cleanup(httpServer.Close)

	url := "ws" + httpServer.URL[len("http"):] + "/api/tournaments/1/ws"
	_, _, err := websocket.Dial(context.Background(), url, nil)
	if err == nil {
		t.Fatalf("expected dial with no token to fail")
	}
}
```

- [ ] **Step 6: Run the tests**

Run: `go test ./internal/server/... -run TestBroadcast -v` and `go test ./internal/server/... -run TestWebSocket -v`
Expected: PASS

- [ ] **Step 7: Run the full suite and commit**

Run: `go build ./... && go test ./... -count=1`
Expected: PASS, all packages.

```bash
git add go.mod go.sum internal/server/broadcast.go internal/server/handlers_ws.go internal/server/ws_test.go internal/server/server.go
git commit -m "feat: add WebSocket broadcast hub and connection endpoint"
```

---

## Task 2: Course domain and CRUD endpoints

**Files:**
- Create: `internal/schedule/model.go`
- Create: `internal/schedule/repo.go`
- Create: `internal/schedule/course.go`
- Create: `internal/schedule/course_test.go`
- Create: `internal/store/migrations/0010_courses.sql`
- Create: `internal/server/handlers_courses.go`
- Create: `internal/server/courses_test.go`
- Modify: `internal/server/server.go` (add `schedule` field, register 3 routes)
- Modify: `internal/store/store_test.go` (migration count 9 -> 10)

**Interfaces:**
- Consumes: `(*Server).hub.broadcast(tournamentID int64, msg []byte)` (Task 1).
- Produces: `schedule.Course{ID, TournamentID int64, Name string, HeatIntervalSeconds, DelayOffsetSeconds int}` (all fields carry `json` tags: `id`, `tournament_id`, `name`, `heat_interval_seconds`, `delay_offset_seconds`); `schedule.Repo` type + `schedule.NewRepo(s *store.Store) *Repo`; `schedule.ErrCourseNotFound`; `(*schedule.Repo).CreateCourse(tournamentID int64, name string, heatIntervalSeconds int) (*Course, error)`; `(*schedule.Repo).ListCourses(tournamentID int64) ([]Course, error)`; `(*schedule.Repo).GetCourse(id int64) (*Course, error)`; `schedule.CourseUpdate{Name *string, HeatIntervalSeconds *int, DelayOffsetSeconds *int}`; `(*schedule.Repo).UpdateCourse(id int64, upd CourseUpdate) (*Course, error)`. `(*Server).schedule *schedule.Repo` field. Every later task that touches courses (none do directly) or needs `s.schedule` (Tasks 3-9) relies on this field existing.

- [ ] **Step 1: Write the migration**

Create `internal/store/migrations/0010_courses.sql`:

```sql
CREATE TABLE courses (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    tournament_id INTEGER NOT NULL REFERENCES tournaments(id),
    name TEXT NOT NULL,
    heat_interval_seconds INTEGER NOT NULL,
    delay_offset_seconds INTEGER NOT NULL DEFAULT 0
);
```

- [ ] **Step 2: Bump the migration count**

Modify `internal/store/store_test.go`: change both `if count != 9` to `if count != 10` (two occurrences — the initial-open assertion and the reopen-doesn't-reapply assertion).

Run: `go test ./internal/store/... -run TestOpenAppliesMigrationsOnce -v`
Expected: PASS (10 migration files now exist on disk, matching the updated assertion).

- [ ] **Step 3: Write the model and repo skeleton**

Create `internal/schedule/model.go`:

```go
package schedule

type Course struct {
	ID                  int64  `json:"id"`
	TournamentID        int64  `json:"tournament_id"`
	Name                string `json:"name"`
	HeatIntervalSeconds int    `json:"heat_interval_seconds"`
	DelayOffsetSeconds  int    `json:"delay_offset_seconds"`
}
```

Create `internal/schedule/repo.go`:

```go
package schedule

import (
	"database/sql"

	"tournamentstudio/internal/store"
)

type Repo struct {
	db *sql.DB
}

func NewRepo(s *store.Store) *Repo {
	return &Repo{db: s.DB}
}
```

- [ ] **Step 4: Write the failing repo test**

Create `internal/schedule/course_test.go`:

```go
package schedule

import (
	"path/filepath"
	"testing"

	"tournamentstudio/internal/store"
)

func newTestRepo(t *testing.T) *Repo {
	t.Helper()
	s, err := store.Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	t.Cleanup(func() { s.DB.Close() })
	return NewRepo(s)
}

func TestCreateAndListCourses(t *testing.T) {
	r := newTestRepo(t)

	c1, err := r.CreateCourse(1, "Course A", 300)
	if err != nil {
		t.Fatalf("CreateCourse: %v", err)
	}
	if c1.ID == 0 {
		t.Fatalf("expected a non-zero ID")
	}
	if c1.DelayOffsetSeconds != 0 {
		t.Fatalf("expected new course to default to 0 delay offset, got %d", c1.DelayOffsetSeconds)
	}

	if _, err := r.CreateCourse(1, "Course B", 240); err != nil {
		t.Fatalf("CreateCourse: %v", err)
	}
	if _, err := r.CreateCourse(2, "Other Tournament's Course", 300); err != nil {
		t.Fatalf("CreateCourse: %v", err)
	}

	courses, err := r.ListCourses(1)
	if err != nil {
		t.Fatalf("ListCourses: %v", err)
	}
	if len(courses) != 2 {
		t.Fatalf("expected 2 courses for tournament 1, got %d", len(courses))
	}
}

func TestGetCourseNotFound(t *testing.T) {
	r := newTestRepo(t)
	if _, err := r.GetCourse(999); err != ErrCourseNotFound {
		t.Fatalf("expected ErrCourseNotFound, got %v", err)
	}
}

func TestUpdateCourse(t *testing.T) {
	r := newTestRepo(t)
	c, err := r.CreateCourse(1, "Course A", 300)
	if err != nil {
		t.Fatalf("CreateCourse: %v", err)
	}

	newOffset := 900
	updated, err := r.UpdateCourse(c.ID, CourseUpdate{DelayOffsetSeconds: &newOffset})
	if err != nil {
		t.Fatalf("UpdateCourse: %v", err)
	}
	if updated.DelayOffsetSeconds != 900 {
		t.Fatalf("expected delay offset 900, got %d", updated.DelayOffsetSeconds)
	}
	if updated.Name != "Course A" || updated.HeatIntervalSeconds != 300 {
		t.Fatalf("expected other fields unchanged, got %+v", updated)
	}

	newName := "Course A (renamed)"
	updated2, err := r.UpdateCourse(c.ID, CourseUpdate{Name: &newName})
	if err != nil {
		t.Fatalf("UpdateCourse: %v", err)
	}
	if updated2.Name != "Course A (renamed)" || updated2.DelayOffsetSeconds != 900 {
		t.Fatalf("expected name updated and prior offset preserved, got %+v", updated2)
	}
}
```

- [ ] **Step 5: Run the tests to verify they fail**

Run: `go test ./internal/schedule/... -v`
Expected: FAIL (compile error — `CreateCourse` etc. undefined)

- [ ] **Step 6: Implement the repo methods**

Create `internal/schedule/course.go`:

```go
package schedule

import (
	"database/sql"
	"errors"
)

var ErrCourseNotFound = errors.New("course not found")

func (r *Repo) CreateCourse(tournamentID int64, name string, heatIntervalSeconds int) (*Course, error) {
	res, err := r.db.Exec(
		`INSERT INTO courses (tournament_id, name, heat_interval_seconds, delay_offset_seconds) VALUES (?, ?, ?, 0)`,
		tournamentID, name, heatIntervalSeconds,
	)
	if err != nil {
		return nil, err
	}
	id, err := res.LastInsertId()
	if err != nil {
		return nil, err
	}
	return &Course{ID: id, TournamentID: tournamentID, Name: name, HeatIntervalSeconds: heatIntervalSeconds, DelayOffsetSeconds: 0}, nil
}

func (r *Repo) ListCourses(tournamentID int64) ([]Course, error) {
	rows, err := r.db.Query(
		`SELECT id, tournament_id, name, heat_interval_seconds, delay_offset_seconds FROM courses WHERE tournament_id = ? ORDER BY id`,
		tournamentID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var courses []Course
	for rows.Next() {
		var c Course
		if err := rows.Scan(&c.ID, &c.TournamentID, &c.Name, &c.HeatIntervalSeconds, &c.DelayOffsetSeconds); err != nil {
			return nil, err
		}
		courses = append(courses, c)
	}
	return courses, rows.Err()
}

func (r *Repo) GetCourse(id int64) (*Course, error) {
	row := r.db.QueryRow(
		`SELECT id, tournament_id, name, heat_interval_seconds, delay_offset_seconds FROM courses WHERE id = ?`,
		id,
	)
	var c Course
	if err := row.Scan(&c.ID, &c.TournamentID, &c.Name, &c.HeatIntervalSeconds, &c.DelayOffsetSeconds); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrCourseNotFound
		}
		return nil, err
	}
	return &c, nil
}

type CourseUpdate struct {
	Name                *string
	HeatIntervalSeconds *int
	DelayOffsetSeconds  *int
}

func (r *Repo) UpdateCourse(id int64, upd CourseUpdate) (*Course, error) {
	c, err := r.GetCourse(id)
	if err != nil {
		return nil, err
	}
	if upd.Name != nil {
		c.Name = *upd.Name
	}
	if upd.HeatIntervalSeconds != nil {
		c.HeatIntervalSeconds = *upd.HeatIntervalSeconds
	}
	if upd.DelayOffsetSeconds != nil {
		c.DelayOffsetSeconds = *upd.DelayOffsetSeconds
	}
	if _, err := r.db.Exec(
		`UPDATE courses SET name = ?, heat_interval_seconds = ?, delay_offset_seconds = ? WHERE id = ?`,
		c.Name, c.HeatIntervalSeconds, c.DelayOffsetSeconds, id,
	); err != nil {
		return nil, err
	}
	return c, nil
}
```

- [ ] **Step 7: Run the repo tests to verify they pass**

Run: `go test ./internal/schedule/... -v`
Expected: PASS

- [ ] **Step 8: Wire the schedule repo into the server**

Modify `internal/server/server.go` — add `schedule *schedule.Repo` to the `Server` struct and `schedule: schedule.NewRepo(s),` to `New`, and add the import `"tournamentstudio/internal/schedule"`:

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
		hub:         newBroadcastHub(),
		schedule:    schedule.NewRepo(s),
	}
	srv.routes()
	return srv
}
```

Add these three lines inside `routes()`, anywhere after `organizerOnly`/`authenticated` are declared and before the closing brace:

```go
	s.mux.Handle("POST /api/tournaments/{id}/courses", organizerOnly(http.HandlerFunc(s.handleCreateCourse)))
	s.mux.Handle("GET /api/tournaments/{id}/courses", authenticated(http.HandlerFunc(s.handleListCourses)))
	s.mux.Handle("PATCH /api/tournaments/{id}/courses/{course_id}", organizerOnly(http.HandlerFunc(s.handleUpdateCourse)))
```

- [ ] **Step 9: Write the HTTP handlers**

Create `internal/server/handlers_courses.go`:

```go
package server

import (
	"encoding/json"
	"net/http"
	"strconv"

	"tournamentstudio/internal/schedule"
)

func courseToResponse(c *schedule.Course) schedule.Course {
	return *c
}

type createCourseRequest struct {
	Name                string `json:"name"`
	HeatIntervalSeconds int    `json:"heat_interval_seconds"`
}

func (s *Server) handleCreateCourse(w http.ResponseWriter, r *http.Request) {
	tournamentID, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		http.Error(w, "invalid tournament id", http.StatusBadRequest)
		return
	}

	var req createCourseRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}
	if req.Name == "" {
		http.Error(w, "name is required", http.StatusBadRequest)
		return
	}
	if req.HeatIntervalSeconds < 1 {
		http.Error(w, "heat_interval_seconds must be at least 1", http.StatusBadRequest)
		return
	}

	c, err := s.schedule.CreateCourse(tournamentID, req.Name, req.HeatIntervalSeconds)
	if err != nil {
		http.Error(w, "could not create course", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(courseToResponse(c))
}

func (s *Server) handleListCourses(w http.ResponseWriter, r *http.Request) {
	tournamentID, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		http.Error(w, "invalid tournament id", http.StatusBadRequest)
		return
	}

	courses, err := s.schedule.ListCourses(tournamentID)
	if err != nil {
		http.Error(w, "could not list courses", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]any{"courses": courses})
}

type updateCourseRequest struct {
	Name                *string `json:"name"`
	HeatIntervalSeconds *int    `json:"heat_interval_seconds"`
	DelayOffsetSeconds  *int    `json:"delay_offset_seconds"`
}

func (s *Server) handleUpdateCourse(w http.ResponseWriter, r *http.Request) {
	tournamentID, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		http.Error(w, "invalid tournament id", http.StatusBadRequest)
		return
	}
	courseID, err := strconv.ParseInt(r.PathValue("course_id"), 10, 64)
	if err != nil {
		http.Error(w, "invalid course id", http.StatusBadRequest)
		return
	}

	existing, err := s.schedule.GetCourse(courseID)
	if err != nil {
		if err == schedule.ErrCourseNotFound {
			http.Error(w, "course not found", http.StatusNotFound)
			return
		}
		http.Error(w, "could not get course", http.StatusInternalServerError)
		return
	}
	if existing.TournamentID != tournamentID {
		http.Error(w, "course not found", http.StatusNotFound)
		return
	}

	var req updateCourseRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}
	if req.Name != nil && *req.Name == "" {
		http.Error(w, "name cannot be empty", http.StatusBadRequest)
		return
	}
	if req.HeatIntervalSeconds != nil && *req.HeatIntervalSeconds < 1 {
		http.Error(w, "heat_interval_seconds must be at least 1", http.StatusBadRequest)
		return
	}

	updated, err := s.schedule.UpdateCourse(courseID, schedule.CourseUpdate{
		Name:                req.Name,
		HeatIntervalSeconds: req.HeatIntervalSeconds,
		DelayOffsetSeconds:  req.DelayOffsetSeconds,
	})
	if err != nil {
		http.Error(w, "could not update course", http.StatusInternalServerError)
		return
	}

	if req.DelayOffsetSeconds != nil {
		msg, _ := json.Marshal(map[string]any{
			"type":                 "delay_offset_changed",
			"course_id":            updated.ID,
			"delay_offset_seconds": updated.DelayOffsetSeconds,
		})
		s.hub.broadcast(tournamentID, msg)
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(courseToResponse(updated))
}
```

- [ ] **Step 10: Write the HTTP tests**

Create `internal/server/courses_test.go`:

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

// createTestCourse creates a course via the real HTTP endpoint and
// returns its ID, for reuse by every later task's tests that need a
// course to schedule heats onto.
func createTestCourse(t *testing.T, s *Server, token string, tournamentID int64, name string, heatIntervalSeconds int) int64 {
	t.Helper()
	body, _ := json.Marshal(map[string]any{"name": name, "heat_interval_seconds": heatIntervalSeconds})
	req := httptest.NewRequest(http.MethodPost, fmt.Sprintf("/api/tournaments/%d/courses", tournamentID), bytes.NewReader(body))
	req.Header.Set("Authorization", "Bearer "+token)
	rec := httptest.NewRecorder()
	s.ServeHTTP(rec, req)
	if rec.Code != http.StatusCreated {
		t.Fatalf("create course: expected 201, got %d: %s", rec.Code, rec.Body.String())
	}
	var created struct {
		ID int64 `json:"id"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &created); err != nil {
		t.Fatalf("decode created course: %v", err)
	}
	return created.ID
}

func TestCreateAndListCoursesHTTP(t *testing.T) {
	s := newTestServer(t)
	token := loginAs(t, s, "organizer1", "pw", auth.RoleOrganizer)
	tournamentID := createTestTournament(t, s, token)

	createTestCourse(t, s, token, tournamentID, "Course A", 300)
	createTestCourse(t, s, token, tournamentID, "Course B", 240)

	req := httptest.NewRequest(http.MethodGet, fmt.Sprintf("/api/tournaments/%d/courses", tournamentID), nil)
	req.Header.Set("Authorization", "Bearer "+token)
	rec := httptest.NewRecorder()
	s.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}

	var resp struct {
		Courses []struct {
			Name string `json:"name"`
		} `json:"courses"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(resp.Courses) != 2 {
		t.Fatalf("expected 2 courses, got %d", len(resp.Courses))
	}
}

func TestCreateCourseRejectsMissingName(t *testing.T) {
	s := newTestServer(t)
	token := loginAs(t, s, "organizer1", "pw", auth.RoleOrganizer)
	tournamentID := createTestTournament(t, s, token)

	body, _ := json.Marshal(map[string]any{"heat_interval_seconds": 300})
	req := httptest.NewRequest(http.MethodPost, fmt.Sprintf("/api/tournaments/%d/courses", tournamentID), bytes.NewReader(body))
	req.Header.Set("Authorization", "Bearer "+token)
	rec := httptest.NewRecorder()
	s.ServeHTTP(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", rec.Code)
	}
}

func TestCreateCourseForbiddenForTimeEntry(t *testing.T) {
	s := newTestServer(t)
	organizerToken := loginAs(t, s, "organizer1", "pw", auth.RoleOrganizer)
	tournamentID := createTestTournament(t, s, organizerToken)

	timeEntryToken := loginAs(t, s, "timekeeper1", "pw", auth.RoleTimeEntry)
	body, _ := json.Marshal(map[string]any{"name": "Course A", "heat_interval_seconds": 300})
	req := httptest.NewRequest(http.MethodPost, fmt.Sprintf("/api/tournaments/%d/courses", tournamentID), bytes.NewReader(body))
	req.Header.Set("Authorization", "Bearer "+timeEntryToken)
	rec := httptest.NewRecorder()
	s.ServeHTTP(rec, req)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("expected 403, got %d", rec.Code)
	}
}

func TestUpdateCourseDelayOffsetBroadcasts(t *testing.T) {
	s := newTestServer(t)
	token := loginAs(t, s, "organizer1", "pw", auth.RoleOrganizer)
	tournamentID := createTestTournament(t, s, token)
	courseID := createTestCourse(t, s, token, tournamentID, "Course A", 300)

	httpServer := httptest.NewServer(s)
	t.Cleanup(httpServer.Close)
	conn := dialWS(t, httpServer, tournamentID, token)

	body, _ := json.Marshal(map[string]any{"delay_offset_seconds": 900})
	req := httptest.NewRequest(http.MethodPatch, fmt.Sprintf("/api/tournaments/%d/courses/%d", tournamentID, courseID), bytes.NewReader(body))
	req.Header.Set("Authorization", "Bearer "+token)
	rec := httptest.NewRecorder()
	s.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}

	var updated struct {
		DelayOffsetSeconds int `json:"delay_offset_seconds"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &updated); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if updated.DelayOffsetSeconds != 900 {
		t.Fatalf("expected 900, got %d", updated.DelayOffsetSeconds)
	}

	readCtx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	_, msg, err := conn.Read(readCtx)
	if err != nil {
		t.Fatalf("expected a broadcast message: %v", err)
	}
	var evt struct {
		Type               string `json:"type"`
		CourseID           int64  `json:"course_id"`
		DelayOffsetSeconds int    `json:"delay_offset_seconds"`
	}
	if err := json.Unmarshal(msg, &evt); err != nil {
		t.Fatalf("decode broadcast: %v", err)
	}
	if evt.Type != "delay_offset_changed" || evt.CourseID != courseID || evt.DelayOffsetSeconds != 900 {
		t.Fatalf("unexpected broadcast event: %+v", evt)
	}
}

func TestUpdateCourseNotFoundForWrongTournament(t *testing.T) {
	s := newTestServer(t)
	token := loginAs(t, s, "organizer1", "pw", auth.RoleOrganizer)
	tournamentA := createTestTournament(t, s, token)
	tournamentB := createTestTournament(t, s, token)
	courseID := createTestCourse(t, s, token, tournamentA, "Course A", 300)

	body, _ := json.Marshal(map[string]any{"name": "Hijacked"})
	req := httptest.NewRequest(http.MethodPatch, fmt.Sprintf("/api/tournaments/%d/courses/%d", tournamentB, courseID), bytes.NewReader(body))
	req.Header.Set("Authorization", "Bearer "+token)
	rec := httptest.NewRecorder()
	s.ServeHTTP(rec, req)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", rec.Code)
	}
}
```

Add the two missing imports (`context`, `time`) to this file's import block: `"context"` and `"time"` alongside the existing ones, since `TestUpdateCourseDelayOffsetBroadcasts` uses `context.WithTimeout`/`time.Second`.

- [ ] **Step 11: Run the tests to verify they pass**

Run: `go build ./... && go test ./internal/schedule/... ./internal/server/... -v`
Expected: PASS

- [ ] **Step 12: Run the full suite and commit**

Run: `go build ./... && go test ./... -count=1`
Expected: PASS, all packages.

```bash
git add internal/schedule internal/store/migrations/0010_courses.sql internal/store/store_test.go internal/server/handlers_courses.go internal/server/courses_test.go internal/server/server.go
git commit -m "feat: add course domain and CRUD endpoints"
```

---

## Task 3: Heat domain and round-scheduling endpoint

**Files:**
- Modify: `internal/schedule/model.go` (add `Heat`, `HeatStatus`)
- Create: `internal/schedule/heat.go`
- Create: `internal/schedule/heat_test.go`
- Create: `internal/store/migrations/0011_heats.sql`
- Create: `internal/server/handlers_schedule_round.go`
- Create: `internal/server/schedule_round_test.go`
- Modify: `internal/server/server.go` (register 1 route)
- Modify: `internal/store/store_test.go` (migration count 10 -> 11)

**Interfaces:**
- Consumes: `schedule.Repo`, `schedule.ErrCourseNotFound` (Task 2); `s.rounds.GetRound`, `round.ErrNotFound`, `s.rounds.ListGroups`, `round.Group` (Plan 2, unchanged).
- Produces: `schedule.HeatStatus` (`HeatScheduled = "scheduled"`, `HeatClosed = "closed"`); `schedule.Heat{ID, RoundID int64, GroupID, DivisionID *int64, CourseID int64, PlannedStart time.Time, Status HeatStatus}`; `schedule.ErrGroupAlreadyScheduled`; `schedule.GroupAssignment{GroupID, CourseID int64}`; `(*schedule.Repo).ScheduleGroupHeats(tournamentID, roundID int64, assignments []GroupAssignment, startAt *time.Time) ([]Heat, error)`; `(*schedule.Repo).GetHeat(id int64) (*Heat, error)`; `schedule.ErrHeatNotFound`. `heatResponse{ID, RoundID int64, GroupID, DivisionID *int64, CourseID int64, PlannedStart, Status string}` and `heatToResponse(h schedule.Heat) heatResponse` in package `server` — Tasks 4, 7, 8, 9 all reuse `heatToResponse` and the `heatResponse` type, never redefine them.

- [ ] **Step 1: Write the migration**

Create `internal/store/migrations/0011_heats.sql`:

```sql
CREATE TABLE heats (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    round_id INTEGER NOT NULL REFERENCES pre_phase_rounds(id),
    group_id INTEGER REFERENCES groups(id),
    division_id INTEGER REFERENCES divisions(id),
    course_id INTEGER NOT NULL REFERENCES courses(id),
    planned_start TEXT NOT NULL,
    status TEXT NOT NULL DEFAULT 'scheduled'
);
```

(`divisions` does not exist yet — created in Task 6 — but SQLite does not validate a `REFERENCES` target's existence at `CREATE TABLE` time, only at the point a non-null value is actually inserted, and by then every migration through this plan has already run. Confirmed safe by direct test before writing this plan.)

- [ ] **Step 2: Bump the migration count**

Modify `internal/store/store_test.go`: change both `if count != 10` to `if count != 11`.

- [ ] **Step 3: Extend the model**

Modify `internal/schedule/model.go` — add `"time"` to the imports and append:

```go
type HeatStatus string

const (
	HeatScheduled HeatStatus = "scheduled"
	HeatClosed    HeatStatus = "closed"
)

type Heat struct {
	ID           int64
	RoundID      int64
	GroupID      *int64
	DivisionID   *int64
	CourseID     int64
	PlannedStart time.Time
	Status       HeatStatus
}
```

- [ ] **Step 4: Write the failing repo test**

Create `internal/schedule/heat_test.go`:

```go
package schedule

import (
	"testing"
	"time"
)

func TestScheduleGroupHeatsAutoSequencesOnSameCourse(t *testing.T) {
	r := newTestRepo(t)
	course, err := r.CreateCourse(1, "Course A", 300)
	if err != nil {
		t.Fatalf("CreateCourse: %v", err)
	}

	start := time.Date(2026, 9, 1, 9, 0, 0, 0, time.UTC)
	heats, err := r.ScheduleGroupHeats(1, 10, []GroupAssignment{
		{GroupID: 100, CourseID: course.ID},
		{GroupID: 101, CourseID: course.ID},
	}, &start)
	if err != nil {
		t.Fatalf("ScheduleGroupHeats: %v", err)
	}
	if len(heats) != 2 {
		t.Fatalf("expected 2 heats, got %d", len(heats))
	}
	if !heats[0].PlannedStart.Equal(start) {
		t.Fatalf("expected first heat at %v, got %v", start, heats[0].PlannedStart)
	}
	wantSecond := start.Add(300 * time.Second)
	if !heats[1].PlannedStart.Equal(wantSecond) {
		t.Fatalf("expected second heat at %v, got %v", wantSecond, heats[1].PlannedStart)
	}
	if heats[0].RoundID != 10 || *heats[0].GroupID != 100 || heats[0].DivisionID != nil {
		t.Fatalf("unexpected heat shape: %+v", heats[0])
	}
	if heats[0].Status != HeatScheduled {
		t.Fatalf("expected status %q, got %q", HeatScheduled, heats[0].Status)
	}
}

func TestScheduleGroupHeatsContinuesAfterExistingHeatsOnSameCourse(t *testing.T) {
	r := newTestRepo(t)
	course, err := r.CreateCourse(1, "Course A", 300)
	if err != nil {
		t.Fatalf("CreateCourse: %v", err)
	}

	start := time.Date(2026, 9, 1, 9, 0, 0, 0, time.UTC)
	firstBatch, err := r.ScheduleGroupHeats(1, 10, []GroupAssignment{{GroupID: 100, CourseID: course.ID}}, &start)
	if err != nil {
		t.Fatalf("ScheduleGroupHeats (round 1): %v", err)
	}

	secondBatch, err := r.ScheduleGroupHeats(1, 11, []GroupAssignment{{GroupID: 200, CourseID: course.ID}}, nil)
	if err != nil {
		t.Fatalf("ScheduleGroupHeats (round 2): %v", err)
	}
	wantSecond := firstBatch[0].PlannedStart.Add(300 * time.Second)
	if !secondBatch[0].PlannedStart.Equal(wantSecond) {
		t.Fatalf("expected round 2's heat to continue the sequence at %v, got %v", wantSecond, secondBatch[0].PlannedStart)
	}
}

func TestScheduleGroupHeatsRejectsAlreadyScheduledGroup(t *testing.T) {
	r := newTestRepo(t)
	course, err := r.CreateCourse(1, "Course A", 300)
	if err != nil {
		t.Fatalf("CreateCourse: %v", err)
	}
	start := time.Date(2026, 9, 1, 9, 0, 0, 0, time.UTC)
	if _, err := r.ScheduleGroupHeats(1, 10, []GroupAssignment{{GroupID: 100, CourseID: course.ID}}, &start); err != nil {
		t.Fatalf("ScheduleGroupHeats: %v", err)
	}

	if _, err := r.ScheduleGroupHeats(1, 10, []GroupAssignment{{GroupID: 100, CourseID: course.ID}}, &start); err != ErrGroupAlreadyScheduled {
		t.Fatalf("expected ErrGroupAlreadyScheduled, got %v", err)
	}
}

func TestScheduleGroupHeatsRejectsUnknownCourse(t *testing.T) {
	r := newTestRepo(t)
	start := time.Date(2026, 9, 1, 9, 0, 0, 0, time.UTC)
	if _, err := r.ScheduleGroupHeats(1, 10, []GroupAssignment{{GroupID: 100, CourseID: 999}}, &start); err != ErrCourseNotFound {
		t.Fatalf("expected ErrCourseNotFound, got %v", err)
	}
}

func TestScheduleGroupHeatsRejectsCourseFromAnotherTournament(t *testing.T) {
	r := newTestRepo(t)
	course, err := r.CreateCourse(2, "Someone Else's Course", 300)
	if err != nil {
		t.Fatalf("CreateCourse: %v", err)
	}
	start := time.Date(2026, 9, 1, 9, 0, 0, 0, time.UTC)
	if _, err := r.ScheduleGroupHeats(1, 10, []GroupAssignment{{GroupID: 100, CourseID: course.ID}}, &start); err != ErrCourseNotFound {
		t.Fatalf("expected ErrCourseNotFound for a cross-tournament course, got %v", err)
	}
}

func TestGetHeatNotFound(t *testing.T) {
	r := newTestRepo(t)
	if _, err := r.GetHeat(999); err != ErrHeatNotFound {
		t.Fatalf("expected ErrHeatNotFound, got %v", err)
	}
}
```

- [ ] **Step 5: Run the tests to verify they fail**

Run: `go test ./internal/schedule/... -v`
Expected: FAIL (compile error — `ScheduleGroupHeats`, `GetHeat`, `GroupAssignment`, `ErrGroupAlreadyScheduled`, `ErrHeatNotFound` undefined)

- [ ] **Step 6: Implement the repo methods**

Create `internal/schedule/heat.go`:

```go
package schedule

import (
	"database/sql"
	"errors"
	"time"
)

var ErrHeatNotFound = errors.New("heat not found")
var ErrGroupAlreadyScheduled = errors.New("group already has a scheduled heat")

// GroupAssignment pairs a round's group with the course it should race
// on, for ScheduleGroupHeats.
type GroupAssignment struct {
	GroupID  int64
	CourseID int64
}

// ScheduleGroupHeats creates one Heat per assignment, transactionally.
// Every assignment's course must belong to tournamentID and must not
// already have created a heat for that group. Heats on the same course
// within one call are auto-sequenced HeatIntervalSeconds apart, in
// assignments' order; the first heat on a given course starts at
// startAt if provided, else one interval after that course's
// currently-latest heat, else now if the course has no heats yet.
func (r *Repo) ScheduleGroupHeats(tournamentID, roundID int64, assignments []GroupAssignment, startAt *time.Time) ([]Heat, error) {
	tx, err := r.db.Begin()
	if err != nil {
		return nil, err
	}

	nextStart := make(map[int64]time.Time)
	created := make([]Heat, 0, len(assignments))

	for _, a := range assignments {
		var courseTournamentID int64
		var intervalSeconds int
		row := tx.QueryRow(`SELECT tournament_id, heat_interval_seconds FROM courses WHERE id = ?`, a.CourseID)
		if err := row.Scan(&courseTournamentID, &intervalSeconds); err != nil {
			tx.Rollback()
			if errors.Is(err, sql.ErrNoRows) {
				return nil, ErrCourseNotFound
			}
			return nil, err
		}
		if courseTournamentID != tournamentID {
			tx.Rollback()
			return nil, ErrCourseNotFound
		}

		var existingCount int
		if err := tx.QueryRow(`SELECT COUNT(*) FROM heats WHERE group_id = ?`, a.GroupID).Scan(&existingCount); err != nil {
			tx.Rollback()
			return nil, err
		}
		if existingCount > 0 {
			tx.Rollback()
			return nil, ErrGroupAlreadyScheduled
		}

		start, ok := nextStart[a.CourseID]
		if !ok {
			start, err = courseAnchor(tx, a.CourseID, startAt)
			if err != nil {
				tx.Rollback()
				return nil, err
			}
		}
		nextStart[a.CourseID] = start.Add(time.Duration(intervalSeconds) * time.Second)

		groupID := a.GroupID
		res, err := tx.Exec(
			`INSERT INTO heats (round_id, group_id, division_id, course_id, planned_start, status) VALUES (?, ?, NULL, ?, ?, ?)`,
			roundID, groupID, a.CourseID, start.Format(time.RFC3339), string(HeatScheduled),
		)
		if err != nil {
			tx.Rollback()
			return nil, err
		}
		heatID, err := res.LastInsertId()
		if err != nil {
			tx.Rollback()
			return nil, err
		}
		created = append(created, Heat{
			ID: heatID, RoundID: roundID, GroupID: &groupID, CourseID: a.CourseID,
			PlannedStart: start, Status: HeatScheduled,
		})
	}

	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return created, nil
}

// courseAnchor determines the base PlannedStart for the first new heat
// scheduled on courseID in one ScheduleGroupHeats/ScheduleDivisionHeats
// call: startAt if the caller supplied one, else one interval after the
// course's currently-latest scheduled heat, else now if the course has
// no heats yet.
func courseAnchor(tx *sql.Tx, courseID int64, startAt *time.Time) (time.Time, error) {
	if startAt != nil {
		return *startAt, nil
	}
	var latest sql.NullString
	if err := tx.QueryRow(`SELECT MAX(planned_start) FROM heats WHERE course_id = ?`, courseID).Scan(&latest); err != nil {
		return time.Time{}, err
	}
	if !latest.Valid {
		return time.Now().UTC(), nil
	}
	t, err := time.Parse(time.RFC3339, latest.String)
	if err != nil {
		return time.Time{}, err
	}
	var intervalSeconds int
	if err := tx.QueryRow(`SELECT heat_interval_seconds FROM courses WHERE id = ?`, courseID).Scan(&intervalSeconds); err != nil {
		return time.Time{}, err
	}
	return t.Add(time.Duration(intervalSeconds) * time.Second), nil
}

func (r *Repo) GetHeat(id int64) (*Heat, error) {
	row := r.db.QueryRow(
		`SELECT id, round_id, group_id, division_id, course_id, planned_start, status FROM heats WHERE id = ?`,
		id,
	)
	var h Heat
	var groupID, divisionID sql.NullInt64
	var plannedStart, status string
	if err := row.Scan(&h.ID, &h.RoundID, &groupID, &divisionID, &h.CourseID, &plannedStart, &status); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrHeatNotFound
		}
		return nil, err
	}
	if groupID.Valid {
		v := groupID.Int64
		h.GroupID = &v
	}
	if divisionID.Valid {
		v := divisionID.Int64
		h.DivisionID = &v
	}
	t, err := time.Parse(time.RFC3339, plannedStart)
	if err != nil {
		return nil, err
	}
	h.PlannedStart = t
	h.Status = HeatStatus(status)
	return &h, nil
}
```

- [ ] **Step 7: Run the repo tests to verify they pass**

Run: `go test ./internal/schedule/... -v`
Expected: PASS

- [ ] **Step 8: Register the route**

Add this line inside `routes()` in `internal/server/server.go`, alongside the other round routes:

```go
	s.mux.Handle("POST /api/tournaments/{id}/rounds/{round_id}/schedule", organizerOnly(http.HandlerFunc(s.handleScheduleRound)))
```

- [ ] **Step 9: Write the handler**

Create `internal/server/handlers_schedule_round.go`:

```go
package server

import (
	"encoding/json"
	"net/http"
	"strconv"
	"time"

	"tournamentstudio/internal/round"
	"tournamentstudio/internal/schedule"
)

type scheduleAssignment struct {
	GroupID  int64 `json:"group_id"`
	CourseID int64 `json:"course_id"`
}

type scheduleRoundRequest struct {
	Assignments []scheduleAssignment `json:"assignments"`
	StartAt     *string              `json:"start_at"`
}

type heatResponse struct {
	ID           int64  `json:"id"`
	RoundID      int64  `json:"round_id"`
	GroupID      *int64 `json:"group_id"`
	DivisionID   *int64 `json:"division_id"`
	CourseID     int64  `json:"course_id"`
	PlannedStart string `json:"planned_start"`
	Status       string `json:"status"`
}

func heatToResponse(h schedule.Heat) heatResponse {
	return heatResponse{
		ID:           h.ID,
		RoundID:      h.RoundID,
		GroupID:      h.GroupID,
		DivisionID:   h.DivisionID,
		CourseID:     h.CourseID,
		PlannedStart: h.PlannedStart.Format(time.RFC3339),
		Status:       string(h.Status),
	}
}

func parseStartAt(raw *string) (*time.Time, error) {
	if raw == nil {
		return nil, nil
	}
	t, err := time.Parse(time.RFC3339, *raw)
	if err != nil {
		return nil, err
	}
	return &t, nil
}

func (s *Server) handleScheduleRound(w http.ResponseWriter, r *http.Request) {
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

	var req scheduleRoundRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}
	startAt, err := parseStartAt(req.StartAt)
	if err != nil {
		http.Error(w, "start_at must be RFC3339", http.StatusBadRequest)
		return
	}

	groups, err := s.rounds.ListGroups(roundID)
	if err != nil {
		http.Error(w, "could not list groups", http.StatusInternalServerError)
		return
	}
	if len(req.Assignments) != len(groups) {
		http.Error(w, "assignments must cover every group of the round exactly once", http.StatusBadRequest)
		return
	}
	roundGroupIDs := make(map[int64]bool, len(groups))
	for _, g := range groups {
		roundGroupIDs[g.ID] = true
	}
	seen := make(map[int64]bool, len(req.Assignments))
	assignments := make([]schedule.GroupAssignment, 0, len(req.Assignments))
	for _, a := range req.Assignments {
		if !roundGroupIDs[a.GroupID] || seen[a.GroupID] {
			http.Error(w, "assignments must cover every group of the round exactly once", http.StatusBadRequest)
			return
		}
		seen[a.GroupID] = true
		assignments = append(assignments, schedule.GroupAssignment{GroupID: a.GroupID, CourseID: a.CourseID})
	}

	heats, err := s.schedule.ScheduleGroupHeats(tournamentID, roundID, assignments, startAt)
	if err != nil {
		if err == schedule.ErrCourseNotFound {
			http.Error(w, "unknown course", http.StatusBadRequest)
			return
		}
		if err == schedule.ErrGroupAlreadyScheduled {
			http.Error(w, "one or more groups already have a scheduled heat", http.StatusConflict)
			return
		}
		http.Error(w, "could not schedule round", http.StatusInternalServerError)
		return
	}

	resp := make([]heatResponse, len(heats))
	for i, h := range heats {
		resp[i] = heatToResponse(h)
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(map[string]any{"heats": resp})
}
```

- [ ] **Step 10: Write the HTTP tests**

Create `internal/server/schedule_round_test.go`:

```go
package server

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"tournamentstudio/internal/auth"
)

func TestScheduleRoundCreatesHeatsForEveryGroup(t *testing.T) {
	s := newTestServer(t)
	token := loginAs(t, s, "organizer1", "pw", auth.RoleOrganizer)
	tournamentID := createTestTournament(t, s, token)
	roundID, _ := createTestRound(t, s, token, tournamentID, [][]string{{"A1", "A2"}, {"B1", "B2"}})
	courseID := createTestCourse(t, s, token, tournamentID, "Course A", 300)

	groups, err := s.rounds.ListGroups(roundID)
	if err != nil {
		t.Fatalf("ListGroups: %v", err)
	}

	body, _ := json.Marshal(map[string]any{
		"assignments": []map[string]any{
			{"group_id": groups[0].ID, "course_id": courseID},
			{"group_id": groups[1].ID, "course_id": courseID},
		},
		"start_at": "2026-09-01T09:00:00Z",
	})
	req := httptest.NewRequest(http.MethodPost, fmt.Sprintf("/api/tournaments/%d/rounds/%d/schedule", tournamentID, roundID), bytes.NewReader(body))
	req.Header.Set("Authorization", "Bearer "+token)
	rec := httptest.NewRecorder()
	s.ServeHTTP(rec, req)
	if rec.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", rec.Code, rec.Body.String())
	}

	var resp struct {
		Heats []struct {
			ID           int64  `json:"id"`
			GroupID      int64  `json:"group_id"`
			PlannedStart string `json:"planned_start"`
			Status       string `json:"status"`
		} `json:"heats"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(resp.Heats) != 2 {
		t.Fatalf("expected 2 heats, got %d", len(resp.Heats))
	}
	first, err := time.Parse(time.RFC3339, resp.Heats[0].PlannedStart)
	if err != nil {
		t.Fatalf("parse first planned_start: %v", err)
	}
	second, err := time.Parse(time.RFC3339, resp.Heats[1].PlannedStart)
	if err != nil {
		t.Fatalf("parse second planned_start: %v", err)
	}
	if second.Sub(first) != 300*time.Second {
		t.Fatalf("expected heats 300s apart, got %v apart", second.Sub(first))
	}
	if resp.Heats[0].Status != "scheduled" {
		t.Fatalf("expected status scheduled, got %q", resp.Heats[0].Status)
	}
}

func TestScheduleRoundRejectsIncompleteAssignments(t *testing.T) {
	s := newTestServer(t)
	token := loginAs(t, s, "organizer1", "pw", auth.RoleOrganizer)
	tournamentID := createTestTournament(t, s, token)
	roundID, _ := createTestRound(t, s, token, tournamentID, [][]string{{"A1"}, {"B1"}})
	courseID := createTestCourse(t, s, token, tournamentID, "Course A", 300)

	groups, err := s.rounds.ListGroups(roundID)
	if err != nil {
		t.Fatalf("ListGroups: %v", err)
	}

	body, _ := json.Marshal(map[string]any{
		"assignments": []map[string]any{{"group_id": groups[0].ID, "course_id": courseID}},
	})
	req := httptest.NewRequest(http.MethodPost, fmt.Sprintf("/api/tournaments/%d/rounds/%d/schedule", tournamentID, roundID), bytes.NewReader(body))
	req.Header.Set("Authorization", "Bearer "+token)
	rec := httptest.NewRecorder()
	s.ServeHTTP(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestScheduleRoundRejectsDoubleScheduling(t *testing.T) {
	s := newTestServer(t)
	token := loginAs(t, s, "organizer1", "pw", auth.RoleOrganizer)
	tournamentID := createTestTournament(t, s, token)
	roundID, _ := createTestRound(t, s, token, tournamentID, [][]string{{"A1"}})
	courseID := createTestCourse(t, s, token, tournamentID, "Course A", 300)

	groups, err := s.rounds.ListGroups(roundID)
	if err != nil {
		t.Fatalf("ListGroups: %v", err)
	}
	body, _ := json.Marshal(map[string]any{
		"assignments": []map[string]any{{"group_id": groups[0].ID, "course_id": courseID}},
	})

	req1 := httptest.NewRequest(http.MethodPost, fmt.Sprintf("/api/tournaments/%d/rounds/%d/schedule", tournamentID, roundID), bytes.NewReader(body))
	req1.Header.Set("Authorization", "Bearer "+token)
	rec1 := httptest.NewRecorder()
	s.ServeHTTP(rec1, req1)
	if rec1.Code != http.StatusCreated {
		t.Fatalf("expected first schedule to succeed with 201, got %d: %s", rec1.Code, rec1.Body.String())
	}

	req2 := httptest.NewRequest(http.MethodPost, fmt.Sprintf("/api/tournaments/%d/rounds/%d/schedule", tournamentID, roundID), bytes.NewReader(body))
	req2.Header.Set("Authorization", "Bearer "+token)
	rec2 := httptest.NewRecorder()
	s.ServeHTTP(rec2, req2)
	if rec2.Code != http.StatusConflict {
		t.Fatalf("expected second schedule to be rejected with 409, got %d: %s", rec2.Code, rec2.Body.String())
	}
}

func TestScheduleRoundForbiddenForTimeEntry(t *testing.T) {
	s := newTestServer(t)
	organizerToken := loginAs(t, s, "organizer1", "pw", auth.RoleOrganizer)
	tournamentID := createTestTournament(t, s, organizerToken)
	roundID, _ := createTestRound(t, s, organizerToken, tournamentID, [][]string{{"A1"}})
	courseID := createTestCourse(t, s, organizerToken, tournamentID, "Course A", 300)
	groups, _ := s.rounds.ListGroups(roundID)

	timeEntryToken := loginAs(t, s, "timekeeper1", "pw", auth.RoleTimeEntry)
	body, _ := json.Marshal(map[string]any{
		"assignments": []map[string]any{{"group_id": groups[0].ID, "course_id": courseID}},
	})
	req := httptest.NewRequest(http.MethodPost, fmt.Sprintf("/api/tournaments/%d/rounds/%d/schedule", tournamentID, roundID), bytes.NewReader(body))
	req.Header.Set("Authorization", "Bearer "+timeEntryToken)
	rec := httptest.NewRecorder()
	s.ServeHTTP(rec, req)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("expected 403, got %d", rec.Code)
	}
}
```

- [ ] **Step 11: Run the tests to verify they pass**

Run: `go build ./... && go test ./internal/schedule/... ./internal/server/... -v`
Expected: PASS

- [ ] **Step 12: Run the full suite and commit**

Run: `go build ./... && go test ./... -count=1`
Expected: PASS, all packages.

```bash
git add internal/schedule internal/store/migrations/0011_heats.sql internal/store/store_test.go internal/server/handlers_schedule_round.go internal/server/schedule_round_test.go internal/server/server.go
git commit -m "feat: add heat domain and round-scheduling endpoint"
```

---

## Task 4: HeatResult domain and heat-results endpoint

This task adds the new heat-scoped results endpoint alongside Plan 2's
existing round-scoped one (`POST /rounds/{round_id}/results` keeps
working exactly as before). Task 5 retires the old endpoint and cuts
every caller over — the two endpoints briefly coexisting between this
task and Task 5 is expected, not a bug.

**Files:**
- Modify: `internal/schedule/model.go` (add `HeatResult`)
- Create: `internal/schedule/result.go`
- Create: `internal/schedule/result_test.go`
- Create: `internal/store/migrations/0012_heat_results.sql`
- Create: `internal/server/handlers_heat_results.go`
- Create: `internal/server/heat_results_test.go`
- Modify: `internal/server/server.go` (register 1 route)
- Modify: `internal/store/store_test.go` (migration count 11 -> 12)
- Modify: `internal/round/repo.go` (add `GetGroup`)

**Interfaces:**
- Consumes: `schedule.Heat`, `schedule.HeatStatus`, `schedule.ErrHeatNotFound`, `(*schedule.Repo).GetHeat` (Task 3); `s.rounds.GetRound`, `round.ErrNotFound`, `round.StatusClosed` (Plan 2, unchanged); `heatResponse`, `heatToResponse` (Task 3, reused, never redefined).
- Produces: `schedule.HeatResult{HeatID int64, TeamID string, TimeSeconds *float64, Status string}`; `(*schedule.Repo).SubmitHeatResults(heatID int64, results []HeatResult) error` (transactional, validate-then-write discipline enforced by the caller); `(*schedule.Repo).ListHeatResults(heatID int64) ([]HeatResult, error)`; `(*schedule.Repo).ListResultsForRound(roundID int64) ([]HeatResult, error)` (aggregates every heat belonging to a round); `(*schedule.Repo).SetHeatStatus(id int64, status HeatStatus) error`; `(*schedule.Repo).ListHeatsForRound(roundID int64) ([]Heat, error)`; `round.Repo.GetGroup(id int64) (*Group, error)`. Task 5 consumes `ListResultsForRound` to replace `round_common.go`'s source of truth. Task 9 consumes `ListHeatsForRound`.

- [ ] **Step 1: Write the migration**

Create `internal/store/migrations/0012_heat_results.sql`:

```sql
CREATE TABLE heat_results (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    heat_id INTEGER NOT NULL REFERENCES heats(id),
    team_id TEXT NOT NULL,
    time_seconds REAL,
    status TEXT,
    UNIQUE(heat_id, team_id)
);
```

- [ ] **Step 2: Bump the migration count**

Modify `internal/store/store_test.go`: change both `if count != 11` to `if count != 12`.

- [ ] **Step 3: Extend the model**

Modify `internal/schedule/model.go` — append. Unlike `Heat`, this one
carries `json` tags directly on the domain struct (matching `Course`'s
precedent from Task 2) because the heat-results handler broadcasts it
over WebSocket as-is later in this task — without tags here, that
broadcast would leak PascalCase field names and violate this plan's
snake_case convention:

```go
type HeatResult struct {
	HeatID      int64    `json:"heat_id"`
	TeamID      string   `json:"team_id"`
	TimeSeconds *float64 `json:"time_seconds"`
	Status      string   `json:"status"`
}
```

- [ ] **Step 4: Add `round.Repo.GetGroup`**

Modify `internal/round/repo.go` — add this method (it's the one small,
additive exception to "round.Group is untouched" this plan makes: the
heat-results handler needs to resolve a single group's `TeamIDs` from a
heat's `GroupID`, and no such single-group lookup existed before):

```go
func (r *Repo) GetGroup(id int64) (*Group, error) {
	row := r.db.QueryRow(`SELECT id, round_id, team_ids FROM groups WHERE id = ?`, id)
	var g Group
	var teamIDsJSON string
	if err := row.Scan(&g.ID, &g.RoundID, &teamIDsJSON); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	if err := json.Unmarshal([]byte(teamIDsJSON), &g.TeamIDs); err != nil {
		return nil, err
	}
	return &g, nil
}
```

Add a test for it to `internal/round/repo_test.go`, reusing the file's
existing `newTestStore`/`seedTournament` helpers (this package's actual
test-setup pattern — confirmed by reading the file, not assumed):

```go
func TestGetGroup(t *testing.T) {
	s := newTestStore(t)
	tournamentID := seedTournament(t, s)
	repo := NewRepo(s)

	_, groups, err := repo.CreateRound(tournamentID, 1, [][]string{{"t1", "t2"}})
	if err != nil {
		t.Fatalf("CreateRound: %v", err)
	}

	g, err := repo.GetGroup(groups[0].ID)
	if err != nil {
		t.Fatalf("GetGroup: %v", err)
	}
	if g.RoundID != groups[0].RoundID || len(g.TeamIDs) != 2 {
		t.Fatalf("unexpected group: %+v", g)
	}

	if _, err := repo.GetGroup(999); err != ErrNotFound {
		t.Fatalf("expected ErrNotFound, got %v", err)
	}
}
```

Run: `go test ./internal/round/... -run TestGetGroup -v`
Expected: PASS

- [ ] **Step 5: Write the failing repo test**

Create `internal/schedule/result_test.go`:

```go
package schedule

import "testing"

func setupHeat(t *testing.T) int64 {
	t.Helper()
	r := newTestRepo(t)
	return r.mustCreateTestHeat(t)
}

// mustCreateTestHeat creates a course and a group-heat scheduled onto it,
// for tests in this file that only need a valid heat ID to submit
// results against.
func (r *Repo) mustCreateTestHeat(t *testing.T) int64 {
	t.Helper()
	course, err := r.CreateCourse(1, "Course A", 300)
	if err != nil {
		t.Fatalf("CreateCourse: %v", err)
	}
	heats, err := r.ScheduleGroupHeats(1, 10, []GroupAssignment{{GroupID: 100, CourseID: course.ID}}, nil)
	if err != nil {
		t.Fatalf("ScheduleGroupHeats: %v", err)
	}
	return heats[0].ID
}

func TestSubmitAndListHeatResults(t *testing.T) {
	r := newTestRepo(t)
	heatID := r.mustCreateTestHeat(t)

	time1 := 100.5
	if err := r.SubmitHeatResults(heatID, []HeatResult{
		{TeamID: "t1", TimeSeconds: &time1},
		{TeamID: "t2", Status: "DNF"},
	}); err != nil {
		t.Fatalf("SubmitHeatResults: %v", err)
	}

	results, err := r.ListHeatResults(heatID)
	if err != nil {
		t.Fatalf("ListHeatResults: %v", err)
	}
	if len(results) != 2 {
		t.Fatalf("expected 2 results, got %d", len(results))
	}
}

func TestSubmitHeatResultsUpsertsOnResubmission(t *testing.T) {
	r := newTestRepo(t)
	heatID := r.mustCreateTestHeat(t)

	first := 130.0
	if err := r.SubmitHeatResults(heatID, []HeatResult{{TeamID: "t1", TimeSeconds: &first}}); err != nil {
		t.Fatalf("SubmitHeatResults: %v", err)
	}
	corrected := 124.11
	if err := r.SubmitHeatResults(heatID, []HeatResult{{TeamID: "t1", TimeSeconds: &corrected}}); err != nil {
		t.Fatalf("SubmitHeatResults (correction): %v", err)
	}

	results, err := r.ListHeatResults(heatID)
	if err != nil {
		t.Fatalf("ListHeatResults: %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("expected exactly 1 row after resubmission, got %d", len(results))
	}
	if *results[0].TimeSeconds != 124.11 {
		t.Fatalf("expected corrected time 124.11, got %v", *results[0].TimeSeconds)
	}
}

func TestListResultsForRoundAggregatesAcrossHeats(t *testing.T) {
	r := newTestRepo(t)
	course, err := r.CreateCourse(1, "Course A", 300)
	if err != nil {
		t.Fatalf("CreateCourse: %v", err)
	}
	heats, err := r.ScheduleGroupHeats(1, 10, []GroupAssignment{
		{GroupID: 100, CourseID: course.ID},
		{GroupID: 101, CourseID: course.ID},
	}, nil)
	if err != nil {
		t.Fatalf("ScheduleGroupHeats: %v", err)
	}

	t1 := 100.0
	t2 := 110.0
	if err := r.SubmitHeatResults(heats[0].ID, []HeatResult{{TeamID: "t1", TimeSeconds: &t1}}); err != nil {
		t.Fatalf("SubmitHeatResults heat 0: %v", err)
	}
	if err := r.SubmitHeatResults(heats[1].ID, []HeatResult{{TeamID: "t2", TimeSeconds: &t2}}); err != nil {
		t.Fatalf("SubmitHeatResults heat 1: %v", err)
	}

	// A result submitted to a different round's heat must not leak in.
	otherHeats, err := r.ScheduleGroupHeats(1, 11, []GroupAssignment{{GroupID: 200, CourseID: course.ID}}, nil)
	if err != nil {
		t.Fatalf("ScheduleGroupHeats (other round): %v", err)
	}
	t3 := 999.0
	if err := r.SubmitHeatResults(otherHeats[0].ID, []HeatResult{{TeamID: "t3", TimeSeconds: &t3}}); err != nil {
		t.Fatalf("SubmitHeatResults other round: %v", err)
	}

	results, err := r.ListResultsForRound(10)
	if err != nil {
		t.Fatalf("ListResultsForRound: %v", err)
	}
	if len(results) != 2 {
		t.Fatalf("expected 2 results for round 10, got %d: %+v", len(results), results)
	}
}

func TestSetHeatStatus(t *testing.T) {
	r := newTestRepo(t)
	heatID := r.mustCreateTestHeat(t)

	if err := r.SetHeatStatus(heatID, HeatClosed); err != nil {
		t.Fatalf("SetHeatStatus: %v", err)
	}
	h, err := r.GetHeat(heatID)
	if err != nil {
		t.Fatalf("GetHeat: %v", err)
	}
	if h.Status != HeatClosed {
		t.Fatalf("expected status closed, got %q", h.Status)
	}
}

func TestListHeatsForRound(t *testing.T) {
	r := newTestRepo(t)
	course, err := r.CreateCourse(1, "Course A", 300)
	if err != nil {
		t.Fatalf("CreateCourse: %v", err)
	}
	if _, err := r.ScheduleGroupHeats(1, 10, []GroupAssignment{
		{GroupID: 100, CourseID: course.ID},
		{GroupID: 101, CourseID: course.ID},
	}, nil); err != nil {
		t.Fatalf("ScheduleGroupHeats: %v", err)
	}
	if _, err := r.ScheduleGroupHeats(1, 11, []GroupAssignment{{GroupID: 200, CourseID: course.ID}}, nil); err != nil {
		t.Fatalf("ScheduleGroupHeats (round 11): %v", err)
	}

	heats, err := r.ListHeatsForRound(10)
	if err != nil {
		t.Fatalf("ListHeatsForRound: %v", err)
	}
	if len(heats) != 2 {
		t.Fatalf("expected 2 heats for round 10, got %d", len(heats))
	}
}
```

- [ ] **Step 6: Run the tests to verify they fail**

Run: `go test ./internal/schedule/... -v`
Expected: FAIL (compile error — `SubmitHeatResults`, `ListHeatResults`, `ListResultsForRound`, `SetHeatStatus`, `ListHeatsForRound` undefined)

- [ ] **Step 7: Implement the repo methods**

Create `internal/schedule/result.go`:

```go
package schedule

import "database/sql"

const submitHeatResultSQL = `INSERT INTO heat_results (heat_id, team_id, time_seconds, status) VALUES (?, ?, ?, ?)
	 ON CONFLICT(heat_id, team_id) DO UPDATE SET time_seconds = excluded.time_seconds, status = excluded.status`

// SubmitHeatResults writes every result in one transaction, rolling
// back on any error so a batch submission never partially commits.
func (r *Repo) SubmitHeatResults(heatID int64, results []HeatResult) error {
	tx, err := r.db.Begin()
	if err != nil {
		return err
	}
	for _, res := range results {
		if _, err := tx.Exec(submitHeatResultSQL, heatID, res.TeamID, res.TimeSeconds, nullIfEmptyResult(res.Status)); err != nil {
			tx.Rollback()
			return err
		}
	}
	return tx.Commit()
}

func nullIfEmptyResult(s string) any {
	if s == "" {
		return nil
	}
	return s
}

func (r *Repo) ListHeatResults(heatID int64) ([]HeatResult, error) {
	rows, err := r.db.Query(`SELECT heat_id, team_id, time_seconds, status FROM heat_results WHERE heat_id = ?`, heatID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanHeatResults(rows)
}

// ListResultsForRound aggregates every result across every heat
// belonging to roundID (via heats.round_id) -- the source of a round's
// ranking input from Task 5 onward.
func (r *Repo) ListResultsForRound(roundID int64) ([]HeatResult, error) {
	rows, err := r.db.Query(
		`SELECT hr.heat_id, hr.team_id, hr.time_seconds, hr.status
		 FROM heat_results hr
		 JOIN heats h ON h.id = hr.heat_id
		 WHERE h.round_id = ?`,
		roundID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanHeatResults(rows)
}

func scanHeatResults(rows *sql.Rows) ([]HeatResult, error) {
	var results []HeatResult
	for rows.Next() {
		var res HeatResult
		var timeSeconds sql.NullFloat64
		var status sql.NullString
		if err := rows.Scan(&res.HeatID, &res.TeamID, &timeSeconds, &status); err != nil {
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

func (r *Repo) SetHeatStatus(id int64, status HeatStatus) error {
	_, err := r.db.Exec(`UPDATE heats SET status = ? WHERE id = ?`, string(status), id)
	return err
}

func (r *Repo) ListHeatsForRound(roundID int64) ([]Heat, error) {
	rows, err := r.db.Query(
		`SELECT id, round_id, group_id, division_id, course_id, planned_start, status FROM heats WHERE round_id = ? ORDER BY id`,
		roundID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var heats []Heat
	for rows.Next() {
		h, err := scanHeatRow(rows)
		if err != nil {
			return nil, err
		}
		heats = append(heats, h)
	}
	return heats, rows.Err()
}
```

Modify `internal/schedule/heat.go` — `GetHeat` and `ListHeatsForRound` share
identical row-scanning logic. Extract it: replace the body of `GetHeat`
(everything after the `QueryRow` call) with a call to a new shared
`scanHeatRow` helper that takes anything satisfying a one-row `Scan`
call. Add this type and helper to `heat.go`:

```go
type rowScanner interface {
	Scan(dest ...any) error
}

func scanHeatRow(row rowScanner) (Heat, error) {
	var h Heat
	var groupID, divisionID sql.NullInt64
	var plannedStart, status string
	if err := row.Scan(&h.ID, &h.RoundID, &groupID, &divisionID, &h.CourseID, &plannedStart, &status); err != nil {
		return Heat{}, err
	}
	if groupID.Valid {
		v := groupID.Int64
		h.GroupID = &v
	}
	if divisionID.Valid {
		v := divisionID.Int64
		h.DivisionID = &v
	}
	t, err := time.Parse(time.RFC3339, plannedStart)
	if err != nil {
		return Heat{}, err
	}
	h.PlannedStart = t
	h.Status = HeatStatus(status)
	return h, nil
}
```

Then rewrite `GetHeat` to:

```go
func (r *Repo) GetHeat(id int64) (*Heat, error) {
	row := r.db.QueryRow(
		`SELECT id, round_id, group_id, division_id, course_id, planned_start, status FROM heats WHERE id = ?`,
		id,
	)
	h, err := scanHeatRow(row)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrHeatNotFound
		}
		return nil, err
	}
	return &h, nil
}
```

(`*sql.Row` and `*sql.Rows` both satisfy the one-method `rowScanner`
interface used above, so `scanHeatRow` works for both `QueryRow`'s
single row and `Query`'s per-row iteration in `ListHeatsForRound`.)

- [ ] **Step 8: Run the repo tests to verify they pass**

Run: `go test ./internal/round/... ./internal/schedule/... -v`
Expected: PASS

- [ ] **Step 9: Register the route**

Add this line inside `routes()` in `internal/server/server.go`, using the
existing `resultsWriter` var (already declared, already used by the old
round-level results route):

```go
	s.mux.Handle("POST /api/tournaments/{id}/heats/{heat_id}/results", resultsWriter(http.HandlerFunc(s.handleSubmitHeatResults)))
```

- [ ] **Step 10: Write the handler**

Create `internal/server/handlers_heat_results.go`:

```go
package server

import (
	"encoding/json"
	"net/http"
	"strconv"

	"tournamentstudio/internal/round"
	"tournamentstudio/internal/schedule"
)

type submitHeatResultsRequest map[string]struct {
	TimeSeconds *float64 `json:"time_seconds"`
	Status      string   `json:"status"`
}

var validHeatResultStatus = map[string]bool{
	"DNF": true,
	"DSQ": true,
	"DNS": true,
}

func (s *Server) handleSubmitHeatResults(w http.ResponseWriter, r *http.Request) {
	tournamentID, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		http.Error(w, "invalid tournament id", http.StatusBadRequest)
		return
	}
	heatID, err := strconv.ParseInt(r.PathValue("heat_id"), 10, 64)
	if err != nil {
		http.Error(w, "invalid heat id", http.StatusBadRequest)
		return
	}

	heat, err := s.schedule.GetHeat(heatID)
	if err != nil {
		if err == schedule.ErrHeatNotFound {
			http.Error(w, "heat not found", http.StatusNotFound)
			return
		}
		http.Error(w, "could not get heat", http.StatusInternalServerError)
		return
	}

	pr, err := s.rounds.GetRound(heat.RoundID)
	if err != nil {
		http.Error(w, "could not get round", http.StatusInternalServerError)
		return
	}
	if pr.TournamentID != tournamentID {
		http.Error(w, "heat not found", http.StatusNotFound)
		return
	}

	if heat.GroupID == nil {
		// Division-heat results are wired up in Task 7, once divisions
		// and their heats exist at all.
		http.Error(w, "division heat results are not yet supported", http.StatusNotImplemented)
		return
	}
	group, err := s.rounds.GetGroup(*heat.GroupID)
	if err != nil {
		http.Error(w, "could not get group", http.StatusInternalServerError)
		return
	}

	var req submitHeatResultsRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}

	results := make([]schedule.HeatResult, 0, len(req))
	for teamID, entry := range req {
		if entry.TimeSeconds == nil && entry.Status == "" {
			http.Error(w, "each result must have either time_seconds or status", http.StatusBadRequest)
			return
		}
		if entry.Status != "" && !validHeatResultStatus[entry.Status] {
			http.Error(w, "status must be one of DNF, DSQ, DNS", http.StatusBadRequest)
			return
		}
		results = append(results, schedule.HeatResult{
			TeamID:      teamID,
			TimeSeconds: entry.TimeSeconds,
			Status:      entry.Status,
		})
	}

	if err := s.schedule.SubmitHeatResults(heatID, results); err != nil {
		http.Error(w, "could not save results", http.StatusInternalServerError)
		return
	}

	validTeamIDs := make(map[string]bool, len(group.TeamIDs))
	for _, id := range group.TeamIDs {
		validTeamIDs[id] = true
	}

	allResults, err := s.schedule.ListHeatResults(heatID)
	if err != nil {
		http.Error(w, "could not list results", http.StatusInternalServerError)
		return
	}
	haveResult := make(map[string]bool, len(allResults))
	for _, res := range allResults {
		haveResult[res.TeamID] = true
	}

	heatComplete := len(validTeamIDs) > 0
	for teamID := range validTeamIDs {
		if !haveResult[teamID] {
			heatComplete = false
			break
		}
	}

	if heatComplete {
		if err := s.schedule.SetHeatStatus(heatID, schedule.HeatClosed); err != nil {
			http.Error(w, "could not close heat", http.StatusInternalServerError)
			return
		}

		roundHeats, err := s.schedule.ListHeatsForRound(heat.RoundID)
		if err != nil {
			http.Error(w, "could not list round heats", http.StatusInternalServerError)
			return
		}
		roundComplete := true
		for _, h := range roundHeats {
			if h.GroupID != nil && h.Status != schedule.HeatClosed {
				roundComplete = false
				break
			}
		}
		if roundComplete {
			if err := s.rounds.SetStatus(heat.RoundID, round.StatusClosed); err != nil {
				http.Error(w, "could not close round", http.StatusInternalServerError)
				return
			}
		}
	}

	msg, _ := json.Marshal(map[string]any{
		"type":    "result_submitted",
		"heat_id": heatID,
		"results": allResults,
	})
	s.hub.broadcast(tournamentID, msg)

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]int{"results_recorded": len(allResults)})
}
```

- [ ] **Step 11: Write the HTTP tests**

Create `internal/server/heat_results_test.go`:

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

// scheduleTestRound schedules every group of roundID onto a single
// freshly created course and returns a map from that group's ID to its
// new heat's ID.
func scheduleTestRound(t *testing.T, s *Server, token string, tournamentID, roundID int64) map[int64]int64 {
	t.Helper()
	courseID := createTestCourse(t, s, token, tournamentID, "Course A", 300)
	groups, err := s.rounds.ListGroups(roundID)
	if err != nil {
		t.Fatalf("ListGroups: %v", err)
	}
	assignments := make([]map[string]any, len(groups))
	for i, g := range groups {
		assignments[i] = map[string]any{"group_id": g.ID, "course_id": courseID}
	}
	body, _ := json.Marshal(map[string]any{"assignments": assignments})
	req := httptest.NewRequest(http.MethodPost, fmt.Sprintf("/api/tournaments/%d/rounds/%d/schedule", tournamentID, roundID), bytes.NewReader(body))
	req.Header.Set("Authorization", "Bearer "+token)
	rec := httptest.NewRecorder()
	s.ServeHTTP(rec, req)
	if rec.Code != http.StatusCreated {
		t.Fatalf("schedule round: expected 201, got %d: %s", rec.Code, rec.Body.String())
	}
	var resp struct {
		Heats []struct {
			ID      int64 `json:"id"`
			GroupID int64 `json:"group_id"`
		} `json:"heats"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode schedule response: %v", err)
	}
	heatByGroup := make(map[int64]int64, len(resp.Heats))
	for _, h := range resp.Heats {
		heatByGroup[h.GroupID] = h.ID
	}
	return heatByGroup
}

func submitHeatResults(t *testing.T, s *Server, token string, tournamentID, heatID int64, body map[string]any) *httptest.ResponseRecorder {
	t.Helper()
	b, _ := json.Marshal(body)
	req := httptest.NewRequest(http.MethodPost, fmt.Sprintf("/api/tournaments/%d/heats/%d/results", tournamentID, heatID), bytes.NewReader(b))
	req.Header.Set("Authorization", "Bearer "+token)
	rec := httptest.NewRecorder()
	s.ServeHTTP(rec, req)
	return rec
}

func TestSubmitHeatResultsClosesHeatAndCascadesToRound(t *testing.T) {
	s := newTestServer(t)
	token := loginAs(t, s, "organizer1", "pw", auth.RoleOrganizer)
	tournamentID := createTestTournament(t, s, token)
	roundID, ids := createTestRound(t, s, token, tournamentID, [][]string{{"t1", "t2"}})
	heatByGroup := scheduleTestRound(t, s, token, tournamentID, roundID)
	var heatID int64
	for _, id := range heatByGroup {
		heatID = id
	}

	partial := submitHeatResults(t, s, token, tournamentID, heatID, map[string]any{
		ids["t1"]: map[string]any{"time_seconds": 100.0},
	})
	if partial.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", partial.Code, partial.Body.String())
	}
	h, err := s.schedule.GetHeat(heatID)
	if err != nil {
		t.Fatalf("GetHeat: %v", err)
	}
	if h.Status != "scheduled" {
		t.Fatalf("expected heat still scheduled after partial submission, got %s", h.Status)
	}

	remaining := submitHeatResults(t, s, token, tournamentID, heatID, map[string]any{
		ids["t2"]: map[string]any{"status": "DNF"},
	})
	if remaining.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", remaining.Code, remaining.Body.String())
	}
	h2, err := s.schedule.GetHeat(heatID)
	if err != nil {
		t.Fatalf("GetHeat: %v", err)
	}
	if h2.Status != "closed" {
		t.Fatalf("expected heat closed after all results submitted, got %s", h2.Status)
	}
	pr, err := s.rounds.GetRound(roundID)
	if err != nil {
		t.Fatalf("GetRound: %v", err)
	}
	if pr.Status != round.StatusClosed {
		t.Fatalf("expected round closed once its only heat closed, got %s", pr.Status)
	}
}

func TestSubmitHeatResultsRejectsInvalidStatus(t *testing.T) {
	s := newTestServer(t)
	token := loginAs(t, s, "organizer1", "pw", auth.RoleOrganizer)
	tournamentID := createTestTournament(t, s, token)
	roundID, ids := createTestRound(t, s, token, tournamentID, [][]string{{"t1"}})
	heatByGroup := scheduleTestRound(t, s, token, tournamentID, roundID)
	var heatID int64
	for _, id := range heatByGroup {
		heatID = id
	}

	rec := submitHeatResults(t, s, token, tournamentID, heatID, map[string]any{
		ids["t1"]: map[string]any{"status": "dnf"},
	})
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for lowercase status, got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestSubmitHeatResultsInvalidEntryLeavesNothingCommitted(t *testing.T) {
	s := newTestServer(t)
	token := loginAs(t, s, "organizer1", "pw", auth.RoleOrganizer)
	tournamentID := createTestTournament(t, s, token)
	roundID, ids := createTestRound(t, s, token, tournamentID, [][]string{{"t1", "t2"}})
	heatByGroup := scheduleTestRound(t, s, token, tournamentID, roundID)
	var heatID int64
	for _, id := range heatByGroup {
		heatID = id
	}

	rec := submitHeatResults(t, s, token, tournamentID, heatID, map[string]any{
		ids["t1"]: map[string]any{"time_seconds": 100.0},
		ids["t2"]: map[string]any{"status": "not-a-real-status"},
	})
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d: %s", rec.Code, rec.Body.String())
	}

	results, err := s.schedule.ListHeatResults(heatID)
	if err != nil {
		t.Fatalf("ListHeatResults: %v", err)
	}
	if len(results) != 0 {
		t.Fatalf("expected zero results committed after a rejected batch, got %d", len(results))
	}
}

func TestSubmitHeatResultsAllowsTimeEntryRole(t *testing.T) {
	s := newTestServer(t)
	organizerToken := loginAs(t, s, "organizer1", "pw", auth.RoleOrganizer)
	tournamentID := createTestTournament(t, s, organizerToken)
	roundID, ids := createTestRound(t, s, organizerToken, tournamentID, [][]string{{"t1"}})
	heatByGroup := scheduleTestRound(t, s, organizerToken, tournamentID, roundID)
	var heatID int64
	for _, id := range heatByGroup {
		heatID = id
	}

	timeEntryToken := loginAs(t, s, "timekeeper1", "pw", auth.RoleTimeEntry)
	rec := submitHeatResults(t, s, timeEntryToken, tournamentID, heatID, map[string]any{
		ids["t1"]: map[string]any{"time_seconds": 100.0},
	})
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200 for time_entry role, got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestSubmitHeatResultsForbiddenForSpectator(t *testing.T) {
	s := newTestServer(t)
	organizerToken := loginAs(t, s, "organizer1", "pw", auth.RoleOrganizer)
	tournamentID := createTestTournament(t, s, organizerToken)
	roundID, ids := createTestRound(t, s, organizerToken, tournamentID, [][]string{{"t1"}})
	heatByGroup := scheduleTestRound(t, s, organizerToken, tournamentID, roundID)
	var heatID int64
	for _, id := range heatByGroup {
		heatID = id
	}

	spectatorToken := loginAs(t, s, "spectator1", "pw", auth.RoleSpectator)
	rec := submitHeatResults(t, s, spectatorToken, tournamentID, heatID, map[string]any{
		ids["t1"]: map[string]any{"time_seconds": 100.0},
	})
	if rec.Code != http.StatusForbidden {
		t.Fatalf("expected 403, got %d", rec.Code)
	}
}

func TestSubmitHeatResultsBroadcasts(t *testing.T) {
	s := newTestServer(t)
	token := loginAs(t, s, "organizer1", "pw", auth.RoleOrganizer)
	tournamentID := createTestTournament(t, s, token)
	roundID, ids := createTestRound(t, s, token, tournamentID, [][]string{{"t1"}})
	heatByGroup := scheduleTestRound(t, s, token, tournamentID, roundID)
	var heatID int64
	for _, id := range heatByGroup {
		heatID = id
	}

	httpServer := httptest.NewServer(s)
	t.Cleanup(httpServer.Close)
	conn := dialWS(t, httpServer, tournamentID, token)

	rec := submitHeatResults(t, s, token, tournamentID, heatID, map[string]any{
		ids["t1"]: map[string]any{"time_seconds": 100.0},
	})
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}

	readCtx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	_, msg, err := conn.Read(readCtx)
	if err != nil {
		t.Fatalf("expected a broadcast message: %v", err)
	}
	var evt struct {
		Type   string `json:"type"`
		HeatID int64  `json:"heat_id"`
	}
	if err := json.Unmarshal(msg, &evt); err != nil {
		t.Fatalf("decode broadcast: %v", err)
	}
	if evt.Type != "result_submitted" || evt.HeatID != heatID {
		t.Fatalf("unexpected broadcast event: %+v", evt)
	}
}
```

Add `"context"` and `"time"` to this file's imports (used by
`TestSubmitHeatResultsBroadcasts`).

- [ ] **Step 12: Run the tests to verify they pass**

Run: `go build ./... && go test ./internal/round/... ./internal/schedule/... ./internal/server/... -v`
Expected: PASS

- [ ] **Step 13: Run the full suite and commit**

Run: `go build ./... && go test ./... -count=1`
Expected: PASS, all packages.

```bash
git add internal/schedule internal/store/migrations/0012_heat_results.sql internal/store/store_test.go internal/round/repo.go internal/round/repo_test.go internal/server/handlers_heat_results.go internal/server/heat_results_test.go internal/server/server.go
git commit -m "feat: add heat results domain and endpoint"
```

---

## Task 5: Retire the round-level results endpoint

This task cuts every caller over from Plan 2's round-scoped results
endpoint to Task 4's heat-scoped one, then deletes the old endpoint and
its now-dead repo code entirely. No dual-write, no compatibility shim —
after this task, `POST /rounds/{round_id}/results` no longer exists.

**Files:**
- Delete: `internal/server/handlers_round_results.go`
- Modify: `internal/server/round_results_test.go` (keep only the shared
  helpers; delete every test function — Task 4's `heat_results_test.go`
  already covers the same behaviors at heat scope)
- Modify: `internal/server/round_common.go` (source `resultsByTeam` from
  `schedule.Repo.ListResultsForRound` instead of `round.Repo.ListResults`)
- Modify: `internal/server/round_next_test.go` (3 call sites)
- Modify: `internal/server/round_divisions_test.go` (1 call site)
- Modify: `internal/server/server.go` (remove the old route)
- Modify: `internal/round/repo.go` (remove `SubmitResult`, `SubmitResults`,
  `ListResults`, `submitResultSQL`, `nullIfEmpty`)
- Modify: `internal/round/repo_test.go` (remove the corresponding tests)
- Modify: `internal/round/model.go` (remove `Result`, now fully unused)

**Interfaces:**
- Consumes: `(*schedule.Repo).ListResultsForRound` (Task 4); `scheduleTestRound`, `submitHeatResults` (Task 4's test helpers in `heat_results_test.go`, package `server` — reused, not redefined).
- Produces: `submitHeatResultsForRound(t *testing.T, s *Server, token string, tournamentID, roundID int64, ids map[string]string, pairs ...any)` — the new drop-in test helper every round-level-results call site in this package switches to. No other task calls it (it exists purely to keep `round_next_test.go`/`round_divisions_test.go` readable after the cutover).

- [ ] **Step 1: Add the replacement test helper**

Modify `internal/server/round_results_test.go`. Keep everything through
`mapLabels` (the shared helpers other test files import: `createRealTeams`,
`uniqueLabels`, `createTestRound`, `resultsBodyFor`, `mapLabels`) exactly
as-is. Delete every test function below them
(`TestSubmitResultsClosesRoundWhenComplete` through
`TestSubmitResultsForbiddenForSpectator` — everything from that point to
the end of the file). In their place, add this one new helper:

```go
// submitHeatResultsForRound schedules every group of roundID onto a
// single freshly created course (via scheduleTestRound, defined in
// heat_results_test.go), then submits results for each of that
// course's heats via the real per-heat results endpoint, keyed by the
// label -> real-team-ID map createTestRound returns. It's the drop-in
// replacement for the old round-level "POST .../rounds/{id}/results"
// call every test in this package used before this task retired that
// endpoint. pairs is a flat label/entry sequence, e.g. "t1",
// map[string]any{"time_seconds": 100.0}, "t2", map[string]any{"status": "DNF"}.
func submitHeatResultsForRound(t *testing.T, s *Server, token string, tournamentID, roundID int64, ids map[string]string, pairs ...any) {
	t.Helper()

	heatByGroup := scheduleTestRound(t, s, token, tournamentID, roundID)
	groups, err := s.rounds.ListGroups(roundID)
	if err != nil {
		t.Fatalf("ListGroups: %v", err)
	}
	groupOf := make(map[string]int64, len(groups))
	for _, g := range groups {
		for _, teamID := range g.TeamIDs {
			groupOf[teamID] = g.ID
		}
	}

	byGroup := make(map[int64]map[string]any)
	for i := 0; i < len(pairs); i += 2 {
		label := pairs[i].(string)
		entry := pairs[i+1]
		teamID, ok := ids[label]
		if !ok {
			t.Fatalf("unknown label %q", label)
		}
		groupID, ok := groupOf[teamID]
		if !ok {
			t.Fatalf("team %q (label %q) is not in any of round %d's groups", teamID, label, roundID)
		}
		if byGroup[groupID] == nil {
			byGroup[groupID] = make(map[string]any)
		}
		byGroup[groupID][teamID] = entry
	}

	for groupID, body := range byGroup {
		heatID := heatByGroup[groupID]
		rec := submitHeatResults(t, s, token, tournamentID, heatID, body)
		if rec.Code != http.StatusOK {
			t.Fatalf("submit heat results for group %d: expected 200, got %d: %s", groupID, rec.Code, rec.Body.String())
		}
	}
}
```

The file's final state is: package/import block, `createRealTeams`,
`uniqueLabels`, `createTestRound`, `resultsBodyFor`, `mapLabels`, then
this new `submitHeatResultsForRound` — no `Test...` functions remain in
this file. (`resultsBodyFor` becomes unused by this file itself once its
own tests are gone, but stays exported-within-package for
`round_next_test.go`/`round_divisions_test.go`'s existing calls — leave
it in place, do not delete it.)

- [ ] **Step 2: Update `round_next_test.go`'s three call sites**

Modify `internal/server/round_next_test.go`. In
`TestNextRoundComputesReseededGroups`, replace this block:

```go
	resultsBody, _ := json.Marshal(resultsBodyFor(ids,
		"A1", map[string]any{"time_seconds": 120.0},
		"A2", map[string]any{"time_seconds": 121.0},
		"A3", map[string]any{"time_seconds": 122.0},
		"A4", map[string]any{"time_seconds": 123.0},
		"B1", map[string]any{"time_seconds": 120.0},
		"B2", map[string]any{"time_seconds": 121.0},
		"B3", map[string]any{"time_seconds": 122.0},
		"B4", map[string]any{"time_seconds": 123.0},
	))
	resultsReq := httptest.NewRequest(http.MethodPost, fmt.Sprintf("/api/tournaments/%d/rounds/%d/results", tournamentID, roundID), bytes.NewReader(resultsBody))
	resultsReq.Header.Set("Authorization", "Bearer "+token)
	resultsRec := httptest.NewRecorder()
	s.ServeHTTP(resultsRec, resultsReq)
	if resultsRec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", resultsRec.Code, resultsRec.Body.String())
	}
```

with:

```go
	submitHeatResultsForRound(t, s, token, tournamentID, roundID, ids,
		"A1", map[string]any{"time_seconds": 120.0},
		"A2", map[string]any{"time_seconds": 121.0},
		"A3", map[string]any{"time_seconds": 122.0},
		"A4", map[string]any{"time_seconds": 123.0},
		"B1", map[string]any{"time_seconds": 120.0},
		"B2", map[string]any{"time_seconds": 121.0},
		"B3", map[string]any{"time_seconds": 122.0},
		"B4", map[string]any{"time_seconds": 123.0},
	)
```

In `TestFullRoundLifecycleWithRealTeams`, replace this block:

```go
	resultsBody, _ := json.Marshal(map[string]any{
		team1: map[string]any{"time_seconds": 118.4},
		team2: map[string]any{"time_seconds": 121.9},
	})
	resultsReq := httptest.NewRequest(http.MethodPost, fmt.Sprintf("/api/tournaments/%d/rounds/%d/results", tournamentID, createdRound.ID), bytes.NewReader(resultsBody))
	resultsReq.Header.Set("Authorization", "Bearer "+token)
	resultsRec := httptest.NewRecorder()
	s.ServeHTTP(resultsRec, resultsReq)
	if resultsRec.Code != http.StatusOK {
		t.Fatalf("submit results: expected 200, got %d: %s", resultsRec.Code, resultsRec.Body.String())
	}
```

with:

```go
	submitHeatResultsForRound(t, s, token, tournamentID, createdRound.ID,
		map[string]string{"team1": team1, "team2": team2},
		"team1", map[string]any{"time_seconds": 118.4},
		"team2", map[string]any{"time_seconds": 121.9},
	)
```

In `TestNextRoundIsIdempotentAgainstDoubleSubmission`, replace this block:

```go
	resultsBody, _ := json.Marshal(resultsBodyFor(ids,
		"t1", map[string]any{"time_seconds": 100.0},
		"t2", map[string]any{"time_seconds": 110.0},
	))
	resultsReq := httptest.NewRequest(http.MethodPost, fmt.Sprintf("/api/tournaments/%d/rounds/%d/results", tournamentID, roundID), bytes.NewReader(resultsBody))
	resultsReq.Header.Set("Authorization", "Bearer "+token)
	resultsRec := httptest.NewRecorder()
	s.ServeHTTP(resultsRec, resultsReq)
	if resultsRec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", resultsRec.Code, resultsRec.Body.String())
	}
```

with:

```go
	submitHeatResultsForRound(t, s, token, tournamentID, roundID, ids,
		"t1", map[string]any{"time_seconds": 100.0},
		"t2", map[string]any{"time_seconds": 110.0},
	)
```

After these three edits, `round_next_test.go` no longer uses `bytes` or
`fmt` for the deleted blocks specifically, but both packages are still
used elsewhere in the file (`fmt.Sprintf` for the `/next` and
`createTestTeamHTTP` requests, `bytes.NewReader` for the round-creation
request in `TestFullRoundLifecycleWithRealTeams`) — leave the import
block unchanged; do not remove any imports from this file.

- [ ] **Step 3: Update `round_divisions_test.go`'s one call site**

Modify `internal/server/round_divisions_test.go`. Replace this block:

```go
	resultsBody, _ := json.Marshal(resultsBodyFor(ids,
		"t1", map[string]any{"time_seconds": 100.0},
		"t2", map[string]any{"time_seconds": 110.0},
		"t3", map[string]any{"time_seconds": 200.0},
		"t4", map[string]any{"time_seconds": 210.0},
		"t5", map[string]any{"time_seconds": 105.0},
		"t6", map[string]any{"time_seconds": 115.0},
		"t7", map[string]any{"time_seconds": 205.0},
		"t8", map[string]any{"time_seconds": 215.0},
	))
	resultsReq := httptest.NewRequest(http.MethodPost, fmt.Sprintf("/api/tournaments/%d/rounds/%d/results", tournamentID, roundID), bytes.NewReader(resultsBody))
	resultsReq.Header.Set("Authorization", "Bearer "+token)
	resultsRec := httptest.NewRecorder()
	s.ServeHTTP(resultsRec, resultsReq)
```

with:

```go
	submitHeatResultsForRound(t, s, token, tournamentID, roundID, ids,
		"t1", map[string]any{"time_seconds": 100.0},
		"t2", map[string]any{"time_seconds": 110.0},
		"t3", map[string]any{"time_seconds": 200.0},
		"t4", map[string]any{"time_seconds": 210.0},
		"t5", map[string]any{"time_seconds": 105.0},
		"t6", map[string]any{"time_seconds": 115.0},
		"t7", map[string]any{"time_seconds": 205.0},
		"t8", map[string]any{"time_seconds": 215.0},
	)
```

This file's remaining code (`bytes`, `fmt`, `http`, `httptest` imports)
is still used by the `/divisions` request built right after this block —
leave the import block unchanged.

- [ ] **Step 4: Delete the old endpoint**

Delete `internal/server/handlers_round_results.go` entirely.

- [ ] **Step 5: Remove the old route**

Modify `internal/server/server.go` — remove this one line from `routes()`:

```go
	s.mux.Handle("POST /api/tournaments/{id}/rounds/{round_id}/results", resultsWriter(http.HandlerFunc(s.handleSubmitResults)))
```

`resultsWriter` stays declared and used — Task 4's heat-results route
already uses it.

- [ ] **Step 6: Cut `round_common.go` over to heat-scoped results**

Modify `internal/server/round_common.go` — change the `resultsByTeam`
field's type and its one call site. Replace:

```go
type roundContext struct {
	tournamentID  int64
	round         *round.PrePhaseRound
	tournament    *tournament.Tournament
	ttp           *plugin.TournamentTypePlugin
	groups        []round.Group
	resultsByTeam map[string]round.Result
}
```

with:

```go
type roundContext struct {
	tournamentID  int64
	round         *round.PrePhaseRound
	tournament    *tournament.Tournament
	ttp           *plugin.TournamentTypePlugin
	groups        []round.Group
	resultsByTeam map[string]schedule.HeatResult
}
```

and replace:

```go
	results, err := s.rounds.ListResults(roundID)
	if err != nil {
		http.Error(w, "could not list results", http.StatusInternalServerError)
		return nil, false
	}
	resultsByTeam := make(map[string]round.Result, len(results))
```

with:

```go
	results, err := s.schedule.ListResultsForRound(roundID)
	if err != nil {
		http.Error(w, "could not list results", http.StatusInternalServerError)
		return nil, false
	}
	resultsByTeam := make(map[string]schedule.HeatResult, len(results))
```

Add `"tournamentstudio/internal/schedule"` to this file's imports.
`schedule.HeatResult` has the identical field shape `round.Result` did
(`TeamID string`, `TimeSeconds *float64`, `Status string`), so
`handlers_round_next.go` and `handlers_round_divisions.go` — which only
read `ctx.resultsByTeam[teamID].TimeSeconds`/`.Status` — need no changes
at all.

- [ ] **Step 7: Trim `round.Repo`**

Modify `internal/round/repo.go` — remove `SubmitResult`, `SubmitResults`,
`submitResultSQL`, `ListResults`, and `nullIfEmpty` (everything from
`const submitResultSQL = ...` through the final `nullIfEmpty` function
at the end of the file). The file's final content, in full:

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

// CountRounds returns how many rounds exist for tournamentID with the
// given round number. Under normal operation this is always 0 or 1 (the
// unique index added in migration 0009 enforces that); it's exposed
// mainly so tests can confirm a duplicate "next round" request was fully
// rejected rather than merely returning an error after partially
// succeeding.
func (r *Repo) CountRounds(tournamentID int64, roundNumber int) (int, error) {
	var count int
	err := r.db.QueryRow(
		`SELECT COUNT(*) FROM pre_phase_rounds WHERE tournament_id = ? AND round_number = ?`,
		tournamentID, roundNumber,
	).Scan(&count)
	return count, err
}

// RoundExists reports whether a round with the given round number already
// exists for the tournament, so callers can guard against creating a
// duplicate (e.g. a double-submitted "next round" request).
func (r *Repo) RoundExists(tournamentID int64, roundNumber int) (bool, error) {
	count, err := r.CountRounds(tournamentID, roundNumber)
	if err != nil {
		return false, err
	}
	return count > 0, nil
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

func (r *Repo) GetGroup(id int64) (*Group, error) {
	row := r.db.QueryRow(`SELECT id, round_id, team_ids FROM groups WHERE id = ?`, id)
	var g Group
	var teamIDsJSON string
	if err := row.Scan(&g.ID, &g.RoundID, &teamIDsJSON); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	if err := json.Unmarshal([]byte(teamIDsJSON), &g.TeamIDs); err != nil {
		return nil, err
	}
	return &g, nil
}

func (r *Repo) SetStatus(roundID int64, status Status) error {
	_, err := r.db.Exec(`UPDATE pre_phase_rounds SET status = ? WHERE id = ?`, string(status), roundID)
	return err
}
```

- [ ] **Step 8: Remove `round.Result` and its tests**

Modify `internal/round/model.go` — remove the `Result` struct (the file's
final content is `Status`, `PrePhaseRound`, and `Group` only; `Result` is
now unused anywhere in the codebase).

Modify `internal/round/repo_test.go` — delete
`TestSubmitAndListResults`, `TestSubmitResultsWritesAllRowsInOneTransaction`,
and `TestSubmitResultUpsertsOnResubmission` in full (everything from
`func TestSubmitAndListResults` to the end of the file — these tested
the methods just removed from `repo.go`). `TestGetGroup`, added in Task
4, stays.

- [ ] **Step 9: Run the tests to verify everything still passes**

Run: `go build ./...`
Expected: succeeds — if it doesn't, the most likely cause is a leftover
reference to `round.Result`, `s.rounds.SubmitResult(s)`, or
`s.rounds.ListResults` somewhere not covered above; grep for those three
names across `internal/` and fix any remaining call site the same way
Step 6 fixed `round_common.go`.

Run: `go test ./internal/round/... ./internal/schedule/... ./internal/server/... -v`
Expected: PASS — in particular, confirm every test in `round_next_test.go`
and `round_divisions_test.go` (including `TestNextRoundComputesReseededGroups`'s
exact-composition assertions and `TestComputeDivisionsSplitsRankedTeams`'s
flat-ranking assertions) still passes unchanged in outcome, now driven
through heat-scoped result submission instead of the old round-scoped
endpoint.

- [ ] **Step 10: Run the full suite and commit**

Run: `go build ./... && go test ./... -count=1`
Expected: PASS, all packages.

```bash
git add internal/server/round_results_test.go internal/server/round_common.go internal/server/round_next_test.go internal/server/round_divisions_test.go internal/server/server.go internal/round/repo.go internal/round/repo_test.go internal/round/model.go
git rm internal/server/handlers_round_results.go
git commit -m "refactor: retire round-level results endpoint in favor of heat-scoped results"
```

---

## Task 6: Division persistence

**Files:**
- Modify: `internal/schedule/model.go` (add `Division`)
- Create: `internal/schedule/division.go`
- Create: `internal/schedule/division_test.go`
- Create: `internal/store/migrations/0013_divisions.sql`
- Modify: `internal/server/handlers_round_divisions.go` (persist instead of only computing)
- Modify: `internal/server/round_divisions_test.go` (assert persistence)
- Modify: `internal/store/store_test.go` (migration count 12 -> 13)

**Interfaces:**
- Consumes: `ctx.tournamentID`, `ctx.round.ID` (from `loadClosedRoundContext`, Plan 2's `round_common.go`); `plugin.Cut`, `plugin.Division{Name, TeamIDs}`, `(*plugin.TournamentTypePlugin).DivisionCuts` (Plan 2, unchanged).
- Produces: `schedule.Division{ID, TournamentID, RoundID int64, Name string, TeamIDs []string}`; `schedule.NewDivision{Name string, TeamIDs []string}`; `(*schedule.Repo).CreateDivisions(tournamentID, roundID int64, divisions []NewDivision) ([]Division, error)` (transactional); `(*schedule.Repo).ListDivisionsForRound(roundID int64) ([]Division, error)`; `(*schedule.Repo).GetDivision(id int64) (*Division, error)`; `schedule.ErrDivisionNotFound`. `divisionResponse{ID int64, Name string, TeamIDs []string}` in package `server` — Task 7 reuses `schedule.Division`/`GetDivision`/`ListDivisionsForRound` directly, never redefines them.

- [ ] **Step 1: Write the migration**

Create `internal/store/migrations/0013_divisions.sql`:

```sql
CREATE TABLE divisions (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    tournament_id INTEGER NOT NULL REFERENCES tournaments(id),
    round_id INTEGER NOT NULL REFERENCES pre_phase_rounds(id),
    name TEXT NOT NULL,
    team_ids TEXT NOT NULL
);
```

- [ ] **Step 2: Bump the migration count**

Modify `internal/store/store_test.go`: change both `if count != 12` to `if count != 13`.

- [ ] **Step 3: Extend the model**

Modify `internal/schedule/model.go` — append:

```go
type Division struct {
	ID           int64
	TournamentID int64
	RoundID      int64
	Name         string
	TeamIDs      []string
}
```

- [ ] **Step 4: Write the failing repo test**

Create `internal/schedule/division_test.go`:

```go
package schedule

import "testing"

func TestCreateAndListDivisionsForRound(t *testing.T) {
	r := newTestRepo(t)

	created, err := r.CreateDivisions(1, 10, []NewDivision{
		{Name: "Gold Final", TeamIDs: []string{"t1", "t2"}},
		{Name: "Final", TeamIDs: []string{"t3", "t4", "t5"}},
	})
	if err != nil {
		t.Fatalf("CreateDivisions: %v", err)
	}
	if len(created) != 2 {
		t.Fatalf("expected 2 divisions, got %d", len(created))
	}
	if created[0].ID == 0 || created[1].ID == 0 {
		t.Fatalf("expected non-zero IDs, got %+v", created)
	}
	if created[0].RoundID != 10 || created[0].TournamentID != 1 {
		t.Fatalf("unexpected division: %+v", created[0])
	}

	listed, err := r.ListDivisionsForRound(10)
	if err != nil {
		t.Fatalf("ListDivisionsForRound: %v", err)
	}
	if len(listed) != 2 {
		t.Fatalf("expected 2 divisions listed, got %d", len(listed))
	}
	if len(listed[0].TeamIDs) != 2 || len(listed[1].TeamIDs) != 3 {
		t.Fatalf("unexpected team_ids round trip: %+v", listed)
	}
}

func TestGetDivisionNotFound(t *testing.T) {
	r := newTestRepo(t)
	if _, err := r.GetDivision(999); err != ErrDivisionNotFound {
		t.Fatalf("expected ErrDivisionNotFound, got %v", err)
	}
}

func TestListDivisionsForRoundScopesToRound(t *testing.T) {
	r := newTestRepo(t)
	if _, err := r.CreateDivisions(1, 10, []NewDivision{{Name: "Final", TeamIDs: []string{"t1"}}}); err != nil {
		t.Fatalf("CreateDivisions round 10: %v", err)
	}
	if _, err := r.CreateDivisions(1, 11, []NewDivision{{Name: "Final", TeamIDs: []string{"t2"}}}); err != nil {
		t.Fatalf("CreateDivisions round 11: %v", err)
	}

	listed, err := r.ListDivisionsForRound(10)
	if err != nil {
		t.Fatalf("ListDivisionsForRound: %v", err)
	}
	if len(listed) != 1 {
		t.Fatalf("expected 1 division for round 10, got %d", len(listed))
	}
}
```

- [ ] **Step 5: Run the tests to verify they fail**

Run: `go test ./internal/schedule/... -v`
Expected: FAIL (compile error — `CreateDivisions`, `NewDivision`, `ListDivisionsForRound`, `GetDivision`, `ErrDivisionNotFound` undefined)

- [ ] **Step 6: Implement the repo methods**

Create `internal/schedule/division.go`:

```go
package schedule

import (
	"database/sql"
	"encoding/json"
	"errors"
)

var ErrDivisionNotFound = errors.New("division not found")

type NewDivision struct {
	Name    string
	TeamIDs []string
}

// CreateDivisions persists every division in one transaction, rolling
// back on any error.
func (r *Repo) CreateDivisions(tournamentID, roundID int64, divisions []NewDivision) ([]Division, error) {
	tx, err := r.db.Begin()
	if err != nil {
		return nil, err
	}

	created := make([]Division, 0, len(divisions))
	for _, d := range divisions {
		teamIDsJSON, err := json.Marshal(d.TeamIDs)
		if err != nil {
			tx.Rollback()
			return nil, err
		}
		res, err := tx.Exec(
			`INSERT INTO divisions (tournament_id, round_id, name, team_ids) VALUES (?, ?, ?, ?)`,
			tournamentID, roundID, d.Name, string(teamIDsJSON),
		)
		if err != nil {
			tx.Rollback()
			return nil, err
		}
		id, err := res.LastInsertId()
		if err != nil {
			tx.Rollback()
			return nil, err
		}
		created = append(created, Division{ID: id, TournamentID: tournamentID, RoundID: roundID, Name: d.Name, TeamIDs: d.TeamIDs})
	}

	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return created, nil
}

func (r *Repo) ListDivisionsForRound(roundID int64) ([]Division, error) {
	rows, err := r.db.Query(
		`SELECT id, tournament_id, round_id, name, team_ids FROM divisions WHERE round_id = ? ORDER BY id`,
		roundID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var divisions []Division
	for rows.Next() {
		var d Division
		var teamIDsJSON string
		if err := rows.Scan(&d.ID, &d.TournamentID, &d.RoundID, &d.Name, &teamIDsJSON); err != nil {
			return nil, err
		}
		if err := json.Unmarshal([]byte(teamIDsJSON), &d.TeamIDs); err != nil {
			return nil, err
		}
		divisions = append(divisions, d)
	}
	return divisions, rows.Err()
}

func (r *Repo) GetDivision(id int64) (*Division, error) {
	row := r.db.QueryRow(`SELECT id, tournament_id, round_id, name, team_ids FROM divisions WHERE id = ?`, id)
	var d Division
	var teamIDsJSON string
	if err := row.Scan(&d.ID, &d.TournamentID, &d.RoundID, &d.Name, &teamIDsJSON); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrDivisionNotFound
		}
		return nil, err
	}
	if err := json.Unmarshal([]byte(teamIDsJSON), &d.TeamIDs); err != nil {
		return nil, err
	}
	return &d, nil
}
```

- [ ] **Step 7: Run the repo tests to verify they pass**

Run: `go test ./internal/schedule/... -v`
Expected: PASS

- [ ] **Step 8: Persist divisions from the compute-divisions endpoint**

Modify `internal/server/handlers_round_divisions.go` to its full new content:

```go
package server

import (
	"encoding/json"
	"net/http"

	"tournamentstudio/internal/plugin"
	"tournamentstudio/internal/ranking"
	"tournamentstudio/internal/schedule"
)

type computeDivisionsRequest struct {
	Cuts []plugin.Cut `json:"cuts"`
}

type divisionResponse struct {
	ID      int64    `json:"id"`
	Name    string   `json:"name"`
	TeamIDs []string `json:"team_ids"`
}

func (s *Server) handleComputeDivisions(w http.ResponseWriter, r *http.Request) {
	var req computeDivisionsRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}

	ctx, ok := s.loadClosedRoundContext(w, r)
	if !ok {
		return
	}

	var allResults []ranking.TeamResult
	for _, g := range ctx.groups {
		for _, teamID := range g.TeamIDs {
			res := ctx.resultsByTeam[teamID]
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

	divisions, err := ctx.ttp.DivisionCuts(rankedIDs, req.Cuts)
	if err != nil {
		http.Error(w, "plugin error computing divisions: "+err.Error(), http.StatusInternalServerError)
		return
	}

	newDivisions := make([]schedule.NewDivision, len(divisions))
	for i, d := range divisions {
		newDivisions[i] = schedule.NewDivision{Name: d.Name, TeamIDs: d.TeamIDs}
	}
	created, err := s.schedule.CreateDivisions(ctx.tournamentID, ctx.round.ID, newDivisions)
	if err != nil {
		http.Error(w, "could not save divisions", http.StatusInternalServerError)
		return
	}

	resp := make([]divisionResponse, len(created))
	for i, d := range created {
		resp[i] = divisionResponse{ID: d.ID, Name: d.Name, TeamIDs: d.TeamIDs}
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]any{"divisions": resp})
}
```

- [ ] **Step 9: Assert persistence in the HTTP test**

Modify `internal/server/round_divisions_test.go`'s
`TestComputeDivisionsSplitsRankedTeams` — its existing decode/assertions
on `resp.Divisions[0]`/`[1]`'s `Name`/`TeamIDs` are unaffected by the new
`id` field (Go's JSON decoder ignores fields the target struct doesn't
declare) and stay exactly as they are. Add this block at the very end of
the test function, after the existing `TeamIDs` assertions, to prove the
divisions were actually persisted and not just computed:

```go
	persisted, err := s.schedule.ListDivisionsForRound(roundID)
	if err != nil {
		t.Fatalf("ListDivisionsForRound: %v", err)
	}
	if len(persisted) != 2 {
		t.Fatalf("expected 2 persisted divisions, got %d", len(persisted))
	}
	if persisted[0].Name != "Gold Final" || len(persisted[0].TeamIDs) != 3 {
		t.Fatalf("unexpected persisted Gold Final: %+v", persisted[0])
	}
	if persisted[1].Name != "Final" || len(persisted[1].TeamIDs) != 5 {
		t.Fatalf("unexpected persisted Final: %+v", persisted[1])
	}
```

- [ ] **Step 10: Run the tests to verify they pass**

Run: `go build ./... && go test ./internal/schedule/... ./internal/server/... -v`
Expected: PASS

- [ ] **Step 11: Run the full suite and commit**

Run: `go build ./... && go test ./... -count=1`
Expected: PASS, all packages.

```bash
git add internal/schedule internal/store/migrations/0013_divisions.sql internal/store/store_test.go internal/server/handlers_round_divisions.go internal/server/round_divisions_test.go
git commit -m "feat: persist divisions when computed"
```

---

## Task 7: Division heat-scheduling endpoint, and heat-results support for division heats

**Files:**
- Modify: `internal/schedule/heat.go` (extract shared course-validation
  helper, add `ScheduleDivisionHeats`)
- Modify: `internal/schedule/heat_test.go` (add tests for the new method)
- Create: `internal/server/handlers_schedule_division.go`
- Create: `internal/server/schedule_division_test.go`
- Modify: `internal/server/handlers_heat_results.go` (replace the
  "division heat results are not yet supported" branch with real
  handling)
- Modify: `internal/server/heat_results_test.go` (add a division-heat
  results test)
- Modify: `internal/server/server.go` (register 1 route)

**Interfaces:**
- Consumes: `schedule.Division`, `schedule.ErrDivisionNotFound`, `(*schedule.Repo).GetDivision` (Task 6); `heatResponse`, `heatToResponse`, `parseStartAt` (Task 3, reused).
- Produces: `schedule.DivisionAssignment{DivisionID, CourseID int64}`; `schedule.ErrDivisionAlreadyScheduled`; `(*schedule.Repo).ScheduleDivisionHeats(tournamentID int64, assignments []DivisionAssignment, startAt *time.Time) ([]Heat, error)`. No later task consumes these directly — this is the last piece of the scheduling surface.

- [ ] **Step 1: Write the failing repo tests**

Modify `internal/schedule/heat_test.go` — append:

```go
func TestScheduleDivisionHeatsAutoSequencesOnSameCourse(t *testing.T) {
	r := newTestRepo(t)
	course, err := r.CreateCourse(1, "Course A", 300)
	if err != nil {
		t.Fatalf("CreateCourse: %v", err)
	}
	divisions, err := r.CreateDivisions(1, 10, []NewDivision{
		{Name: "Gold Final", TeamIDs: []string{"t1", "t2"}},
		{Name: "Final", TeamIDs: []string{"t3", "t4"}},
	})
	if err != nil {
		t.Fatalf("CreateDivisions: %v", err)
	}

	start := time.Date(2026, 9, 1, 9, 0, 0, 0, time.UTC)
	heats, err := r.ScheduleDivisionHeats(1, []DivisionAssignment{
		{DivisionID: divisions[0].ID, CourseID: course.ID},
		{DivisionID: divisions[1].ID, CourseID: course.ID},
	}, &start)
	if err != nil {
		t.Fatalf("ScheduleDivisionHeats: %v", err)
	}
	if len(heats) != 2 {
		t.Fatalf("expected 2 heats, got %d", len(heats))
	}
	if heats[0].GroupID != nil || *heats[0].DivisionID != divisions[0].ID {
		t.Fatalf("unexpected heat shape: %+v", heats[0])
	}
	if heats[0].RoundID != 10 {
		t.Fatalf("expected heat's RoundID to come from the division's RoundID, got %d", heats[0].RoundID)
	}
	wantSecond := start.Add(300 * time.Second)
	if !heats[1].PlannedStart.Equal(wantSecond) {
		t.Fatalf("expected second heat at %v, got %v", wantSecond, heats[1].PlannedStart)
	}
}

func TestScheduleDivisionHeatsRejectsAlreadyScheduledDivision(t *testing.T) {
	r := newTestRepo(t)
	course, err := r.CreateCourse(1, "Course A", 300)
	if err != nil {
		t.Fatalf("CreateCourse: %v", err)
	}
	divisions, err := r.CreateDivisions(1, 10, []NewDivision{{Name: "Final", TeamIDs: []string{"t1"}}})
	if err != nil {
		t.Fatalf("CreateDivisions: %v", err)
	}
	start := time.Date(2026, 9, 1, 9, 0, 0, 0, time.UTC)
	if _, err := r.ScheduleDivisionHeats(1, []DivisionAssignment{{DivisionID: divisions[0].ID, CourseID: course.ID}}, &start); err != nil {
		t.Fatalf("ScheduleDivisionHeats: %v", err)
	}

	if _, err := r.ScheduleDivisionHeats(1, []DivisionAssignment{{DivisionID: divisions[0].ID, CourseID: course.ID}}, &start); err != ErrDivisionAlreadyScheduled {
		t.Fatalf("expected ErrDivisionAlreadyScheduled, got %v", err)
	}
}

func TestScheduleDivisionHeatsRejectsUnknownDivision(t *testing.T) {
	r := newTestRepo(t)
	course, err := r.CreateCourse(1, "Course A", 300)
	if err != nil {
		t.Fatalf("CreateCourse: %v", err)
	}
	start := time.Date(2026, 9, 1, 9, 0, 0, 0, time.UTC)
	if _, err := r.ScheduleDivisionHeats(1, []DivisionAssignment{{DivisionID: 999, CourseID: course.ID}}, &start); err != ErrDivisionNotFound {
		t.Fatalf("expected ErrDivisionNotFound, got %v", err)
	}
}

func TestScheduleDivisionHeatsRejectsDivisionFromAnotherTournament(t *testing.T) {
	r := newTestRepo(t)
	course, err := r.CreateCourse(1, "Course A", 300)
	if err != nil {
		t.Fatalf("CreateCourse: %v", err)
	}
	divisions, err := r.CreateDivisions(2, 20, []NewDivision{{Name: "Final", TeamIDs: []string{"t1"}}})
	if err != nil {
		t.Fatalf("CreateDivisions: %v", err)
	}
	start := time.Date(2026, 9, 1, 9, 0, 0, 0, time.UTC)
	if _, err := r.ScheduleDivisionHeats(1, []DivisionAssignment{{DivisionID: divisions[0].ID, CourseID: course.ID}}, &start); err != ErrDivisionNotFound {
		t.Fatalf("expected ErrDivisionNotFound for a cross-tournament division, got %v", err)
	}
}
```

- [ ] **Step 2: Run the tests to verify they fail**

Run: `go test ./internal/schedule/... -v`
Expected: FAIL (compile error — `ScheduleDivisionHeats`, `DivisionAssignment`, `ErrDivisionAlreadyScheduled` undefined)

- [ ] **Step 3: Implement `ScheduleDivisionHeats`, extracting the shared course-validation helper**

Modify `internal/schedule/heat.go` to its full new content:

```go
package schedule

import (
	"database/sql"
	"errors"
	"time"
)

var ErrHeatNotFound = errors.New("heat not found")
var ErrGroupAlreadyScheduled = errors.New("group already has a scheduled heat")
var ErrDivisionAlreadyScheduled = errors.New("division already has a scheduled heat")

// GroupAssignment pairs a round's group with the course it should race
// on, for ScheduleGroupHeats.
type GroupAssignment struct {
	GroupID  int64
	CourseID int64
}

// DivisionAssignment pairs a division with the course it should race
// on, for ScheduleDivisionHeats.
type DivisionAssignment struct {
	DivisionID int64
	CourseID   int64
}

// validateCourseForSchedule confirms courseID belongs to tournamentID
// and returns its HeatIntervalSeconds, or ErrCourseNotFound if either
// check fails. Shared by ScheduleGroupHeats and ScheduleDivisionHeats.
func validateCourseForSchedule(tx *sql.Tx, tournamentID, courseID int64) (int, error) {
	var courseTournamentID int64
	var intervalSeconds int
	row := tx.QueryRow(`SELECT tournament_id, heat_interval_seconds FROM courses WHERE id = ?`, courseID)
	if err := row.Scan(&courseTournamentID, &intervalSeconds); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return 0, ErrCourseNotFound
		}
		return 0, err
	}
	if courseTournamentID != tournamentID {
		return 0, ErrCourseNotFound
	}
	return intervalSeconds, nil
}

// ScheduleGroupHeats creates one Heat per assignment, transactionally.
// Every assignment's course must belong to tournamentID and must not
// already have created a heat for that group. Heats on the same course
// within one call are auto-sequenced HeatIntervalSeconds apart, in
// assignments' order; the first heat on a given course starts at
// startAt if provided, else one interval after that course's
// currently-latest heat, else now if the course has no heats yet.
func (r *Repo) ScheduleGroupHeats(tournamentID, roundID int64, assignments []GroupAssignment, startAt *time.Time) ([]Heat, error) {
	tx, err := r.db.Begin()
	if err != nil {
		return nil, err
	}

	nextStart := make(map[int64]time.Time)
	created := make([]Heat, 0, len(assignments))

	for _, a := range assignments {
		intervalSeconds, err := validateCourseForSchedule(tx, tournamentID, a.CourseID)
		if err != nil {
			tx.Rollback()
			return nil, err
		}

		var existingCount int
		if err := tx.QueryRow(`SELECT COUNT(*) FROM heats WHERE group_id = ?`, a.GroupID).Scan(&existingCount); err != nil {
			tx.Rollback()
			return nil, err
		}
		if existingCount > 0 {
			tx.Rollback()
			return nil, ErrGroupAlreadyScheduled
		}

		start, ok := nextStart[a.CourseID]
		if !ok {
			start, err = courseAnchor(tx, a.CourseID, startAt)
			if err != nil {
				tx.Rollback()
				return nil, err
			}
		}
		nextStart[a.CourseID] = start.Add(time.Duration(intervalSeconds) * time.Second)

		groupID := a.GroupID
		res, err := tx.Exec(
			`INSERT INTO heats (round_id, group_id, division_id, course_id, planned_start, status) VALUES (?, ?, NULL, ?, ?, ?)`,
			roundID, groupID, a.CourseID, start.Format(time.RFC3339), string(HeatScheduled),
		)
		if err != nil {
			tx.Rollback()
			return nil, err
		}
		heatID, err := res.LastInsertId()
		if err != nil {
			tx.Rollback()
			return nil, err
		}
		created = append(created, Heat{
			ID: heatID, RoundID: roundID, GroupID: &groupID, CourseID: a.CourseID,
			PlannedStart: start, Status: HeatScheduled,
		})
	}

	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return created, nil
}

// ScheduleDivisionHeats creates one Heat per assignment, transactionally,
// the division-heat counterpart to ScheduleGroupHeats. Every
// assignment's course must belong to tournamentID and must not already
// have created a heat for that division; each division's RoundID (the
// round it was cut from) becomes the created heat's RoundID. Same
// same-course auto-sequencing rules as ScheduleGroupHeats.
func (r *Repo) ScheduleDivisionHeats(tournamentID int64, assignments []DivisionAssignment, startAt *time.Time) ([]Heat, error) {
	tx, err := r.db.Begin()
	if err != nil {
		return nil, err
	}

	nextStart := make(map[int64]time.Time)
	created := make([]Heat, 0, len(assignments))

	for _, a := range assignments {
		var divTournamentID, divRoundID int64
		row := tx.QueryRow(`SELECT tournament_id, round_id FROM divisions WHERE id = ?`, a.DivisionID)
		if err := row.Scan(&divTournamentID, &divRoundID); err != nil {
			tx.Rollback()
			if errors.Is(err, sql.ErrNoRows) {
				return nil, ErrDivisionNotFound
			}
			return nil, err
		}
		if divTournamentID != tournamentID {
			tx.Rollback()
			return nil, ErrDivisionNotFound
		}

		intervalSeconds, err := validateCourseForSchedule(tx, tournamentID, a.CourseID)
		if err != nil {
			tx.Rollback()
			return nil, err
		}

		var existingCount int
		if err := tx.QueryRow(`SELECT COUNT(*) FROM heats WHERE division_id = ?`, a.DivisionID).Scan(&existingCount); err != nil {
			tx.Rollback()
			return nil, err
		}
		if existingCount > 0 {
			tx.Rollback()
			return nil, ErrDivisionAlreadyScheduled
		}

		start, ok := nextStart[a.CourseID]
		if !ok {
			start, err = courseAnchor(tx, a.CourseID, startAt)
			if err != nil {
				tx.Rollback()
				return nil, err
			}
		}
		nextStart[a.CourseID] = start.Add(time.Duration(intervalSeconds) * time.Second)

		divisionID := a.DivisionID
		res, err := tx.Exec(
			`INSERT INTO heats (round_id, group_id, division_id, course_id, planned_start, status) VALUES (?, NULL, ?, ?, ?, ?)`,
			divRoundID, divisionID, a.CourseID, start.Format(time.RFC3339), string(HeatScheduled),
		)
		if err != nil {
			tx.Rollback()
			return nil, err
		}
		heatID, err := res.LastInsertId()
		if err != nil {
			tx.Rollback()
			return nil, err
		}
		created = append(created, Heat{
			ID: heatID, RoundID: divRoundID, DivisionID: &divisionID, CourseID: a.CourseID,
			PlannedStart: start, Status: HeatScheduled,
		})
	}

	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return created, nil
}

// courseAnchor determines the base PlannedStart for the first new heat
// scheduled on courseID in one ScheduleGroupHeats/ScheduleDivisionHeats
// call: startAt if the caller supplied one, else one interval after the
// course's currently-latest scheduled heat, else now if the course has
// no heats yet.
func courseAnchor(tx *sql.Tx, courseID int64, startAt *time.Time) (time.Time, error) {
	if startAt != nil {
		return *startAt, nil
	}
	var latest sql.NullString
	if err := tx.QueryRow(`SELECT MAX(planned_start) FROM heats WHERE course_id = ?`, courseID).Scan(&latest); err != nil {
		return time.Time{}, err
	}
	if !latest.Valid {
		return time.Now().UTC(), nil
	}
	t, err := time.Parse(time.RFC3339, latest.String)
	if err != nil {
		return time.Time{}, err
	}
	var intervalSeconds int
	if err := tx.QueryRow(`SELECT heat_interval_seconds FROM courses WHERE id = ?`, courseID).Scan(&intervalSeconds); err != nil {
		return time.Time{}, err
	}
	return t.Add(time.Duration(intervalSeconds) * time.Second), nil
}

type rowScanner interface {
	Scan(dest ...any) error
}

func scanHeatRow(row rowScanner) (Heat, error) {
	var h Heat
	var groupID, divisionID sql.NullInt64
	var plannedStart, status string
	if err := row.Scan(&h.ID, &h.RoundID, &groupID, &divisionID, &h.CourseID, &plannedStart, &status); err != nil {
		return Heat{}, err
	}
	if groupID.Valid {
		v := groupID.Int64
		h.GroupID = &v
	}
	if divisionID.Valid {
		v := divisionID.Int64
		h.DivisionID = &v
	}
	t, err := time.Parse(time.RFC3339, plannedStart)
	if err != nil {
		return Heat{}, err
	}
	h.PlannedStart = t
	h.Status = HeatStatus(status)
	return h, nil
}

func (r *Repo) GetHeat(id int64) (*Heat, error) {
	row := r.db.QueryRow(
		`SELECT id, round_id, group_id, division_id, course_id, planned_start, status FROM heats WHERE id = ?`,
		id,
	)
	h, err := scanHeatRow(row)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrHeatNotFound
		}
		return nil, err
	}
	return &h, nil
}
```

- [ ] **Step 4: Run the repo tests to verify they pass**

Run: `go test ./internal/schedule/... -v`
Expected: PASS

- [ ] **Step 5: Register the route**

Add this line inside `routes()` in `internal/server/server.go`:

```go
	s.mux.Handle("POST /api/tournaments/{id}/divisions/schedule", organizerOnly(http.HandlerFunc(s.handleScheduleDivisions)))
```

- [ ] **Step 6: Write the handler**

Create `internal/server/handlers_schedule_division.go`:

```go
package server

import (
	"encoding/json"
	"net/http"
	"strconv"

	"tournamentstudio/internal/schedule"
)

type divisionScheduleAssignment struct {
	DivisionID int64 `json:"division_id"`
	CourseID   int64 `json:"course_id"`
}

type scheduleDivisionsRequest struct {
	Assignments []divisionScheduleAssignment `json:"assignments"`
	StartAt     *string                      `json:"start_at"`
}

func (s *Server) handleScheduleDivisions(w http.ResponseWriter, r *http.Request) {
	tournamentID, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		http.Error(w, "invalid tournament id", http.StatusBadRequest)
		return
	}

	var req scheduleDivisionsRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}
	startAt, err := parseStartAt(req.StartAt)
	if err != nil {
		http.Error(w, "start_at must be RFC3339", http.StatusBadRequest)
		return
	}
	if len(req.Assignments) == 0 {
		http.Error(w, "at least one assignment is required", http.StatusBadRequest)
		return
	}

	assignments := make([]schedule.DivisionAssignment, len(req.Assignments))
	for i, a := range req.Assignments {
		assignments[i] = schedule.DivisionAssignment{DivisionID: a.DivisionID, CourseID: a.CourseID}
	}

	heats, err := s.schedule.ScheduleDivisionHeats(tournamentID, assignments, startAt)
	if err != nil {
		if err == schedule.ErrCourseNotFound {
			http.Error(w, "unknown course", http.StatusBadRequest)
			return
		}
		if err == schedule.ErrDivisionNotFound {
			http.Error(w, "unknown division", http.StatusBadRequest)
			return
		}
		if err == schedule.ErrDivisionAlreadyScheduled {
			http.Error(w, "one or more divisions already have a scheduled heat", http.StatusConflict)
			return
		}
		http.Error(w, "could not schedule divisions", http.StatusInternalServerError)
		return
	}

	resp := make([]heatResponse, len(heats))
	for i, h := range heats {
		resp[i] = heatToResponse(h)
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(map[string]any{"heats": resp})
}
```

Note this handler intentionally does not check that a division belongs
to `tournamentID` at the HTTP layer before calling
`ScheduleDivisionHeats` — `schedule.Repo.ScheduleDivisionHeats` already
does that check itself (mirroring `ScheduleGroupHeats`'s course check)
and returns `ErrDivisionNotFound` for a cross-tournament division,
exactly like an unknown one. There is deliberately no
`ErrDivisionAlreadyScheduled`-style "assignments must cover every
division of the round" completeness check here (unlike
`handleScheduleRound`'s group-completeness check) — per the spec, a
division-schedule request schedules exactly the `division_id`s listed,
nothing more, since divisions aren't a URL-scoped "complete set" the way
a round's groups are; scheduling divisions in batches is valid.

- [ ] **Step 7: Add division-heat support to the results endpoint**

Modify `internal/server/handlers_heat_results.go` — replace this block:

```go
	if heat.GroupID == nil {
		// Division-heat results are wired up in Task 7, once divisions
		// and their heats exist at all.
		http.Error(w, "division heat results are not yet supported", http.StatusNotImplemented)
		return
	}
	group, err := s.rounds.GetGroup(*heat.GroupID)
	if err != nil {
		http.Error(w, "could not get group", http.StatusInternalServerError)
		return
	}
```

with:

```go
	var validTeamIDsSource []string
	if heat.GroupID != nil {
		group, err := s.rounds.GetGroup(*heat.GroupID)
		if err != nil {
			http.Error(w, "could not get group", http.StatusInternalServerError)
			return
		}
		validTeamIDsSource = group.TeamIDs
	} else {
		division, err := s.schedule.GetDivision(*heat.DivisionID)
		if err != nil {
			http.Error(w, "could not get division", http.StatusInternalServerError)
			return
		}
		validTeamIDsSource = division.TeamIDs
	}
```

Then further down in the same function, replace this line:

```go
	validTeamIDs := make(map[string]bool, len(group.TeamIDs))
	for _, id := range group.TeamIDs {
		validTeamIDs[id] = true
	}
```

with:

```go
	validTeamIDs := make(map[string]bool, len(validTeamIDsSource))
	for _, id := range validTeamIDsSource {
		validTeamIDs[id] = true
	}
```

And in the round-closure cascade block further down, replace:

```go
		roundHeats, err := s.schedule.ListHeatsForRound(heat.RoundID)
		if err != nil {
			http.Error(w, "could not list round heats", http.StatusInternalServerError)
			return
		}
		roundComplete := true
		for _, h := range roundHeats {
			if h.GroupID != nil && h.Status != schedule.HeatClosed {
				roundComplete = false
				break
			}
		}
		if roundComplete {
			if err := s.rounds.SetStatus(heat.RoundID, round.StatusClosed); err != nil {
				http.Error(w, "could not close round", http.StatusInternalServerError)
				return
			}
		}
```

with:

```go
		if heat.GroupID != nil {
			// Only a round's own group-heats gate that round's closure --
			// its divisions' heats (added after the round is already
			// closed, once /divisions has run) must never reopen or
			// re-trigger this check.
			roundHeats, err := s.schedule.ListHeatsForRound(heat.RoundID)
			if err != nil {
				http.Error(w, "could not list round heats", http.StatusInternalServerError)
				return
			}
			roundComplete := true
			for _, h := range roundHeats {
				if h.GroupID != nil && h.Status != schedule.HeatClosed {
					roundComplete = false
					break
				}
			}
			if roundComplete {
				if err := s.rounds.SetStatus(heat.RoundID, round.StatusClosed); err != nil {
					http.Error(w, "could not close round", http.StatusInternalServerError)
					return
				}
			}
		}
```

(A division heat closing never needs to check round closure — the round
that produced it is already closed by the time `/divisions` could have
been called at all, and re-running the same "every group-heat closed"
check against a round whose heats now include division heats too would
incorrectly require the divisions to close before the check could pass,
since the loop's `h.GroupID != nil` guard already only exists to protect
against exactly that. Gating the entire block on `heat.GroupID != nil`
is the simplest way to make the intent explicit rather than relying
solely on that inner guard.)

- [ ] **Step 8: Write the tests**

Create `internal/server/schedule_division_test.go`:

```go
package server

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"tournamentstudio/internal/auth"
)

func TestScheduleDivisionsCreatesHeats(t *testing.T) {
	s := newTestServer(t)
	token := loginAs(t, s, "organizer1", "pw", auth.RoleOrganizer)
	tournamentID := createTestTournament(t, s, token)
	roundID, ids := createTestRound(t, s, token, tournamentID, [][]string{{"t1", "t2", "t3", "t4"}})
	submitHeatResultsForRound(t, s, token, tournamentID, roundID, ids,
		"t1", map[string]any{"time_seconds": 100.0},
		"t2", map[string]any{"time_seconds": 110.0},
		"t3", map[string]any{"time_seconds": 120.0},
		"t4", map[string]any{"time_seconds": 130.0},
	)

	divisionsBody, _ := json.Marshal(map[string]any{
		"cuts": []map[string]any{{"name": "Gold Final", "size": 2}},
	})
	divisionsReq := httptest.NewRequest(http.MethodPost, fmt.Sprintf("/api/tournaments/%d/rounds/%d/divisions", tournamentID, roundID), bytes.NewReader(divisionsBody))
	divisionsReq.Header.Set("Authorization", "Bearer "+token)
	divisionsRec := httptest.NewRecorder()
	s.ServeHTTP(divisionsRec, divisionsReq)
	if divisionsRec.Code != http.StatusOK {
		t.Fatalf("compute divisions: expected 200, got %d: %s", divisionsRec.Code, divisionsRec.Body.String())
	}
	var divisionsResp struct {
		Divisions []struct {
			ID int64 `json:"id"`
		} `json:"divisions"`
	}
	if err := json.Unmarshal(divisionsRec.Body.Bytes(), &divisionsResp); err != nil {
		t.Fatalf("decode divisions: %v", err)
	}
	if len(divisionsResp.Divisions) != 2 {
		t.Fatalf("expected 2 divisions, got %d", len(divisionsResp.Divisions))
	}

	courseID := createTestCourse(t, s, token, tournamentID, "Finals Course", 300)
	scheduleBody, _ := json.Marshal(map[string]any{
		"assignments": []map[string]any{
			{"division_id": divisionsResp.Divisions[0].ID, "course_id": courseID},
			{"division_id": divisionsResp.Divisions[1].ID, "course_id": courseID},
		},
		"start_at": "2026-09-01T13:00:00Z",
	})
	scheduleReq := httptest.NewRequest(http.MethodPost, fmt.Sprintf("/api/tournaments/%d/divisions/schedule", tournamentID), bytes.NewReader(scheduleBody))
	scheduleReq.Header.Set("Authorization", "Bearer "+token)
	scheduleRec := httptest.NewRecorder()
	s.ServeHTTP(scheduleRec, scheduleReq)
	if scheduleRec.Code != http.StatusCreated {
		t.Fatalf("schedule divisions: expected 201, got %d: %s", scheduleRec.Code, scheduleRec.Body.String())
	}

	var scheduled struct {
		Heats []struct {
			DivisionID   int64  `json:"division_id"`
			PlannedStart string `json:"planned_start"`
		} `json:"heats"`
	}
	if err := json.Unmarshal(scheduleRec.Body.Bytes(), &scheduled); err != nil {
		t.Fatalf("decode schedule response: %v", err)
	}
	if len(scheduled.Heats) != 2 {
		t.Fatalf("expected 2 heats, got %d", len(scheduled.Heats))
	}
	first, _ := time.Parse(time.RFC3339, scheduled.Heats[0].PlannedStart)
	second, _ := time.Parse(time.RFC3339, scheduled.Heats[1].PlannedStart)
	if second.Sub(first) != 300*time.Second {
		t.Fatalf("expected division heats 300s apart, got %v", second.Sub(first))
	}
}

func TestScheduleDivisionsRejectsUnknownDivision(t *testing.T) {
	s := newTestServer(t)
	token := loginAs(t, s, "organizer1", "pw", auth.RoleOrganizer)
	tournamentID := createTestTournament(t, s, token)
	courseID := createTestCourse(t, s, token, tournamentID, "Course A", 300)

	body, _ := json.Marshal(map[string]any{
		"assignments": []map[string]any{{"division_id": 999, "course_id": courseID}},
	})
	req := httptest.NewRequest(http.MethodPost, fmt.Sprintf("/api/tournaments/%d/divisions/schedule", tournamentID), bytes.NewReader(body))
	req.Header.Set("Authorization", "Bearer "+token)
	rec := httptest.NewRecorder()
	s.ServeHTTP(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d: %s", rec.Code, rec.Body.String())
	}
}
```

Modify `internal/server/heat_results_test.go` — append a test proving
division-heat results work end to end and correctly do NOT re-touch the
already-closed round's status:

```go
func TestSubmitDivisionHeatResultsWorksAndDoesNotReopenRound(t *testing.T) {
	s := newTestServer(t)
	token := loginAs(t, s, "organizer1", "pw", auth.RoleOrganizer)
	tournamentID := createTestTournament(t, s, token)
	roundID, ids := createTestRound(t, s, token, tournamentID, [][]string{{"t1", "t2"}})
	submitHeatResultsForRound(t, s, token, tournamentID, roundID, ids,
		"t1", map[string]any{"time_seconds": 100.0},
		"t2", map[string]any{"time_seconds": 110.0},
	)

	pr, err := s.rounds.GetRound(roundID)
	if err != nil {
		t.Fatalf("GetRound: %v", err)
	}
	if pr.Status != round.StatusClosed {
		t.Fatalf("expected round closed before computing divisions, got %s", pr.Status)
	}

	divisionsBody, _ := json.Marshal(map[string]any{"cuts": []map[string]any{{"name": "Final", "size": 2}}})
	divisionsReq := httptest.NewRequest(http.MethodPost, fmt.Sprintf("/api/tournaments/%d/rounds/%d/divisions", tournamentID, roundID), bytes.NewReader(divisionsBody))
	divisionsReq.Header.Set("Authorization", "Bearer "+token)
	divisionsRec := httptest.NewRecorder()
	s.ServeHTTP(divisionsRec, divisionsReq)
	var divisionsResp struct {
		Divisions []struct {
			ID int64 `json:"id"`
		} `json:"divisions"`
	}
	if err := json.Unmarshal(divisionsRec.Body.Bytes(), &divisionsResp); err != nil {
		t.Fatalf("decode divisions: %v", err)
	}

	courseID := createTestCourse(t, s, token, tournamentID, "Finals Course", 300)
	scheduleBody, _ := json.Marshal(map[string]any{
		"assignments": []map[string]any{{"division_id": divisionsResp.Divisions[0].ID, "course_id": courseID}},
	})
	scheduleReq := httptest.NewRequest(http.MethodPost, fmt.Sprintf("/api/tournaments/%d/divisions/schedule", tournamentID), bytes.NewReader(scheduleBody))
	scheduleReq.Header.Set("Authorization", "Bearer "+token)
	scheduleRec := httptest.NewRecorder()
	s.ServeHTTP(scheduleRec, scheduleReq)
	var scheduled struct {
		Heats []struct {
			ID int64 `json:"id"`
		} `json:"heats"`
	}
	if err := json.Unmarshal(scheduleRec.Body.Bytes(), &scheduled); err != nil {
		t.Fatalf("decode schedule response: %v", err)
	}
	divisionHeatID := scheduled.Heats[0].ID

	rec := submitHeatResults(t, s, token, tournamentID, divisionHeatID, map[string]any{
		ids["t1"]: map[string]any{"time_seconds": 90.0},
		ids["t2"]: map[string]any{"time_seconds": 95.0},
	})
	if rec.Code != http.StatusOK {
		t.Fatalf("submit division heat results: expected 200, got %d: %s", rec.Code, rec.Body.String())
	}

	h, err := s.schedule.GetHeat(divisionHeatID)
	if err != nil {
		t.Fatalf("GetHeat: %v", err)
	}
	if h.Status != "closed" {
		t.Fatalf("expected division heat closed, got %s", h.Status)
	}

	prAfter, err := s.rounds.GetRound(roundID)
	if err != nil {
		t.Fatalf("GetRound: %v", err)
	}
	if prAfter.Status != round.StatusClosed {
		t.Fatalf("expected round to remain closed, got %s", prAfter.Status)
	}
}
```

- [ ] **Step 9: Run the tests to verify they pass**

Run: `go build ./... && go test ./internal/schedule/... ./internal/server/... -v`
Expected: PASS

- [ ] **Step 10: Run the full suite and commit**

Run: `go build ./... && go test ./... -count=1`
Expected: PASS, all packages.

```bash
git add internal/schedule/heat.go internal/schedule/heat_test.go internal/server/handlers_schedule_division.go internal/server/schedule_division_test.go internal/server/handlers_heat_results.go internal/server/heat_results_test.go internal/server/server.go
git commit -m "feat: add division heat-scheduling endpoint and division-heat results support"
```

---

## Task 8: Heat manual override endpoint

**Files:**
- Modify: `internal/schedule/heat.go` (add `UpdateHeatPlannedStart`)
- Modify: `internal/schedule/heat_test.go` (add a test for it)
- Create: `internal/server/handlers_heat.go`
- Create: `internal/server/heat_test.go`
- Modify: `internal/server/server.go` (register 1 route)

**Interfaces:**
- Consumes: `heatResponse`, `heatToResponse`, `parseStartAt` (Task 3, reused).
- Produces: `(*schedule.Repo).UpdateHeatPlannedStart(id int64, start time.Time) (*Heat, error)`. No later task consumes this.

- [ ] **Step 1: Write the failing repo test**

Modify `internal/schedule/heat_test.go` — append:

```go
func TestUpdateHeatPlannedStart(t *testing.T) {
	r := newTestRepo(t)
	heatID := r.mustCreateTestHeat(t)

	newStart := time.Date(2026, 9, 1, 14, 0, 0, 0, time.UTC)
	updated, err := r.UpdateHeatPlannedStart(heatID, newStart)
	if err != nil {
		t.Fatalf("UpdateHeatPlannedStart: %v", err)
	}
	if !updated.PlannedStart.Equal(newStart) {
		t.Fatalf("expected planned_start %v, got %v", newStart, updated.PlannedStart)
	}
}

func TestUpdateHeatPlannedStartNotFound(t *testing.T) {
	r := newTestRepo(t)
	newStart := time.Date(2026, 9, 1, 14, 0, 0, 0, time.UTC)
	if _, err := r.UpdateHeatPlannedStart(999, newStart); err != ErrHeatNotFound {
		t.Fatalf("expected ErrHeatNotFound, got %v", err)
	}
}
```

(`mustCreateTestHeat` is the helper Task 4's `result_test.go` defined as
a method on `*Repo` — reused here, not redefined.)

- [ ] **Step 2: Run the tests to verify they fail**

Run: `go test ./internal/schedule/... -v`
Expected: FAIL (compile error — `UpdateHeatPlannedStart` undefined)

- [ ] **Step 3: Implement the repo method**

Modify `internal/schedule/heat.go` — append:

```go
func (r *Repo) UpdateHeatPlannedStart(id int64, start time.Time) (*Heat, error) {
	res, err := r.db.Exec(`UPDATE heats SET planned_start = ? WHERE id = ?`, start.Format(time.RFC3339), id)
	if err != nil {
		return nil, err
	}
	affected, err := res.RowsAffected()
	if err != nil {
		return nil, err
	}
	if affected == 0 {
		return nil, ErrHeatNotFound
	}
	return r.GetHeat(id)
}
```

- [ ] **Step 4: Run the repo tests to verify they pass**

Run: `go test ./internal/schedule/... -v`
Expected: PASS

- [ ] **Step 5: Register the route**

Add this line inside `routes()` in `internal/server/server.go`:

```go
	s.mux.Handle("PATCH /api/tournaments/{id}/heats/{heat_id}", organizerOnly(http.HandlerFunc(s.handleUpdateHeat)))
```

- [ ] **Step 6: Write the handler**

Create `internal/server/handlers_heat.go`:

```go
package server

import (
	"encoding/json"
	"net/http"
	"strconv"

	"tournamentstudio/internal/schedule"
)

type updateHeatRequest struct {
	PlannedStart string `json:"planned_start"`
}

func (s *Server) handleUpdateHeat(w http.ResponseWriter, r *http.Request) {
	tournamentID, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		http.Error(w, "invalid tournament id", http.StatusBadRequest)
		return
	}
	heatID, err := strconv.ParseInt(r.PathValue("heat_id"), 10, 64)
	if err != nil {
		http.Error(w, "invalid heat id", http.StatusBadRequest)
		return
	}

	heat, err := s.schedule.GetHeat(heatID)
	if err != nil {
		if err == schedule.ErrHeatNotFound {
			http.Error(w, "heat not found", http.StatusNotFound)
			return
		}
		http.Error(w, "could not get heat", http.StatusInternalServerError)
		return
	}
	pr, err := s.rounds.GetRound(heat.RoundID)
	if err != nil {
		http.Error(w, "could not get round", http.StatusInternalServerError)
		return
	}
	if pr.TournamentID != tournamentID {
		http.Error(w, "heat not found", http.StatusNotFound)
		return
	}

	var req updateHeatRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}
	startAt, err := parseStartAt(&req.PlannedStart)
	if err != nil {
		http.Error(w, "planned_start must be RFC3339", http.StatusBadRequest)
		return
	}

	updated, err := s.schedule.UpdateHeatPlannedStart(heatID, *startAt)
	if err != nil {
		http.Error(w, "could not update heat", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(heatToResponse(*updated))
}
```

- [ ] **Step 7: Write the HTTP tests**

Create `internal/server/heat_test.go`:

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

func TestUpdateHeatOverridesPlannedStart(t *testing.T) {
	s := newTestServer(t)
	token := loginAs(t, s, "organizer1", "pw", auth.RoleOrganizer)
	tournamentID := createTestTournament(t, s, token)
	roundID, _ := createTestRound(t, s, token, tournamentID, [][]string{{"t1"}})
	heatByGroup := scheduleTestRound(t, s, token, tournamentID, roundID)
	var heatID int64
	for _, id := range heatByGroup {
		heatID = id
	}

	body, _ := json.Marshal(map[string]any{"planned_start": "2026-09-01T15:30:00Z"})
	req := httptest.NewRequest(http.MethodPatch, fmt.Sprintf("/api/tournaments/%d/heats/%d", tournamentID, heatID), bytes.NewReader(body))
	req.Header.Set("Authorization", "Bearer "+token)
	rec := httptest.NewRecorder()
	s.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}

	var resp struct {
		PlannedStart string `json:"planned_start"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if resp.PlannedStart != "2026-09-01T15:30:00Z" {
		t.Fatalf("expected planned_start 2026-09-01T15:30:00Z, got %s", resp.PlannedStart)
	}
}

func TestUpdateHeatNotFoundForWrongTournament(t *testing.T) {
	s := newTestServer(t)
	token := loginAs(t, s, "organizer1", "pw", auth.RoleOrganizer)
	tournamentA := createTestTournament(t, s, token)
	tournamentB := createTestTournament(t, s, token)
	roundID, _ := createTestRound(t, s, token, tournamentA, [][]string{{"t1"}})
	heatByGroup := scheduleTestRound(t, s, token, tournamentA, roundID)
	var heatID int64
	for _, id := range heatByGroup {
		heatID = id
	}

	body, _ := json.Marshal(map[string]any{"planned_start": "2026-09-01T15:30:00Z"})
	req := httptest.NewRequest(http.MethodPatch, fmt.Sprintf("/api/tournaments/%d/heats/%d", tournamentB, heatID), bytes.NewReader(body))
	req.Header.Set("Authorization", "Bearer "+token)
	rec := httptest.NewRecorder()
	s.ServeHTTP(rec, req)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", rec.Code)
	}
}

func TestUpdateHeatForbiddenForTimeEntry(t *testing.T) {
	s := newTestServer(t)
	organizerToken := loginAs(t, s, "organizer1", "pw", auth.RoleOrganizer)
	tournamentID := createTestTournament(t, s, organizerToken)
	roundID, _ := createTestRound(t, s, organizerToken, tournamentID, [][]string{{"t1"}})
	heatByGroup := scheduleTestRound(t, s, organizerToken, tournamentID, roundID)
	var heatID int64
	for _, id := range heatByGroup {
		heatID = id
	}

	timeEntryToken := loginAs(t, s, "timekeeper1", "pw", auth.RoleTimeEntry)
	body, _ := json.Marshal(map[string]any{"planned_start": "2026-09-01T15:30:00Z"})
	req := httptest.NewRequest(http.MethodPatch, fmt.Sprintf("/api/tournaments/%d/heats/%d", tournamentID, heatID), bytes.NewReader(body))
	req.Header.Set("Authorization", "Bearer "+timeEntryToken)
	rec := httptest.NewRecorder()
	s.ServeHTTP(rec, req)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("expected 403, got %d", rec.Code)
	}
}
```

- [ ] **Step 8: Run the tests to verify they pass**

Run: `go build ./... && go test ./internal/schedule/... ./internal/server/... -v`
Expected: PASS

- [ ] **Step 9: Run the full suite and commit**

Run: `go build ./... && go test ./... -count=1`
Expected: PASS, all packages.

```bash
git add internal/schedule/heat.go internal/schedule/heat_test.go internal/server/handlers_heat.go internal/server/heat_test.go internal/server/server.go
git commit -m "feat: add heat manual-override endpoint"
```

---

## Task 9: Schedule/standings read endpoint

**Files:**
- Modify: `internal/schedule/heat.go` (add `ListHeatsForTournament`)
- Modify: `internal/schedule/heat_test.go` (add a test for it)
- Create: `internal/server/handlers_schedule_read.go`
- Create: `internal/server/schedule_read_test.go`
- Modify: `internal/server/server.go` (register 1 route)

**Interfaces:**
- Consumes: everything from Tasks 1-8 — this is the plan's terminal
  read endpoint, aggregating `schedule.Heat`, `schedule.Course`,
  `schedule.HeatResult` into one response.
- Produces: `(*schedule.Repo).ListHeatsForTournament(tournamentID int64) ([]Heat, error)`. Nothing later in this plan consumes it — Plan 4's UI is this endpoint's real consumer.

- [ ] **Step 1: Write the failing repo test**

Modify `internal/schedule/heat_test.go` — append:

```go
func TestListHeatsForTournament(t *testing.T) {
	r := newTestRepo(t)
	course, err := r.CreateCourse(1, "Course A", 300)
	if err != nil {
		t.Fatalf("CreateCourse: %v", err)
	}
	if _, err := r.ScheduleGroupHeats(1, 10, []GroupAssignment{{GroupID: 100, CourseID: course.ID}}, nil); err != nil {
		t.Fatalf("ScheduleGroupHeats: %v", err)
	}
	if _, err := r.ScheduleGroupHeats(2, 20, []GroupAssignment{{GroupID: 200, CourseID: course.ID}}, nil); err != nil {
		// This second call uses tournamentID 2 for a course that belongs to
		// tournament 1 -- it's expected to fail with ErrCourseNotFound,
		// proving cross-tenant isolation. Confirm that, then move on: the
		// point of this test is ListHeatsForTournament(1) below, not this
		// negative case.
		if err != ErrCourseNotFound {
			t.Fatalf("expected ErrCourseNotFound scheduling against another tournament's course, got %v", err)
		}
	}

	heats, err := r.ListHeatsForTournament(1)
	if err != nil {
		t.Fatalf("ListHeatsForTournament: %v", err)
	}
	if len(heats) != 1 {
		t.Fatalf("expected 1 heat for tournament 1, got %d", len(heats))
	}
}
```

- [ ] **Step 2: Run the test to verify it fails**

Run: `go test ./internal/schedule/... -v`
Expected: FAIL (compile error — `ListHeatsForTournament` undefined)

- [ ] **Step 3: Implement the repo method**

Modify `internal/schedule/heat.go` — append:

```go
// ListHeatsForTournament returns every heat belonging to tournamentID --
// both round-group heats and division heats alike, since Heat.RoundID
// always resolves back to a pre_phase_rounds row, which always carries
// a tournament_id -- ordered by PlannedStart, the natural schedule
// order.
func (r *Repo) ListHeatsForTournament(tournamentID int64) ([]Heat, error) {
	rows, err := r.db.Query(
		`SELECT h.id, h.round_id, h.group_id, h.division_id, h.course_id, h.planned_start, h.status
		 FROM heats h
		 JOIN pre_phase_rounds pr ON pr.id = h.round_id
		 WHERE pr.tournament_id = ?
		 ORDER BY h.planned_start`,
		tournamentID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var heats []Heat
	for rows.Next() {
		h, err := scanHeatRow(rows)
		if err != nil {
			return nil, err
		}
		heats = append(heats, h)
	}
	return heats, rows.Err()
}
```

- [ ] **Step 4: Run the repo test to verify it passes**

Run: `go test ./internal/schedule/... -v`
Expected: PASS

- [ ] **Step 5: Register the route**

Add this line inside `routes()` in `internal/server/server.go`:

```go
	s.mux.Handle("GET /api/tournaments/{id}/schedule", authenticated(http.HandlerFunc(s.handleGetSchedule)))
```

- [ ] **Step 6: Write the handler**

Create `internal/server/handlers_schedule_read.go`:

```go
package server

import (
	"encoding/json"
	"net/http"
	"strconv"
	"time"

	"tournamentstudio/internal/schedule"
)

// scheduleHeatResponse's Results field reuses schedule.HeatResult
// directly (it already carries json tags -- see Task 4) rather than a
// separate response type. Each entry's own "heat_id" is technically
// redundant with the enclosing heat's "id", but not wrong, and it isn't
// worth a third near-identical struct just to drop one field.
type scheduleHeatResponse struct {
	ID             int64                 `json:"id"`
	RoundID        int64                 `json:"round_id"`
	GroupID        *int64                `json:"group_id"`
	DivisionID     *int64                `json:"division_id"`
	CourseID       int64                 `json:"course_id"`
	PlannedStart   string                `json:"planned_start"`
	EffectiveStart string                `json:"effective_start"`
	Status         string                `json:"status"`
	Results        []schedule.HeatResult `json:"results"`
}

func (s *Server) handleGetSchedule(w http.ResponseWriter, r *http.Request) {
	tournamentID, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		http.Error(w, "invalid tournament id", http.StatusBadRequest)
		return
	}

	heats, err := s.schedule.ListHeatsForTournament(tournamentID)
	if err != nil {
		http.Error(w, "could not list heats", http.StatusInternalServerError)
		return
	}

	courseCache := make(map[int64]*schedule.Course, len(heats))
	resp := make([]scheduleHeatResponse, 0, len(heats))
	for _, h := range heats {
		course, ok := courseCache[h.CourseID]
		if !ok {
			course, err = s.schedule.GetCourse(h.CourseID)
			if err != nil {
				http.Error(w, "could not get course", http.StatusInternalServerError)
				return
			}
			courseCache[h.CourseID] = course
		}
		effective := h.PlannedStart.Add(time.Duration(course.DelayOffsetSeconds) * time.Second)

		results, err := s.schedule.ListHeatResults(h.ID)
		if err != nil {
			http.Error(w, "could not list heat results", http.StatusInternalServerError)
			return
		}

		resp = append(resp, scheduleHeatResponse{
			ID:             h.ID,
			RoundID:        h.RoundID,
			GroupID:        h.GroupID,
			DivisionID:     h.DivisionID,
			CourseID:       h.CourseID,
			PlannedStart:   h.PlannedStart.Format(time.RFC3339),
			EffectiveStart: effective.Format(time.RFC3339),
			Status:         string(h.Status),
			Results:        results,
		})
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]any{"heats": resp})
}
```

- [ ] **Step 7: Write the HTTP tests**

Create `internal/server/schedule_read_test.go`:

```go
package server

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"tournamentstudio/internal/auth"
)

func TestGetScheduleReturnsEffectiveTimeAndResults(t *testing.T) {
	s := newTestServer(t)
	token := loginAs(t, s, "organizer1", "pw", auth.RoleOrganizer)
	tournamentID := createTestTournament(t, s, token)
	roundID, ids := createTestRound(t, s, token, tournamentID, [][]string{{"t1", "t2"}})
	heatByGroup := scheduleTestRound(t, s, token, tournamentID, roundID)
	var heatID int64
	for _, id := range heatByGroup {
		heatID = id
	}

	scheduleReq := httptest.NewRequest(http.MethodGet, fmt.Sprintf("/api/tournaments/%d/schedule", tournamentID), nil)
	scheduleReq.Header.Set("Authorization", "Bearer "+token)
	scheduleRec := httptest.NewRecorder()
	s.ServeHTTP(scheduleRec, scheduleReq)

	var before struct {
		Heats []struct {
			ID             int64  `json:"id"`
			PlannedStart   string `json:"planned_start"`
			EffectiveStart string `json:"effective_start"`
		} `json:"heats"`
	}
	if err := json.Unmarshal(scheduleRec.Body.Bytes(), &before); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(before.Heats) != 1 {
		t.Fatalf("expected 1 heat, got %d", len(before.Heats))
	}
	if before.Heats[0].PlannedStart != before.Heats[0].EffectiveStart {
		t.Fatalf("expected effective_start to equal planned_start with a zero delay offset")
	}

	// Nudge the course's delay offset, confirm effective_start shifts.
	var courseID int64
	getHeatResp, err := s.schedule.GetHeat(heatID)
	if err != nil {
		t.Fatalf("GetHeat: %v", err)
	}
	courseID = getHeatResp.CourseID
	patchBody, _ := json.Marshal(map[string]any{"delay_offset_seconds": 600})
	patchReq := httptest.NewRequest(http.MethodPatch, fmt.Sprintf("/api/tournaments/%d/courses/%d", tournamentID, courseID), bytes.NewReader(patchBody))
	patchReq.Header.Set("Authorization", "Bearer "+token)
	patchRec := httptest.NewRecorder()
	s.ServeHTTP(patchRec, patchReq)
	if patchRec.Code != http.StatusOK {
		t.Fatalf("patch course: expected 200, got %d: %s", patchRec.Code, patchRec.Body.String())
	}

	submitHeatResults(t, s, token, tournamentID, heatID, map[string]any{
		ids["t1"]: map[string]any{"time_seconds": 100.0},
	})

	afterReq := httptest.NewRequest(http.MethodGet, fmt.Sprintf("/api/tournaments/%d/schedule", tournamentID), nil)
	afterReq.Header.Set("Authorization", "Bearer "+token)
	afterRec := httptest.NewRecorder()
	s.ServeHTTP(afterRec, afterReq)

	var after struct {
		Heats []struct {
			PlannedStart   string `json:"planned_start"`
			EffectiveStart string `json:"effective_start"`
			Results        []struct {
				TeamID      string   `json:"team_id"`
				TimeSeconds *float64 `json:"time_seconds"`
			} `json:"results"`
		} `json:"heats"`
	}
	if err := json.Unmarshal(afterRec.Body.Bytes(), &after); err != nil {
		t.Fatalf("decode: %v", err)
	}
	planned, _ := time.Parse(time.RFC3339, after.Heats[0].PlannedStart)
	effective, _ := time.Parse(time.RFC3339, after.Heats[0].EffectiveStart)
	if effective.Sub(planned) != 600*time.Second {
		t.Fatalf("expected effective_start 600s after planned_start, got %v", effective.Sub(planned))
	}
	if len(after.Heats[0].Results) != 1 || after.Heats[0].Results[0].TeamID != ids["t1"] {
		t.Fatalf("expected 1 result for t1, got %+v", after.Heats[0].Results)
	}
}

func TestGetScheduleAllowsSpectator(t *testing.T) {
	s := newTestServer(t)
	organizerToken := loginAs(t, s, "organizer1", "pw", auth.RoleOrganizer)
	tournamentID := createTestTournament(t, s, organizerToken)

	spectatorToken := loginAs(t, s, "spectator1", "pw", auth.RoleSpectator)
	req := httptest.NewRequest(http.MethodGet, fmt.Sprintf("/api/tournaments/%d/schedule", tournamentID), nil)
	req.Header.Set("Authorization", "Bearer "+spectatorToken)
	rec := httptest.NewRecorder()
	s.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
}
```

- [ ] **Step 8: Run the tests to verify they pass**

Run: `go build ./... && go test ./internal/schedule/... ./internal/server/... -v`
Expected: PASS

- [ ] **Step 9: Run the full suite and commit**

Run: `go build ./... && go test ./... -count=1`
Expected: PASS, all packages.

```bash
git add internal/schedule/heat.go internal/schedule/heat_test.go internal/server/handlers_schedule_read.go internal/server/schedule_read_test.go internal/server/server.go
git commit -m "feat: add schedule/standings read endpoint"
```

---
