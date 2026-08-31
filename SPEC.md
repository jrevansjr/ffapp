# SPEC.md — Product Requirements and Build Plan

Companion to `AGENTS.md` (which holds the durable rules). This file describes *what* to build and in what order. Once a milestone is implemented, the code and migrations become the source of truth; treat the details here as the original intent, not a document to reconcile against forever.

Target league: **12 teams, 1 QB, 0.5 PPR, Sleeper.** Keep the data model reasonably league-agnostic where it's easy, but do not build a league-rules or scoring engine, and do not hardcode recommendation logic on these assumptions.

Three pages: **Overview**, **Draft Day**, **Admin**. Simple top nav: `Overview | Draft Day | Admin`. Routes via React Router: `/overview`, `/draft`, `/admin`; `/` redirects to `/overview`.

---

## Visual style

Compact data tool, not a marketing site. Neutral/light theme, clear typography, restrained borders, compact spacing, consistent numeric alignment, shadcn/ui where useful, simple cards only where they aid grouping. No gradients, glassmorphism, decorative animation, hero sections, oversized cards, bright FF-themed palettes, or icon overload. Design for laptop/desktop; don't break badly at smaller widths, but don't optimize for phones.

Accessibility basics: semantic controls, real `<button>` elements, labeled inputs, a clear selected state on selectable rows, no hover-only access to important information. Keyboard row-selection is nice-to-have, never blocking.

Loading: small skeletons / muted text, no full-screen spinners. Empty states: plain sentences ("No players match these filters.", "No weekly data available.", "No active Sleeper draft configured."). API errors: concise user-facing message.

---

## Page 1 — Overview (`/overview`)

Dense, sortable, filterable TanStack Table of the player pool. Filters: **position** and **NFL team** as simple dropdowns/compact controls — no query-builder UI.

Columns, in this order unless a small UX adjustment is clearly better:

1. Name
2. Position
3. Team
4. Years Experience (stored from Sleeper's `years_exp`)
5. FantasyPros Aggregate ADP
6. FantasyPros overall Expert Consensus Ranking (ECR)
7. FantasyPros position rank
8. FantasyPros overall tier
9. ECR minimum expert rank
10. ECR maximum expert rank
11. ECR standard deviation
12. 2025 Total Fantasy Points (0.5 PPR)
13. 2025 Games Played
14. 2025 Average Fantasy Points
15. 2025 Targets Per Game
16. 2025 Rushing Attempts Per Game (derived from season totals and games played)
17. 2026 FantasyPros Projected Passing Yards
18. 2026 FantasyPros Projected Passing TDs
19. 2026 FantasyPros Projected Rushing Yards
20. 2026 FantasyPros Projected Rushing TDs
21. 2026 FantasyPros Projected Receiving Yards
22. 2026 FantasyPros Projected Receiving TDs

Numeric columns sort numerically; missing values render `—` (never `0`) and sort last. No server-side pagination or virtualization — the pool is small.

**Drafted players stay in the table**: gray the name, optionally mute the row, never hide it.

---

## Page 2 — Draft Day (`/draft`)

The primary live-draft page.

```text
+---------------------------------------------------------------+
| Draft status / pick information                               |
+--------------------------------------+------------------------+
| Available Players                    | Player Inspector       |
| sortable/filterable table            | summary                |
|                                      | charts                 |
|                                      | projections / ADPs     |
|                                      | manual action          |
+--------------------------------------+------------------------+
```

**Status bar:** current mode (live / manual / not configured), last-synced time, and a stale indicator + message when Sleeper is unavailable. *Optional, only if it stays trivial:* current pick number and "picks until your turn" derived from `draft_order` + snake order in `GET /draft/{id}` — genuinely useful, but never at the cost of core functionality.

**Available-player table.** Columns: Name, Position, NFL team, FantasyPros overall tier, overall ECR, position rank, and Aggregate ADP. Default sort: Aggregate ADP ascending, missing ADP last. Filters: position, NFL team, tier; a search box only if simple. Taken players are **removed** from this table, updating as the draft-state query receives new picks — no full page refresh, ever.

**Selection.** Clicking a row selects the player into a persistent right-side inspector (no navigation to a player page). Selecting another row replaces the inspector immediately. Selected player ID is local frontend state.

### Player Inspector

Prioritize quick draft decisions.

- **Header:** name, position, NFL team, FantasyPros overall tier, Aggregate ADP, overall ECR, and position rank.
- **Expert range:** minimum rank, maximum rank, and standard deviation from the experts included in the ECR response. These describe disagreement; they are not an app-generated recommendation.
- **2025 performance:** weekly 0.5-PPR average, high, median, and low grouped with games played, receptions, targets/game, and rushing attempts/game above the charts. Median computed correctly. No weekly data → empty state, not fake zeros.
- **Two compact Recharts line charts:** weekly fantasy points for every player, plus a position-specific volume chart. WR/TE show weekly targets; RB shows rushing attempts and targets together with separate axes; QB shows weekly passing yards. X = week, Y = value, tooltips required, optional average reference line if visually clean, no elaborate interactions.
- **2026 FantasyPros projections:** a separate, position-aware card below the charts. QB shows passing and rushing yards/TDs; RB shows rushing and receiving yards/TDs; WR/TE show receiving yards/TDs. These forecasts are not betting odds.
- **Manual action:** `Mark Drafted` button (see fallback below).
- No recommendation scores or decision logic of any kind.

**Chart component:** one reusable weekly trend chart taking a title, weekly data, one or two series, and an optional average reference. Summary stats (avg/high/median/low) are calculated in **one** place — prefer the Go backend in the detail DTO for consistency and testability.

### Manual fallback

`Mark Drafted` requires a configured Sleeper draft ID and creates a `source='manual'` pick on that active local draft. The backend returns the updated draft state, so the player immediately leaves the available list and grays out on Overview without waiting for the next poll. Draft Day lists each manual pick with its own Undo action. Manual picks survive refresh because they are DB rows; official Sleeper picks are never removable through this fallback. When official synchronization catches up, its pick number/player identity replaces any conflicting manual row per AGENTS.md §7.

---

## Page 3 — Admin (`/admin`)

Bare settings page. Plain controlled inputs, no form libraries.

Fields: Sleeper username, league ID, draft ID, polling enabled, polling interval (ms). Button: `Save Settings`, plus optionally `Refresh Draft State` once live synchronization exists. The last player-import time may be shown read-only. Player-pool imports are deliberate CLI operations, not browser actions. Small save-success / validation messages. Validate: interval is a positive reasonable number; IDs are strings; empty values allowed during setup. **No Sleeper token / API key / password / credential fields exist.**

---

## Backend API

Small REST surface. Names may evolve slightly; keep the shape close to this. Explicit DTOs, not raw rows.

```text
GET  /api/health                       → { "status": "ok" }
GET  /api/settings                     → settings object
PUT  /api/settings                     → accepts full settings object
GET  /api/nfl-teams                    → for filters/display
GET  /api/players?position=&team=&available_only=
GET  /api/players/{id}                 → { player, season, draft, projections, odds, weekly: [] }
GET  /api/draft/state                  → see below
POST /api/draft/manual-picks           → { "player_id": 42 }
DELETE /api/draft/manual-picks/{id}
```

`/api/players` returns the fields Overview and Draft Day need (name, position, team, years experience, Aggregate ADP, ECR/position rank/expert range, tier, 2025 season summary fields, 2026 FantasyPros projections, and `is_taken`). These are presentation fields — they do not imply denormalized columns. It reads persisted SQLite data only and never calls a provider; imports and draft-pick polling are separate operations. The dormant odds DTO remains separate for later sportsbook data.

`/api/draft/state` is a **read** endpoint: reads settings; returns a clean "not configured" state when no draft ID exists; reads the latest synchronized state from SQLite; resolves local player IDs; combines official + manual picks without duplicates; falls back to persisted picks when Sleeper is down. Kicker and defense picks remain in `picks` but are excluded from unknown-player warnings because those positions are intentionally outside the player pool. Shape (may evolve; preserve the concepts):

```json
{
  "draft_id": "1234567890",
  "mode": "live",
  "status": "stale",
  "polling_enabled": true,
  "stale": true,
  "last_synced_at": "2026-08-15T18:30:00Z",
  "message": "Sleeper is temporarily unavailable; showing last known draft state.",
  "picks": [],
  "taken_player_ids": [],
  "unknown_sleeper_player_ids": []
}
```

Frontend: one TanStack Query per concern — `["settings"]`, `["players"]`, `["player", id]`, `["draft-state"]`, `["nfl-teams"]`. Overview fetches the small persisted player pool once and filters it locally. One draft-state query feeds the whole Draft Day page (no redundant nested-component refetches). Mutations for settings and manual picks invalidate only affected queries. All requests use relative `/api/...` paths through the Vite proxy.

---

## Database schema direction

Normalized; season/source-varying data in its own tables. SQLite types and conventions per AGENTS.md §4 (`INTEGER PRIMARY KEY` ids, `TEXT` Sleeper IDs and ISO-8601 UTC timestamps, `REAL` for points/ADP/lines, `INTEGER` 0/1 booleans). Migrations are the source of truth once written — don't add columns with no current or near-term use.

- **`nfl_teams`** — `id, abbreviation (unique), name`. No win totals here (season/source-varying → `odds`).
- **`players`** — core identity (`id, sleeper_player_id, first_name, last_name, position, nfl_team_id, birth_date, active`), Sleeper profile/status (`status, number, college, height, weight, birth_country, years_exp, depth_chart_position, depth_chart_order, injury_status, injury_start_date, practice_participation`), and nullable cross-provider IDs (`gsis_id, fantasypros_id, espn_id, sportradar_id, rotowire_id, rotoworld_id, yahoo_id, fantasy_data_id, stats_id`). External IDs are TEXT even when an upstream payload uses numbers. `gsis_id` is the intended exact nflverse join; `fantasypros_id` is the intended exact FantasyPros ADP join. Store birth date and compute age; store `years_exp` directly. No availability here.
- **`player_season_stats`** — key `(player_id, season)`: games_played, fantasy_points_half_ppr, passing_yards, targets, receptions, rushing_attempts, receiving_yards, rushing_yards, receiving_touchdowns, rushing_touchdowns. Derive per-game metrics; don't store them.
- **`player_week_stats`** — key `(player_id, season, week)`: same stat fields. The approved stats importer calculates half-PPR points with one small explicit function from raw weekly stats.
- **`player_adp`** — key `(player_id, season, source)`: adp, updated_at. M6.3 stores FantasyPros Aggregate ADP as source `fantasypros`; additional sources are optional future work.
- **`player_rankings`** — key `(player_id, season, source)`: overall_rank, position_rank, rank_min, rank_max, rank_std_dev, updated_at. M6.3 stores all-position half-PPR Draft ECR as source `fantasypros`.
- **`player_tiers`** — key `(player_id, season, source)`: tier, updated_at. M6.3 stores the overall tier supplied with FantasyPros Draft ECR; no tier algorithm.
- **`player_projections`** — key `(player_id, season, source)`: nullable passing/rushing/receiving yards and touchdowns plus updated_at. M6.4 stores FantasyPros preseason forecasts separately from historical stats and sportsbook odds.
- **`odds`** — one generic table: `id, season, source, market, player_id (nullable), nfl_team_id (nullable), line, over_price (nullable), under_price (nullable), captured_at`. Exactly one of player/team identifies the subject. Markets like `total_touchdowns`, `regular_season_wins`. No consensus math, no ingestion yet.
- **`drafts`** — `id, sleeper_draft_id, sleeper_league_id, mode (live|manual), status, last_synced_at (nullable), last_sync_error (nullable), created_at, updated_at`. Live is the main path; sync health stays with its draft so changing the active setting preserves history.
- **`draft_picks`** — `id, draft_id, pick_number, round (nullable), draft_slot (nullable), roster_id (nullable), picked_by (nullable), sleeper_player_id, player_id (nullable), source (sleeper|manual), player_first_name/player_last_name/player_position/player_team (nullable), created_at`. Preserve `sleeper_player_id` and pick metadata even when unmapped; idempotent upserts; no duplicate active picks for one player in one draft.
- **`app_settings`** — singleton row `id = 1`: sleeper_username, sleeper_league_id, sleeper_draft_id, polling_enabled (default 1), polling_interval_ms (default 2000), players_synced_at (nullable), updated_at.

Deferred until actually needed (do not create now): `fantasy_rosters`, `fantasy_roster_players`, replay mode and its `replay` enum values. This is a draft-day tool for this season; roster snapshots and replay serve needs that may never materialize.

Indexes when they're free to add: unique `players.sleeper_player_id`; `players.position`; `players.nfl_team_id`; `player_week_stats(player_id, season, week)`; `player_adp(player_id, season, source)`; `player_rankings(player_id, season, source)`; `draft_picks(draft_id, sleeper_player_id)`; `odds(player_id, season, market)`; `odds(nfl_team_id, season, market)`. Nothing else prematurely.

---

## Real-data ingestion workflow

M1's fictional seed was temporary scaffolding for M2–M5 and is retired in M6.1. Production data is built with `go run ./cmd/data`; small deterministic fixtures remain in `_test.go` files so automated tests never depend on external services.

Each dataset has one dedicated Go loader and is imported only by an explicit command. Imports never run on server startup, page load, or an API request. Loaders validate before committing, use idempotent upserts, preserve source/season/capture metadata, and report inserted, updated, skipped, and unmatched records. A failed refresh keeps the last committed data. `rebuild --confirm` backs up the existing SQLite database, builds and validates a temporary replacement, and installs it only after success.

Before each M6.x dataset begins, present for approval: the target table/columns; primary and fallback sources; cost, credentials, terms/access, and freshness; ingestion method; files and dependencies; identity matching; and acceptance checks. Prefer official APIs or provider downloads. Never bypass authentication or paywalls, guess identity matches silently, or switch sources without reporting the problem and obtaining approval. Provider credentials, if later chosen, are environment variables documented through `.env.example`, never committed or stored in application settings.

Current source direction, subject to the per-milestone approval gate where not already approved:

- NFL teams: reviewed static 32-team list.
- Players: Sleeper `/players/nfl`, cached locally because Sleeper requests at most one fetch per day.
- 2025 weekly/season stats: approved nflverse weekly CSV, joined by GSIS ID. Missing local GSIS IDs may be backfilled only by exact Sleeper ID from the DynastyProcess crosswalk; names are diagnostic only. Season rows are derived from weekly rows. The fixed half-PPR formula is 0.04/pass yard, 4/pass TD, -2/interception, 0.1/rush or receiving yard, 6/rush or receiving TD, 0.5/reception, -2/fumble lost, and 2/passing, rushing, or receiving two-point conversion, with no bonuses.
- 2026 draft reference data: the official FantasyPros API with active paid HOF access supplies two independently cached all-position, half-PPR responses: Aggregate ADP (`type=ADP`) and Draft ECR (`type=DRAFT`). The ECR response supplies overall ECR, position rank, overall tier, minimum/maximum expert ranks, and standard deviation. Responses below the completeness threshold are rejected. Each authenticated request is a separate explicit CLI action requiring fresh user approval. Database loads and rebuilds are cache-only. Join FantasyPros player IDs through the DynastyProcess crosswalk already cached by M6.2; do not download another identity table and never name-match.
- 2026 projections: one independently cached FantasyPros preseason response for QB/RB/WR/TE. Persist passing, rushing, and receiving yards/touchdowns exactly by `players.fantasypros_id`; irrelevant position fields remain null. The API labels the response's scoring as standard even when half-PPR is requested, but these volume statistics are scoring-independent. Refreshing projections is one authenticated request requiring fresh user approval; loads and rebuilds are cache-only.
- Odds: structured season-futures API such as SportsDataIO or reviewed sportsbook CSV. Store named sources; consensus methodology requires a separate decision.

---

## Milestones

Small increments; app runnable after each. Run the AGENTS.md §1 checks before declaring any milestone done.

**M0 — Scaffold.** Vite + React + TS frontend (Tailwind, shadcn/ui, React Router with the three routes and top nav), Go backend with health endpoint, Vite `/api` proxy configured, README. *Done when:* both processes start with the two documented commands, the frontend displays backend health through the proxy (proving the no-CORS setup works), `data/` is gitignored.

**M1 — Schema + seed.** Initial embedded migrations for all non-deferred tables, applied automatically at startup; `cmd/seed`. *Done when:* deleting the DB file and starting the server recreates the schema from scratch; seed loads and is idempotent; sample players and weekly rows query cleanly; settings row exists with defaults.

**M2 — Core read API.** NFL teams, player list, player detail, settings GET/PUT, explicit DTOs. *Done when:* Overview data arrives in one reasonable request, player detail includes the weekly series, missing values handled cleanly.

**M3 — Admin.** Bare settings page. *Done when:* values load from SQLite, save, and survive restart; no credential field exists.

**M4 — Overview.** Full table on sample data. *Done when:* all columns visible, both filters work, numeric sorting works, missing values render `—`, drafted-styling structurally supported.

**M5 — Draft Day (sample).** Available table, local name/position/NFL-team/tier filters, row selection, inspector, two position-aware charts, summary stats, ADP/tier/odds display, and settings-aware sample status. *Done when:* selection updates the inspector, charts handle sample data, sample-taken players vanish from Draft Day and gray on Overview.

**M6.1 — Real teams + player pool.** `cmd/data` foundation, reviewed NFL-team loader, cached Sleeper `/players/nfl` client, active QB/RB/WR/TE upserts, GSIS identity, safe rebuild, and production-seed retirement. *Done when:* a confirmed rebuild preserves a recoverable backup and produces 32 teams plus real active fantasy players; re-running is idempotent and respects the once-daily cache; external IDs/profile fields persist; sample IDs/data paths are gone; stats/ADP/tiers/odds/drafts/picks remain empty; Overview and Draft Day show real players with unavailable future values as `—`.

**M6.2 — 2025 historical stats.** Download/cache nflverse regular-season weekly data and the DynastyProcess ID crosswalk; calculate common half-PPR points; safely backfill missing GSIS IDs by exact Sleeper ID; transactionally replace 2025 weekly stats and derive season totals. *Done when:* all 18 weeks are validated; weekly charts and season summaries use consistent real rows; repeated imports are stable; provider/cache failures preserve committed data; identity conflicts, ambiguous mappings, and unmatched IDs are reported rather than guessed. No API key is required.

**M6.3 — FantasyPros draft data.** Cache the approved Aggregate ADP and Draft ECR API queries independently, backfill exact FantasyPros IDs through the DynastyProcess crosswalk, and transactionally populate `player_adp`, `player_rankings`, and `player_tiers`. *Done when:* 2026 Aggregate ADP, overall ECR, position rank, overall tier, expert minimum/maximum, and standard deviation are persisted with FantasyPros provenance/timestamps; failed or incomplete refreshes preserve the prior cache/database; no name guessing occurs; and Overview/Draft Day expose every field with missing values as `—`.

**M6.4 — FantasyPros projections placeholder.** Cache one approved all-position 2026 FantasyPros projection response and transactionally populate `player_projections` by exact FantasyPros ID. *Done when:* passing/rushing/receiving yards and TD forecasts are persisted with source/timestamp metadata; incomplete loads preserve prior data; no name guessing occurs; Overview shows all six fields; and Draft Day separates position-aware forecasts from grouped 2025 history. The `odds` table remains empty.

**M6.5 — Full real-data audit.** Register all approved M6.x loaders in the one build/rebuild entry point and verify completeness, provider-ID uniqueness, UTC metadata, foreign keys, SQLite integrity, and UI behavior. *Done when:* a clean rebuild produces the approved real reference dataset with no synthetic rows and all read APIs/UI states remain healthy. The completed audit uses an offline full-pipeline fixture in automated tests and verified one real clean rebuild across all M6.1–M6.4 datasets.

**M7 — Live draft sync.** Poller + `/api/draft/state`. *Done when:* entering a real draft ID makes Draft Day follow it; exactly one poller runs when enabled+configured; changing ID/interval or disabling cleanly restarts/stops it; new picks remove players without reload; repeated polling never duplicates picks; unknown IDs don't crash sync; Sleeper outage falls back to last known state with a stale flag; polling is toggleable and the interval changes without recompiling. **A Sleeper CPU mock draft is the end-to-end test target — run one before the real draft.**

**M8 — Manual fallback.** Mark-drafted + per-pick undo for the configured draft. *Done when:* manual marking updates availability instantly, survives refresh, cannot delete official picks or another draft's history, and official sync never produces contradictory duplicates.

**M8.1 — Real season odds.** After live sync and manual fallback, choose/approve sources for player futures and populate `odds`. Team win totals remain hidden unless explicitly requested again. *Done when:* markets use named sources/capture times, unmatched subjects are reported, and missing markets remain missing rather than zero.

**M8.2 — Potential morality index.** If pursued, import a manually supplied, source-dated dataset based on USA Today's NFL arrest records and display a user-defined 0–5 score. Agree on the scoring rubric and identity matches before adding schema or UI; preserve the underlying source facts separately from the subjective score, and do not present an arrest record as an objective measure of morality.

**M9 — Polish.** Layout, error/loading states, table density, chart legibility, stale-status UX, README, naming, dead code, unnecessary abstractions. No new major features.

---

## Deferred, on purpose

Single-binary build (Go serving `frontend/dist` via `embed.FS` — the Vite proxy setup makes this a drop-in later), replay mode, fantasy roster caching, drafted-by-roster display, and the optional M8.2 morality-index dataset. Revisit after the season, if ever.
