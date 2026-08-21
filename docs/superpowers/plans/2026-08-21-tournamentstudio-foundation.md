# TournamentStudio Foundation Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Build the backend foundation of TournamentStudio: a single Go binary with an embedded SQLite store, role-based local accounts, an i18n loader, and Tournament + Team domain logic (manual entry and CSV/XLSX import) exposed over a JSON HTTP API.

**Architecture:** A single Go module (`tournamentstudio`) compiled to one binary. `internal/store` owns the embedded SQLite database and a forward-only SQL migration runner; every other domain package (`internal/auth`, `internal/tournament`, `internal/team`) is a thin repository over `*sql.DB` plus a plain Go model. `internal/server` is the only package that knows about HTTP — it composes the repositories, wires routes with Go 1.22's method+pattern `http.ServeMux`, and enforces roles via middleware. `internal/importer` turns uploaded CSV/XLSX files into validated `team.Team` values without touching HTTP or SQL itself.

**Tech Stack:** Go 1.22 (stdlib `net/http` routing, no external router), `modernc.org/sqlite` (pure-Go, no CGo — keeps the single-binary/offline promise), `golang.org/x/crypto/bcrypt`, `github.com/xuri/excelize/v2` for XLSX.

**Spec:** `docs/superpowers/specs/2026-08-21-tournament-studio-design.md`

## Global Constraints

- Single Go binary, no external runtime dependency (spec §3).
- SQLite via a pure-Go driver — no CGo (spec §3).
- Role enforcement happens server-side on every write endpoint, not just hidden in the UI (spec §3).
- Local accounts only — no external auth provider (spec §3).
- English + German ship built in; additional languages are JSON files dropped into an external folder, no rebuild required (spec §8).
- Manual team entry must go through the identical schema/validation as import (spec §6, and the 2026-08-21 chat correction: "If no import is available it should also [be] addable manually").

---

## File Structure

```
cmd/tournamentstudio/main.go        entrypoint: opens the store, starts the HTTP server
internal/store/store.go             SQLite open + migration runner
internal/store/migrations/*.sql     one file per migration, embedded
internal/auth/user.go               Role type, User model, password hashing
internal/auth/repo.go               user CRUD against SQLite
internal/auth/session.go            session tokens
internal/i18n/i18n.go               translation catalog: embedded + external JSON bundles
internal/i18n/bundles/en.json       built-in English strings
internal/i18n/bundles/de.json       built-in German strings
internal/tournament/model.go        Tournament model
internal/tournament/repo.go         tournament CRUD against SQLite
internal/team/model.go              Team model
internal/team/repo.go               team CRUD against SQLite
internal/importer/csv.go            CSV -> raw rows
internal/importer/xlsx.go           XLSX -> raw rows
internal/importer/validate.go       raw rows -> validated team.Team + problems
internal/server/server.go           Server struct, route table
internal/server/middleware.go       requireRole() and session-from-context
internal/server/handlers_auth.go    POST /api/login, GET /api/whoami
internal/server/handlers_tournament.go
internal/server/handlers_team.go
internal/server/handlers_import.go
```

Every `internal/*` package (except `server`) is usable and testable with zero knowledge of HTTP. `server` is the only package allowed to import `net/http`.

---

### Task 1: Project skeleton and health endpoint

**Files:**
- Create: `go.mod`
- Create: `internal/server/server.go`
- Create: `internal/server/server_test.go`
- Create: `cmd/tournamentstudio/main.go`

**Interfaces:**
- Produces: `server.New() *Server` (this signature changes in Task 4 once the server gains dependencies — that's expected), `(*Server).ServeHTTP(w http.ResponseWriter, r *http.Request)`.

- [ ] **Step 1: Initialize the Go module**

```bash
cd /home/rain/Projects/TournamentStudio
go mod init tournamentstudio
```

Open `go.mod` and confirm it declares `go 1.22` or later (edit it by hand if `go mod init` picked an older toolchain version):

```
module tournamentstudio

go 1.22
```

- [ ] **Step 2: Write the failing test**

Create `internal/server/server_test.go`:

```go
package server

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestHealthz(t *testing.T) {
	s := New()
	req := httptest.NewRequest(http.MethodGet, "/healthz", nil)
	rec := httptest.NewRecorder()
	s.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
	if got := rec.Body.String(); got != `{"status":"ok"}`+"\n" {
		t.Fatalf("unexpected body: %q", got)
	}
}
```

- [ ] **Step 3: Run the test to verify it fails**

Run: `go test ./internal/server/...`
Expected: FAIL — package `internal/server` does not exist yet (`New` undefined).

- [ ] **Step 4: Implement the server**

Create `internal/server/server.go`:

```go
package server

import (
	"encoding/json"
	"net/http"
)

type Server struct {
	mux *http.ServeMux
}

func New() *Server {
	s := &Server{mux: http.NewServeMux()}
	s.routes()
	return s
}

func (s *Server) routes() {
	s.mux.HandleFunc("GET /healthz", s.handleHealthz)
}

func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	s.mux.ServeHTTP(w, r)
}

func (s *Server) handleHealthz(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
}
```

- [ ] **Step 5: Run the test to verify it passes**

Run: `go test ./internal/server/...`
Expected: PASS

- [ ] **Step 6: Add the entrypoint**

Create `cmd/tournamentstudio/main.go`:

```go
package main

import (
	"log"
	"net/http"

	"tournamentstudio/internal/server"
)

func main() {
	s := server.New()
	addr := ":8080"
	log.Printf("listening on %s", addr)
	if err := http.ListenAndServe(addr, s); err != nil {
		log.Fatal(err)
	}
}
```

Run: `go build ./...`
Expected: builds with no errors.

- [ ] **Step 7: Commit**

```bash
git add go.mod internal/server cmd/tournamentstudio
git commit -m "feat: add server skeleton with health endpoint"
```

---

### Task 2: SQLite store with migration runner

**Files:**
- Create: `internal/store/store.go`
- Create: `internal/store/migrations/0001_bootstrap.sql`
- Create: `internal/store/store_test.go`

**Interfaces:**
- Produces: `store.Open(path string) (*Store, error)`, `Store.DB *sql.DB`.

- [ ] **Step 1: Add the SQLite driver dependency**

```bash
go get modernc.org/sqlite
```

- [ ] **Step 2: Write the failing test**

Create `internal/store/store_test.go`:

```go
package store

import (
	"path/filepath"
	"testing"
)

func TestOpenAppliesMigrationsOnce(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "test.db")

	s, err := Open(dbPath)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer s.DB.Close()

	var count int
	if err := s.DB.QueryRow(`SELECT COUNT(*) FROM schema_migrations`).Scan(&count); err != nil {
		t.Fatalf("query schema_migrations: %v", err)
	}
	if count != 1 {
		t.Fatalf("expected 1 migration applied, got %d", count)
	}

	// Reopening the same database must not reapply migrations.
	s2, err := Open(dbPath)
	if err != nil {
		t.Fatalf("second Open: %v", err)
	}
	defer s2.DB.Close()

	if err := s2.DB.QueryRow(`SELECT COUNT(*) FROM schema_migrations`).Scan(&count); err != nil {
		t.Fatalf("query schema_migrations: %v", err)
	}
	if count != 1 {
		t.Fatalf("expected migrations not reapplied, still 1, got %d", count)
	}
}
```

- [ ] **Step 3: Run the test to verify it fails**

Run: `go test ./internal/store/...`
Expected: FAIL — package does not exist yet.

- [ ] **Step 4: Add the first migration**

Create `internal/store/migrations/0001_bootstrap.sql`:

```sql
CREATE TABLE IF NOT EXISTS _bootstrap (
    id INTEGER PRIMARY KEY
);
```

- [ ] **Step 5: Implement the store and migration runner**

Create `internal/store/store.go`:

```go
package store

import (
	"database/sql"
	"embed"
	"fmt"

	_ "modernc.org/sqlite"
)

//go:embed migrations/*.sql
var migrationsFS embed.FS

type Store struct {
	DB *sql.DB
}

func Open(path string) (*Store, error) {
	db, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, fmt.Errorf("open db: %w", err)
	}
	if err := db.Ping(); err != nil {
		return nil, fmt.Errorf("ping db: %w", err)
	}

	s := &Store{DB: db}
	if err := s.migrate(); err != nil {
		return nil, fmt.Errorf("migrate: %w", err)
	}
	return s, nil
}

func (s *Store) migrate() error {
	if _, err := s.DB.Exec(`CREATE TABLE IF NOT EXISTS schema_migrations (version TEXT PRIMARY KEY)`); err != nil {
		return err
	}

	entries, err := migrationsFS.ReadDir("migrations")
	if err != nil {
		return err
	}

	for _, entry := range entries {
		name := entry.Name()

		var applied bool
		if err := s.DB.QueryRow(`SELECT EXISTS(SELECT 1 FROM schema_migrations WHERE version = ?)`, name).Scan(&applied); err != nil {
			return err
		}
		if applied {
			continue
		}

		contents, err := migrationsFS.ReadFile("migrations/" + name)
		if err != nil {
			return err
		}

		tx, err := s.DB.Begin()
		if err != nil {
			return err
		}
		if _, err := tx.Exec(string(contents)); err != nil {
			tx.Rollback()
			return fmt.Errorf("apply %s: %w", name, err)
		}
		if _, err := tx.Exec(`INSERT INTO schema_migrations (version) VALUES (?)`, name); err != nil {
			tx.Rollback()
			return err
		}
		if err := tx.Commit(); err != nil {
			return err
		}
	}

	return nil
}
```

- [ ] **Step 6: Run the test to verify it passes**

Run: `go test ./internal/store/...`
Expected: PASS

- [ ] **Step 7: Commit**

```bash
git add go.mod go.sum internal/store
git commit -m "feat: add SQLite store with migration runner"
```

---

### Task 3: User model and repository

**Files:**
- Create: `internal/auth/user.go`
- Create: `internal/auth/repo.go`
- Create: `internal/store/migrations/0002_users.sql`
- Create: `internal/auth/repo_test.go`

**Interfaces:**
- Consumes: `store.Store` (Task 2).
- Produces: `auth.Role` (`RoleOrganizer`, `RoleTimeEntry`, `RoleSpectator`), `auth.User{ID, Username, PasswordHash, Role}`, `auth.NewRepo(s *store.Store) *Repo`, `(*Repo).Create(username, plainPassword string, role Role) (*User, error)`, `(*Repo).FindByUsername(username string) (*User, error)`, `auth.CheckPassword(hash, plain string) bool`, `auth.ErrNotFound`, `auth.ErrDuplicateUsername`.

- [ ] **Step 1: Add the bcrypt dependency**

```bash
go get golang.org/x/crypto/bcrypt
```

- [ ] **Step 2: Add the users migration**

Create `internal/store/migrations/0002_users.sql`:

```sql
CREATE TABLE users (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    username TEXT NOT NULL UNIQUE,
    password_hash TEXT NOT NULL,
    role TEXT NOT NULL
);
```

- [ ] **Step 3: Write the failing test**

Create `internal/auth/repo_test.go`:

```go
package auth

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

func TestCreateAndFindUser(t *testing.T) {
	repo := NewRepo(newTestStore(t))

	u, err := repo.Create("organizer1", "correct-horse", RoleOrganizer)
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if u.ID == 0 {
		t.Fatalf("expected non-zero ID")
	}

	found, err := repo.FindByUsername("organizer1")
	if err != nil {
		t.Fatalf("FindByUsername: %v", err)
	}
	if found.Role != RoleOrganizer {
		t.Fatalf("expected role organizer, got %s", found.Role)
	}
	if !CheckPassword(found.PasswordHash, "correct-horse") {
		t.Fatalf("expected password to verify")
	}
	if CheckPassword(found.PasswordHash, "wrong-password") {
		t.Fatalf("expected wrong password to fail verification")
	}
}

func TestCreateDuplicateUsername(t *testing.T) {
	repo := NewRepo(newTestStore(t))
	if _, err := repo.Create("dup", "pw", RoleSpectator); err != nil {
		t.Fatalf("first Create: %v", err)
	}
	if _, err := repo.Create("dup", "pw2", RoleSpectator); err != ErrDuplicateUsername {
		t.Fatalf("expected ErrDuplicateUsername, got %v", err)
	}
}

func TestFindByUsernameNotFound(t *testing.T) {
	repo := NewRepo(newTestStore(t))
	if _, err := repo.FindByUsername("nobody"); err != ErrNotFound {
		t.Fatalf("expected ErrNotFound, got %v", err)
	}
}
```

- [ ] **Step 4: Run the test to verify it fails**

Run: `go test ./internal/auth/...`
Expected: FAIL — package does not exist yet.

- [ ] **Step 5: Implement the user model**

Create `internal/auth/user.go`:

```go
package auth

import "golang.org/x/crypto/bcrypt"

type Role string

const (
	RoleOrganizer Role = "organizer"
	RoleTimeEntry Role = "time_entry"
	RoleSpectator Role = "spectator"
)

type User struct {
	ID           int64
	Username     string
	PasswordHash string
	Role         Role
}

func HashPassword(plain string) (string, error) {
	b, err := bcrypt.GenerateFromPassword([]byte(plain), bcrypt.DefaultCost)
	if err != nil {
		return "", err
	}
	return string(b), nil
}

func CheckPassword(hash, plain string) bool {
	return bcrypt.CompareHashAndPassword([]byte(hash), []byte(plain)) == nil
}
```

- [ ] **Step 6: Implement the repository**

Create `internal/auth/repo.go`:

```go
package auth

import (
	"database/sql"
	"errors"
	"fmt"
	"strings"

	"tournamentstudio/internal/store"
)

var ErrNotFound = errors.New("user not found")
var ErrDuplicateUsername = errors.New("username already exists")

type Repo struct {
	db *sql.DB
}

func NewRepo(s *store.Store) *Repo {
	return &Repo{db: s.DB}
}

func (r *Repo) Create(username, plainPassword string, role Role) (*User, error) {
	hash, err := HashPassword(plainPassword)
	if err != nil {
		return nil, fmt.Errorf("hash password: %w", err)
	}

	res, err := r.db.Exec(`INSERT INTO users (username, password_hash, role) VALUES (?, ?, ?)`, username, hash, string(role))
	if err != nil {
		if strings.Contains(err.Error(), "UNIQUE constraint failed") {
			return nil, ErrDuplicateUsername
		}
		return nil, err
	}
	id, err := res.LastInsertId()
	if err != nil {
		return nil, err
	}
	return &User{ID: id, Username: username, PasswordHash: hash, Role: role}, nil
}

func (r *Repo) FindByUsername(username string) (*User, error) {
	row := r.db.QueryRow(`SELECT id, username, password_hash, role FROM users WHERE username = ?`, username)
	var u User
	var role string
	if err := row.Scan(&u.ID, &u.Username, &u.PasswordHash, &role); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	u.Role = Role(role)
	return &u, nil
}
```

- [ ] **Step 7: Run the test to verify it passes**

Run: `go test ./internal/auth/...`
Expected: PASS

- [ ] **Step 8: Commit**

```bash
git add go.mod go.sum internal/auth internal/store/migrations/0002_users.sql
git commit -m "feat: add user model and repository"
```

---

### Task 4: Sessions, login endpoint, and role middleware

**Files:**
- Create: `internal/auth/session.go`
- Create: `internal/store/migrations/0003_sessions.sql`
- Create: `internal/server/middleware.go`
- Create: `internal/server/handlers_auth.go`
- Create: `internal/server/auth_test.go`
- Modify: `internal/server/server.go` (constructor now takes a `*store.Store`, routes gain login/whoami)
- Modify: `internal/server/server_test.go` (update `TestHealthz` to use the new constructor)
- Modify: `cmd/tournamentstudio/main.go` (open the store and pass it to `server.New`)

**Interfaces:**
- Consumes: `auth.NewRepo`, `auth.Repo.FindByUsername`, `auth.CheckPassword` (Task 3).
- Produces: `auth.Session{Token, UserID, Role}`, `auth.NewSessionRepo(s *store.Store) *SessionRepo`, `(*SessionRepo).Create(userID int64, role Role) (*Session, error)`, `(*SessionRepo).Find(token string) (*Session, error)`, `auth.ErrInvalidSession`, `server.New(s *store.Store) *Server` (breaking change from Task 1/2), `(*Server).requireRole(roles ...auth.Role) func(http.Handler) http.Handler`.

- [ ] **Step 1: Add the sessions migration**

Create `internal/store/migrations/0003_sessions.sql`:

```sql
CREATE TABLE sessions (
    token TEXT PRIMARY KEY,
    user_id INTEGER NOT NULL REFERENCES users(id),
    role TEXT NOT NULL,
    created_at TEXT NOT NULL DEFAULT (datetime('now'))
);
```

- [ ] **Step 2: Implement sessions**

Create `internal/auth/session.go`:

```go
package auth

import (
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"errors"

	"tournamentstudio/internal/store"
)

var ErrInvalidSession = errors.New("invalid session")

type Session struct {
	Token  string
	UserID int64
	Role   Role
}

type SessionRepo struct {
	db *sql.DB
}

func NewSessionRepo(s *store.Store) *SessionRepo {
	return &SessionRepo{db: s.DB}
}

func (r *SessionRepo) Create(userID int64, role Role) (*Session, error) {
	tokenBytes := make([]byte, 32)
	if _, err := rand.Read(tokenBytes); err != nil {
		return nil, err
	}
	token := hex.EncodeToString(tokenBytes)

	if _, err := r.db.Exec(`INSERT INTO sessions (token, user_id, role) VALUES (?, ?, ?)`, token, userID, string(role)); err != nil {
		return nil, err
	}
	return &Session{Token: token, UserID: userID, Role: role}, nil
}

func (r *SessionRepo) Find(token string) (*Session, error) {
	row := r.db.QueryRow(`SELECT token, user_id, role FROM sessions WHERE token = ?`, token)
	var s Session
	var role string
	if err := row.Scan(&s.Token, &s.UserID, &role); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrInvalidSession
		}
		return nil, err
	}
	s.Role = Role(role)
	return &s, nil
}
```

Run: `go test ./internal/auth/...`
Expected: PASS (no new tests in this step, but confirms nothing broke).

- [ ] **Step 3: Update the server constructor and routes to depend on the store**

Modify `internal/server/server.go`:

```go
package server

import (
	"encoding/json"
	"net/http"

	"tournamentstudio/internal/auth"
	"tournamentstudio/internal/store"
)

type Server struct {
	mux      *http.ServeMux
	users    *auth.Repo
	sessions *auth.SessionRepo
}

func New(s *store.Store) *Server {
	srv := &Server{
		mux:      http.NewServeMux(),
		users:    auth.NewRepo(s),
		sessions: auth.NewSessionRepo(s),
	}
	srv.routes()
	return srv
}

func (s *Server) routes() {
	s.mux.HandleFunc("GET /healthz", s.handleHealthz)
	s.mux.HandleFunc("POST /api/login", s.handleLogin)

	authenticated := s.requireRole(auth.RoleOrganizer, auth.RoleTimeEntry, auth.RoleSpectator)
	s.mux.Handle("GET /api/whoami", authenticated(http.HandlerFunc(s.handleWhoAmI)))
}

func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	s.mux.ServeHTTP(w, r)
}

func (s *Server) handleHealthz(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
}
```

- [ ] **Step 4: Add the role middleware**

Create `internal/server/middleware.go`:

```go
package server

import (
	"context"
	"net/http"
	"strings"

	"tournamentstudio/internal/auth"
)

type contextKey string

const sessionContextKey contextKey = "session"

func (s *Server) requireRole(roles ...auth.Role) func(http.Handler) http.Handler {
	allowed := make(map[auth.Role]bool, len(roles))
	for _, r := range roles {
		allowed[r] = true
	}

	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			authHeader := r.Header.Get("Authorization")
			if !strings.HasPrefix(authHeader, "Bearer ") {
				http.Error(w, "missing bearer token", http.StatusUnauthorized)
				return
			}
			token := strings.TrimPrefix(authHeader, "Bearer ")
			if token == "" {
				http.Error(w, "missing bearer token", http.StatusUnauthorized)
				return
			}

			sess, err := s.sessions.Find(token)
			if err != nil {
				http.Error(w, "invalid session", http.StatusUnauthorized)
				return
			}
			if !allowed[sess.Role] {
				http.Error(w, "forbidden", http.StatusForbidden)
				return
			}

			ctx := context.WithValue(r.Context(), sessionContextKey, sess)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

func sessionFromContext(r *http.Request) *auth.Session {
	sess, _ := r.Context().Value(sessionContextKey).(*auth.Session)
	return sess
}
```

- [ ] **Step 5: Add the login and whoami handlers**

Create `internal/server/handlers_auth.go`:

```go
package server

import (
	"encoding/json"
	"net/http"

	"tournamentstudio/internal/auth"
)

type loginRequest struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

type loginResponse struct {
	Token string `json:"token"`
	Role  string `json:"role"`
}

func (s *Server) handleLogin(w http.ResponseWriter, r *http.Request) {
	var req loginRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}

	user, err := s.users.FindByUsername(req.Username)
	if err != nil {
		http.Error(w, "invalid credentials", http.StatusUnauthorized)
		return
	}
	if !auth.CheckPassword(user.PasswordHash, req.Password) {
		http.Error(w, "invalid credentials", http.StatusUnauthorized)
		return
	}

	sess, err := s.sessions.Create(user.ID, user.Role)
	if err != nil {
		http.Error(w, "could not create session", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(loginResponse{Token: sess.Token, Role: string(user.Role)})
}

func (s *Server) handleWhoAmI(w http.ResponseWriter, r *http.Request) {
	sess := sessionFromContext(r)
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"role": string(sess.Role)})
}
```

- [ ] **Step 6: Update `server_test.go` for the new constructor**

Replace the contents of `internal/server/server_test.go`:

```go
package server

import (
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"

	"tournamentstudio/internal/store"
)

func newTestServer(t *testing.T) *Server {
	t.Helper()
	s, err := store.Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	t.Cleanup(func() { s.DB.Close() })
	return New(s)
}

func TestHealthz(t *testing.T) {
	s := newTestServer(t)
	req := httptest.NewRequest(http.MethodGet, "/healthz", nil)
	rec := httptest.NewRecorder()
	s.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
	if got := rec.Body.String(); got != `{"status":"ok"}`+"\n" {
		t.Fatalf("unexpected body: %q", got)
	}
}
```

- [ ] **Step 7: Write the failing login/whoami test**

Create `internal/server/auth_test.go`:

```go
package server

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"tournamentstudio/internal/auth"
)

func TestLoginSuccessAndWhoAmI(t *testing.T) {
	s := newTestServer(t)
	if _, err := s.users.Create("organizer1", "correct-horse", auth.RoleOrganizer); err != nil {
		t.Fatalf("seed user: %v", err)
	}

	body, _ := json.Marshal(map[string]string{"username": "organizer1", "password": "correct-horse"})
	req := httptest.NewRequest(http.MethodPost, "/api/login", bytes.NewReader(body))
	rec := httptest.NewRecorder()
	s.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	var loginResp struct {
		Token string `json:"token"`
		Role  string `json:"role"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &loginResp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if loginResp.Token == "" {
		t.Fatalf("expected non-empty token")
	}
	if loginResp.Role != "organizer" {
		t.Fatalf("expected role organizer, got %s", loginResp.Role)
	}

	whoReq := httptest.NewRequest(http.MethodGet, "/api/whoami", nil)
	whoReq.Header.Set("Authorization", "Bearer "+loginResp.Token)
	whoRec := httptest.NewRecorder()
	s.ServeHTTP(whoRec, whoReq)
	if whoRec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", whoRec.Code)
	}
}

func TestLoginInvalidCredentials(t *testing.T) {
	s := newTestServer(t)
	if _, err := s.users.Create("organizer1", "correct-horse", auth.RoleOrganizer); err != nil {
		t.Fatalf("seed user: %v", err)
	}

	body, _ := json.Marshal(map[string]string{"username": "organizer1", "password": "wrong"})
	req := httptest.NewRequest(http.MethodPost, "/api/login", bytes.NewReader(body))
	rec := httptest.NewRecorder()
	s.ServeHTTP(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", rec.Code)
	}
}

func TestWhoAmIWithoutToken(t *testing.T) {
	s := newTestServer(t)
	req := httptest.NewRequest(http.MethodGet, "/api/whoami", nil)
	rec := httptest.NewRecorder()
	s.ServeHTTP(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", rec.Code)
	}
}
```

- [ ] **Step 8: Run the tests to verify they pass**

Run: `go test ./internal/server/... ./internal/auth/...`
Expected: PASS

- [ ] **Step 9: Update `main.go` to open the store**

Replace `cmd/tournamentstudio/main.go`:

```go
package main

import (
	"log"
	"net/http"
	"os"

	"tournamentstudio/internal/server"
	"tournamentstudio/internal/store"
)

func main() {
	dbPath := os.Getenv("TOURNAMENTSTUDIO_DB")
	if dbPath == "" {
		dbPath = "tournamentstudio.db"
	}

	st, err := store.Open(dbPath)
	if err != nil {
		log.Fatalf("open store: %v", err)
	}
	defer st.DB.Close()

	s := server.New(st)
	addr := ":8080"
	log.Printf("listening on %s", addr)
	if err := http.ListenAndServe(addr, s); err != nil {
		log.Fatal(err)
	}
}
```

Run: `go build ./...`
Expected: builds with no errors.

- [ ] **Step 10: Commit**

```bash
git add internal/auth internal/server internal/store/migrations/0003_sessions.sql cmd/tournamentstudio/main.go
git commit -m "feat: add sessions, login endpoint, and role middleware"
```

---

### Task 5: i18n catalog

**Files:**
- Create: `internal/i18n/i18n.go`
- Create: `internal/i18n/bundles/en.json`
- Create: `internal/i18n/bundles/de.json`
- Create: `internal/i18n/i18n_test.go`

**Interfaces:**
- Produces: `i18n.Load(externalDir string) (*Catalog, error)`, `(*Catalog).Translate(lang, key string) string`.

This package is self-contained and not yet wired into HTTP responses — the schedule/standings view in a later plan will consume it. It's fully working and tested on its own here, which is what makes the "drop a JSON file into `languages/`" customization promise in the spec (§8, §9) real starting now.

- [ ] **Step 1: Write the failing test**

Create `internal/i18n/i18n_test.go`:

```go
package i18n

import (
	"os"
	"path/filepath"
	"testing"
)

func TestTranslateBuiltIn(t *testing.T) {
	c, err := Load("")
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if got := c.Translate("de", "next_heat"); got != "NÄCHSTER" {
		t.Fatalf("expected NÄCHSTER, got %s", got)
	}
	if got := c.Translate("en", "next_heat"); got != "NEXT" {
		t.Fatalf("expected NEXT, got %s", got)
	}
}

func TestTranslateFallsBackToEnglish(t *testing.T) {
	c, err := Load("")
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if got := c.Translate("fr", "next_heat"); got != "NEXT" {
		t.Fatalf("expected fallback to NEXT, got %s", got)
	}
}

func TestTranslateUnknownKeyReturnsKey(t *testing.T) {
	c, err := Load("")
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if got := c.Translate("en", "nonexistent_key"); got != "nonexistent_key" {
		t.Fatalf("expected key echoed back, got %s", got)
	}
}

func TestExternalLanguageDropIn(t *testing.T) {
	dir := t.TempDir()
	frFile := filepath.Join(dir, "fr.json")
	if err := os.WriteFile(frFile, []byte(`{"next_heat": "SUIVANT"}`), 0o644); err != nil {
		t.Fatalf("write fixture: %v", err)
	}

	c, err := Load(dir)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if got := c.Translate("fr", "next_heat"); got != "SUIVANT" {
		t.Fatalf("expected SUIVANT, got %s", got)
	}
}
```

- [ ] **Step 2: Run the test to verify it fails**

Run: `go test ./internal/i18n/...`
Expected: FAIL — package does not exist yet.

- [ ] **Step 3: Add the built-in bundles**

Create `internal/i18n/bundles/en.json`:

```json
{
  "next_heat": "NEXT",
  "on_time": "ON TIME",
  "submit_heat": "Submit Heat"
}
```

Create `internal/i18n/bundles/de.json`:

```json
{
  "next_heat": "NÄCHSTER",
  "on_time": "PÜNKTLICH",
  "submit_heat": "Lauf einreichen"
}
```

- [ ] **Step 4: Implement the catalog**

Create `internal/i18n/i18n.go`:

```go
package i18n

import (
	"embed"
	"encoding/json"
	"os"
	"path/filepath"
)

//go:embed bundles/*.json
var defaultBundles embed.FS

type Catalog struct {
	strings map[string]map[string]string
}

func Load(externalDir string) (*Catalog, error) {
	c := &Catalog{strings: make(map[string]map[string]string)}

	entries, err := defaultBundles.ReadDir("bundles")
	if err != nil {
		return nil, err
	}
	for _, e := range entries {
		lang := langFromFilename(e.Name())
		data, err := defaultBundles.ReadFile("bundles/" + e.Name())
		if err != nil {
			return nil, err
		}
		var m map[string]string
		if err := json.Unmarshal(data, &m); err != nil {
			return nil, err
		}
		c.strings[lang] = m
	}

	if externalDir != "" {
		if err := c.loadExternal(externalDir); err != nil {
			return nil, err
		}
	}

	return c, nil
}

func (c *Catalog) loadExternal(dir string) error {
	entries, err := os.ReadDir(dir)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return err
	}

	for _, e := range entries {
		if e.IsDir() || filepath.Ext(e.Name()) != ".json" {
			continue
		}
		lang := langFromFilename(e.Name())
		data, err := os.ReadFile(filepath.Join(dir, e.Name()))
		if err != nil {
			return err
		}
		var m map[string]string
		if err := json.Unmarshal(data, &m); err != nil {
			return err
		}
		if c.strings[lang] == nil {
			c.strings[lang] = make(map[string]string)
		}
		for k, v := range m {
			c.strings[lang][k] = v
		}
	}
	return nil
}

func langFromFilename(name string) string {
	return name[:len(name)-len(filepath.Ext(name))]
}

func (c *Catalog) Translate(lang, key string) string {
	if m, ok := c.strings[lang]; ok {
		if v, ok := m[key]; ok {
			return v
		}
	}
	if m, ok := c.strings["en"]; ok {
		if v, ok := m[key]; ok {
			return v
		}
	}
	return key
}
```

- [ ] **Step 5: Run the test to verify it passes**

Run: `go test ./internal/i18n/...`
Expected: PASS

- [ ] **Step 6: Commit**

```bash
git add internal/i18n
git commit -m "feat: add i18n catalog with built-in and drop-in language bundles"
```

---

### Task 6: Tournament domain and HTTP endpoints

**Files:**
- Create: `internal/tournament/model.go`
- Create: `internal/tournament/repo.go`
- Create: `internal/tournament/repo_test.go`
- Create: `internal/store/migrations/0004_tournaments.sql`
- Create: `internal/server/handlers_tournament.go`
- Create: `internal/server/tournament_test.go`
- Modify: `internal/server/server.go` (add `tournaments` field, wire routes)

**Interfaces:**
- Consumes: `store.Store` (Task 2), `(*Server).requireRole` (Task 4).
- Produces: `tournament.Tournament{ID, Name, SportPluginID, TournamentTypeID, Language, Status}`, `tournament.NewRepo(s *store.Store) *Repo`, `(*Repo).Create(t Tournament) (*Tournament, error)`, `(*Repo).Get(id int64) (*Tournament, error)`, `(*Repo).List() ([]Tournament, error)`, `tournament.ErrNotFound`.

- [ ] **Step 1: Add the tournaments migration**

Create `internal/store/migrations/0004_tournaments.sql`:

```sql
CREATE TABLE tournaments (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    name TEXT NOT NULL,
    sport_plugin_id TEXT NOT NULL,
    tournament_type_id TEXT NOT NULL,
    language TEXT NOT NULL DEFAULT 'en',
    status TEXT NOT NULL DEFAULT 'draft'
);
```

- [ ] **Step 2: Write the failing repo test**

Create `internal/tournament/repo_test.go`:

```go
package tournament

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

func TestCreateAndGetTournament(t *testing.T) {
	repo := NewRepo(newTestStore(t))

	created, err := repo.Create(Tournament{
		Name:              "Herbstregatta Rheinauen",
		SportPluginID:     "dragonboat",
		TournamentTypeID:  "timed-heats-reseeding",
		Language:          "de",
	})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if created.ID == 0 {
		t.Fatalf("expected non-zero ID")
	}
	if created.Status != "draft" {
		t.Fatalf("expected status draft, got %s", created.Status)
	}

	fetched, err := repo.Get(created.ID)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if fetched.Name != "Herbstregatta Rheinauen" {
		t.Fatalf("unexpected name: %s", fetched.Name)
	}
}

func TestListTournaments(t *testing.T) {
	repo := NewRepo(newTestStore(t))
	if _, err := repo.Create(Tournament{Name: "A", SportPluginID: "dragonboat", TournamentTypeID: "timed-heats-reseeding", Language: "en"}); err != nil {
		t.Fatalf("Create A: %v", err)
	}
	if _, err := repo.Create(Tournament{Name: "B", SportPluginID: "dragonboat", TournamentTypeID: "timed-heats-reseeding", Language: "en"}); err != nil {
		t.Fatalf("Create B: %v", err)
	}

	list, err := repo.List()
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(list) != 2 {
		t.Fatalf("expected 2 tournaments, got %d", len(list))
	}
}

func TestGetTournamentNotFound(t *testing.T) {
	repo := NewRepo(newTestStore(t))
	if _, err := repo.Get(999); err != ErrNotFound {
		t.Fatalf("expected ErrNotFound, got %v", err)
	}
}
```

- [ ] **Step 3: Run the test to verify it fails**

Run: `go test ./internal/tournament/...`
Expected: FAIL — package does not exist yet.

- [ ] **Step 4: Implement the model and repository**

Create `internal/tournament/model.go`:

```go
package tournament

type Tournament struct {
	ID                int64
	Name              string
	SportPluginID     string
	TournamentTypeID  string
	Language          string
	Status            string
}
```

Create `internal/tournament/repo.go`:

```go
package tournament

import (
	"database/sql"
	"errors"

	"tournamentstudio/internal/store"
)

var ErrNotFound = errors.New("tournament not found")

type Repo struct {
	db *sql.DB
}

func NewRepo(s *store.Store) *Repo {
	return &Repo{db: s.DB}
}

func (r *Repo) Create(t Tournament) (*Tournament, error) {
	res, err := r.db.Exec(
		`INSERT INTO tournaments (name, sport_plugin_id, tournament_type_id, language, status) VALUES (?, ?, ?, ?, ?)`,
		t.Name, t.SportPluginID, t.TournamentTypeID, t.Language, "draft",
	)
	if err != nil {
		return nil, err
	}
	id, err := res.LastInsertId()
	if err != nil {
		return nil, err
	}
	t.ID = id
	t.Status = "draft"
	return &t, nil
}

func (r *Repo) Get(id int64) (*Tournament, error) {
	row := r.db.QueryRow(`SELECT id, name, sport_plugin_id, tournament_type_id, language, status FROM tournaments WHERE id = ?`, id)
	var t Tournament
	if err := row.Scan(&t.ID, &t.Name, &t.SportPluginID, &t.TournamentTypeID, &t.Language, &t.Status); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	return &t, nil
}

func (r *Repo) List() ([]Tournament, error) {
	rows, err := r.db.Query(`SELECT id, name, sport_plugin_id, tournament_type_id, language, status FROM tournaments ORDER BY id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var result []Tournament
	for rows.Next() {
		var t Tournament
		if err := rows.Scan(&t.ID, &t.Name, &t.SportPluginID, &t.TournamentTypeID, &t.Language, &t.Status); err != nil {
			return nil, err
		}
		result = append(result, t)
	}
	return result, rows.Err()
}
```

- [ ] **Step 5: Run the test to verify it passes**

Run: `go test ./internal/tournament/...`
Expected: PASS

- [ ] **Step 6: Wire the server to the tournament repo**

Modify `internal/server/server.go` — add the field, construct it, and register the routes:

```go
type Server struct {
	mux         *http.ServeMux
	users       *auth.Repo
	sessions    *auth.SessionRepo
	tournaments *tournament.Repo
}

func New(s *store.Store) *Server {
	srv := &Server{
		mux:         http.NewServeMux(),
		users:       auth.NewRepo(s),
		sessions:    auth.NewSessionRepo(s),
		tournaments: tournament.NewRepo(s),
	}
	srv.routes()
	return srv
}

func (s *Server) routes() {
	s.mux.HandleFunc("GET /healthz", s.handleHealthz)
	s.mux.HandleFunc("POST /api/login", s.handleLogin)

	authenticated := s.requireRole(auth.RoleOrganizer, auth.RoleTimeEntry, auth.RoleSpectator)
	organizerOnly := s.requireRole(auth.RoleOrganizer)

	s.mux.Handle("GET /api/whoami", authenticated(http.HandlerFunc(s.handleWhoAmI)))
	s.mux.Handle("POST /api/tournaments", organizerOnly(http.HandlerFunc(s.handleCreateTournament)))
	s.mux.Handle("GET /api/tournaments", authenticated(http.HandlerFunc(s.handleListTournaments)))
	s.mux.Handle("GET /api/tournaments/{id}", authenticated(http.HandlerFunc(s.handleGetTournament)))
}
```

Add the import: `"tournamentstudio/internal/tournament"`.

- [ ] **Step 7: Add the tournament handlers**

Create `internal/server/handlers_tournament.go`:

```go
package server

import (
	"encoding/json"
	"net/http"
	"strconv"

	"tournamentstudio/internal/tournament"
)

type createTournamentRequest struct {
	Name              string `json:"name"`
	SportPluginID     string `json:"sport_plugin_id"`
	TournamentTypeID  string `json:"tournament_type_id"`
	Language          string `json:"language"`
}

func (s *Server) handleCreateTournament(w http.ResponseWriter, r *http.Request) {
	var req createTournamentRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}
	if req.Name == "" || req.SportPluginID == "" || req.TournamentTypeID == "" {
		http.Error(w, "name, sport_plugin_id and tournament_type_id are required", http.StatusBadRequest)
		return
	}
	if req.Language == "" {
		req.Language = "en"
	}

	created, err := s.tournaments.Create(tournament.Tournament{
		Name:             req.Name,
		SportPluginID:    req.SportPluginID,
		TournamentTypeID: req.TournamentTypeID,
		Language:         req.Language,
	})
	if err != nil {
		http.Error(w, "could not create tournament", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(created)
}

func (s *Server) handleListTournaments(w http.ResponseWriter, r *http.Request) {
	list, err := s.tournaments.List()
	if err != nil {
		http.Error(w, "could not list tournaments", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(list)
}

func (s *Server) handleGetTournament(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		http.Error(w, "invalid tournament id", http.StatusBadRequest)
		return
	}

	t, err := s.tournaments.Get(id)
	if err != nil {
		if err == tournament.ErrNotFound {
			http.Error(w, "tournament not found", http.StatusNotFound)
			return
		}
		http.Error(w, "could not get tournament", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(t)
}
```

- [ ] **Step 8: Write the failing HTTP test**

Create `internal/server/tournament_test.go`:

```go
package server

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"tournamentstudio/internal/auth"
)

func loginAs(t *testing.T, s *Server, username, password string, role auth.Role) string {
	t.Helper()
	if _, err := s.users.Create(username, password, role); err != nil {
		t.Fatalf("seed user: %v", err)
	}
	body, _ := json.Marshal(map[string]string{"username": username, "password": password})
	req := httptest.NewRequest(http.MethodPost, "/api/login", bytes.NewReader(body))
	rec := httptest.NewRecorder()
	s.ServeHTTP(rec, req)

	var resp struct {
		Token string `json:"token"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode login response: %v", err)
	}
	return resp.Token
}

func TestCreateAndListTournament(t *testing.T) {
	s := newTestServer(t)
	token := loginAs(t, s, "organizer1", "pw", auth.RoleOrganizer)

	body, _ := json.Marshal(map[string]string{
		"name":                "Herbstregatta Rheinauen",
		"sport_plugin_id":     "dragonboat",
		"tournament_type_id":  "timed-heats-reseeding",
		"language":            "de",
	})
	req := httptest.NewRequest(http.MethodPost, "/api/tournaments", bytes.NewReader(body))
	req.Header.Set("Authorization", "Bearer "+token)
	rec := httptest.NewRecorder()
	s.ServeHTTP(rec, req)
	if rec.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", rec.Code, rec.Body.String())
	}

	listReq := httptest.NewRequest(http.MethodGet, "/api/tournaments", nil)
	listReq.Header.Set("Authorization", "Bearer "+token)
	listRec := httptest.NewRecorder()
	s.ServeHTTP(listRec, listReq)
	if listRec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", listRec.Code)
	}

	var list []map[string]any
	if err := json.Unmarshal(listRec.Body.Bytes(), &list); err != nil {
		t.Fatalf("decode list: %v", err)
	}
	if len(list) != 1 {
		t.Fatalf("expected 1 tournament, got %d", len(list))
	}
}

func TestCreateTournamentForbiddenForSpectator(t *testing.T) {
	s := newTestServer(t)
	token := loginAs(t, s, "spec1", "pw", auth.RoleSpectator)

	body, _ := json.Marshal(map[string]string{"name": "X", "sport_plugin_id": "dragonboat", "tournament_type_id": "timed-heats-reseeding"})
	req := httptest.NewRequest(http.MethodPost, "/api/tournaments", bytes.NewReader(body))
	req.Header.Set("Authorization", "Bearer "+token)
	rec := httptest.NewRecorder()
	s.ServeHTTP(rec, req)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("expected 403, got %d", rec.Code)
	}
}
```

- [ ] **Step 9: Run the tests to verify they pass**

Run: `go test ./internal/tournament/... ./internal/server/...`
Expected: PASS

- [ ] **Step 10: Commit**

```bash
git add internal/tournament internal/server internal/store/migrations/0004_tournaments.sql
git commit -m "feat: add tournament domain and HTTP endpoints"
```

---

### Task 7: Team domain, manual entry endpoint

**Files:**
- Create: `internal/team/model.go`
- Create: `internal/team/repo.go`
- Create: `internal/team/repo_test.go`
- Create: `internal/store/migrations/0005_teams.sql`
- Create: `internal/server/handlers_team.go`
- Create: `internal/server/team_test.go`
- Modify: `internal/server/server.go` (add `teams` field, wire routes)

**Interfaces:**
- Consumes: `store.Store` (Task 2), `(*Server).requireRole` (Task 4).
- Produces: `team.Team{ID, TournamentID, Name, Club, ExtraFields map[string]string}`, `team.NewRepo(s *store.Store) *Repo`, `(*Repo).Create(t Team) (*Team, error)`, `(*Repo).ListByTournament(tournamentID int64) ([]Team, error)`.

- [ ] **Step 1: Add the teams migration**

Create `internal/store/migrations/0005_teams.sql`:

```sql
CREATE TABLE teams (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    tournament_id INTEGER NOT NULL REFERENCES tournaments(id),
    name TEXT NOT NULL,
    club TEXT NOT NULL DEFAULT '',
    extra_fields TEXT NOT NULL DEFAULT '{}'
);
```

- [ ] **Step 2: Write the failing repo test**

Create `internal/team/repo_test.go`:

```go
package team

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

func TestCreateAndListTeams(t *testing.T) {
	s := newTestStore(t)
	if _, err := s.DB.Exec(`INSERT INTO tournaments (id, name, sport_plugin_id, tournament_type_id, language, status) VALUES (1, 'Test', 'dragonboat', 'timed-heats-reseeding', 'en', 'draft')`); err != nil {
		t.Fatalf("seed tournament: %v", err)
	}
	repo := NewRepo(s)

	created, err := repo.Create(Team{
		TournamentID: 1,
		Name:         "Möwe RC Kiel",
		Club:         "Möwe Ruderclub e.V.",
		ExtraFields:  map[string]string{"boat_class": "standard"},
	})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if created.ID == 0 {
		t.Fatalf("expected non-zero ID")
	}

	teams, err := repo.ListByTournament(1)
	if err != nil {
		t.Fatalf("ListByTournament: %v", err)
	}
	if len(teams) != 1 {
		t.Fatalf("expected 1 team, got %d", len(teams))
	}
	if teams[0].ExtraFields["boat_class"] != "standard" {
		t.Fatalf("expected boat_class standard, got %v", teams[0].ExtraFields)
	}
}
```

- [ ] **Step 3: Run the test to verify it fails**

Run: `go test ./internal/team/...`
Expected: FAIL — package does not exist yet.

- [ ] **Step 4: Implement the model and repository**

Create `internal/team/model.go`:

```go
package team

type Team struct {
	ID           int64
	TournamentID int64
	Name         string
	Club         string
	ExtraFields  map[string]string
}
```

Create `internal/team/repo.go`:

```go
package team

import (
	"database/sql"
	"encoding/json"

	"tournamentstudio/internal/store"
)

type Repo struct {
	db *sql.DB
}

func NewRepo(s *store.Store) *Repo {
	return &Repo{db: s.DB}
}

func (r *Repo) Create(t Team) (*Team, error) {
	extra, err := json.Marshal(t.ExtraFields)
	if err != nil {
		return nil, err
	}

	res, err := r.db.Exec(
		`INSERT INTO teams (tournament_id, name, club, extra_fields) VALUES (?, ?, ?, ?)`,
		t.TournamentID, t.Name, t.Club, string(extra),
	)
	if err != nil {
		return nil, err
	}
	id, err := res.LastInsertId()
	if err != nil {
		return nil, err
	}
	t.ID = id
	return &t, nil
}

func (r *Repo) ListByTournament(tournamentID int64) ([]Team, error) {
	rows, err := r.db.Query(`SELECT id, tournament_id, name, club, extra_fields FROM teams WHERE tournament_id = ? ORDER BY id`, tournamentID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var result []Team
	for rows.Next() {
		var t Team
		var extra string
		if err := rows.Scan(&t.ID, &t.TournamentID, &t.Name, &t.Club, &extra); err != nil {
			return nil, err
		}
		if extra != "" {
			if err := json.Unmarshal([]byte(extra), &t.ExtraFields); err != nil {
				return nil, err
			}
		}
		result = append(result, t)
	}
	return result, rows.Err()
}
```

- [ ] **Step 5: Run the test to verify it passes**

Run: `go test ./internal/team/...`
Expected: PASS

- [ ] **Step 6: Wire the server to the team repo**

Modify `internal/server/server.go` — add the field, construct it, register routes:

```go
type Server struct {
	mux         *http.ServeMux
	users       *auth.Repo
	sessions    *auth.SessionRepo
	tournaments *tournament.Repo
	teams       *team.Repo
}

func New(s *store.Store) *Server {
	srv := &Server{
		mux:         http.NewServeMux(),
		users:       auth.NewRepo(s),
		sessions:    auth.NewSessionRepo(s),
		tournaments: tournament.NewRepo(s),
		teams:       team.NewRepo(s),
	}
	srv.routes()
	return srv
}
```

Add to `routes()`, inside the existing function body:

```go
	s.mux.Handle("POST /api/tournaments/{id}/teams", organizerOnly(http.HandlerFunc(s.handleCreateTeam)))
	s.mux.Handle("GET /api/tournaments/{id}/teams", authenticated(http.HandlerFunc(s.handleListTeams)))
```

Add the import: `"tournamentstudio/internal/team"`.

- [ ] **Step 7: Add the team handlers**

Create `internal/server/handlers_team.go`:

```go
package server

import (
	"encoding/json"
	"net/http"
	"strconv"

	"tournamentstudio/internal/team"
)

type createTeamRequest struct {
	Name        string            `json:"name"`
	Club        string            `json:"club"`
	ExtraFields map[string]string `json:"extra_fields"`
}

func (s *Server) handleCreateTeam(w http.ResponseWriter, r *http.Request) {
	tournamentID, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		http.Error(w, "invalid tournament id", http.StatusBadRequest)
		return
	}

	var req createTeamRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}
	if req.Name == "" {
		http.Error(w, "name is required", http.StatusBadRequest)
		return
	}

	created, err := s.teams.Create(team.Team{
		TournamentID: tournamentID,
		Name:         req.Name,
		Club:         req.Club,
		ExtraFields:  req.ExtraFields,
	})
	if err != nil {
		http.Error(w, "could not create team", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(created)
}

func (s *Server) handleListTeams(w http.ResponseWriter, r *http.Request) {
	tournamentID, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		http.Error(w, "invalid tournament id", http.StatusBadRequest)
		return
	}

	list, err := s.teams.ListByTournament(tournamentID)
	if err != nil {
		http.Error(w, "could not list teams", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(list)
}
```

- [ ] **Step 8: Write the failing HTTP test**

Create `internal/server/team_test.go`:

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

func createTestTournament(t *testing.T, s *Server, token string) int64 {
	t.Helper()
	body, _ := json.Marshal(map[string]string{
		"name":                "Herbstregatta Rheinauen",
		"sport_plugin_id":     "dragonboat",
		"tournament_type_id":  "timed-heats-reseeding",
	})
	req := httptest.NewRequest(http.MethodPost, "/api/tournaments", bytes.NewReader(body))
	req.Header.Set("Authorization", "Bearer "+token)
	rec := httptest.NewRecorder()
	s.ServeHTTP(rec, req)

	var created struct{ ID int64 }
	if err := json.Unmarshal(rec.Body.Bytes(), &created); err != nil {
		t.Fatalf("decode created tournament: %v", err)
	}
	return created.ID
}

func TestCreateAndListTeamsManually(t *testing.T) {
	s := newTestServer(t)
	token := loginAs(t, s, "organizer1", "pw", auth.RoleOrganizer)
	tournamentID := createTestTournament(t, s, token)

	body, _ := json.Marshal(map[string]any{
		"name":         "Rhein Dragons Köln",
		"club":         "Rhein Dragons Köln e.V.",
		"extra_fields": map[string]string{"boat_class": "standard"},
	})
	path := fmt.Sprintf("/api/tournaments/%d/teams", tournamentID)
	req := httptest.NewRequest(http.MethodPost, path, bytes.NewReader(body))
	req.Header.Set("Authorization", "Bearer "+token)
	rec := httptest.NewRecorder()
	s.ServeHTTP(rec, req)
	if rec.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", rec.Code, rec.Body.String())
	}

	listReq := httptest.NewRequest(http.MethodGet, path, nil)
	listReq.Header.Set("Authorization", "Bearer "+token)
	listRec := httptest.NewRecorder()
	s.ServeHTTP(listRec, listReq)

	var teams []map[string]any
	if err := json.Unmarshal(listRec.Body.Bytes(), &teams); err != nil {
		t.Fatalf("decode teams: %v", err)
	}
	if len(teams) != 1 {
		t.Fatalf("expected 1 team, got %d", len(teams))
	}
}
```

- [ ] **Step 9: Run the tests to verify they pass**

Run: `go test ./internal/team/... ./internal/server/...`
Expected: PASS

- [ ] **Step 10: Commit**

```bash
git add internal/team internal/server internal/store/migrations/0005_teams.sql
git commit -m "feat: add team domain and manual entry endpoint"
```

---

### Task 8: CSV import parsing and validation

**Files:**
- Create: `internal/importer/csv.go`
- Create: `internal/importer/validate.go`
- Create: `internal/importer/csv_test.go`

**Interfaces:**
- Consumes: `team.Team` (Task 7).
- Produces: `importer.ParseCSV(r io.Reader) ([]map[string]string, error)`, `importer.RowProblem{RowIndex int, Message string}`, `importer.ValidationResult{Teams []team.Team, Problems []RowProblem}`, `importer.Validate(tournamentID int64, rows []map[string]string) ValidationResult`.

- [ ] **Step 1: Write the failing tests**

Create `internal/importer/csv_test.go`:

```go
package importer

import (
	"strings"
	"testing"
)

func TestParseCSV(t *testing.T) {
	input := "name,club,boat_class\nMöwe RC Kiel,Möwe Ruderclub e.V.,standard\nWassermann Berlin,,standard\n"
	rows, err := ParseCSV(strings.NewReader(input))
	if err != nil {
		t.Fatalf("ParseCSV: %v", err)
	}
	if len(rows) != 2 {
		t.Fatalf("expected 2 rows, got %d", len(rows))
	}
	if rows[0]["name"] != "Möwe RC Kiel" {
		t.Fatalf("unexpected name: %s", rows[0]["name"])
	}
}

func TestValidateFlagsMissingName(t *testing.T) {
	rows := []map[string]string{
		{"name": "Rhein Dragons Köln", "club": "Rhein Dragons Köln e.V."},
		{"name": "", "club": "No Name Rowing"},
	}
	result := Validate(1, rows)
	if len(result.Teams) != 1 {
		t.Fatalf("expected 1 valid team, got %d", len(result.Teams))
	}
	if len(result.Problems) != 1 {
		t.Fatalf("expected 1 problem, got %d", len(result.Problems))
	}
	if result.Problems[0].RowIndex != 1 {
		t.Fatalf("expected problem at row index 1, got %d", result.Problems[0].RowIndex)
	}
}

func TestValidateCapturesExtraFields(t *testing.T) {
	rows := []map[string]string{
		{"name": "Nixe Hamburg", "club": "Nixe Hamburg e.V.", "boat_class": "standard"},
	}
	result := Validate(1, rows)
	if result.Teams[0].ExtraFields["boat_class"] != "standard" {
		t.Fatalf("expected boat_class standard, got %v", result.Teams[0].ExtraFields)
	}
}
```

- [ ] **Step 2: Run the tests to verify they fail**

Run: `go test ./internal/importer/...`
Expected: FAIL — package does not exist yet.

- [ ] **Step 3: Implement CSV parsing**

Create `internal/importer/csv.go`:

```go
package importer

import (
	"encoding/csv"
	"fmt"
	"io"
)

func ParseCSV(r io.Reader) ([]map[string]string, error) {
	reader := csv.NewReader(r)
	reader.TrimLeadingSpace = true

	records, err := reader.ReadAll()
	if err != nil {
		return nil, fmt.Errorf("parse csv: %w", err)
	}
	if len(records) == 0 {
		return nil, nil
	}

	header := records[0]
	var rows []map[string]string
	for _, record := range records[1:] {
		row := make(map[string]string, len(header))
		for i, col := range header {
			if i < len(record) {
				row[col] = record[i]
			}
		}
		rows = append(rows, row)
	}
	return rows, nil
}
```

- [ ] **Step 4: Implement validation**

Create `internal/importer/validate.go`:

```go
package importer

import (
	"strings"

	"tournamentstudio/internal/team"
)

type RowProblem struct {
	RowIndex int
	Message  string
}

type ValidationResult struct {
	Teams    []team.Team
	Problems []RowProblem
}

func Validate(tournamentID int64, rows []map[string]string) ValidationResult {
	var result ValidationResult

	for i, row := range rows {
		name := strings.TrimSpace(row["name"])
		if name == "" {
			result.Problems = append(result.Problems, RowProblem{RowIndex: i, Message: "missing team name"})
			continue
		}

		extra := make(map[string]string)
		for k, v := range row {
			if k == "name" || k == "club" {
				continue
			}
			extra[k] = v
		}

		result.Teams = append(result.Teams, team.Team{
			TournamentID: tournamentID,
			Name:         name,
			Club:         strings.TrimSpace(row["club"]),
			ExtraFields:  extra,
		})
	}

	return result
}
```

- [ ] **Step 5: Run the tests to verify they pass**

Run: `go test ./internal/importer/...`
Expected: PASS

- [ ] **Step 6: Commit**

```bash
git add internal/importer
git commit -m "feat: add CSV import parsing and row validation"
```

---

### Task 9: XLSX import parsing

**Files:**
- Create: `internal/importer/xlsx.go`
- Create: `internal/importer/xlsx_test.go`

**Interfaces:**
- Produces: `importer.ParseXLSX(r io.Reader) ([]map[string]string, error)` — same shape as `ParseCSV` from Task 8, so callers can dispatch on file extension without an if/else on return types.

- [ ] **Step 1: Add the excelize dependency**

```bash
go get github.com/xuri/excelize/v2
```

- [ ] **Step 2: Write the failing test**

Create `internal/importer/xlsx_test.go`:

```go
package importer

import (
	"bytes"
	"testing"

	"github.com/xuri/excelize/v2"
)

func writeFixtureXLSX(t *testing.T) *bytes.Buffer {
	t.Helper()
	f := excelize.NewFile()
	defer f.Close()

	sheet := f.GetSheetName(0)
	rows := [][]string{
		{"name", "club", "boat_class"},
		{"Sturmvögel Dresden", "Sturmvögel Dresden e.V.", "standard"},
		{"Kraken Paddling Club", "", "standard"},
	}
	for i, row := range rows {
		for j, val := range row {
			cell, err := excelize.CoordinatesToCellName(j+1, i+1)
			if err != nil {
				t.Fatalf("CoordinatesToCellName: %v", err)
			}
			if err := f.SetCellValue(sheet, cell, val); err != nil {
				t.Fatalf("SetCellValue: %v", err)
			}
		}
	}

	var buf bytes.Buffer
	if err := f.Write(&buf); err != nil {
		t.Fatalf("Write: %v", err)
	}
	return &buf
}

func TestParseXLSX(t *testing.T) {
	buf := writeFixtureXLSX(t)
	rows, err := ParseXLSX(buf)
	if err != nil {
		t.Fatalf("ParseXLSX: %v", err)
	}
	if len(rows) != 2 {
		t.Fatalf("expected 2 rows, got %d", len(rows))
	}
	if rows[0]["name"] != "Sturmvögel Dresden" {
		t.Fatalf("unexpected name: %s", rows[0]["name"])
	}
	if rows[1]["club"] != "" {
		t.Fatalf("expected empty club, got %s", rows[1]["club"])
	}
}
```

- [ ] **Step 3: Run the test to verify it fails**

Run: `go test ./internal/importer/...`
Expected: FAIL — `ParseXLSX` undefined.

- [ ] **Step 4: Implement XLSX parsing**

Create `internal/importer/xlsx.go`:

```go
package importer

import (
	"fmt"
	"io"

	"github.com/xuri/excelize/v2"
)

func ParseXLSX(r io.Reader) ([]map[string]string, error) {
	f, err := excelize.OpenReader(r)
	if err != nil {
		return nil, fmt.Errorf("open xlsx: %w", err)
	}
	defer f.Close()

	sheet := f.GetSheetName(0)
	records, err := f.GetRows(sheet)
	if err != nil {
		return nil, fmt.Errorf("read sheet: %w", err)
	}
	if len(records) == 0 {
		return nil, nil
	}

	header := records[0]
	var rows []map[string]string
	for _, record := range records[1:] {
		row := make(map[string]string, len(header))
		for i, col := range header {
			if i < len(record) {
				row[col] = record[i]
			}
		}
		rows = append(rows, row)
	}
	return rows, nil
}
```

- [ ] **Step 5: Run the test to verify it passes**

Run: `go test ./internal/importer/...`
Expected: PASS

- [ ] **Step 6: Commit**

```bash
git add go.mod go.sum internal/importer
git commit -m "feat: add XLSX import parsing"
```

---

### Task 10: Import HTTP endpoint and end-to-end integration test

**Files:**
- Create: `internal/server/handlers_import.go`
- Create: `internal/server/import_test.go`
- Modify: `internal/server/server.go` (register the import route)

**Interfaces:**
- Consumes: `importer.ParseCSV`, `importer.ParseXLSX`, `importer.Validate` (Tasks 8, 9), `s.teams.Create` (Task 7).
- Produces: `POST /api/tournaments/{id}/teams/import` — multipart form field `file`, `.csv` or `.xlsx`; response `{"imported": int, "problems": []RowProblem}`.

- [ ] **Step 1: Register the route**

Modify `internal/server/server.go`, inside `routes()`:

```go
	s.mux.Handle("POST /api/tournaments/{id}/teams/import", organizerOnly(http.HandlerFunc(s.handleImportTeams)))
```

- [ ] **Step 2: Implement the handler**

Create `internal/server/handlers_import.go`:

```go
package server

import (
	"encoding/json"
	"net/http"
	"strconv"
	"strings"

	"tournamentstudio/internal/importer"
)

func (s *Server) handleImportTeams(w http.ResponseWriter, r *http.Request) {
	tournamentID, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		http.Error(w, "invalid tournament id", http.StatusBadRequest)
		return
	}

	if err := r.ParseMultipartForm(10 << 20); err != nil {
		http.Error(w, "invalid multipart form", http.StatusBadRequest)
		return
	}
	file, header, err := r.FormFile("file")
	if err != nil {
		http.Error(w, "missing file", http.StatusBadRequest)
		return
	}
	defer file.Close()

	var rows []map[string]string
	switch {
	case strings.HasSuffix(strings.ToLower(header.Filename), ".xlsx"):
		rows, err = importer.ParseXLSX(file)
	case strings.HasSuffix(strings.ToLower(header.Filename), ".csv"):
		rows, err = importer.ParseCSV(file)
	default:
		http.Error(w, "unsupported file type: use .csv or .xlsx", http.StatusBadRequest)
		return
	}
	if err != nil {
		http.Error(w, "could not parse file: "+err.Error(), http.StatusBadRequest)
		return
	}

	result := importer.Validate(tournamentID, rows)
	for _, t := range result.Teams {
		if _, err := s.teams.Create(t); err != nil {
			http.Error(w, "could not save team: "+err.Error(), http.StatusInternalServerError)
			return
		}
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]any{
		"imported": len(result.Teams),
		"problems": result.Problems,
	})
}
```

- [ ] **Step 3: Write the failing integration test**

Create `internal/server/import_test.go`:

```go
package server

import (
	"bytes"
	"encoding/json"
	"fmt"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"testing"

	"tournamentstudio/internal/auth"
)

func TestImportCSVTeamsEndToEnd(t *testing.T) {
	s := newTestServer(t)
	token := loginAs(t, s, "organizer1", "pw", auth.RoleOrganizer)
	tournamentID := createTestTournament(t, s, token)

	var body bytes.Buffer
	mw := multipart.NewWriter(&body)
	part, err := mw.CreateFormFile("file", "teams.csv")
	if err != nil {
		t.Fatalf("CreateFormFile: %v", err)
	}
	part.Write([]byte("name,club\nMöwe RC Kiel,Möwe Ruderclub e.V.\nWassermann Berlin,\n"))
	mw.Close()

	importReq := httptest.NewRequest(http.MethodPost, fmt.Sprintf("/api/tournaments/%d/teams/import", tournamentID), &body)
	importReq.Header.Set("Authorization", "Bearer "+token)
	importReq.Header.Set("Content-Type", mw.FormDataContentType())
	importRec := httptest.NewRecorder()
	s.ServeHTTP(importRec, importReq)
	if importRec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", importRec.Code, importRec.Body.String())
	}

	var importResp struct {
		Imported int `json:"imported"`
	}
	if err := json.Unmarshal(importRec.Body.Bytes(), &importResp); err != nil {
		t.Fatalf("decode import response: %v", err)
	}
	if importResp.Imported != 2 {
		t.Fatalf("expected 2 imported, got %d", importResp.Imported)
	}

	listReq := httptest.NewRequest(http.MethodGet, fmt.Sprintf("/api/tournaments/%d/teams", tournamentID), nil)
	listReq.Header.Set("Authorization", "Bearer "+token)
	listRec := httptest.NewRecorder()
	s.ServeHTTP(listRec, listReq)

	var teams []map[string]any
	if err := json.Unmarshal(listRec.Body.Bytes(), &teams); err != nil {
		t.Fatalf("decode teams: %v", err)
	}
	if len(teams) != 2 {
		t.Fatalf("expected 2 teams, got %d", len(teams))
	}
}
```

- [ ] **Step 4: Run the full test suite**

Run: `go test ./...`
Expected: PASS, all packages.

- [ ] **Step 5: Commit**

```bash
git add internal/server
git commit -m "feat: add team import HTTP endpoint"
```

---

## Self-Review Notes

- **Spec coverage:** single Go binary (Task 1, 10) · pure-Go SQLite (Task 2) · local accounts + server-side role enforcement (Tasks 3–4, exercised on every write route in Tasks 6–7, 10) · EN/DE + drop-in language bundles (Task 5) · tournament creation (Task 6) · team manual entry and CSV/XLSX import through identical validation (Tasks 7–10).
- **Deferred to later plans, not forgotten:** WASM plugin host and the Dragonboat/Timed-Heats plugins (Plan 2), courses/heats/delay offsets and result recording (Plan 3), the schedule/standings UI, print export, and wiring `i18n.Catalog` into responses (Plan 4). The `sport_plugin_id` / `tournament_type_id` fields on `Tournament` are free-text strings in this plan because the plugin registry that would validate them doesn't exist until Plan 2.
- **Type consistency checked:** `auth.Role` used consistently across `user.go`, `session.go`, `middleware.go`, `handlers_auth.go`; `tournament.Repo` / `team.Repo` constructor and method names match what `server.go` and the importer call; `importer.ParseCSV` and `importer.ParseXLSX` share the exact same signature and return shape so `handlers_import.go` can branch on file extension without reshaping data.

---

**Plan complete and saved to `docs/superpowers/plans/2026-08-21-tournamentstudio-foundation.md`.** Two execution options:

**1. Subagent-Driven (recommended)** — I dispatch a fresh subagent per task, review between tasks, fast iteration.

**2. Inline Execution** — Execute tasks in this session using executing-plans, batch execution with checkpoints.

**Which approach?**
