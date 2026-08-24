# Fantasy Football Draft App

A personal, local-first fantasy football draft-day dashboard. M6.2 builds a persistent SQLite database from real NFL teams, active Sleeper QB/RB/WR/TE players, and 2025 nflverse weekly statistics.

## Prerequisites

The repository pins its tools in `mise.toml`:

- Go 1.26.6
- Node.js 24.19.0

With [mise](https://mise.jdx.dev/) installed:

```bash
mise install
cd frontend && npm ci
```

## Build the local database

Run data commands from `backend/`. They are explicit: starting the server or opening a page never contacts Sleeper.

For the first real-data database, or to replace the former sample database:

```bash
cd backend
go run ./cmd/data rebuild --confirm
```

Stop the backend before rebuilding. When `./data/draft.db` already exists, the command creates a consistent timestamped backup under `./data/backups/`, builds and validates a temporary database, and replaces the active database only after success.

M6.2's build contains:

- the reviewed list of 32 NFL teams;
- active QB/RB/WR/TE players from Sleeper's public `/players/nfl` endpoint;
- Sleeper profile fields and available cross-provider IDs;
- 2025 regular-season weekly stats from nflverse, joined exactly by GSIS ID;
- 2025 season totals derived from the imported weekly rows.

The Sleeper player response is cached under `./data/import-cache/`. Re-running within 24 hours uses that cache instead of repeating Sleeper's large player request. The nflverse stats and DynastyProcess ID crosswalk are also cached there and reused indefinitely because they describe a completed historical season. These directories and SQLite files are gitignored.

Useful incremental commands:

```bash
go run ./cmd/data load teams
go run ./cmd/data load players
go run ./cmd/data load stats
go run ./cmd/data load stats --refresh
go run ./cmd/data build
```

All loaders are idempotent. `build` currently runs teams, players, then stats. `load stats` uses the validated local cache when present; add `--refresh` only when you deliberately want to download and validate both provider files again. A stats refresh replaces only 2025 rows, in one transaction. Later M6.x milestones will add ADP, tiers, and odds to the same entry point.

The stats import uses public downloads and needs no API key:

- [nflverse weekly player stats](https://github.com/nflverse/nflverse-data/releases/tag/stats_player)
- [DynastyProcess player-ID crosswalk](https://github.com/dynastyprocess/data)

Sleeper-provided GSIS IDs are authoritative when present. The crosswalk fills only missing GSIS IDs by exact Sleeper ID; conflicts and unmatched players are printed rather than guessed by name. Weekly half-PPR points use `0.04 × passing yards + 4 × passing TDs - 2 × interceptions + 0.1 × rushing/receiving yards + 6 × rushing/receiving TDs + 0.5 × receptions - 2 × fumbles lost + 2 × two-point conversions`, with no bonuses.

`DB_PATH` changes the database location from the default `./data/draft.db`. The import cache and backups live beside the configured database.

## Run locally

Start the backend:

```bash
cd backend
go run ./cmd/server
```

The API listens on `http://localhost:8080` by default. `BACKEND_PORT` can override the port. The server opens the existing SQLite file and applies pending embedded migrations without fetching provider data.

In another terminal, start the frontend:

```bash
cd frontend
npm run dev
```

Open `http://localhost:5173`. Vite proxies relative `/api` requests to `http://localhost:8080`; the Go server does not enable CORS. The header health indicator proves the frontend is communicating with the backend through that proxy.

## Current M6.2 behavior

Overview and Draft Day show the real imported player pool. Overview season metrics and Draft Day player-inspector charts now use persisted 2025 weekly and season data. The player importer stores current Sleeper identity, team, experience, depth-chart, injury, and cross-provider metadata.

ADP, tiers, odds, and live picks are deliberately not populated yet. Those UI fields render `—`, and all players remain available. Those datasets will be implemented and approved one milestone at a time in M6.3–M6.6. Live Sleeper draft synchronization begins in M7.

Admin continues to store Sleeper username, league ID, draft ID, polling toggle, and polling interval in SQLite. Player-pool imports are terminal commands rather than Admin actions. Sleeper's public API requires no credential, so no token/API-key/password field exists.

Available routes:

- `/overview`
- `/draft`
- `/admin`
- `GET /api/health`
- `GET /api/nfl-teams`
- `GET /api/players?position=&team=&available_only=`
- `GET /api/players/{id}`
- `GET /api/settings`
- `PUT /api/settings`

Inspect the local API directly:

```bash
curl http://localhost:8080/api/health
curl http://localhost:8080/api/nfl-teams
curl 'http://localhost:8080/api/players?position=QB'
curl http://localhost:8080/api/settings
```

Inspect the database with the SQLite CLI:

```bash
sqlite3 -header -box data/draft.db \
  "SELECT position, COUNT(*) AS players FROM players WHERE active = 1 GROUP BY position ORDER BY position;"

sqlite3 -header -box data/draft.db \
  "SELECT season, COUNT(*) AS weekly_rows, COUNT(DISTINCT player_id) AS players, MIN(week) AS first_week, MAX(week) AS last_week FROM player_week_stats GROUP BY season;"

sqlite3 -header -box data/draft.db \
  "SELECT p.first_name || ' ' || p.last_name AS player, s.games_played, ROUND(s.fantasy_points_half_ppr, 1) AS half_ppr FROM player_season_stats s JOIN players p ON p.id = s.player_id WHERE s.season = 2025 ORDER BY s.fantasy_points_half_ppr DESC LIMIT 10;"
```

## Checks

```bash
cd backend
go test ./...
go vet ./...

cd ../frontend
npm run lint
npm run build
```
