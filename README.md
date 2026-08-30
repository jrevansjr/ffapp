# Fantasy Football Draft App

A personal, local-first fantasy football draft-day dashboard. M6.4 builds a persistent SQLite database from real NFL teams, active Sleeper QB/RB/WR/TE players, 2025 nflverse weekly statistics, and 2026 FantasyPros draft rankings plus volume projections.

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

Run data commands from `backend/`. They are explicit: starting the server or opening a page never contacts a provider.

Full FantasyPros requests require an API key with active paid HOF access; free prototype keys may return sample data below the importer's completeness threshold. Create the ignored local configuration file once, then paste the key after `FANTASYPROS_API_KEY=`:

```bash
cd backend
cp .env.example .env
```

Never commit `.env` or paste the key into application settings. An exported shell variable takes precedence over `.env` if you prefer `export FANTASYPROS_API_KEY=...`.

Each command below makes exactly one authenticated FantasyPros API request and updates only that dataset's ignored cache. Obtain fresh approval before running each command so provider usage remains visible:

```bash
go run ./cmd/data refresh fantasypros adp
go run ./cmd/data refresh fantasypros ecr
go run ./cmd/data refresh fantasypros projections
```

Once all three caches exist, database builds and loads never contact FantasyPros.

For the first real-data database, or to replace the former sample database:

```bash
cd backend
go run ./cmd/data rebuild --confirm
```

Stop the backend before rebuilding. When `./data/draft.db` already exists, the command creates a consistent timestamped backup under `./data/backups/`, builds and validates a temporary database, and replaces the active database only after success.

M6.4's build contains:

- the reviewed list of 32 NFL teams;
- active QB/RB/WR/TE players from Sleeper's public `/players/nfl` endpoint;
- Sleeper profile fields and available cross-provider IDs;
- 2025 regular-season weekly stats from nflverse, joined exactly by GSIS ID;
- 2025 season totals derived from the imported weekly rows;
- 2026 FantasyPros Aggregate ADP;
- 2026 half-PPR Draft ECR, position rank, overall tier, and expert disagreement fields.
- 2026 FantasyPros passing, rushing, and receiving yards/touchdown projections.

The Sleeper player response is cached under `./data/import-cache/`. Re-running within 24 hours uses that cache instead of repeating Sleeper's large player request. The nflverse stats and DynastyProcess ID crosswalk are also cached there and reused indefinitely because they describe a completed historical season. Aggregate ADP, Draft ECR, and projections have independent response/metadata caches. These directories and SQLite files are gitignored.

Useful incremental commands:

```bash
go run ./cmd/data load teams
go run ./cmd/data load players
go run ./cmd/data load stats
go run ./cmd/data load stats --refresh
go run ./cmd/data load fantasypros
go run ./cmd/data load projections
go run ./cmd/data build
```

All loaders are idempotent. `build` currently runs teams, players, stats, FantasyPros draft data, then projections. `load stats` uses the validated local cache when present; add `--refresh` only when you deliberately want to download and validate both public provider files again. `load fantasypros`, `load projections`, `build`, and `rebuild` are cache-only and cannot make a FantasyPros request. Each refresh caches one response; the loaders validate and transactionally replace their own 2026 rows.

The stats import uses public downloads and needs no API key:

- [nflverse weekly player stats](https://github.com/nflverse/nflverse-data/releases/tag/stats_player)
- [DynastyProcess player-ID crosswalk](https://github.com/dynastyprocess/data)

Sleeper-provided GSIS IDs are authoritative when present. The crosswalk fills only missing GSIS IDs by exact Sleeper ID; conflicts and unmatched players are printed rather than guessed by name. Weekly half-PPR points use `0.04 × passing yards + 4 × passing TDs - 2 × interceptions + 0.1 × rushing/receiving yards + 6 × rushing/receiving TDs + 0.5 × receptions - 2 × fumbles lost + 2 × two-point conversions`, with no bonuses.

Draft rankings come from two all-position, half-PPR requests to the official FantasyPros API. Aggregate ADP describes market draft position. Draft ECR is the overall Expert Consensus Ranking; position rank is stored separately, and the same ECR response supplies the FantasyPros overall tier, minimum/maximum expert ranks, and standard deviation. The importer enforces a minimum row count rather than caching incomplete rankings. The already-cached DynastyProcess crosswalk maps FantasyPros IDs to local players by exact Sleeper ID; no second identity download or name guessing is used. Conflicts, ambiguous IDs, and high-priority unmatched players are reported.

Projections come from one all-position FantasyPros preseason request. The API labels this response `STD` even when `HALF` is requested; the imported passing/rushing/receiving yards and touchdowns are volume values and do not depend on scoring format. They are joined only through the exact FantasyPros IDs already stored on players. Forecasts live in `player_projections`, separate from both historical stats and the still-empty sportsbook `odds` table.

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

## Current M6.4 behavior

Overview and Draft Day show the real imported player pool. Overview season metrics and Draft Day player-inspector charts now use persisted 2025 weekly and season data. The player importer stores current Sleeper identity, team, experience, depth-chart, injury, and cross-provider metadata.

After the three explicit refreshes and a cache-only load/rebuild, Overview shows Aggregate ADP, ECR fields, and all six projection columns. Draft Day groups all 2025 performance values above the charts and shows a separate position-aware 2026 FantasyPros projection card below them. Missing or unmatched values remain `—` rather than zero.

Odds and live picks are deliberately not populated yet. Projections are not presented as odds, and team-win totals are hidden. The final real-data audit is M6.5, live Sleeper synchronization begins in M7, manual fallback is M8, and real season odds are deferred until afterward.

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

sqlite3 -header -box data/draft.db \
  "SELECT COUNT(*) AS players, ROUND(MIN(adp), 1) AS first_adp, MAX(updated_at) AS updated_at FROM player_adp WHERE season = 2026 AND source = 'fantasypros';"

sqlite3 -header -box data/draft.db \
  "SELECT p.first_name || ' ' || p.last_name AS player, r.overall_rank AS ecr, p.position || r.position_rank AS pos_rank, t.tier, r.rank_min, r.rank_max, ROUND(r.rank_std_dev, 1) AS rank_std_dev FROM player_rankings r JOIN players p ON p.id = r.player_id JOIN player_tiers t ON t.player_id = r.player_id AND t.season = r.season AND t.source = r.source WHERE r.season = 2026 AND r.source = 'fantasypros' ORDER BY r.overall_rank LIMIT 20;"

sqlite3 -header -box data/draft.db \
  "SELECT p.first_name || ' ' || p.last_name AS player, p.position, ROUND(x.passing_yards, 1) AS pass_yds, ROUND(x.rushing_yards, 1) AS rush_yds, ROUND(x.receiving_yards, 1) AS rec_yds FROM player_projections x JOIN players p ON p.id = x.player_id WHERE x.season = 2026 AND x.source = 'fantasypros' ORDER BY p.position, p.last_name LIMIT 20;"
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
