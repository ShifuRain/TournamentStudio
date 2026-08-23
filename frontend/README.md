# TournamentStudio web UI

This is the React 19 + Vite + TypeScript + Tailwind frontend for
[TournamentStudio](../README.md) — the setup flow (login, tournament
list/create/detail, teams list/add/import) and the i18n (en/de) layer
around it. The Go backend embeds this app's production build via
`//go:embed` and serves it for every non-`/api` route, so the compiled
Go binary is a single self-contained executable with no separate
frontend deployment step at runtime.

## Development

```bash
npm install
npm run dev
```

This starts the Vite dev server with hot module reloading. It expects
the Go backend to be running separately (see the root
[README](../README.md)) and proxies API calls to it — check
`vite.config.ts` for the current dev-server proxy target if you need to
point it at a non-default backend port.

## Building for the Go binary

```bash
npm run build
```

This runs `tsc -b && vite build` and writes the production build to
`../internal/webui/dist`, which `internal/webui/webui.go` embeds into
the Go binary at compile time via `//go:embed all:dist`. You must run
this at least once before `go build` will succeed on a fresh checkout —
see [Building from source](../README.md#building-from-source) in the
root README.

## Tests

Unit/component tests (Vitest + Testing Library):

```bash
npm test
```

End-to-end tests (Playwright, in `e2e/`):

```bash
npm run test:e2e
```

## Linting

```bash
npm run lint
```
