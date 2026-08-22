# Fantasy Football Draft App

A personal, local-first fantasy football draft-day dashboard. Milestone M2 adds a local JSON API for NFL teams, player summaries and details, and application settings on top of the persistent SQLite store.

## Prerequisites

The repository pins its tools in `mise.toml`:

- Go 1.26.6
- Node.js 24.19.0

With [mise](https://mise.jdx.dev/) installed, run:

```bash
mise install
```

Install frontend packages once:

```bash
cd frontend && npm ci
```

## Run locally

Start the backend:

```bash
cd backend && go run ./cmd/server
```

The API listens on `http://localhost:8080` by default. `BACKEND_PORT` can override the port when needed.

At startup, the backend opens `./data/draft.db` and automatically applies any pending embedded migrations. Existing data is preserved between runs, including when the M2 migration adds nullable player-profile fields. Set `DB_PATH` to use a different database file; its parent directory is created automatically.

In another terminal, start the frontend:

```bash
cd frontend && npm run dev
```

Open `http://localhost:5173`. The Vite development server proxies relative `/api` requests to `http://localhost:8080`; the Go server does not enable CORS.

## Sample data

Load the fictional sample dataset with:

```bash
cd backend && go run ./cmd/seed
```

The seed contains 60 clearly fictional players with synthetic profile fields, cross-provider IDs, stats, ADP, tiers, odds, and draft picks. It is idempotent and safe to run again: existing sample rows are updated rather than duplicated. Seeding is explicit and never runs automatically when the backend starts.

## Available routes

- `/overview`
- `/draft`
- `/admin`
- `GET /api/health`
- `GET /api/nfl-teams`
- `GET /api/players?position=&team=&available_only=`
- `GET /api/players/{id}`
- `GET /api/settings`
- `PUT /api/settings`

The frontend header displays the result of `/api/health`, providing a visible check that the two processes are communicating through the proxy. The player endpoints combine normalized player, historical stats, ADP, tier, odds, and active-draft pick data into explicit response objects; there are intentionally no raw table endpoints.

After seeding and starting the backend, inspect the API directly with:

```bash
curl http://localhost:8080/api/nfl-teams
curl 'http://localhost:8080/api/players?position=QB&available_only=true'
curl http://localhost:8080/api/players/1
curl http://localhost:8080/api/settings
```

`PUT /api/settings` accepts the complete editable settings object. Empty Sleeper IDs are valid during setup, and the polling interval must be between 500 and 60000 milliseconds. Sleeper's public API requires no credential, so there are deliberately no token, API-key, or password settings:

```bash
curl -X PUT http://localhost:8080/api/settings \
  -H 'Content-Type: application/json' \
  -d '{"sleeper_username":"","sleeper_league_id":"","sleeper_draft_id":"","polling_enabled":true,"polling_interval_ms":2000}'
```

## Checks

Run the backend checks:

```bash
cd backend && go test ./... && go vet ./...
```

Run the frontend checks:

```bash
cd frontend && npm run lint && npm run build
```
