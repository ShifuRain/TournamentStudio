# TournamentStudio

TournamentStudio is a free/open-source, offline-first application for
running sports club tournaments. It ships as a single Go binary with an
embedded SQLite database and an embedded Lua plugin engine — no external
services, no internet connection required.

Sport-specific and tournament-format-specific logic (grouping, reseeding,
division cuts) is pluggable via sandboxed Lua scripts, not hardcoded.
TournamentStudio ships with a `dragonboat` sport plugin and a
`timed-heats-reseeding` tournament-type plugin (qualification heats with
cross-group reseeding, followed by multi-division finals).

## Status

Backend/API only at this stage — tournament and team management, the Lua
plugin engine, and the qualification-round/reseeding/division-cut
endpoints are implemented and tested. There is no bundled web UI yet.

## Requirements

- [Go](https://go.dev/) 1.25 or later (this repo pins `go = "latest"` via
  [mise](https://mise.jdx.dev/), but any Go 1.25+ toolchain works)
- No CGo toolchain needed — the SQLite driver
  ([`modernc.org/sqlite`](https://pkg.go.dev/modernc.org/sqlite)) and the
  Lua runtime ([`gopher-lua`](https://github.com/yuin/gopher-lua)) are
  both pure Go

## Installation

Clone the repository and build the binary. The Go binary embeds a built
copy of the web UI via `//go:embed`, so the frontend must be built once
*before* any `go build` — see
[Building from source](#building-from-source) for the full two-step
process:

```bash
git clone https://github.com/ShifuRain/TournamentStudio.git
cd TournamentStudio
cd frontend && npm install && npm run build
cd ..
go build -o tournamentstudio ./cmd/tournamentstudio
```

This produces a single self-contained `tournamentstudio` binary. Cross-
compile for another OS/architecture the normal Go way, e.g. for Windows:

```bash
GOOS=windows GOARCH=amd64 go build -o tournamentstudio.exe ./cmd/tournamentstudio
```

## Building from source

`internal/webui/dist/` is git-ignored and empty on a fresh clone — it is
populated by building the frontend, and `go build` (or `go test`) fails
with an unhelpful error until that's done, because
`internal/webui/webui.go` does a compile-time `//go:embed all:dist` of
that directory. Always build the frontend first:

```bash
cd frontend
npm install
npm run build
cd ..
```

`npm run build` writes the compiled static assets to
`internal/webui/dist/`. Once that directory is populated, any Go build
command works normally:

```bash
go build -o tournamentstudio ./cmd/tournamentstudio
# or, for development:
go build ./...
go test ./... -count=1
```

You only need to re-run the frontend build when frontend source changes
— it's not required again for pure Go changes, as long as `dist/`
already exists. See [`frontend/README.md`](frontend/README.md) for
frontend-specific dev/test commands.

## Starting offline

TournamentStudio needs nothing but the binary — it stores everything in
a single SQLite file next to it and never calls out to the network.

```bash
./tournamentstudio
```

On first run it creates `tournamentstudio.db` in the current directory,
applies its schema migrations, and — since the database has no users yet
— creates an initial Organizer account named `admin` with a random
password printed once to the log:

```
Created initial admin account 'admin' with password: 3f9a2c1e8b7d4f0a — save this, it will not be shown again
```

Save that password; it is not shown again. The server then listens on
`http://localhost:8080` (or any other machine's LAN IP on port 8080, so
teammates on the same WiFi — timekeepers, other organizers, a spectator
screen — can join by pointing a browser at `http://<your-ip>:8080`; this
is the same server process either way, there is no separate
single-device/multi-device mode).

To back up or move your data, just copy the `.db` file while the server
is stopped.

### Configuration

Everything is configured via environment variables — there is no config
file to edit:

| Variable | Default | Purpose |
|---|---|---|
| `TOURNAMENTSTUDIO_DB` | `tournamentstudio.db` | Path to the SQLite database file |
| `TOURNAMENTSTUDIO_PLUGINS` | `plugins` | Directory scanned for extra `.lua` plugin files, in addition to the bundled `dragonboat`/`timed-heats-reseeding` plugins |
| `TOURNAMENTSTUDIO_ADMIN_USER` / `TOURNAMENTSTUDIO_ADMIN_PASSWORD` | *(none)* | Set both to choose the initial admin account's credentials instead of the auto-generated ones. Only used the very first time the database is empty |

Example, running with an explicit admin account and a database file
elsewhere:

```bash
TOURNAMENTSTUDIO_DB=/var/lib/tournamentstudio/data.db \
TOURNAMENTSTUDIO_ADMIN_USER=organizer \
TOURNAMENTSTUDIO_ADMIN_PASSWORD='change-me' \
./tournamentstudio
```

### Installing a community plugin

Drop a `.lua` file into the directory named by `TOURNAMENTSTUDIO_PLUGINS`
(`plugins/` by default) and restart the server. A plugin that fails to
load (syntax error, missing required fields) is skipped with a logged
warning — it never prevents the rest of the server or other plugins from
starting. `GET /api/plugins` (any logged-in role) lists everything
currently loaded, bundled and external alike.

## Hosting on a server

Because the whole application is one binary plus one SQLite file, hosting
it for a club is just "run the binary somewhere reachable and keep it
running." A minimal `systemd` unit works well for a small club server:

```ini
# /etc/systemd/system/tournamentstudio.service
[Unit]
Description=TournamentStudio
After=network.target

[Service]
Type=simple
User=tournamentstudio
WorkingDirectory=/opt/tournamentstudio
Environment=TOURNAMENTSTUDIO_DB=/opt/tournamentstudio/data/tournamentstudio.db
ExecStart=/opt/tournamentstudio/tournamentstudio
Restart=on-failure

[Install]
WantedBy=multi-user.target
```

```bash
sudo systemctl daemon-reload
sudo systemctl enable --now tournamentstudio
```

The server listens on port `8080` on all interfaces. To expose it on the
club's local network as-is, that's it — anyone on the same WiFi can reach
`http://<server-ip>:8080`. To expose it further (a public URL, TLS, a
custom domain), put a reverse proxy in front of it rather than modifying
the binary — for example, Caddy:

```
tournament.yourclub.org {
    reverse_proxy localhost:8080
}
```

or nginx:

```nginx
server {
    listen 443 ssl;
    server_name tournament.yourclub.org;

    location / {
        proxy_pass http://127.0.0.1:8080;
        proxy_set_header Host $host;
        proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
    }
}
```

Since there's no cloud dependency, "hosting on a server" and "running
offline on a laptop at the venue" are the same binary in the same mode —
pick whichever fits the event; nothing about the deployment changes the
application's behavior.

## API overview

All endpoints are JSON over HTTP, under `/api`. Authenticate with
`POST /api/login` (`{"username", "password"}` → `{"token", "role"}`),
then send `Authorization: Bearer <token>` on subsequent requests.

Three roles gate every write endpoint server-side: **Organizer** (full
control), **Time Entry** (can submit race results only), **Spectator**
(read-only).

| Endpoint | Role | Purpose |
|---|---|---|
| `POST /api/login` | — | Authenticate |
| `GET /api/whoami` | any | Current session's user/role |
| `POST /api/logout` | any | Invalidate the session |
| `POST /api/tournaments` | Organizer | Create a tournament |
| `GET /api/tournaments` | any | List tournaments |
| `GET /api/tournaments/{id}` | any | Get one tournament |
| `POST /api/tournaments/{id}/teams` | Organizer | Add a team |
| `GET /api/tournaments/{id}/teams` | any | List teams |
| `POST /api/tournaments/{id}/teams/import` | Organizer | Bulk-import teams from CSV/XLSX |
| `POST /api/tournaments/{id}/rounds` | Organizer | Create a qualification round with explicit groups |
| `POST /api/tournaments/{id}/rounds/{round_id}/results` | Organizer, Time Entry | Submit a team's time/status; round auto-closes once every team has a result |
| `POST /api/tournaments/{id}/rounds/{round_id}/next` | Organizer | Compute the next round's reseeded groups |
| `POST /api/tournaments/{id}/rounds/{round_id}/divisions` | Organizer | Compute final divisions from a closed round's ranking |
| `GET /api/plugins` | any | List loaded sport and tournament-type plugins |

## Development

The frontend must be built at least once before these commands work —
see [Building from source](#building-from-source):

```bash
go build ./...
go test ./... -count=1
```

Design docs and implementation plans live under
`docs/superpowers/specs/` and `docs/superpowers/plans/`.

## License

FOSS — license file to be added.
