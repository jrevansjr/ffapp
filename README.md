# Fantasy Football Draft App

A personal, local-first fantasy football draft-day dashboard. Milestone M5 adds an available-player worklist and player inspector for practicing draft decisions with local sample data.

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

At startup, the backend opens `./data/draft.db` and automatically applies any pending embedded migrations. Existing data is preserved between runs. Set `DB_PATH` to use a different database file; its parent directory is created automatically.

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

The seed contains 60 clearly fictional players with synthetic profile fields, cross-provider IDs, stats, ADP, tiers, odds, and draft picks. It is idempotent and safe to run again: existing sample rows are updated rather than duplicated. Seeding is explicit and never runs automatically when the backend starts. After updating to M5, rerun it once to populate the new synthetic QB passing-yard data added by migration `003`.

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

## Player Overview

Open `http://localhost:5173/overview` to view the persisted player pool. Overview loads all active players from the local Go API once, then applies position and NFL-team filters in the browser. Every column is sortable; FantasyPros Aggregate ADP is the initial sort, and missing values render as `—` after real values.

This page never calls Sleeper. Current sample players come from `go run ./cmd/seed`; the future real-player import will make one explicit Sleeper player-pool request and persist its results in SQLite. Live draft polling is a separate future flow that will retrieve picks for only the draft ID configured on Admin.

Taken players remain visible with muted, struck-through names. To exercise that state with the sample data, save `sample-draft-2026` as the Draft ID on Admin, then return to Overview.

## Draft Day

Open `http://localhost:5173/draft` to use the local draft workspace. The left pane contains only available players and supports name, position, NFL-team, and tier filters. Select a row to load that player's persisted detail into the inspector without navigating away from the page.

The inspector shows weekly half-PPR range statistics, ADP, tier, odds, and two trend charts. Every player has a fantasy-points chart. WR and TE players receive a targets chart, RB players receive a dual-axis rushing-attempts and targets chart, and QB players receive a passing-yards chart. These trends are context for comparing consistency and weekly opportunity; they are not app-generated recommendations.

To exercise taken-player removal, save `sample-draft-2026` as the Draft ID on Admin. The six persisted sample picks disappear from Draft Day but remain muted on Overview. Changing to an ID with no persisted picks starts with all players available without deleting the old sample draft.

M5 reads only the Go API and SQLite. It does not call Sleeper or poll for draft changes. Live synchronization and manual draft actions remain later milestones.

## Admin settings

Open `http://localhost:5173/admin` with both processes running to edit the Sleeper username, league ID, draft ID, polling toggle, and polling interval. `Save Settings` writes the complete form to SQLite; saved values survive browser refreshes and backend restarts. Empty Sleeper identifiers are valid while the app is being configured.

The polling interval must be a whole number from 500 through 60000 milliseconds. Sleeper's public API has no credential, so the Admin page deliberately contains no token, API-key, or password fields. Player-pool synchronization remains deferred to its later milestone.

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
