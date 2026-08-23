# Web UI Foundation & Setup Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Ship a React + Vite + TypeScript SPA, embedded into the Go binary, covering login, i18n, tournament creation, and team management (manual + CSV/XLSX import) — the first UI code in the project, and the foundation Plans 4b/4c build on.

**Architecture:** Two small additive backend changes (an i18n endpoint, `roster_fields` exposure) plus a new `frontend/` React project whose build output is embedded into the Go binary via `internal/webui` (mirroring the existing `internal/i18n`/`internal/plugin` `go:embed` pattern) and served by a new SPA-fallback catch-all route. The frontend is a standalone TypeScript project with its own test suite (Vitest + React Testing Library, Playwright for one real end-to-end test) — it does not touch Go test files except where a Go handler/type changes.

**Tech Stack:** Go (unchanged), React 19 + Vite 8 + TypeScript 6 + Tailwind CSS 4 (`@tailwindcss/vite`) + React Router 7 + TanStack Query 5 + react-i18next 17 — all versions confirmed by a real `npm create vite` + install before writing this plan, not assumed.

**Spec:** `docs/superpowers/specs/2026-08-23-tournamentstudio-web-ui-foundation-design.md`

## Global Constraints

- The frontend build output is git-ignored (`internal/webui/dist/`), not committed — matching standard practice. `go build`/`go test` require `frontend/`'s build to have run first (`npm install && npm run build` in `frontend/`), documented in each task that needs it; this two-step process was already approved in the spec's Non-Goals.
- `internal/webui`'s `go:embed all:dist` requires `internal/webui/dist/` to contain at least one file at Go compile time — every task's own verification step runs the frontend build first, so this is never actually a problem in practice, but it's why Task 3 must run before any other task's `go build`/`go test` can succeed on a clean checkout.
- JSON API convention (established since Plan 1): snake_case field names via explicit struct tags — applies to the two backend changes in Tasks 1-2; the frontend TypeScript types in Task 4 mirror those exact field names.
- Every screen's task adds that screen's translation keys to both `internal/i18n/bundles/en.json` and `de.json` in the same commit — no screen ships with only-English strings.
- Translation keys are flat `snake_case` (e.g. `login_title`), matching the existing bundled keys (`next_heat`, `on_time`). Never a dotted key like `login.title` — `internal/i18n.Catalog.Strings` returns a genuinely flat map, and i18next's default `keySeparator` treats `.` as a nested-object path, which would silently fail to resolve against a flat map. A prefix + underscore (`login_title`, `login_username`) keeps a screen's keys visually grouped without tripping that behavior.
- Frontend component/unit tests use Vitest + React Testing Library with `fetch` mocked; the one Playwright end-to-end test (Task 12) is the only place a real browser and a real running Go binary are both involved.
- `server.New`'s signature changes in Task 1 (`New(s, plugins, catalog)`) — every task after Task 1 that touches `server_test.go`'s `newTestServer` helper already sees the updated signature; no task needs to change it again.

---

## Task 1: i18n backend — catalog exports and the `GET /api/i18n/{lang}` endpoint

**Files:**
- Modify: `internal/i18n/i18n.go` (add `Languages`, `Strings`)
- Modify: `internal/i18n/i18n_test.go` (add tests for both)
- Modify: `internal/server/server.go` (add `i18n` field, change `New`'s signature, register the route)
- Modify: `internal/server/server_test.go` (update `newTestServer`'s `New` call)
- Create: `internal/server/handlers_i18n.go`
- Create: `internal/server/i18n_test.go`
- Modify: `cmd/tournamentstudio/main.go` (load the catalog, pass it into `server.New`)

**Interfaces:**
- Produces: `(*i18n.Catalog).Languages() []string`; `(*i18n.Catalog).Strings(lang string) map[string]string` (English merged underneath, then overridden by `lang`'s own keys — a language with no entries at all returns pure English, matching `Translate`'s existing fallback behavior). `(*Server).i18n *i18n.Catalog` field; `server.New(s *store.Store, plugins *plugin.Engine, catalog *i18n.Catalog) *Server`. Every later task's frontend code (Task 5) consumes `GET /api/i18n/{lang}` as a flat `Record<string, string>`.

- [ ] **Step 1: Write the failing catalog test**

Modify `internal/i18n/i18n_test.go` — append:

```go
func TestLanguages(t *testing.T) {
	c, err := Load("")
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	langs := c.Languages()
	if len(langs) != 2 {
		t.Fatalf("expected 2 built-in languages (en, de), got %d: %v", len(langs), langs)
	}
}

func TestStringsMergesEnglishUnderneath(t *testing.T) {
	c, err := Load("")
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	de := c.Strings("de")
	if de["next_heat"] != "NÄCHSTER" {
		t.Fatalf("expected German next_heat, got %q", de["next_heat"])
	}
	if de["on_time"] != "ON TIME" {
		t.Fatalf("expected English fallback for a key de.json doesn't override, got %q", de["on_time"])
	}
}

func TestStringsUnknownLanguageReturnsEnglish(t *testing.T) {
	c, err := Load("")
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	fr := c.Strings("fr")
	if fr["next_heat"] != "NEXT" {
		t.Fatalf("expected pure English fallback for an unknown language, got %q", fr["next_heat"])
	}
}
```

- [ ] **Step 2: Run the tests to verify they fail**

Run: `go test ./internal/i18n/... -v`
Expected: FAIL (compile error — `Languages`, `Strings` undefined)

- [ ] **Step 3: Implement the catalog methods**

Modify `internal/i18n/i18n.go` — add `"sort"` to the imports, and append:

```go
func (c *Catalog) Languages() []string {
	langs := make([]string, 0, len(c.strings))
	for lang := range c.strings {
		langs = append(langs, lang)
	}
	sort.Strings(langs)
	return langs
}

// Strings returns lang's full flat key->string map, with English's keys
// merged underneath first so a partially-translated (or entirely
// unknown) language never surfaces a raw key -- the same fallback
// Translate already applies per-key, just returning the whole map at
// once for the frontend's translation layer.
func (c *Catalog) Strings(lang string) map[string]string {
	merged := make(map[string]string, len(c.strings["en"]))
	for k, v := range c.strings["en"] {
		merged[k] = v
	}
	for k, v := range c.strings[lang] {
		merged[k] = v
	}
	return merged
}
```

- [ ] **Step 4: Run the catalog tests to verify they pass**

Run: `go test ./internal/i18n/... -v`
Expected: PASS

- [ ] **Step 5: Wire the catalog into the server**

Modify `internal/server/server.go` — add the import
`"tournamentstudio/internal/i18n"`, add `i18n *i18n.Catalog` to the
`Server` struct, change `New`'s signature and body, and register the
route:

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
}

func New(s *store.Store, plugins *plugin.Engine, catalog *i18n.Catalog) *Server {
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
	}
	srv.routes()
	return srv
}
```

Add this line inside `routes()`, alongside the other unauthenticated
routes near the top (`GET /healthz`, `POST /api/login`) — this endpoint
is deliberately not behind `authenticated`, since the login screen
itself needs translated labels before anyone is logged in:

```go
	s.mux.HandleFunc("GET /api/i18n/{lang}", s.handleI18n)
```

- [ ] **Step 6: Write the handler**

Create `internal/server/handlers_i18n.go`:

```go
package server

import (
	"encoding/json"
	"net/http"
)

func (s *Server) handleI18n(w http.ResponseWriter, r *http.Request) {
	lang := r.PathValue("lang")
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(s.i18n.Strings(lang))
}
```

- [ ] **Step 7: Fix the now-broken `New` call sites**

Modify `internal/server/server_test.go`'s `newTestServer` — the file
currently calls `New(s, engine)`; find every direct `New(...)` call
across `internal/server/*_test.go` (grep for `server.New(` and `= New(`
within the package) and add a third argument. For `newTestServer` in
`server_test.go`, load a catalog from an empty external dir (built-in
bundles only, matching the pattern `plugin.Load(t.TempDir())` already
uses):

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

	catalog, err := i18n.Load(t.TempDir())
	if err != nil {
		t.Fatalf("load i18n: %v", err)
	}

	return New(s, engine, catalog)
}
```

Add `"tournamentstudio/internal/i18n"` to this file's imports. If any
other test file in `internal/server` calls `New(...)` directly (not
through `newTestServer`), update those call sites the same way — grep
first to find them; Plan 2's Task 7 hit exactly this situation
(`auth_test.go`/`import_test.go` called `New(st)` directly), so check
those two files specifically.

- [ ] **Step 8: Write the HTTP test**

Create `internal/server/i18n_test.go`:

```go
package server

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestGetI18nReturnsFlatMap(t *testing.T) {
	s := newTestServer(t)

	req := httptest.NewRequest(http.MethodGet, "/api/i18n/de", nil)
	rec := httptest.NewRecorder()
	s.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}

	var m map[string]string
	if err := json.Unmarshal(rec.Body.Bytes(), &m); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if m["next_heat"] != "NÄCHSTER" {
		t.Fatalf("expected German translation, got %q", m["next_heat"])
	}
}

func TestGetI18nRequiresNoAuth(t *testing.T) {
	s := newTestServer(t)

	req := httptest.NewRequest(http.MethodGet, "/api/i18n/en", nil)
	// Deliberately no Authorization header.
	rec := httptest.NewRecorder()
	s.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200 with no auth header, got %d: %s", rec.Code, rec.Body.String())
	}
}
```

- [ ] **Step 9: Run the tests to verify they pass**

Run: `go build ./... && go test ./internal/i18n/... ./internal/server/... -v`
Expected: PASS

- [ ] **Step 10: Wire the catalog into `main.go`**

Modify `cmd/tournamentstudio/main.go` — add the import
`"tournamentstudio/internal/i18n"`, and add this block before
`s := server.New(...)`, then update that call:

```go
	languagesDir := os.Getenv("TOURNAMENTSTUDIO_LANGUAGES")
	if languagesDir == "" {
		languagesDir = "languages"
	}
	catalog, err := i18n.Load(languagesDir)
	if err != nil {
		log.Fatalf("load i18n: %v", err)
	}

	s := server.New(st, engine, catalog)
```

- [ ] **Step 11: Run the full suite and commit**

Run: `go build ./... && go test ./... -count=1`
Expected: PASS, all packages.

```bash
git add internal/i18n internal/server/server.go internal/server/server_test.go internal/server/handlers_i18n.go internal/server/i18n_test.go cmd/tournamentstudio/main.go
git commit -m "feat: expose i18n catalog via GET /api/i18n/{lang}"
```

---

## Task 2: Expose `roster_fields` from `GET /api/plugins`

**Files:**
- Modify: `internal/plugin/types.go` (add `json` tags to `RosterField`)
- Modify: `internal/server/handlers_plugins.go` (add `RosterFields` to the sport response, populate it)
- Modify: `internal/server/plugins_test.go` (assert the field is present)

**Interfaces:**
- Consumes: `plugin.SportPlugin.RosterFields []RosterField` (already populated by `parseSportPlugin`, unchanged).
- Produces: `plugin.RosterField{Key, Label string, Required bool}` now carries `json:"key"`/`json:"label"`/`json:"required"` tags. `pluginSportResponse.RosterFields []plugin.RosterField`. Task 4's frontend TypeScript types mirror this shape.

- [ ] **Step 1: Add JSON tags to the domain type**

Modify `internal/plugin/types.go` — change `RosterField` to:

```go
type RosterField struct {
	Key      string `json:"key"`
	Label    string `json:"label"`
	Required bool   `json:"required"`
}
```

(This matches the precedent already set by `plugin.Cut`/`plugin.Division`
in Plan 2 — tags go directly on the domain struct, reused in the HTTP
response rather than duplicated into a parallel DTO.)

- [ ] **Step 2: Write the failing test**

Modify `internal/server/plugins_test.go`'s `TestGetPluginsListsBundledPlugins`.
The bundled `dragonboat` plugin declares exactly one roster field —
`{key = "boat_class", label = "Boat class", required = false}` (confirmed
by reading `internal/plugin/bundled/dragonboat.lua`). Replace the
existing decode struct and the sport-finding loop with:

```go
	var resp struct {
		Sports []struct {
			ID           string `json:"id"`
			RosterFields []struct {
				Key      string `json:"key"`
				Label    string `json:"label"`
				Required bool   `json:"required"`
			} `json:"roster_fields"`
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
			if len(sp.RosterFields) != 1 {
				t.Fatalf("expected 1 roster field, got %d", len(sp.RosterFields))
			}
			if sp.RosterFields[0].Key != "boat_class" || sp.RosterFields[0].Label != "Boat class" || sp.RosterFields[0].Required != false {
				t.Fatalf("unexpected roster field: %+v", sp.RosterFields[0])
			}
		}
	}
	if !foundSport {
		t.Fatalf("expected dragonboat in sports list, got %v", resp.Sports)
	}
```

(Leave the rest of the test function — the request setup and the
`tournament_types` loop below this block — unchanged.)

- [ ] **Step 3: Run the test to verify it fails**

Run: `go test ./internal/server/... -run TestGetPlugins -v`
Expected: FAIL (missing field on the response, or compile error if the
decode struct was extended without the handler producing it yet)

- [ ] **Step 4: Implement**

Modify `internal/server/handlers_plugins.go`:

```go
package server

import (
	"encoding/json"
	"net/http"

	"tournamentstudio/internal/plugin"
)

type pluginSportResponse struct {
	ID                        string                `json:"id"`
	DisplayName               string                `json:"display_name"`
	CompatibleTournamentTypes []string              `json:"compatible_tournament_types"`
	RosterFields              []plugin.RosterField  `json:"roster_fields"`
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
			RosterFields:              sp.RosterFields,
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

(`RosterFields` on `pluginSportResponse` will `omitempty`-style show as
`null` in JSON for a sport plugin with zero declared fields, since it's
a nil slice with no `omitempty` tag — matching how every other
slice-typed field in this codebase's responses already behaves, e.g.
`CompatibleTournamentTypes`. Do not add `omitempty` — consistency with
the rest of this response matters more than shaving a few bytes.)

- [ ] **Step 5: Run the test to verify it passes**

Run: `go test ./internal/server/... -run TestGetPlugins -v`
Expected: PASS

- [ ] **Step 6: Run the full suite and commit**

Run: `go build ./... && go test ./... -count=1`
Expected: PASS, all packages.

```bash
git add internal/plugin/types.go internal/server/handlers_plugins.go internal/server/plugins_test.go
git commit -m "feat: expose roster_fields from GET /api/plugins"
```

---

## Task 3: Frontend scaffold and Go embedding pipeline

This is the largest task in the plan: it creates the whole `frontend/`
project, proves the full pipeline (React source → `vite build` →
`go:embed` → served by the Go binary → real page in a real request)
works end to end, and wires the SPA-fallback route into the server.
Every later frontend task builds inside `frontend/src/` without
touching this task's scaffolding again.

**Files:**
- Create: `frontend/package.json`, `frontend/vite.config.ts`,
  `frontend/tsconfig.json`, `frontend/tsconfig.app.json`,
  `frontend/tsconfig.node.json`, `frontend/index.html`
- Create: `frontend/src/main.tsx`, `frontend/src/App.tsx`,
  `frontend/src/index.css`, `frontend/src/vite-env.d.ts`
- Create: `frontend/tests/setup.ts`, `frontend/src/App.test.tsx`
- Create: `internal/webui/webui.go`
- Create: `internal/server/handlers_webui.go`
- Create: `internal/server/webui_test.go`
- Modify: `internal/server/server.go` (add `webFS` field, change `New`'s
  signature again, register the catch-all route)
- Modify: `internal/server/server_test.go` (update `newTestServer`'s
  `New` call — a `testing/fstest.MapFS`, not a real embedded FS)
- Modify: `cmd/tournamentstudio/main.go` (derive the embedded FS,
  pass it into `server.New`)
- Modify: `.gitignore` (ignore `internal/webui/dist/`,
  `frontend/node_modules/`)

**Interfaces:**
- Produces: `webui.DistFS embed.FS` (package `internal/webui`);
  `server.New(s *store.Store, plugins *plugin.Engine, catalog *i18n.Catalog, webFS fs.FS) *Server` (the 4th and final positional
  parameter this plan adds — no later task changes this signature
  again). The frontend's `frontend/src/` directory structure
  (`main.tsx`, `App.tsx`, `index.css`) is what every later frontend task
  builds inside.

- [ ] **Step 1: Scaffold the Vite project**

From the repo root:

```bash
npm create vite@latest frontend -- --template react-ts
cd frontend
npm install
```

This produces a standard Vite + React + TypeScript scaffold (confirmed
against real npm registry versions before writing this plan: React
19.2.8, Vite 8.2.2, TypeScript 6.0.2, `@vitejs/plugin-react` 6.0.4 — the
exact versions `npm install` resolves may have moved on since; that's
expected and fine, this plan does not pin exact versions).

- [ ] **Step 2: Add the remaining dependencies**

Still inside `frontend/`:

```bash
npm install react-router-dom @tanstack/react-query i18next react-i18next
npm install -D tailwindcss @tailwindcss/vite vitest @testing-library/react @testing-library/jest-dom @testing-library/user-event jsdom
```

- [ ] **Step 3: Configure Vite — Tailwind, the embed-friendly build output, the dev proxy, and Vitest**

Replace `frontend/vite.config.ts` with:

```ts
/// <reference types="vitest/config" />
import { defineConfig } from 'vite'
import react from '@vitejs/plugin-react'
import tailwindcss from '@tailwindcss/vite'

export default defineConfig({
  plugins: [react(), tailwindcss()],
  build: {
    outDir: '../internal/webui/dist',
    emptyOutDir: true,
  },
  server: {
    proxy: {
      '/api': 'http://localhost:8080',
      '/healthz': 'http://localhost:8080',
    },
  },
  test: {
    environment: 'jsdom',
    setupFiles: ['./tests/setup.ts'],
    globals: true,
  },
})
```

`build.outDir` points build output directly at `internal/webui/dist` —
Vite build output is where the Go side's `go:embed` reads from, no
separate copy step. `server.proxy` makes `npm run dev` (Vite's dev
server, typically `localhost:5173`) forward `/api`/`/healthz` requests
to a Go binary running on `localhost:8080`, so frontend development
doesn't require the frontend to be embedded/rebuilt on every change.

Create `frontend/tests/setup.ts`:

```ts
import '@testing-library/jest-dom'
```

- [ ] **Step 4: Replace Tailwind's CSS entry point**

Replace the contents of `frontend/src/index.css` with:

```css
@import "tailwindcss";
```

(Delete any other CSS the scaffold generated into this file — Tailwind
v4's single `@import` line replaces the old `@tailwind base/components/utilities`
three-line form from Tailwind v3.)

- [ ] **Step 5: Write a minimal placeholder `App.tsx` proving the pipeline**

Replace `frontend/src/App.tsx` with:

```tsx
function App() {
  return (
    <div className="p-8">
      <h1 className="text-2xl font-bold">TournamentStudio</h1>
      <p>Frontend pipeline is wired up.</p>
    </div>
  )
}

export default App
```

Replace `frontend/src/main.tsx` with:

```tsx
import { StrictMode } from 'react'
import { createRoot } from 'react-dom/client'
import './index.css'
import App from './App.tsx'

createRoot(document.getElementById('root')!).render(
  <StrictMode>
    <App />
  </StrictMode>,
)
```

Delete `frontend/src/App.css` and any scaffold-generated files under
`frontend/src/assets/` — this task's `App.tsx` doesn't reference them,
and an unused import would fail the TypeScript build.

- [ ] **Step 6: Write a component test**

Create `frontend/src/App.test.tsx`:

```tsx
import { describe, expect, it } from 'vitest'
import { render, screen } from '@testing-library/react'
import App from './App'

describe('App', () => {
  it('renders the app heading', () => {
    render(<App />)
    expect(screen.getByRole('heading', { name: 'TournamentStudio' })).toBeInTheDocument()
  })
})
```

Add a `"test": "vitest run"` script to `frontend/package.json`'s
`scripts` block (alongside the existing `dev`/`build`/`preview`
scripts the scaffold generated).

- [ ] **Step 7: Run the frontend test suite and build**

From `frontend/`:

```bash
npm test
npm run build
```

Expected: the test passes, and `npm run build` succeeds, producing
`internal/webui/dist/index.html` and `internal/webui/dist/assets/*`
(relative to the repo root, since `outDir` points there).

- [ ] **Step 8: Ignore build output and dependencies**

Modify `.gitignore` — add:

```
/internal/webui/dist
/frontend/node_modules
```

- [ ] **Step 9: Write the Go embed package**

Create `internal/webui/webui.go`:

```go
package webui

import "embed"

//go:embed all:dist
var DistFS embed.FS
```

- [ ] **Step 10: Run `go build` to verify the embed compiles**

From the repo root:

Run: `go build ./...`
Expected: succeeds — `internal/webui/dist/` was populated by Step 7's
`npm run build`, so `go:embed all:dist` has real files to embed.

- [ ] **Step 11: Write the failing Go test for the SPA handler**

Create `internal/server/webui_test.go`:

```go
package server

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"testing/fstest"
)

func testWebFS() fstest.MapFS {
	return fstest.MapFS{
		"index.html":        &fstest.MapFile{Data: []byte("<html>index</html>")},
		"assets/app.js":     &fstest.MapFile{Data: []byte("console.log('app')")},
	}
}

func TestWebUIServesIndexAtRoot(t *testing.T) {
	s := newTestServerWithWebFS(t, testWebFS())

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rec := httptest.NewRecorder()
	s.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
	if rec.Body.String() != "<html>index</html>" {
		t.Fatalf("unexpected body: %s", rec.Body.String())
	}
}

func TestWebUIServesRealAssetFile(t *testing.T) {
	s := newTestServerWithWebFS(t, testWebFS())

	req := httptest.NewRequest(http.MethodGet, "/assets/app.js", nil)
	rec := httptest.NewRecorder()
	s.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
	if rec.Body.String() != "console.log('app')" {
		t.Fatalf("unexpected body: %s", rec.Body.String())
	}
}

func TestWebUIFallsBackToIndexForClientSideRoutes(t *testing.T) {
	s := newTestServerWithWebFS(t, testWebFS())

	req := httptest.NewRequest(http.MethodGet, "/tournaments/123", nil)
	rec := httptest.NewRecorder()
	s.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
	if rec.Body.String() != "<html>index</html>" {
		t.Fatalf("expected index.html fallback, got: %s", rec.Body.String())
	}
}
```

- [ ] **Step 12: Run the test to verify it fails**

Run: `go test ./internal/server/... -run TestWebUI -v`
Expected: FAIL (compile error — `newTestServerWithWebFS` undefined)

- [ ] **Step 13: Implement the SPA handler**

Create `internal/server/handlers_webui.go`:

```go
package server

import (
	"io/fs"
	"net/http"
)

// webUIHandler serves the embedded frontend build: a real file at the
// requested path if one exists, else index.html (so React Router's
// client-side routes -- e.g. /tournaments/123 -- survive a hard
// refresh, since only "/" and real asset paths exist as actual files
// in the embedded FS).
func (s *Server) webUIHandler() http.Handler {
	fileServer := http.FileServerFS(s.webFS)
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if _, err := fs.Stat(s.webFS, r.URL.Path[1:]); err != nil {
			r2 := r.Clone(r.Context())
			r2.URL.Path = "/"
			fileServer.ServeHTTP(w, r2)
			return
		}
		fileServer.ServeHTTP(w, r)
	})
}
```

- [ ] **Step 14: Wire `webFS` into the server**

Modify `internal/server/server.go` — add `"io/fs"` to the imports, add
`webFS fs.FS` to the `Server` struct, change `New`'s signature and body,
and register the catch-all route (this must be able to coexist with
every `/api/...` route already registered — Go's `net/http` ServeMux
resolves by pattern specificity, not registration order, so a bare `/`
pattern only ever matches requests nothing more specific already
claimed):

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

Add this line inside `routes()`, anywhere (position doesn't matter,
per the specificity note above):

```go
	s.mux.Handle("/", s.webUIHandler())
```

- [ ] **Step 15: Add the test helper and fix `newTestServer`**

Modify `internal/server/server_test.go` — add `"testing/fstest"` to the
imports, and add this helper alongside `newTestServer`:

```go
// newTestServerWithWebFS is newTestServer's shape, but with an explicit
// webFS -- used by webui_test.go, which needs to control exactly what
// files "exist" to test the SPA-fallback behavior. newTestServer itself
// uses a trivial single-file fstest.MapFS, since no other test in this
// package cares about the embedded frontend's actual content.
func newTestServerWithWebFS(t *testing.T, webFS fs.FS) *Server {
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

	catalog, err := i18n.Load(t.TempDir())
	if err != nil {
		t.Fatalf("load i18n: %v", err)
	}

	return New(s, engine, catalog, webFS)
}

func newTestServer(t *testing.T) *Server {
	t.Helper()
	return newTestServerWithWebFS(t, fstest.MapFS{
		"index.html": &fstest.MapFile{Data: []byte("<html></html>")},
	})
}
```

Remove `newTestServer`'s old body (the one Task 1 last modified) — this
step's version replaces it entirely, factoring the common setup into
`newTestServerWithWebFS`. Add `"io/fs"` to this file's imports too (for
the `fs.FS` parameter type).

- [ ] **Step 16: Run the webui tests to verify they pass**

Run: `go test ./internal/server/... -run TestWebUI -v`
Expected: PASS

- [ ] **Step 17: Wire the embedded FS into `main.go`**

Modify `cmd/tournamentstudio/main.go` — add `"io/fs"` and
`"tournamentstudio/internal/webui"` to the imports, and add this block
before `s := server.New(...)`, then update that call:

```go
	frontendFS, err := fs.Sub(webui.DistFS, "dist")
	if err != nil {
		log.Fatalf("prepare embedded frontend: %v", err)
	}

	s := server.New(st, engine, catalog, frontendFS)
```

- [ ] **Step 18: Run the full Go suite**

Run: `go build ./... && go test ./... -count=1`
Expected: PASS, all packages.

- [ ] **Step 19: Manually verify the real end-to-end pipeline**

This is the one step in this whole plan that must be checked by hand,
not just by an automated test — it's the proof that a real browser
hitting the real running binary actually gets the real frontend:

```bash
go run ./cmd/tournamentstudio &
sleep 1
curl -s http://localhost:8080/ | grep -q "TournamentStudio" && echo "OK: index served"
curl -s http://localhost:8080/tournaments/999 | grep -q "TournamentStudio" && echo "OK: SPA fallback served"
curl -s http://localhost:8080/healthz
kill %1
```

Expected: both `OK:` lines print, and `/healthz` still returns
`{"status":"ok"}` (proving the catch-all route didn't swallow existing
API routes).

- [ ] **Step 20: Commit**

```bash
git add frontend internal/webui internal/server/server.go internal/server/server_test.go internal/server/handlers_webui.go internal/server/webui_test.go cmd/tournamentstudio/main.go .gitignore
git commit -m "feat: scaffold React frontend, embed build output into the Go binary"
```

---

## Task 4: API client and TypeScript types

**Files:**
- Create: `frontend/src/api/types.ts`
- Create: `frontend/src/api/client.ts`
- Create: `frontend/src/api/client.test.ts`

**Interfaces:**
- Produces: `Tournament`, `Team`, `RosterField`, `SportPlugin`,
  `TournamentTypePlugin`, `PluginsResponse`, `LoginResponse`,
  `ImportProblem`, `ImportResult` (all in `types.ts`, field names
  mirroring the Go JSON tags exactly — `snake_case`, matching Tasks 1-2
  and every prior backend plan). `getToken`, `setToken`, `clearToken`,
  `ApiError`, `setUnauthorizedHandler`, `api.get`/`api.post`/`api.patch`/`api.postForm`
  (all in `client.ts`) — every later frontend task's data fetching goes
  through `api`, never a raw `fetch` call.

- [ ] **Step 1: Write the TypeScript types**

Create `frontend/src/api/types.ts`:

```ts
export interface Tournament {
  id: number
  name: string
  sport_plugin_id: string
  tournament_type_id: string
  language: string
  status: string
}

export interface Team {
  id: number
  tournament_id: number
  name: string
  club: string
  extra_fields: Record<string, string>
}

export interface RosterField {
  key: string
  label: string
  required: boolean
}

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

export interface PluginsResponse {
  sports: SportPlugin[]
  tournament_types: TournamentTypePlugin[]
}

export type Role = 'organizer' | 'time_entry' | 'spectator'

export interface LoginResponse {
  token: string
  role: Role
}

export interface ImportProblem {
  row_index: number
  message: string
}

export interface ImportResult {
  imported: number
  problems: ImportProblem[]
}
```

- [ ] **Step 2: Write the failing client tests**

Create `frontend/src/api/client.test.ts`:

```ts
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { api, ApiError, setToken, setUnauthorizedHandler } from './client'

describe('api client', () => {
  beforeEach(() => {
    localStorage.clear()
    vi.stubGlobal('fetch', vi.fn())
  })

  afterEach(() => {
    vi.unstubAllGlobals()
  })

  it('attaches the bearer token when one is stored', async () => {
    setToken('abc123')
    ;(fetch as any).mockResolvedValue(new Response(JSON.stringify({ ok: true }), { status: 200 }))

    await api.get('/api/whoami')

    const [, init] = (fetch as any).mock.calls[0]
    expect(init.headers.get('Authorization')).toBe('Bearer abc123')
  })

  it('clears the token and calls the unauthorized handler on 401', async () => {
    setToken('abc123')
    const handler = vi.fn()
    setUnauthorizedHandler(handler)
    ;(fetch as any).mockResolvedValue(new Response('unauthorized', { status: 401 }))

    await expect(api.get('/api/whoami')).rejects.toBeInstanceOf(ApiError)
    expect(localStorage.getItem('ts_token')).toBeNull()
    expect(handler).toHaveBeenCalled()
  })

  it('throws ApiError with the response body text on other errors', async () => {
    ;(fetch as any).mockResolvedValue(new Response('bad request', { status: 400 }))

    await expect(api.get('/api/whoami')).rejects.toMatchObject({ status: 400, message: 'bad request' })
  })

  it('sends a JSON body with Content-Type on post', async () => {
    ;(fetch as any).mockResolvedValue(new Response(JSON.stringify({ id: 1 }), { status: 201 }))

    await api.post('/api/tournaments', { name: 'Test' })

    const [, init] = (fetch as any).mock.calls[0]
    expect(init.method).toBe('POST')
    expect(init.headers.get('Content-Type')).toBe('application/json')
    expect(init.body).toBe(JSON.stringify({ name: 'Test' }))
  })
})
```

- [ ] **Step 3: Run the tests to verify they fail**

Run (from `frontend/`): `npm test`
Expected: FAIL (module `./client` doesn't exist yet)

- [ ] **Step 4: Implement the client**

Create `frontend/src/api/client.ts`:

```ts
const TOKEN_KEY = 'ts_token'

export function getToken(): string | null {
  return localStorage.getItem(TOKEN_KEY)
}

export function setToken(token: string): void {
  localStorage.setItem(TOKEN_KEY, token)
}

export function clearToken(): void {
  localStorage.removeItem(TOKEN_KEY)
}

export class ApiError extends Error {
  status: number
  constructor(status: number, message: string) {
    super(message)
    this.status = status
  }
}

let onUnauthorized: (() => void) | null = null

export function setUnauthorizedHandler(handler: () => void): void {
  onUnauthorized = handler
}

async function request<T>(path: string, options: RequestInit = {}): Promise<T> {
  const token = getToken()
  const headers = new Headers(options.headers)
  if (token) {
    headers.set('Authorization', `Bearer ${token}`)
  }
  if (options.body && !(options.body instanceof FormData) && !headers.has('Content-Type')) {
    headers.set('Content-Type', 'application/json')
  }

  const res = await fetch(path, { ...options, headers })

  if (res.status === 401) {
    clearToken()
    onUnauthorized?.()
    throw new ApiError(401, 'unauthorized')
  }
  if (!res.ok) {
    const text = await res.text()
    throw new ApiError(res.status, text || res.statusText)
  }
  if (res.status === 204) {
    return undefined as T
  }
  return res.json() as Promise<T>
}

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

- [ ] **Step 5: Run the tests to verify they pass**

Run (from `frontend/`): `npm test`
Expected: PASS

- [ ] **Step 6: Commit**

```bash
git add frontend/src/api
git commit -m "feat: add typed API client"
```

---

## Task 5: i18n frontend wiring

This task wires the translation-key layer itself; it does not touch
`App.tsx` (Task 7's app shell is the first real screen to consume
`useTranslation`, avoiding any conflict with Task 3's placeholder
component this task would otherwise have to fight over).

**Files:**
- Create: `frontend/src/i18n/backend.ts`
- Create: `frontend/src/i18n/i18n.ts`
- Create: `frontend/src/i18n/i18n.test.ts`
- Modify: `frontend/src/main.tsx` (import `./i18n/i18n` for its
  initialization side effect)

**Interfaces:**
- Consumes: `GET /api/i18n/{lang}` (Task 1).
- Produces: `i18n` (default export, the configured `i18next` instance —
  imported for its `initReactI18next` registration side effect by any
  component using `useTranslation`, starting with Task 7);
  `changeLanguage(lang: string): void`; `AVAILABLE_LANGUAGES: string[]`
  (`['en', 'de']` — the two bundled languages; the backend can serve any
  language present via `GET /api/i18n/{lang}`, including future
  drop-in files, but this plan's UI only offers a switcher between the
  two known bundled ones — dynamic discovery of additional languages is
  explicitly out of scope for 4a, not a silent gap).

- [ ] **Step 1: Write the custom i18next backend**

Create `frontend/src/i18n/backend.ts`:

```ts
import type { BackendModule, ReadCallback } from 'i18next'

// A minimal i18next backend that fetches one language's whole flat
// key->string map from GET /api/i18n/{lang} in a single request,
// instead of i18next's usual per-namespace file convention -- this
// project has exactly one namespace (the whole catalog), matching the
// backend's own shape (internal/i18n.Catalog.Strings).
export const apiBackend: BackendModule = {
  type: 'backend',
  init: () => {},
  read: (language: string, _namespace: string, callback: ReadCallback) => {
    fetch(`/api/i18n/${language}`)
      .then((res) => {
        if (!res.ok) {
          throw new Error(`failed to load translations for ${language}: ${res.status}`)
        }
        return res.json()
      })
      .then((data) => callback(null, data))
      .catch((err) => callback(err, null))
  },
}
```

- [ ] **Step 2: Write the i18next init module**

Create `frontend/src/i18n/i18n.ts`:

```ts
import i18n from 'i18next'
import { initReactI18next } from 'react-i18next'
import { apiBackend } from './backend'

const LANGUAGE_STORAGE_KEY = 'ts_language'

export const AVAILABLE_LANGUAGES = ['en', 'de']

function detectLanguage(): string {
  const stored = localStorage.getItem(LANGUAGE_STORAGE_KEY)
  if (stored && AVAILABLE_LANGUAGES.includes(stored)) {
    return stored
  }
  const browserLang = navigator.language.split('-')[0]
  return AVAILABLE_LANGUAGES.includes(browserLang) ? browserLang : 'en'
}

void i18n
  .use(apiBackend)
  .use(initReactI18next)
  .init({
    lng: detectLanguage(),
    fallbackLng: 'en',
    interpolation: { escapeValue: false },
  })

export function changeLanguage(lang: string): void {
  localStorage.setItem(LANGUAGE_STORAGE_KEY, lang)
  void i18n.changeLanguage(lang)
}

export default i18n
```

- [ ] **Step 3: Write the failing test**

Create `frontend/src/i18n/i18n.test.ts`:

```ts
import { beforeEach, describe, expect, it, vi } from 'vitest'

describe('i18n', () => {
  beforeEach(() => {
    localStorage.clear()
    vi.stubGlobal(
      'fetch',
      vi.fn(async (url: string) => {
        const lang = url.split('/').pop()
        const data = lang === 'de' ? { greeting: 'Hallo' } : { greeting: 'Hello' }
        return new Response(JSON.stringify(data), { status: 200 })
      }),
    )
  })

  it('loads translations from the API and resolves a key', async () => {
    const { default: i18n } = await import('./i18n')
    await i18n.init
    await new Promise((resolve) => i18n.on('loaded', resolve))
    expect(i18n.t('greeting')).toBe('Hello')
  })

  it('changeLanguage persists the choice and switches languages', async () => {
    const { default: i18n, changeLanguage } = await import('./i18n')
    await new Promise((resolve) => i18n.on('loaded', resolve))

    changeLanguage('de')
    await new Promise((resolve) => i18n.on('languageChanged', resolve))

    expect(localStorage.getItem('ts_language')).toBe('de')
    expect(i18n.t('greeting')).toBe('Hallo')
  })
})
```

(Both tests dynamically `import('./i18n')` rather than a static
top-level import, since the module runs its `i18n.init()` call as a
side effect at import time — a static import would fire that side
effect before `vi.stubGlobal('fetch', ...)` in `beforeEach` has run.)

- [ ] **Step 4: Run the test to verify it fails**

Run (from `frontend/`): `npm test`
Expected: FAIL (module `./i18n` doesn't exist yet)

- [ ] **Step 5: Run the test to verify it passes**

(Steps 1-2 already implement the module — this step just confirms it.)

Run (from `frontend/`): `npm test`
Expected: PASS

- [ ] **Step 6: Wire the init side effect into the app entry point**

Modify `frontend/src/main.tsx` — add this import alongside the existing
ones, before `App` is rendered:

```ts
import './i18n/i18n'
```

- [ ] **Step 7: Run the full frontend suite and build**

Run (from `frontend/`): `npm test && npm run build`
Expected: PASS, build succeeds.

- [ ] **Step 8: Commit**

```bash
git add frontend/src/i18n frontend/src/main.tsx
git commit -m "feat: wire react-i18next to the backend translation catalog"
```

---

## Task 6: Auth — login screen, auth context, protected routes

**Files:**
- Create: `frontend/src/auth/AuthContext.tsx`
- Create: `frontend/src/auth/AuthContext.test.tsx`
- Create: `frontend/src/auth/ProtectedRoute.tsx`
- Create: `frontend/src/pages/LoginPage.tsx`
- Create: `frontend/src/pages/LoginPage.test.tsx`
- Modify: `internal/i18n/bundles/en.json`, `internal/i18n/bundles/de.json`

**Interfaces:**
- Consumes: `api`, `ApiError`, `getToken`, `setToken`, `clearToken`,
  `setUnauthorizedHandler` (Task 4); `LoginResponse`, `Role` (Task 4).
- Produces: `AuthProvider` (React component), `useAuth(): {token: string
  | null, role: Role | null, login, logout}` — Task 7's app shell reads
  `role` for nav gating and calls `logout`; `ProtectedRoute` (React
  Router layout route component) — Task 7 wraps every authenticated
  route in it.

- [ ] **Step 1: Add translation keys**

Modify `internal/i18n/bundles/en.json` — add:

```json
"login_title": "Sign In",
"login_username": "Username",
"login_password": "Password",
"login_submit": "Sign In",
"login_invalid_credentials": "Invalid username or password.",
"login_generic_error": "Something went wrong. Please try again."
```

Modify `internal/i18n/bundles/de.json` — add:

```json
"login_title": "Anmelden",
"login_username": "Benutzername",
"login_password": "Passwort",
"login_submit": "Anmelden",
"login_invalid_credentials": "Ungültiger Benutzername oder ungültiges Passwort.",
"login_generic_error": "Etwas ist schiefgelaufen. Bitte versuchen Sie es erneut."
```

(Add these as new top-level keys in each file's existing JSON object —
keep the file valid JSON, comma-separated with the existing keys.)

- [ ] **Step 2: Write the auth context**

Create `frontend/src/auth/AuthContext.tsx`:

```tsx
import { createContext, useCallback, useContext, useEffect, useState, type ReactNode } from 'react'
import { api, clearToken, getToken, setToken, setUnauthorizedHandler } from '../api/client'
import type { LoginResponse, Role } from '../api/types'

interface AuthState {
  token: string | null
  role: Role | null
}

interface AuthContextValue extends AuthState {
  login: (username: string, password: string) => Promise<void>
  logout: () => Promise<void>
}

const ROLE_KEY = 'ts_role'

const AuthContext = createContext<AuthContextValue | null>(null)

export function AuthProvider({ children }: { children: ReactNode }) {
  const [state, setState] = useState<AuthState>(() => ({
    token: getToken(),
    role: (localStorage.getItem(ROLE_KEY) as Role | null) ?? null,
  }))

  useEffect(() => {
    setUnauthorizedHandler(() => {
      localStorage.removeItem(ROLE_KEY)
      setState({ token: null, role: null })
    })
  }, [])

  const login = useCallback(async (username: string, password: string) => {
    const resp = await api.post<LoginResponse>('/api/login', { username, password })
    setToken(resp.token)
    localStorage.setItem(ROLE_KEY, resp.role)
    setState({ token: resp.token, role: resp.role })
  }, [])

  const logout = useCallback(async () => {
    try {
      await api.post('/api/logout')
    } finally {
      clearToken()
      localStorage.removeItem(ROLE_KEY)
      setState({ token: null, role: null })
    }
  }, [])

  return <AuthContext.Provider value={{ ...state, login, logout }}>{children}</AuthContext.Provider>
}

export function useAuth(): AuthContextValue {
  const ctx = useContext(AuthContext)
  if (!ctx) {
    throw new Error('useAuth must be used within an AuthProvider')
  }
  return ctx
}
```

- [ ] **Step 3: Write the auth context tests**

Create `frontend/src/auth/AuthContext.test.tsx`:

```tsx
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { AuthProvider, useAuth } from './AuthContext'
import * as client from '../api/client'

vi.mock('../api/client', async () => {
  const actual = await vi.importActual<typeof import('../api/client')>('../api/client')
  return {
    ...actual,
    api: { post: vi.fn(), get: vi.fn(), patch: vi.fn(), postForm: vi.fn() },
  }
})

function Consumer() {
  const { token, role, login, logout } = useAuth()
  return (
    <div>
      <span data-testid="token">{token ?? 'none'}</span>
      <span data-testid="role">{role ?? 'none'}</span>
      <button onClick={() => login('organizer1', 'pw')}>login</button>
      <button onClick={() => logout()}>logout</button>
    </div>
  )
}

describe('AuthContext', () => {
  beforeEach(() => {
    localStorage.clear()
    vi.mocked(client.api.post).mockReset()
  })

  it('login stores token and role', async () => {
    vi.mocked(client.api.post).mockResolvedValue({ token: 'abc', role: 'organizer' })
    render(
      <AuthProvider>
        <Consumer />
      </AuthProvider>,
    )

    await userEvent.click(screen.getByText('login'))

    await waitFor(() => expect(screen.getByTestId('token')).toHaveTextContent('abc'))
    expect(screen.getByTestId('role')).toHaveTextContent('organizer')
    expect(localStorage.getItem('ts_token')).toBe('abc')
  })

  it('logout clears token and role', async () => {
    vi.mocked(client.api.post).mockResolvedValue({ token: 'abc', role: 'organizer' })
    render(
      <AuthProvider>
        <Consumer />
      </AuthProvider>,
    )
    await userEvent.click(screen.getByText('login'))
    await waitFor(() => expect(screen.getByTestId('token')).toHaveTextContent('abc'))

    vi.mocked(client.api.post).mockResolvedValue(undefined)
    await userEvent.click(screen.getByText('logout'))

    await waitFor(() => expect(screen.getByTestId('token')).toHaveTextContent('none'))
    expect(localStorage.getItem('ts_token')).toBeNull()
  })
})
```

- [ ] **Step 4: Run the tests to verify they fail**

Run (from `frontend/`): `npm test`
Expected: FAIL (module `./AuthContext` doesn't exist yet)

- [ ] **Step 5: Run the tests to verify they pass**

(Step 2 already implements the module.)

Run (from `frontend/`): `npm test`
Expected: PASS

- [ ] **Step 6: Write the protected route wrapper**

Create `frontend/src/auth/ProtectedRoute.tsx`:

```tsx
import { Navigate, Outlet } from 'react-router-dom'
import { useAuth } from './AuthContext'

export function ProtectedRoute() {
  const { token } = useAuth()
  if (!token) {
    return <Navigate to="/login" replace />
  }
  return <Outlet />
}
```

- [ ] **Step 7: Write the login page**

Create `frontend/src/pages/LoginPage.tsx`:

```tsx
import { useState, type FormEvent } from 'react'
import { useNavigate } from 'react-router-dom'
import { useTranslation } from 'react-i18next'
import { useAuth } from '../auth/AuthContext'
import { ApiError } from '../api/client'

export function LoginPage() {
  const { t } = useTranslation()
  const { login } = useAuth()
  const navigate = useNavigate()
  const [username, setUsername] = useState('')
  const [password, setPassword] = useState('')
  const [error, setError] = useState<string | null>(null)
  const [submitting, setSubmitting] = useState(false)

  async function handleSubmit(e: FormEvent) {
    e.preventDefault()
    setError(null)
    setSubmitting(true)
    try {
      await login(username, password)
      navigate('/tournaments')
    } catch (err) {
      if (err instanceof ApiError && err.status === 401) {
        setError(t('login_invalid_credentials'))
      } else {
        setError(t('login_generic_error'))
      }
    } finally {
      setSubmitting(false)
    }
  }

  return (
    <div className="flex min-h-screen items-center justify-center bg-gray-50">
      <form onSubmit={handleSubmit} className="w-full max-w-sm space-y-4 rounded-lg bg-white p-8 shadow">
        <h1 className="text-xl font-bold">{t('login_title')}</h1>
        {error && (
          <p role="alert" className="text-sm text-red-600">
            {error}
          </p>
        )}
        <div>
          <label htmlFor="username" className="block text-sm font-medium">
            {t('login_username')}
          </label>
          <input
            id="username"
            value={username}
            onChange={(e) => setUsername(e.target.value)}
            className="mt-1 w-full rounded border px-3 py-2"
            required
          />
        </div>
        <div>
          <label htmlFor="password" className="block text-sm font-medium">
            {t('login_password')}
          </label>
          <input
            id="password"
            type="password"
            value={password}
            onChange={(e) => setPassword(e.target.value)}
            className="mt-1 w-full rounded border px-3 py-2"
            required
          />
        </div>
        <button
          type="submit"
          disabled={submitting}
          className="w-full rounded bg-blue-600 px-4 py-2 text-white disabled:opacity-50"
        >
          {t('login_submit')}
        </button>
      </form>
    </div>
  )
}
```

- [ ] **Step 8: Write the login page tests**

Create `frontend/src/pages/LoginPage.test.tsx`:

```tsx
import { describe, expect, it, vi } from 'vitest'
import { render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { MemoryRouter } from 'react-router-dom'
import { LoginPage } from './LoginPage'
import { ApiError } from '../api/client'

vi.mock('react-i18next', () => ({
  useTranslation: () => ({ t: (key: string) => key }),
}))

const loginMock = vi.fn()
vi.mock('../auth/AuthContext', () => ({
  useAuth: () => ({ login: loginMock, logout: vi.fn(), token: null, role: null }),
}))

describe('LoginPage', () => {
  it('submits username and password', async () => {
    loginMock.mockResolvedValue(undefined)
    render(
      <MemoryRouter>
        <LoginPage />
      </MemoryRouter>,
    )

    await userEvent.type(screen.getByLabelText('login_username'), 'organizer1')
    await userEvent.type(screen.getByLabelText('login_password'), 'secret')
    await userEvent.click(screen.getByText('login_submit'))

    expect(loginMock).toHaveBeenCalledWith('organizer1', 'secret')
  })

  it('shows an error message on invalid credentials', async () => {
    loginMock.mockRejectedValue(new ApiError(401, 'invalid credentials'))
    render(
      <MemoryRouter>
        <LoginPage />
      </MemoryRouter>,
    )

    await userEvent.type(screen.getByLabelText('login_username'), 'organizer1')
    await userEvent.type(screen.getByLabelText('login_password'), 'wrong')
    await userEvent.click(screen.getByText('login_submit'))

    expect(await screen.findByRole('alert')).toHaveTextContent('login_invalid_credentials')
  })
})
```

- [ ] **Step 9: Run the tests to verify they pass**

Run (from `frontend/`): `npm test`
Expected: PASS

- [ ] **Step 10: Run the full frontend suite, the Go suite, and build**

Run (from `frontend/`): `npm test && npm run build`
Run (from the repo root): `go build ./... && go test ./... -count=1`
Expected: PASS.

- [ ] **Step 11: Commit**

```bash
git add frontend/src/auth frontend/src/pages/LoginPage.tsx frontend/src/pages/LoginPage.test.tsx internal/i18n/bundles
git commit -m "feat: add login screen, auth context, and protected routes"
```

---

## Task 7: App shell and router wiring

This task replaces Task 3's placeholder `App.tsx` with the real router,
provider stack, and nav shell. `/tournaments` renders an inline
placeholder here — Task 8 replaces it with the real `TournamentListPage`
by editing this task's route table, not by rebuilding it.

**Files:**
- Modify: `frontend/src/App.tsx` (full rewrite)
- Modify: `frontend/src/main.tsx` (wrap `App` in `Suspense` — needed now
  that a real, rendered component tree uses `useTranslation`, which
  suspends while `GET /api/i18n/{lang}` is in flight)
- Delete: `frontend/src/App.test.tsx` (Task 3's placeholder test
  asserted on text this rewrite removes; superseded by `AppShell.test.tsx`
  below and the Playwright test in Task 12)
- Create: `frontend/src/components/AppShell.tsx`
- Create: `frontend/src/components/AppShell.test.tsx`
- Modify: `internal/i18n/bundles/en.json`, `internal/i18n/bundles/de.json`

**Interfaces:**
- Consumes: `AuthProvider`, `ProtectedRoute`, `useAuth` (Task 6);
  `AVAILABLE_LANGUAGES`, `changeLanguage` (Task 5).
- Produces: `AppShell` (React component, wraps `<Outlet />`) — every
  later page task's routes nest inside it. The `QueryClient` instance
  created in `App.tsx` — Tasks 8-11 import `useQuery`/`useMutation` from
  `@tanstack/react-query` and rely on this single top-level
  `QueryClientProvider` already being in place, never creating their own.

- [ ] **Step 1: Add translation keys**

Modify `internal/i18n/bundles/en.json` — add:

```json
"nav_language": "Language",
"nav_logout": "Log out"
```

Modify `internal/i18n/bundles/de.json` — add:

```json
"nav_language": "Sprache",
"nav_logout": "Abmelden"
```

- [ ] **Step 2: Write the app shell**

Create `frontend/src/components/AppShell.tsx`:

```tsx
import { Link, Outlet } from 'react-router-dom'
import { useTranslation } from 'react-i18next'
import { useAuth } from '../auth/AuthContext'
import { AVAILABLE_LANGUAGES, changeLanguage } from '../i18n/i18n'

export function AppShell() {
  const { t, i18n } = useTranslation()
  const { role, logout } = useAuth()

  return (
    <div className="min-h-screen bg-gray-50">
      <nav className="flex items-center justify-between border-b bg-white px-6 py-3">
        <Link to="/tournaments" className="font-bold">
          TournamentStudio
        </Link>
        <div className="flex items-center gap-4 text-sm">
          <select
            value={i18n.language}
            onChange={(e) => changeLanguage(e.target.value)}
            aria-label={t('nav_language')}
          >
            {AVAILABLE_LANGUAGES.map((lang) => (
              <option key={lang} value={lang}>
                {lang.toUpperCase()}
              </option>
            ))}
          </select>
          {role && <span className="text-gray-500">{role}</span>}
          <button onClick={() => void logout()} className="text-blue-600">
            {t('nav_logout')}
          </button>
        </div>
      </nav>
      <main>
        <Outlet />
      </main>
    </div>
  )
}
```

- [ ] **Step 3: Write the app shell test**

Create `frontend/src/components/AppShell.test.tsx`:

```tsx
import { describe, expect, it, vi } from 'vitest'
import { render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { MemoryRouter, Route, Routes } from 'react-router-dom'
import { AppShell } from './AppShell'

const logoutMock = vi.fn()

vi.mock('react-i18next', () => ({
  useTranslation: () => ({ t: (key: string) => key, i18n: { language: 'en' } }),
}))
vi.mock('../auth/AuthContext', () => ({
  useAuth: () => ({ role: 'organizer', token: 'abc', logout: logoutMock }),
}))
vi.mock('../i18n/i18n', () => ({
  AVAILABLE_LANGUAGES: ['en', 'de'],
  changeLanguage: vi.fn(),
}))

function renderShell() {
  render(
    <MemoryRouter initialEntries={['/inner']}>
      <Routes>
        <Route element={<AppShell />}>
          <Route path="/inner" element={<div>inner content</div>} />
        </Route>
      </Routes>
    </MemoryRouter>,
  )
}

describe('AppShell', () => {
  it('renders the current role and the routed child content', () => {
    renderShell()
    expect(screen.getByText('organizer')).toBeInTheDocument()
    expect(screen.getByText('inner content')).toBeInTheDocument()
  })

  it('calls logout when the logout button is clicked', async () => {
    renderShell()
    await userEvent.click(screen.getByText('nav_logout'))
    expect(logoutMock).toHaveBeenCalled()
  })
})
```

- [ ] **Step 4: Run the test to verify it fails**

Run (from `frontend/`): `npm test`
Expected: FAIL (module `./AppShell` doesn't exist yet)

- [ ] **Step 5: Run the test to verify it passes**

(Step 2 already implements the module.)

Run (from `frontend/`): `npm test`
Expected: PASS

- [ ] **Step 6: Rewrite `App.tsx`**

Replace `frontend/src/App.tsx` in full:

```tsx
import { BrowserRouter, Navigate, Route, Routes } from 'react-router-dom'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { AuthProvider } from './auth/AuthContext'
import { ProtectedRoute } from './auth/ProtectedRoute'
import { AppShell } from './components/AppShell'
import { LoginPage } from './pages/LoginPage'

const queryClient = new QueryClient()

function TournamentsPlaceholder() {
  return <div className="p-8">Tournament list coming soon.</div>
}

function App() {
  return (
    <QueryClientProvider client={queryClient}>
      <AuthProvider>
        <BrowserRouter>
          <Routes>
            <Route path="/login" element={<LoginPage />} />
            <Route element={<ProtectedRoute />}>
              <Route element={<AppShell />}>
                <Route path="/" element={<Navigate to="/tournaments" replace />} />
                <Route path="/tournaments" element={<TournamentsPlaceholder />} />
              </Route>
            </Route>
          </Routes>
        </BrowserRouter>
      </AuthProvider>
    </QueryClientProvider>
  )
}

export default App
```

- [ ] **Step 7: Wrap the app in `Suspense`**

Replace `frontend/src/main.tsx` in full:

```tsx
import { StrictMode, Suspense } from 'react'
import { createRoot } from 'react-dom/client'
import './index.css'
import './i18n/i18n'
import App from './App.tsx'

createRoot(document.getElementById('root')!).render(
  <StrictMode>
    <Suspense fallback={null}>
      <App />
    </Suspense>
  </StrictMode>,
)
```

(`useTranslation` suspends its component while the i18next backend's
`GET /api/i18n/{lang}` request is in flight — this boundary is what
makes that safe anywhere in the tree, starting with `LoginPage`.)

- [ ] **Step 8: Delete the superseded placeholder test**

```bash
rm frontend/src/App.test.tsx
```

- [ ] **Step 9: Run the full frontend suite and build**

Run (from `frontend/`): `npm test && npm run build`
Expected: PASS, build succeeds.

- [ ] **Step 10: Run the Go suite (unaffected, but confirms nothing broke)**

Run (from the repo root): `go build ./... && go test ./... -count=1`
Expected: PASS.

- [ ] **Step 11: Commit**

```bash
git add frontend/src/App.tsx frontend/src/main.tsx frontend/src/components internal/i18n/bundles
git rm frontend/src/App.test.tsx
git commit -m "feat: add app shell, router, and provider wiring"
```

---

## Task 8: Tournament list and create form

**Files:**
- Create: `frontend/src/pages/TournamentListPage.tsx`
- Create: `frontend/src/pages/TournamentListPage.test.tsx`
- Create: `frontend/src/pages/TournamentCreatePage.tsx`
- Create: `frontend/src/pages/TournamentCreatePage.test.tsx`
- Modify: `frontend/src/App.tsx` (replace the `TournamentsPlaceholder`
  route with the real page, add `/tournaments/new`)
- Modify: `internal/i18n/bundles/en.json`, `internal/i18n/bundles/de.json`

**Interfaces:**
- Consumes: `api`, `Tournament`, `PluginsResponse` (Task 4);
  `useAuth` (Task 6); `AVAILABLE_LANGUAGES` (Task 5); the `QueryClient`
  from `App.tsx` (Task 7, unchanged).
- Produces: nothing a later task in this plan consumes directly —
  `/tournaments/:id`'s real page (Task 9) is reached by clicking a
  tournament in this task's list, not by importing anything from it.

- [ ] **Step 1: Add translation keys**

Modify `internal/i18n/bundles/en.json` — add:

```json
"tournament_list_title": "Tournaments",
"tournament_create_link": "Create Tournament",
"loading": "Loading…",
"tournament_create_title": "Create Tournament",
"tournament_name": "Name",
"tournament_language": "Language",
"tournament_sport": "Sport",
"tournament_select_sport": "Select a sport",
"tournament_type": "Tournament Type",
"tournament_select_type": "Select a tournament type",
"tournament_create_error": "Could not create the tournament. Please try again.",
"tournament_create_submit": "Create"
```

Modify `internal/i18n/bundles/de.json` — add:

```json
"tournament_list_title": "Turniere",
"tournament_create_link": "Turnier erstellen",
"loading": "Lädt…",
"tournament_create_title": "Turnier erstellen",
"tournament_name": "Name",
"tournament_language": "Sprache",
"tournament_sport": "Sportart",
"tournament_select_sport": "Sportart auswählen",
"tournament_type": "Turnierart",
"tournament_select_type": "Turnierart auswählen",
"tournament_create_error": "Das Turnier konnte nicht erstellt werden. Bitte versuchen Sie es erneut.",
"tournament_create_submit": "Erstellen"
```

- [ ] **Step 2: Write the tournament list page**

Create `frontend/src/pages/TournamentListPage.tsx`:

```tsx
import { Link } from 'react-router-dom'
import { useTranslation } from 'react-i18next'
import { useQuery } from '@tanstack/react-query'
import { useAuth } from '../auth/AuthContext'
import { api } from '../api/client'
import type { Tournament } from '../api/types'

export function TournamentListPage() {
  const { t } = useTranslation()
  const { role } = useAuth()
  const { data: tournaments, isLoading } = useQuery({
    queryKey: ['tournaments'],
    queryFn: () => api.get<Tournament[]>('/api/tournaments'),
  })

  return (
    <div className="mx-auto max-w-3xl p-8">
      <div className="mb-6 flex items-center justify-between">
        <h1 className="text-xl font-bold">{t('tournament_list_title')}</h1>
        {role === 'organizer' && (
          <Link to="/tournaments/new" className="rounded bg-blue-600 px-4 py-2 text-sm text-white">
            {t('tournament_create_link')}
          </Link>
        )}
      </div>
      {isLoading && <p>{t('loading')}</p>}
      <ul className="divide-y rounded border bg-white">
        {tournaments?.map((tournament) => (
          <li key={tournament.id}>
            <Link to={`/tournaments/${tournament.id}`} className="block px-4 py-3 hover:bg-gray-50">
              {tournament.name}
            </Link>
          </li>
        ))}
      </ul>
    </div>
  )
}
```

- [ ] **Step 3: Write the list page test**

Create `frontend/src/pages/TournamentListPage.test.tsx`:

```tsx
import { describe, expect, it, vi } from 'vitest'
import { render, screen } from '@testing-library/react'
import { MemoryRouter } from 'react-router-dom'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { TournamentListPage } from './TournamentListPage'
import * as client from '../api/client'

vi.mock('react-i18next', () => ({
  useTranslation: () => ({ t: (key: string) => key }),
}))
vi.mock('../auth/AuthContext', () => ({
  useAuth: () => ({ role: 'organizer' }),
}))
vi.mock('../api/client', async () => {
  const actual = await vi.importActual<typeof import('../api/client')>('../api/client')
  return { ...actual, api: { ...actual.api, get: vi.fn() } }
})

function renderPage() {
  const queryClient = new QueryClient()
  render(
    <QueryClientProvider client={queryClient}>
      <MemoryRouter>
        <TournamentListPage />
      </MemoryRouter>
    </QueryClientProvider>,
  )
}

describe('TournamentListPage', () => {
  it('renders tournaments from the API', async () => {
    vi.mocked(client.api.get).mockResolvedValue([
      {
        id: 1,
        name: 'Herbstregatta',
        sport_plugin_id: 'dragonboat',
        tournament_type_id: 'timed-heats-reseeding',
        language: 'de',
        status: 'draft',
      },
    ])
    renderPage()

    expect(await screen.findByText('Herbstregatta')).toBeInTheDocument()
  })

  it('shows a create-tournament link for organizers', async () => {
    vi.mocked(client.api.get).mockResolvedValue([])
    renderPage()

    expect(await screen.findByText('tournament_create_link')).toBeInTheDocument()
  })
})
```

- [ ] **Step 4: Run the test to verify it fails**

Run (from `frontend/`): `npm test`
Expected: FAIL (module `./TournamentListPage` doesn't exist yet)

- [ ] **Step 5: Run the test to verify it passes**

Run (from `frontend/`): `npm test`
Expected: PASS

- [ ] **Step 6: Write the create-tournament page**

Create `frontend/src/pages/TournamentCreatePage.tsx`:

```tsx
import { useState, type FormEvent } from 'react'
import { useNavigate } from 'react-router-dom'
import { useTranslation } from 'react-i18next'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { api } from '../api/client'
import type { PluginsResponse, Tournament } from '../api/types'
import { AVAILABLE_LANGUAGES } from '../i18n/i18n'

export function TournamentCreatePage() {
  const { t } = useTranslation()
  const navigate = useNavigate()
  const queryClient = useQueryClient()

  const { data: plugins } = useQuery({
    queryKey: ['plugins'],
    queryFn: () => api.get<PluginsResponse>('/api/plugins'),
  })

  const [name, setName] = useState('')
  const [language, setLanguage] = useState('en')
  const [sportId, setSportId] = useState('')
  const [typeId, setTypeId] = useState('')

  const selectedSport = plugins?.sports.find((s) => s.id === sportId)
  const compatibleTypes =
    plugins?.tournament_types.filter((tt) => selectedSport?.compatible_tournament_types.includes(tt.id)) ?? []

  const createMutation = useMutation({
    mutationFn: () =>
      api.post<Tournament>('/api/tournaments', {
        name,
        sport_plugin_id: sportId,
        tournament_type_id: typeId,
        language,
      }),
    onSuccess: (tournament) => {
      void queryClient.invalidateQueries({ queryKey: ['tournaments'] })
      navigate(`/tournaments/${tournament.id}`)
    },
  })

  function handleSubmit(e: FormEvent) {
    e.preventDefault()
    createMutation.mutate()
  }

  return (
    <div className="mx-auto max-w-lg p-8">
      <h1 className="mb-6 text-xl font-bold">{t('tournament_create_title')}</h1>
      <form onSubmit={handleSubmit} className="space-y-4">
        <div>
          <label htmlFor="name" className="block text-sm font-medium">
            {t('tournament_name')}
          </label>
          <input
            id="name"
            value={name}
            onChange={(e) => setName(e.target.value)}
            className="mt-1 w-full rounded border px-3 py-2"
            required
          />
        </div>
        <div>
          <label htmlFor="language" className="block text-sm font-medium">
            {t('tournament_language')}
          </label>
          <select
            id="language"
            value={language}
            onChange={(e) => setLanguage(e.target.value)}
            className="mt-1 w-full rounded border px-3 py-2"
          >
            {AVAILABLE_LANGUAGES.map((lang) => (
              <option key={lang} value={lang}>
                {lang.toUpperCase()}
              </option>
            ))}
          </select>
        </div>
        <div>
          <label htmlFor="sport" className="block text-sm font-medium">
            {t('tournament_sport')}
          </label>
          <select
            id="sport"
            value={sportId}
            onChange={(e) => {
              setSportId(e.target.value)
              setTypeId('')
            }}
            className="mt-1 w-full rounded border px-3 py-2"
            required
          >
            <option value="" disabled>
              {t('tournament_select_sport')}
            </option>
            {plugins?.sports.map((sport) => (
              <option key={sport.id} value={sport.id}>
                {sport.display_name}
              </option>
            ))}
          </select>
        </div>
        <div>
          <label htmlFor="type" className="block text-sm font-medium">
            {t('tournament_type')}
          </label>
          <select
            id="type"
            value={typeId}
            onChange={(e) => setTypeId(e.target.value)}
            className="mt-1 w-full rounded border px-3 py-2"
            required
            disabled={!sportId}
          >
            <option value="" disabled>
              {t('tournament_select_type')}
            </option>
            {compatibleTypes.map((tt) => (
              <option key={tt.id} value={tt.id}>
                {tt.id}
              </option>
            ))}
          </select>
        </div>
        {createMutation.isError && (
          <p role="alert" className="text-sm text-red-600">
            {t('tournament_create_error')}
          </p>
        )}
        <button
          type="submit"
          disabled={createMutation.isPending}
          className="w-full rounded bg-blue-600 px-4 py-2 text-white disabled:opacity-50"
        >
          {t('tournament_create_submit')}
        </button>
      </form>
    </div>
  )
}
```

- [ ] **Step 7: Write the create page test**

Create `frontend/src/pages/TournamentCreatePage.test.tsx`:

```tsx
import { describe, expect, it, vi } from 'vitest'
import { render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { MemoryRouter } from 'react-router-dom'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { TournamentCreatePage } from './TournamentCreatePage'
import * as client from '../api/client'

vi.mock('react-i18next', () => ({
  useTranslation: () => ({ t: (key: string) => key }),
}))

const navigateMock = vi.fn()
vi.mock('react-router-dom', async () => {
  const actual = await vi.importActual<typeof import('react-router-dom')>('react-router-dom')
  return { ...actual, useNavigate: () => navigateMock }
})

vi.mock('../api/client', async () => {
  const actual = await vi.importActual<typeof import('../api/client')>('../api/client')
  return { ...actual, api: { ...actual.api, get: vi.fn(), post: vi.fn() } }
})

function renderPage() {
  const queryClient = new QueryClient()
  render(
    <QueryClientProvider client={queryClient}>
      <MemoryRouter>
        <TournamentCreatePage />
      </MemoryRouter>
    </QueryClientProvider>,
  )
}

describe('TournamentCreatePage', () => {
  it('filters tournament types to the selected sport, and submits', async () => {
    vi.mocked(client.api.get).mockResolvedValue({
      sports: [
        {
          id: 'dragonboat',
          display_name: 'Dragonboat',
          compatible_tournament_types: ['timed-heats-reseeding'],
          roster_fields: [],
        },
      ],
      tournament_types: [
        { id: 'timed-heats-reseeding', compatible_sports: ['dragonboat'] },
        { id: 'knockout', compatible_sports: ['some-other-sport'] },
      ],
    })
    vi.mocked(client.api.post).mockResolvedValue({
      id: 42,
      name: 'Test',
      sport_plugin_id: 'dragonboat',
      tournament_type_id: 'timed-heats-reseeding',
      language: 'en',
      status: 'draft',
    })

    renderPage()

    await userEvent.type(await screen.findByLabelText('tournament_name'), 'Test')
    await userEvent.selectOptions(screen.getByLabelText('tournament_sport'), 'dragonboat')

    const typeSelect = screen.getByLabelText('tournament_type') as HTMLSelectElement
    const optionValues = Array.from(typeSelect.options).map((o) => o.value)
    expect(optionValues).toContain('timed-heats-reseeding')
    expect(optionValues).not.toContain('knockout')

    await userEvent.selectOptions(typeSelect, 'timed-heats-reseeding')
    await userEvent.click(screen.getByText('tournament_create_submit'))

    await waitFor(() => expect(navigateMock).toHaveBeenCalledWith('/tournaments/42'))
  })
})
```

- [ ] **Step 8: Run the test to verify it fails, then passes**

Run (from `frontend/`): `npm test`
Expected: FAIL first (module doesn't exist), then PASS once Step 6's
file is in place (it already is by this point — this step just
confirms).

- [ ] **Step 9: Wire the real routes into `App.tsx`**

Modify `frontend/src/App.tsx` — remove the `TournamentsPlaceholder`
function entirely, add the two new imports, and replace the
`/tournaments` route's element:

```tsx
import { TournamentListPage } from './pages/TournamentListPage'
import { TournamentCreatePage } from './pages/TournamentCreatePage'
```

```tsx
                <Route path="/tournaments" element={<TournamentListPage />} />
                <Route path="/tournaments/new" element={<TournamentCreatePage />} />
```

- [ ] **Step 10: Run the full frontend suite and build**

Run (from `frontend/`): `npm test && npm run build`
Expected: PASS, build succeeds.

- [ ] **Step 11: Run the Go suite**

Run (from the repo root): `go build ./... && go test ./... -count=1`
Expected: PASS.

- [ ] **Step 12: Commit**

```bash
git add frontend/src/pages/TournamentListPage.tsx frontend/src/pages/TournamentListPage.test.tsx frontend/src/pages/TournamentCreatePage.tsx frontend/src/pages/TournamentCreatePage.test.tsx frontend/src/App.tsx internal/i18n/bundles
git commit -m "feat: add tournament list and create-tournament screens"
```

---

## Task 9: Tournament detail page with tab strip

The **Teams** tab route renders an inline placeholder here — Task 10
replaces it with the real `TeamsTab`, the same swap pattern Task 8 used
for the tournament list.

**Files:**
- Create: `frontend/src/pages/TournamentDetailPage.tsx`
- Create: `frontend/src/pages/TournamentDetailPage.test.tsx`
- Modify: `frontend/src/App.tsx` (nested route: detail page wraps a
  `teams` child route)
- Modify: `internal/i18n/bundles/en.json`, `internal/i18n/bundles/de.json`

**Interfaces:**
- Consumes: `api`, `Tournament` (Task 4).
- Produces: the nested-route structure under `/tournaments/:id` — Task
  10 replaces this task's `TeamsTabPlaceholder` element with the real
  `TeamsTab`, at the same `teams` path, no route restructuring needed.

- [ ] **Step 1: Add translation keys**

Modify `internal/i18n/bundles/en.json` — add:

```json
"tab_teams": "Teams",
"tab_schedule": "Rounds & Schedule",
"tab_standings": "Live Standings",
"tab_coming_soon": "Coming soon"
```

Modify `internal/i18n/bundles/de.json` — add:

```json
"tab_teams": "Teams",
"tab_schedule": "Runden & Zeitplan",
"tab_standings": "Live-Ergebnisse",
"tab_coming_soon": "Demnächst verfügbar"
```

- [ ] **Step 2: Write the detail page**

Create `frontend/src/pages/TournamentDetailPage.tsx`:

```tsx
import { NavLink, Outlet, useParams } from 'react-router-dom'
import { useTranslation } from 'react-i18next'
import { useQuery } from '@tanstack/react-query'
import { api } from '../api/client'
import type { Tournament } from '../api/types'

export function TournamentDetailPage() {
  const { t } = useTranslation()
  const { id } = useParams<{ id: string }>()
  const { data: tournament, isLoading } = useQuery({
    queryKey: ['tournament', id],
    queryFn: () => api.get<Tournament>(`/api/tournaments/${id}`),
    enabled: !!id,
  })

  const tabLinkClass = ({ isActive }: { isActive: boolean }) =>
    `border-b-2 px-4 py-2 text-sm ${
      isActive ? 'border-blue-600 font-medium text-blue-600' : 'border-transparent text-gray-500'
    }`

  return (
    <div className="mx-auto max-w-3xl p-8">
      {isLoading && <p>{t('loading')}</p>}
      {tournament && (
        <>
          <h1 className="mb-1 text-xl font-bold">{tournament.name}</h1>
          <p className="mb-6 text-sm text-gray-500">{tournament.status}</p>
        </>
      )}
      <nav className="mb-6 flex border-b">
        <NavLink to="teams" className={tabLinkClass}>
          {t('tab_teams')}
        </NavLink>
        <span className="cursor-not-allowed px-4 py-2 text-sm text-gray-300" title={t('tab_coming_soon')}>
          {t('tab_schedule')}
        </span>
        <span className="cursor-not-allowed px-4 py-2 text-sm text-gray-300" title={t('tab_coming_soon')}>
          {t('tab_standings')}
        </span>
      </nav>
      <Outlet />
    </div>
  )
}
```

- [ ] **Step 3: Write the detail page test**

Create `frontend/src/pages/TournamentDetailPage.test.tsx`:

```tsx
import { describe, expect, it, vi } from 'vitest'
import { render, screen } from '@testing-library/react'
import { MemoryRouter, Route, Routes } from 'react-router-dom'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { TournamentDetailPage } from './TournamentDetailPage'
import * as client from '../api/client'

vi.mock('react-i18next', () => ({
  useTranslation: () => ({ t: (key: string) => key }),
}))
vi.mock('../api/client', async () => {
  const actual = await vi.importActual<typeof import('../api/client')>('../api/client')
  return { ...actual, api: { ...actual.api, get: vi.fn() } }
})

function renderPage() {
  const queryClient = new QueryClient()
  render(
    <QueryClientProvider client={queryClient}>
      <MemoryRouter initialEntries={['/tournaments/1/teams']}>
        <Routes>
          <Route path="/tournaments/:id" element={<TournamentDetailPage />}>
            <Route path="teams" element={<div>teams content</div>} />
          </Route>
        </Routes>
      </MemoryRouter>
    </QueryClientProvider>,
  )
}

describe('TournamentDetailPage', () => {
  it('renders the tournament name and the active teams tab content', async () => {
    vi.mocked(client.api.get).mockResolvedValue({
      id: 1,
      name: 'Herbstregatta',
      sport_plugin_id: 'dragonboat',
      tournament_type_id: 'timed-heats-reseeding',
      language: 'de',
      status: 'draft',
    })
    renderPage()

    expect(await screen.findByText('Herbstregatta')).toBeInTheDocument()
    expect(screen.getByText('teams content')).toBeInTheDocument()
    expect(screen.getByText('tab_schedule')).toBeInTheDocument()
  })
})
```

- [ ] **Step 4: Run the test to verify it fails, then passes**

Run (from `frontend/`): `npm test`
Expected: FAIL first (module doesn't exist), PASS once Step 2's file is
in place.

- [ ] **Step 5: Wire the nested route into `App.tsx`**

Modify `frontend/src/App.tsx` — add the import:

```tsx
import { TournamentDetailPage } from './pages/TournamentDetailPage'
```

Add this inline placeholder function near `App`'s other local
components (there are none left at this point — Task 8 removed
`TournamentsPlaceholder` — so this is the first one since):

```tsx
function TeamsTabPlaceholder() {
  return <div className="p-4 text-gray-500">Team management coming soon.</div>
}
```

Replace the `/tournaments` route block with a nested version:

```tsx
                <Route path="/tournaments" element={<TournamentListPage />} />
                <Route path="/tournaments/new" element={<TournamentCreatePage />} />
                <Route path="/tournaments/:id" element={<TournamentDetailPage />}>
                  <Route index element={<Navigate to="teams" replace />} />
                  <Route path="teams" element={<TeamsTabPlaceholder />} />
                </Route>
```

- [ ] **Step 6: Run the full frontend suite and build**

Run (from `frontend/`): `npm test && npm run build`
Expected: PASS, build succeeds.

- [ ] **Step 7: Run the Go suite**

Run (from the repo root): `go build ./... && go test ./... -count=1`
Expected: PASS.

- [ ] **Step 8: Commit**

```bash
git add frontend/src/pages/TournamentDetailPage.tsx frontend/src/pages/TournamentDetailPage.test.tsx frontend/src/App.tsx internal/i18n/bundles
git commit -m "feat: add tournament detail page with tab strip"
```

---

## Task 10: Teams tab — list and manual add form

The "Import from file" link this task adds points at a sibling `import`
route that doesn't exist until Task 11 — clicking it 404s until then.
That's expected: this task's job is the list and manual-add form, not
the import flow.

**Files:**
- Create: `frontend/src/pages/TeamsTab.tsx`
- Create: `frontend/src/pages/TeamsTab.test.tsx`
- Modify: `frontend/src/App.tsx` (replace `TeamsTabPlaceholder` with the
  real `TeamsTab`)
- Modify: `internal/i18n/bundles/en.json`, `internal/i18n/bundles/de.json`

**Interfaces:**
- Consumes: `api`, `Team`, `Tournament`, `PluginsResponse`, `RosterField`
  (Task 4).
- Produces: nothing a later task consumes directly — Task 11's import
  page is a sibling route, reached via this task's link, not via any
  shared component or hook.

- [ ] **Step 1: Add translation keys**

Modify `internal/i18n/bundles/en.json` — add:

```json
"teams_title": "Teams",
"teams_import_link": "Import from file",
"teams_add_title": "Add a team",
"teams_name": "Name",
"teams_club": "Club",
"teams_add_submit": "Add Team"
```

Modify `internal/i18n/bundles/de.json` — add:

```json
"teams_title": "Teams",
"teams_import_link": "Aus Datei importieren",
"teams_add_title": "Team hinzufügen",
"teams_name": "Name",
"teams_club": "Verein",
"teams_add_submit": "Team hinzufügen"
```

- [ ] **Step 2: Write the teams tab**

Create `frontend/src/pages/TeamsTab.tsx`:

```tsx
import { useState, type FormEvent } from 'react'
import { Link, useParams } from 'react-router-dom'
import { useTranslation } from 'react-i18next'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { api } from '../api/client'
import type { PluginsResponse, Team, Tournament } from '../api/types'

export function TeamsTab() {
  const { t } = useTranslation()
  const { id } = useParams<{ id: string }>()
  const queryClient = useQueryClient()

  const { data: tournament } = useQuery({
    queryKey: ['tournament', id],
    queryFn: () => api.get<Tournament>(`/api/tournaments/${id}`),
    enabled: !!id,
  })
  const { data: teams, isLoading } = useQuery({
    queryKey: ['teams', id],
    queryFn: () => api.get<Team[]>(`/api/tournaments/${id}/teams`),
    enabled: !!id,
  })
  const { data: plugins } = useQuery({
    queryKey: ['plugins'],
    queryFn: () => api.get<PluginsResponse>('/api/plugins'),
  })

  const rosterFields = plugins?.sports.find((s) => s.id === tournament?.sport_plugin_id)?.roster_fields ?? []

  const [name, setName] = useState('')
  const [club, setClub] = useState('')
  const [extraFields, setExtraFields] = useState<Record<string, string>>({})

  const createMutation = useMutation({
    mutationFn: () => api.post<Team>(`/api/tournaments/${id}/teams`, { name, club, extra_fields: extraFields }),
    onSuccess: () => {
      void queryClient.invalidateQueries({ queryKey: ['teams', id] })
      setName('')
      setClub('')
      setExtraFields({})
    },
  })

  function handleSubmit(e: FormEvent) {
    e.preventDefault()
    createMutation.mutate()
  }

  return (
    <div>
      <div className="mb-4 flex items-center justify-between">
        <h2 className="text-lg font-semibold">{t('teams_title')}</h2>
        <Link to="import" className="text-sm text-blue-600">
          {t('teams_import_link')}
        </Link>
      </div>

      {isLoading && <p>{t('loading')}</p>}
      <ul className="mb-6 divide-y rounded border bg-white">
        {teams?.map((team) => (
          <li key={team.id} className="px-4 py-3">
            <span className="font-medium">{team.name}</span>
            {team.club && <span className="ml-2 text-sm text-gray-500">{team.club}</span>}
          </li>
        ))}
      </ul>

      <form onSubmit={handleSubmit} className="max-w-md space-y-3 rounded border bg-white p-4">
        <h3 className="font-medium">{t('teams_add_title')}</h3>
        <div>
          <label htmlFor="team-name" className="block text-sm font-medium">
            {t('teams_name')}
          </label>
          <input
            id="team-name"
            value={name}
            onChange={(e) => setName(e.target.value)}
            className="mt-1 w-full rounded border px-3 py-2"
            required
          />
        </div>
        <div>
          <label htmlFor="team-club" className="block text-sm font-medium">
            {t('teams_club')}
          </label>
          <input
            id="team-club"
            value={club}
            onChange={(e) => setClub(e.target.value)}
            className="mt-1 w-full rounded border px-3 py-2"
          />
        </div>
        {rosterFields.map((field) => (
          <div key={field.key}>
            <label htmlFor={`field-${field.key}`} className="block text-sm font-medium">
              {field.label}
            </label>
            <input
              id={`field-${field.key}`}
              value={extraFields[field.key] ?? ''}
              onChange={(e) => setExtraFields((prev) => ({ ...prev, [field.key]: e.target.value }))}
              className="mt-1 w-full rounded border px-3 py-2"
              required={field.required}
            />
          </div>
        ))}
        <button
          type="submit"
          disabled={createMutation.isPending}
          className="rounded bg-blue-600 px-4 py-2 text-sm text-white disabled:opacity-50"
        >
          {t('teams_add_submit')}
        </button>
      </form>
    </div>
  )
}
```

- [ ] **Step 3: Write the teams tab test**

Create `frontend/src/pages/TeamsTab.test.tsx`:

```tsx
import { describe, expect, it, vi } from 'vitest'
import { render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { MemoryRouter, Route, Routes } from 'react-router-dom'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { TeamsTab } from './TeamsTab'
import * as client from '../api/client'

vi.mock('react-i18next', () => ({
  useTranslation: () => ({ t: (key: string) => key }),
}))
vi.mock('../api/client', async () => {
  const actual = await vi.importActual<typeof import('../api/client')>('../api/client')
  return { ...actual, api: { ...actual.api, get: vi.fn(), post: vi.fn() } }
})

function renderTab() {
  const queryClient = new QueryClient()
  render(
    <QueryClientProvider client={queryClient}>
      <MemoryRouter initialEntries={['/tournaments/1/teams']}>
        <Routes>
          <Route path="/tournaments/:id/teams" element={<TeamsTab />} />
        </Routes>
      </MemoryRouter>
    </QueryClientProvider>,
  )
}

describe('TeamsTab', () => {
  it('renders teams and roster-field inputs, then submits a new team', async () => {
    vi.mocked(client.api.get).mockImplementation((path: unknown) => {
      const p = path as string
      if (p === '/api/tournaments/1') {
        return Promise.resolve({
          id: 1,
          name: 'T',
          sport_plugin_id: 'dragonboat',
          tournament_type_id: 'timed-heats-reseeding',
          language: 'en',
          status: 'draft',
        })
      }
      if (p === '/api/tournaments/1/teams') {
        return Promise.resolve([{ id: 5, tournament_id: 1, name: 'Rhein Dragons', club: '', extra_fields: {} }])
      }
      if (p === '/api/plugins') {
        return Promise.resolve({
          sports: [
            {
              id: 'dragonboat',
              display_name: 'Dragonboat',
              compatible_tournament_types: [],
              roster_fields: [{ key: 'boat_class', label: 'Boat class', required: false }],
            },
          ],
          tournament_types: [],
        })
      }
      return Promise.reject(new Error(`unexpected path ${p}`))
    })
    vi.mocked(client.api.post).mockResolvedValue({
      id: 6,
      tournament_id: 1,
      name: 'New Team',
      club: '',
      extra_fields: { boat_class: 'K1' },
    })

    renderTab()

    expect(await screen.findByText('Rhein Dragons')).toBeInTheDocument()
    expect(await screen.findByLabelText('Boat class')).toBeInTheDocument()

    await userEvent.type(screen.getByLabelText('teams_name'), 'New Team')
    await userEvent.type(screen.getByLabelText('Boat class'), 'K1')
    await userEvent.click(screen.getByText('teams_add_submit'))

    await waitFor(() =>
      expect(client.api.post).toHaveBeenCalledWith('/api/tournaments/1/teams', {
        name: 'New Team',
        club: '',
        extra_fields: { boat_class: 'K1' },
      }),
    )
  })
})
```

- [ ] **Step 4: Run the test to verify it fails, then passes**

Run (from `frontend/`): `npm test`
Expected: FAIL first (module doesn't exist), PASS once Step 2's file is
in place.

- [ ] **Step 5: Wire the real tab into `App.tsx`**

Modify `frontend/src/App.tsx` — remove the `TeamsTabPlaceholder`
function, add the import:

```tsx
import { TeamsTab } from './pages/TeamsTab'
```

Replace the `teams` child route's element:

```tsx
                  <Route path="teams" element={<TeamsTab />} />
```

- [ ] **Step 6: Run the full frontend suite and build**

Run (from `frontend/`): `npm test && npm run build`
Expected: PASS, build succeeds.

- [ ] **Step 7: Run the Go suite**

Run (from the repo root): `go build ./... && go test ./... -count=1`
Expected: PASS.

- [ ] **Step 8: Commit**

```bash
git add frontend/src/pages/TeamsTab.tsx frontend/src/pages/TeamsTab.test.tsx frontend/src/App.tsx internal/i18n/bundles
git commit -m "feat: add teams list and manual add-team form"
```

---

## Task 11: Teams import flow

One-shot upload + results screen — no column mapping, no
preview-before-commit, matching the real `POST .../teams/import`
endpoint's actual behavior (§2 of the design spec).

**Files:**
- Create: `frontend/src/pages/TeamImportPage.tsx`
- Create: `frontend/src/pages/TeamImportPage.test.tsx`
- Modify: `frontend/src/App.tsx` (add the `teams/import` sibling route)
- Modify: `internal/i18n/bundles/en.json`, `internal/i18n/bundles/de.json`

**Interfaces:**
- Consumes: `api.postForm`, `ImportResult`, `ImportProblem` (Task 4);
  the `teams` React Query cache key (Task 10, invalidated on a
  successful import so the Teams tab reflects new teams on return).
- Produces: nothing later in this plan consumes — this is 4a's last
  screen.

- [ ] **Step 1: Add translation keys**

Modify `internal/i18n/bundles/en.json` — add (note the `{{count}}`,
`{{row}}`, `{{message}}` interpolation placeholders — i18next
substitutes these at render time):

```json
"import_title": "Import Teams",
"import_file_label": "Choose a CSV or XLSX file",
"import_submit": "Upload",
"import_error": "Could not import the file. Please try again.",
"import_result_summary": "{{count}} team(s) imported.",
"import_row_problem": "Row {{row}}: {{message}}",
"import_back_link": "Back to Teams"
```

Modify `internal/i18n/bundles/de.json` — add:

```json
"import_title": "Teams importieren",
"import_file_label": "CSV- oder XLSX-Datei auswählen",
"import_submit": "Hochladen",
"import_error": "Die Datei konnte nicht importiert werden. Bitte versuchen Sie es erneut.",
"import_result_summary": "{{count}} Team(s) importiert.",
"import_row_problem": "Zeile {{row}}: {{message}}",
"import_back_link": "Zurück zu Teams"
```

- [ ] **Step 2: Write the import page**

Create `frontend/src/pages/TeamImportPage.tsx`:

```tsx
import { useState, type ChangeEvent } from 'react'
import { Link, useParams } from 'react-router-dom'
import { useTranslation } from 'react-i18next'
import { useMutation, useQueryClient } from '@tanstack/react-query'
import { api } from '../api/client'
import type { ImportResult } from '../api/types'

export function TeamImportPage() {
  const { t } = useTranslation()
  const { id } = useParams<{ id: string }>()
  const queryClient = useQueryClient()
  const [file, setFile] = useState<File | null>(null)

  const importMutation = useMutation({
    mutationFn: () => {
      const form = new FormData()
      form.append('file', file as File)
      return api.postForm<ImportResult>(`/api/tournaments/${id}/teams/import`, form)
    },
    onSuccess: () => {
      void queryClient.invalidateQueries({ queryKey: ['teams', id] })
    },
  })

  function handleFileChange(e: ChangeEvent<HTMLInputElement>) {
    setFile(e.target.files?.[0] ?? null)
  }

  function handleSubmit() {
    if (file) {
      importMutation.mutate()
    }
  }

  return (
    <div className="mx-auto max-w-lg p-8">
      <h1 className="mb-6 text-xl font-bold">{t('import_title')}</h1>

      {!importMutation.data && (
        <div className="space-y-4">
          <input type="file" accept=".csv,.xlsx" aria-label={t('import_file_label')} onChange={handleFileChange} />
          <div>
            <button
              onClick={handleSubmit}
              disabled={!file || importMutation.isPending}
              className="rounded bg-blue-600 px-4 py-2 text-sm text-white disabled:opacity-50"
            >
              {t('import_submit')}
            </button>
          </div>
          {importMutation.isError && (
            <p role="alert" className="text-sm text-red-600">
              {t('import_error')}
            </p>
          )}
        </div>
      )}

      {importMutation.data && (
        <div>
          <p className="mb-4">{t('import_result_summary', { count: importMutation.data.imported })}</p>
          {importMutation.data.problems.length > 0 && (
            <ul className="mb-4 list-disc space-y-1 pl-5 text-sm text-red-600">
              {importMutation.data.problems.map((problem, i) => (
                <li key={i}>{t('import_row_problem', { row: problem.row_index, message: problem.message })}</li>
              ))}
            </ul>
          )}
          <Link to={`/tournaments/${id}/teams`} className="text-blue-600">
            {t('import_back_link')}
          </Link>
        </div>
      )}
    </div>
  )
}
```

- [ ] **Step 3: Write the import page test**

Create `frontend/src/pages/TeamImportPage.test.tsx`:

```tsx
import { describe, expect, it, vi } from 'vitest'
import { render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { MemoryRouter, Route, Routes } from 'react-router-dom'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { TeamImportPage } from './TeamImportPage'
import * as client from '../api/client'

vi.mock('react-i18next', () => ({
  useTranslation: () => ({
    t: (key: string, opts?: Record<string, unknown>) => (opts ? `${key}:${JSON.stringify(opts)}` : key),
  }),
}))
vi.mock('../api/client', async () => {
  const actual = await vi.importActual<typeof import('../api/client')>('../api/client')
  return { ...actual, api: { ...actual.api, postForm: vi.fn() } }
})

function renderPage() {
  const queryClient = new QueryClient()
  render(
    <QueryClientProvider client={queryClient}>
      <MemoryRouter initialEntries={['/tournaments/1/teams/import']}>
        <Routes>
          <Route path="/tournaments/:id/teams/import" element={<TeamImportPage />} />
        </Routes>
      </MemoryRouter>
    </QueryClientProvider>,
  )
}

describe('TeamImportPage', () => {
  it('uploads a file and shows the import results', async () => {
    vi.mocked(client.api.postForm).mockResolvedValue({
      imported: 2,
      problems: [{ row_index: 3, message: 'missing team name' }],
    })

    renderPage()

    const file = new File(['name,club\nA,B'], 'teams.csv', { type: 'text/csv' })
    const input = screen.getByLabelText('import_file_label')
    await userEvent.upload(input, file)
    await userEvent.click(screen.getByText('import_submit'))

    await waitFor(() => expect(client.api.postForm).toHaveBeenCalled())
    expect(await screen.findByText(/import_result_summary/)).toBeInTheDocument()
    expect(screen.getByText(/import_row_problem/)).toBeInTheDocument()

    const [path, form] = vi.mocked(client.api.postForm).mock.calls[0]
    expect(path).toBe('/api/tournaments/1/teams/import')
    expect(form.get('file')).toBeInstanceOf(File)
  })
})
```

- [ ] **Step 4: Run the test to verify it fails, then passes**

Run (from `frontend/`): `npm test`
Expected: FAIL first (module doesn't exist), PASS once Step 2's file is
in place.

- [ ] **Step 5: Wire the route into `App.tsx`**

Modify `frontend/src/App.tsx` — add the import:

```tsx
import { TeamImportPage } from './pages/TeamImportPage'
```

Add this sibling route inside the `/tournaments/:id` route block,
alongside the `teams` route:

```tsx
                  <Route path="teams/import" element={<TeamImportPage />} />
```

- [ ] **Step 6: Run the full frontend suite and build**

Run (from `frontend/`): `npm test && npm run build`
Expected: PASS, build succeeds.

- [ ] **Step 7: Run the Go suite**

Run (from the repo root): `go build ./... && go test ./... -count=1`
Expected: PASS.

- [ ] **Step 8: Commit**

```bash
git add frontend/src/pages/TeamImportPage.tsx frontend/src/pages/TeamImportPage.test.tsx frontend/src/App.tsx internal/i18n/bundles
git commit -m "feat: add team CSV/XLSX import flow"
```

---

## Task 12: Playwright end-to-end test

The first true end-to-end test in the whole project: a real browser
driving the real built frontend against a real running Go binary (real
SQLite, real migrations, no mocks anywhere). Everything before this
task tested the frontend and backend separately; this is the one place
that proves they actually work together.

**Files:**
- Create: `frontend/playwright.config.ts`
- Create: `frontend/e2e/run-server.sh`
- Create: `frontend/e2e/fixtures/teams.csv`
- Create: `frontend/e2e/setup-flow.spec.ts`
- Modify: `frontend/package.json` (add `test:e2e` script)
- Modify: `.gitignore` (ignore Playwright's report/output directories)

**Interfaces:**
- Consumes: every screen and every backend endpoint this plan built —
  this task wires nothing new for a later task to consume; it's the
  plan's final verification, not a building block.

- [ ] **Step 1: Install Playwright**

From `frontend/`:

```bash
npm install -D @playwright/test
npx playwright install chromium
```

- [ ] **Step 2: Write the test server launch script**

Create `frontend/e2e/run-server.sh`:

```bash
#!/bin/sh
set -e
cd "$(dirname "$0")/../.."
rm -f /tmp/tournamentstudio-e2e.db
export TOURNAMENTSTUDIO_DB=/tmp/tournamentstudio-e2e.db
export TOURNAMENTSTUDIO_ADMIN_USER=organizer1
export TOURNAMENTSTUDIO_ADMIN_PASSWORD=e2e-test-password
go run ./cmd/tournamentstudio
```

Make it executable:

```bash
chmod +x frontend/e2e/run-server.sh
```

(Removing the db file before every run means `bootstrapAdmin` always
sees an empty database and creates the known
`organizer1`/`e2e-test-password` account fresh — this test never
depends on state left over from a previous run.)

- [ ] **Step 3: Write the Playwright config**

Create `frontend/playwright.config.ts`:

```ts
import { defineConfig } from '@playwright/test'

export default defineConfig({
  testDir: './e2e',
  webServer: {
    command: 'sh e2e/run-server.sh',
    url: 'http://localhost:8080/healthz',
    reuseExistingServer: false,
    timeout: 30_000,
  },
  use: {
    baseURL: 'http://localhost:8080',
  },
})
```

- [ ] **Step 4: Write the CSV fixture**

Create `frontend/e2e/fixtures/teams.csv`:

```csv
name,club
Rhein Dragons,Köln Dragons
,Missing Name FC
```

(Row 0 is valid and gets imported. Row 1 has an empty `name` column —
`importer.Validate` rejects it with `"missing team name"` at
`row_index: 1`, 0-indexed among the *data* rows, not counting the
header — confirmed by reading `internal/importer/validate.go` before
writing this fixture, not assumed.)

- [ ] **Step 5: Write the end-to-end test**

Create `frontend/e2e/setup-flow.spec.ts`:

```ts
import { test, expect } from '@playwright/test'
import path from 'node:path'
import { fileURLToPath } from 'node:url'

const __dirname = path.dirname(fileURLToPath(import.meta.url))

test('login, create a tournament, add a team, and import a CSV with one bad row', async ({ page }) => {
  await page.goto('/login')

  await page.getByLabel('Username').fill('organizer1')
  await page.getByLabel('Password').fill('e2e-test-password')
  await page.getByRole('button', { name: 'Sign In' }).click()

  await expect(page).toHaveURL(/\/tournaments$/)

  await page.getByRole('link', { name: 'Create Tournament' }).click()
  await expect(page).toHaveURL(/\/tournaments\/new$/)

  await page.getByLabel('Name').fill('E2E Regatta')
  await page.getByLabel('Sport').selectOption({ label: 'Dragonboat' })
  await page.getByLabel('Tournament Type').selectOption({ index: 1 })
  await page.getByRole('button', { name: 'Create' }).click()

  await expect(page).toHaveURL(/\/tournaments\/\d+$/)
  await expect(page.getByRole('heading', { name: 'E2E Regatta' })).toBeVisible()

  await page.getByLabel('Name', { exact: true }).fill('Rhein Dragons')
  await page.getByRole('button', { name: 'Add Team' }).click()
  await expect(page.getByText('Rhein Dragons')).toBeVisible()

  await page.getByRole('link', { name: 'Import from file' }).click()
  await expect(page).toHaveURL(/\/teams\/import$/)

  const fixturePath = path.join(__dirname, 'fixtures', 'teams.csv')
  await page.getByLabel('Choose a CSV or XLSX file').setInputFiles(fixturePath)
  await page.getByRole('button', { name: 'Upload' }).click()

  await expect(page.getByText(/1 team\(s\) imported\./)).toBeVisible()
  await expect(page.getByText(/Row 1: missing team name/)).toBeVisible()
})
```

- [ ] **Step 6: Add the `test:e2e` script**

Modify `frontend/package.json`'s `scripts` block — add:

```json
"test:e2e": "playwright test"
```

- [ ] **Step 7: Build the frontend, then run the end-to-end test**

From `frontend/`:

```bash
npm run build
npx playwright test --reporter=line
```

Expected: PASS. Playwright starts `run-server.sh` (which builds nothing
itself — it runs the Go binary directly via `go run`, using whatever
`internal/webui/dist/` this step's `npm run build` just populated),
waits for `/healthz`, runs the test against a real browser, then tears
the server down.

If this fails, check first whether it's a real bug versus a text
mismatch — every string this test matches against (`Sign In`,
`Create Tournament`, `Dragonboat`, `Add Team`, `Import from file`,
`Upload`, the interpolated result/problem strings) must match the
**English** bundle exactly, since `en` is what an English-preferring
browser's `navigator.language` resolves to by default in Playwright's
headless Chromium.

- [ ] **Step 8: Ignore Playwright's output directories**

Modify `.gitignore` — add:

```
/frontend/test-results
/frontend/playwright-report
/frontend/playwright/.cache
```

- [ ] **Step 9: Run the full Go suite one last time**

Run (from the repo root): `go build ./... && go test ./... -count=1`
Expected: PASS.

- [ ] **Step 10: Commit**

```bash
git add frontend/playwright.config.ts frontend/e2e frontend/package.json .gitignore
git commit -m "test: add Playwright end-to-end test for the setup flow"
```

---

## Plan complete

All 12 tasks build, in order: an i18n endpoint and a `roster_fields`
fix on the backend, then the full frontend from an empty `frontend/`
directory through a real embedded, tested, end-to-end-verified setup
flow — login, tournament creation, and team management. Plans 4b (Run)
and 4c (Watch) each get their own brainstorm → spec → plan cycle when
their turn comes, building inside the `frontend/src/` structure and the
`AppShell`/tab-strip navigation this plan establishes.

