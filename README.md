# Fantasy Football Draft App

A personal, local-first fantasy football draft-day dashboard. M8.1 adds a committed 2026 sportsbook-consensus snapshot to the live draft, manual fallback, and real-data foundation.

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

The reference-data build contains:

- the reviewed list of 32 NFL teams;
- active QB/RB/WR/TE players from Sleeper's public `/players/nfl` endpoint;
- Sleeper profile fields and available cross-provider IDs;
- 2025 regular-season weekly stats from nflverse, joined exactly by GSIS ID;
- 2025 season totals derived from the imported weekly rows;
- 2026 FantasyPros Aggregate ADP;
- 2026 half-PPR Draft ECR, position rank, overall tier, and expert disagreement fields.
- 2026 FantasyPros passing, rushing, and receiving yards/touchdown projections.
- 2026 sportsbook-consensus passing, rushing, and receiving yard/TD lines plus all 32 NFL-team win totals.

The Sleeper player response is cached under `./data/import-cache/`. Re-running within 24 hours uses that cache instead of repeating Sleeper's large player request. The nflverse stats and DynastyProcess ID crosswalk are also cached there and reused indefinitely because they describe a completed historical season. Aggregate ADP, Draft ECR, and projections have independent response/metadata caches. These directories and SQLite files are gitignored.

Useful incremental commands:

```bash
go run ./cmd/data load teams
go run ./cmd/data load players
go run ./cmd/data load stats
go run ./cmd/data load stats --refresh
go run ./cmd/data load fantasypros
go run ./cmd/data load projections
go run ./cmd/data load odds
go run ./cmd/data build
```

All loaders are idempotent. `build` currently runs teams, players, stats, FantasyPros draft data, projections, then odds. `load stats` uses the validated local cache when present; add `--refresh` only when you deliberately want to download and validate both public provider files again. FantasyPros loads are cache-only, and odds loads use the embedded snapshot; `build` and `rebuild` therefore never call FantasyPros or a sportsbook. Player and stats downloads retain the cache behavior described above. Each loader validates and transactionally replaces its own rows.

`rebuild --confirm` validates the complete replacement before installing it: expected row coverage, all 18 historical weeks, season/weekly aggregate agreement, exact provider-ID uniqueness, source/season constraints, position-appropriate projections and odds, UTC RFC3339 timestamps, empty draft-history tables, foreign keys, and SQLite integrity. A failed build or audit leaves the current database and its backup intact.

The stats import uses public downloads and needs no API key:

- [nflverse weekly player stats](https://github.com/nflverse/nflverse-data/releases/tag/stats_player)
- [DynastyProcess player-ID crosswalk](https://github.com/dynastyprocess/data)

Sleeper-provided GSIS IDs are authoritative when present. The crosswalk fills only missing GSIS IDs by exact Sleeper ID; conflicts and unmatched players are printed rather than guessed by name. Weekly half-PPR points use `0.04 × passing yards + 4 × passing TDs - 2 × interceptions + 0.1 × rushing/receiving yards + 6 × rushing/receiving TDs + 0.5 × receptions - 2 × fumbles lost + 2 × two-point conversions`, with no bonuses.

Draft rankings come from two all-position, half-PPR requests to the official FantasyPros API. Aggregate ADP describes market draft position. Draft ECR is the overall Expert Consensus Ranking; position rank is stored separately, and the same ECR response supplies the FantasyPros overall tier, minimum/maximum expert ranks, and standard deviation. The importer enforces a minimum row count rather than caching incomplete rankings. The already-cached DynastyProcess crosswalk maps FantasyPros IDs to local players by exact Sleeper ID; no second identity download or name guessing is used. Conflicts, ambiguous IDs, and high-priority unmatched players are reported.

Projections come from one all-position FantasyPros preseason request. The API labels this response `STD` even when `HALF` is requested; the imported passing/rushing/receiving yards and touchdowns are volume values and do not depend on scoring format. They are joined only through the exact FantasyPros IDs already stored on players. Forecasts live in `player_projections`, separate from historical stats and sportsbook lines.

Odds come from the seven committed CSVs in `internal/odds/`, captured at `2026-08-31T01:23:28Z`. The app imports the supplied `Consensus_Line` and `Consensus_Win_Total` values; it does not scrape, call an API, or calculate consensus. Players match only by exact full name plus NFL-team abbreviation, and win totals match exact NFL-team names. Blank, unmatched, and position-invalid rows are reported rather than guessed.

`DB_PATH` changes the database location from the default `./data/draft.db`. The import cache and backups live beside the configured database.

## Run locally

Start the backend:

```bash
cd backend
go run ./cmd/server
```

The API listens on `http://localhost:8080` by default. `BACKEND_PORT` can override the port. The server opens the existing SQLite file and applies pending embedded migrations without importing reference data. If Admin has both a draft ID and polling enabled, the one backend poller immediately reads Sleeper's public picks endpoint and continues at the saved interval.

In another terminal, start the frontend:

```bash
cd frontend
npm run dev
```

Open `http://localhost:5173`. Vite proxies relative `/api` requests to `http://localhost:8080`; the Go server does not enable CORS. The header health indicator proves the frontend is communicating with the backend through that proxy.

### Use a second computer on the same network

The dashboard can be open in multiple browsers at once. Keep the backend command unchanged, but expose Vite on the laptop's local network:

```bash
cd frontend
npm run dev -- --host 0.0.0.0
```

On the second computer, open `http://<laptop-LAN-IP>:5173`. Both browsers read the same SQLite draft state through the same backend poller; filters and selected-player state remain local to each browser. Port 5173 must be allowed through the laptop firewall. This development server has no authentication, so expose it only on a trusted local network and do not port-forward it to the internet.

## Current M8.1 behavior

Overview and Draft Day show the real imported player pool. Overview season metrics and Draft Day player-inspector charts now use persisted 2025 weekly and season data. The player importer stores current Sleeper identity, team, experience, depth-chart, injury, and cross-provider metadata.

After the three explicit refreshes and a cache-only load/rebuild, Overview shows Aggregate ADP, ECR fields, and all six projection columns. Draft Day groups all 2025 performance values above the charts and preserves its position-aware 2026 FantasyPros projection card. A separate 2026 Sportsbook Consensus card shows position-relevant player lines and the player's NFL-team win total. Missing or unmatched values remain `—` rather than zero.

Live mode stores Sleeper picks per draft and derives taken/available state without modifying player rows. Draft Day refreshes the local draft snapshot once per second, so newly selected players leave the available table without a page reload; Overview uses that same snapshot for taken styling. Unknown Sleeper player IDs are retained with pick metadata and do not stop synchronization. Intentionally unsupported kicker and defense picks remain in draft history without producing unknown-player warnings. A failed Sleeper request preserves the last successful picks and marks the snapshot stale.

The backend starts exactly one poller only when a draft ID is configured and polling is enabled. It syncs immediately, then uses the Admin interval (default 2000 ms). Saving a different draft ID or interval cleanly replaces that loop; disabling polling stops it. Enabling the Admin toggle authorizes these repeated public pick requests until it is turned off. Browser refreshes only read SQLite through `GET /api/draft/state` and never trigger Sleeper calls.

With a Sleeper draft ID saved in Admin, selecting an available player exposes a `Mark Drafted` fallback. It immediately records that player against the configured draft and removes them from availability. Manual picks persist across reloads and appear in the status panel with individual Undo buttons; official Sleeper picks cannot be undone there. A later successful Sleeper sync replaces any overlapping manual state without creating duplicate availability. This works while polling is disabled or stale, but deliberately does not work without a configured draft ID.

FantasyPros projections and sportsbook consensus remain visibly separate; neither is presented as historical performance.

The prior clean rebuild was verified with 32 NFL teams, 3,045 active fantasy-position players, 6,033 weekly rows across all 18 weeks, 604 derived season rows, 314 Aggregate ADP rows, 750 ECR/tier rows, and 510 projection rows. M8.1 adds 296 validated consensus rows: 264 player markets and 32 team win totals. The CSVs contain 36 rows without consensus, five unmatched player-market rows, and one position mismatch; these remain absent. SQLite integrity and foreign-key checks pass.

Admin stores Sleeper username, league ID, draft ID, polling toggle, and polling interval in SQLite. Player-pool imports are terminal commands rather than Admin actions. Sleeper's public API requires no credential, so no token/API-key/password field exists.

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
- `GET /api/draft/state`
- `POST /api/draft/manual-picks`
- `DELETE /api/draft/manual-picks/{id}`

Inspect the local API directly:

```bash
curl http://localhost:8080/api/health
curl http://localhost:8080/api/nfl-teams
curl 'http://localhost:8080/api/players?position=QB'
curl http://localhost:8080/api/settings
curl http://localhost:8080/api/draft/state
```

Manual actions are normally easiest from Draft Day. For direct API inspection, use a real local player ID and an ID returned in the updated state's `picks` array:

```bash
curl -X POST -H 'Content-Type: application/json' \
  -d '{"player_id":42}' http://localhost:8080/api/draft/manual-picks

curl -X DELETE http://localhost:8080/api/draft/manual-picks/123
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

sqlite3 -header -box data/draft.db \
  "SELECT market, COUNT(*) AS rows, ROUND(MIN(line), 1) AS minimum, ROUND(MAX(line), 1) AS maximum, MAX(captured_at) AS captured_at FROM odds WHERE season = 2026 AND source = 'sportsbook_consensus' GROUP BY market ORDER BY market;"

sqlite3 -header -box data/draft.db \
  "SELECT d.sleeper_draft_id, dp.pick_number, dp.sleeper_player_id, dp.source, dp.player_first_name, dp.player_last_name FROM draft_picks dp JOIN drafts d ON d.id = dp.draft_id ORDER BY d.id, dp.pick_number;"
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
