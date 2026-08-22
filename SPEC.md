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
2. Team
3. Age (calculated from birth date)
4. FantasyPros Aggregate ADP
5. Sleeper ADP
6. Underdog ADP
7. 2025 Total Fantasy Points (0.5 PPR)
8. 2025 Games Played
9. 2025 Average Fantasy Points
10. 2025 Targets Per Game
11. 2025 Receptions
12. 2026 Vegas O/U TDs
13. 2026 Vegas Team Win O/U
14. 2026 Projected Position Tier

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
|                                      | odds / ADPs            |
|                                      | manual action          |
+--------------------------------------+------------------------+
```

**Status bar:** current mode (live / manual / not configured), last-synced time, and a stale indicator + message when Sleeper is unavailable. *Optional, only if it stays trivial:* current pick number and "picks until your turn" derived from `draft_order` + snake order in `GET /draft/{id}` — genuinely useful, but never at the cost of core functionality.

**Available-player table.** Columns: Name, Position, NFL team, 2026 projected tier, FantasyPros Aggregate ADP. Default sort: Aggregate ADP ascending, missing ADP last. Filters: position, NFL team, tier; a search box only if simple. Taken players are **removed** from this table, updating as the draft-state query receives new picks — no full page refresh, ever.

**Selection.** Clicking a row selects the player into a persistent right-side inspector (no navigation to a player page). Selecting another row replaces the inspector immediately. Selected player ID is local frontend state.

### Player Inspector

Prioritize quick draft decisions.

- **Header:** name, position, NFL team, 2026 tier, Aggregate ADP (Sleeper/Underdog ADP optionally nearby).
- **2025 weekly 0.5-PPR summary:** average, high, median, low. Median computed correctly. No weekly data → empty state, not fake zeros.
- **Three compact Recharts line charts:** weekly fantasy points, weekly targets, weekly rushing attempts (works for every player; zero-heavy data acceptable — no position-specific chart systems). X = week, Y = value, tooltips required, optional average reference line if visually clean, no elaborate interactions.
- **Additional data:** 2025 games played, receptions, targets/game; 2026 Vegas TD O/U and team win O/U; all three ADPs.
- **Manual action:** `Mark Drafted` button (see fallback below).
- No recommendation scores or decision logic of any kind.

**Chart component:** one reusable weekly line chart taking `title, data, xKey, yKey, unit, optionalAverage`. Summary stats (avg/high/median/low) are calculated in **one** place — prefer the Go backend in the detail DTO for consistency and testability.

### Manual fallback

`Mark Drafted` creates a `source='manual'` pick on the active local draft; the player immediately leaves the available list and grays out on Overview; affected TanStack queries are invalidated. Manual picks survive refresh (they're DB rows) and are undoable — list-and-delete or `Undo Last Manual Pick`, whichever is simpler. Idempotency with official Sleeper picks per AGENTS.md §7.

---

## Page 3 — Admin (`/admin`)

Bare settings page. Plain controlled inputs, no form libraries.

Fields: Sleeper username, league ID, draft ID, polling enabled, polling interval (ms). Buttons: `Save Settings`, `Sync Players` (fetches `/players/nfl` once, upserts local players, shows last-synced time), and optionally `Refresh Draft State` if easy. Small save-success / validation messages. Validate: interval is a positive reasonable number; IDs are strings; empty values allowed during setup. **No token / API key / password / credential fields exist.**

---

## Backend API

Small REST surface. Names may evolve slightly; keep the shape close to this. Explicit DTOs, not raw rows.

```text
GET  /api/health                       → { "status": "ok" }
GET  /api/settings                     → settings object
PUT  /api/settings                     → accepts full settings object
GET  /api/nfl-teams                    → for filters/display
GET  /api/players?position=&team=&available_only=
GET  /api/players/{id}                 → { player, season, adp, tier, odds, weekly: [] }
GET  /api/draft/state                  → see below
POST /api/draft/manual-picks           → { "player_id": 42 }
DELETE /api/draft/manual-picks/{id}
POST /api/players/sync                 → triggers the one-shot Sleeper players sync
```

`/api/players` returns the fields Overview and Draft Day need (name, position, team, age, three ADPs, 2025 season summary fields, 2026 odds lines, tier, `is_taken`). These are presentation fields — they do not imply denormalized columns.

`/api/draft/state` is a **read** endpoint: reads settings; returns a clean "not configured" state when no draft ID exists; reads the latest synchronized state from SQLite; resolves local player IDs; combines official + manual picks without duplicates; falls back to persisted picks when Sleeper is down. Shape (may evolve; preserve the concepts):

```json
{
  "draft_id": "1234567890",
  "mode": "live",
  "stale": true,
  "last_synced_at": "2026-08-15T18:30:00Z",
  "message": "Sleeper is temporarily unavailable; showing last known draft state.",
  "picks": [],
  "taken_player_ids": [],
  "unknown_sleeper_player_ids": []
}
```

Frontend: one TanStack Query per concern — `["settings"]`, `["players", filters]`, `["player", id]`, `["draft-state"]`, `["nfl-teams"]`. One draft-state query feeds the whole Draft Day page (no redundant nested-component refetches). Mutations for settings and manual picks invalidate only affected queries. All requests use relative `/api/...` paths through the Vite proxy.

---

## Database schema direction

Normalized; season/source-varying data in its own tables. SQLite types and conventions per AGENTS.md §4 (`INTEGER PRIMARY KEY` ids, `TEXT` Sleeper IDs and ISO-8601 UTC timestamps, `REAL` for points/ADP/lines, `INTEGER` 0/1 booleans). Migrations are the source of truth once written — don't add columns with no current or near-term use.

- **`nfl_teams`** — `id, abbreviation (unique), name`. No win totals here (season/source-varying → `odds`).
- **`players`** — core identity (`id, sleeper_player_id, first_name, last_name, position, nfl_team_id, birth_date, active`), Sleeper profile/status (`status, number, college, height, weight, birth_country, years_exp, depth_chart_position, depth_chart_order, injury_status, injury_start_date, practice_participation`), and nullable cross-provider IDs (`espn_id, sportradar_id, rotowire_id, rotoworld_id, yahoo_id, fantasy_data_id, stats_id`). External IDs are TEXT even when an upstream payload uses numbers. Store birth date and compute age; store `years_exp` directly. No availability here.
- **`player_season_stats`** — key `(player_id, season)`: games_played, fantasy_points_half_ppr, targets, receptions, rushing_attempts, receiving_yards, rushing_yards, receiving_touchdowns, rushing_touchdowns. Derive per-game metrics; don't store them.
- **`player_week_stats`** — key `(player_id, season, week)`: same stat fields. Storing weekly half-PPR points directly is fine for sample data; if raw stats are ingested later, calculate points with one small explicit function.
- **`player_adp`** — key `(player_id, season, source)`: adp, updated_at. Sources: `fantasypros`, `sleeper`, `underdog`.
- **`player_tiers`** — key `(player_id, season, source)`: tier, updated_at. Initial source is sample only; no tier algorithm.
- **`odds`** — one generic table: `id, season, source, market, player_id (nullable), nfl_team_id (nullable), line, over_price (nullable), under_price (nullable), captured_at`. Exactly one of player/team identifies the subject. Markets like `total_touchdowns`, `regular_season_wins`. No consensus math, no ingestion yet.
- **`drafts`** — `id, sleeper_draft_id, sleeper_league_id, mode (live|manual), status, created_at, updated_at`. Live is the main path.
- **`draft_picks`** — `id, draft_id, pick_number, round (nullable), draft_slot (nullable), roster_id (nullable), picked_by (nullable), sleeper_player_id, player_id (nullable), source (sleeper|manual), created_at`. Preserve `sleeper_player_id` even when unmapped; idempotent upserts; no duplicate active picks for one player in one draft.
- **`app_settings`** — singleton row `id = 1`: sleeper_username, sleeper_league_id, sleeper_draft_id, polling_enabled (default 1), polling_interval_ms (default 2000), players_synced_at (nullable), updated_at.

Deferred until actually needed (do not create now): `fantasy_rosters`, `fantasy_roster_players`, replay mode and its `replay` enum values. This is a draft-day tool for this season; roster snapshots and replay serve needs that may never materialize.

Indexes when they're free to add: unique `players.sleeper_player_id`; `players.position`; `players.nfl_team_id`; `player_week_stats(player_id, season, week)`; `player_adp(player_id, season, source)`; `draft_picks(draft_id, sleeper_player_id)`; `odds(player_id, season, market)`; `odds(nfl_team_id, season, market)`. Nothing else prematurely.

---

## Seed data

The first useful version must not depend on external providers. Seeding is `go run ./cmd/seed`: a small Go program that inserts clearly fictional players with clearly synthetic stats (real NFL team abbreviations are fine; label as sample data in README/UI). It must be idempotent — re-running never duplicates rows (upsert or wipe-and-reload sample-tagged data, whichever is simpler).

Seed at least: **~50–75 players** across QB/RB/WR/TE and many NFL teams (enough that Draft Day filtering, tiers, and pick-by-pick removal actually look and behave like a draft); 2025 season stats; 6–8 weeks of 2025 weekly stats per player; all three ADP sources; 2026 sample tiers; 2026 sample player TD lines and team win totals; one local draft with a few sample picks. Generating the synthetic values in a loop with simple randomization is encouraged — don't hand-write 75 players, and don't spend effort making fake data realistic. Spend it making every UI state reachable.

---

## Future data ingestion (context only — do not build)

Real providers are intentionally undecided: FantasyPros/Sleeper/Underdog ADP, 2025 real stats, tiers (manual/imported/proprietary/calculated), sportsbook TD and win totals. CSV import is an acceptable future escape hatch. When ingestion eventually happens: normalize by Sleeper player ID where possible; preserve source, season, and capture time; never overwrite one source's values with another's; surface unconfident player matches instead of guessing silently. Build no provider clients before a provider is chosen.

Note: syncing the **player pool** from Sleeper's free `/players/nfl` endpoint is *in scope* (Milestone 6) — it's identity data the draft board can't function without, not provider stat ingestion.

---

## Milestones

Small increments; app runnable after each. Run the AGENTS.md §1 checks before declaring any milestone done.

**M0 — Scaffold.** Vite + React + TS frontend (Tailwind, shadcn/ui, React Router with the three routes and top nav), Go backend with health endpoint, Vite `/api` proxy configured, README. *Done when:* both processes start with the two documented commands, the frontend displays backend health through the proxy (proving the no-CORS setup works), `data/` is gitignored.

**M1 — Schema + seed.** Initial embedded migrations for all non-deferred tables, applied automatically at startup; `cmd/seed`. *Done when:* deleting the DB file and starting the server recreates the schema from scratch; seed loads and is idempotent; sample players and weekly rows query cleanly; settings row exists with defaults.

**M2 — Core read API.** NFL teams, player list, player detail, settings GET/PUT, explicit DTOs. *Done when:* Overview data arrives in one reasonable request, player detail includes the weekly series, missing values handled cleanly.

**M3 — Admin.** Bare settings page. *Done when:* values load from SQLite, save, and survive restart; no credential field exists.

**M4 — Overview.** Full table on sample data. *Done when:* all columns visible, both filters work, numeric sorting works, missing values render `—`, drafted-styling structurally supported.

**M5 — Draft Day (sample).** Available table, row selection, inspector, three charts, summary stats, ADP/tier/odds display, using sample draft state. *Done when:* selection updates the inspector, charts handle sample data, sample-taken players vanish from Draft Day and gray on Overview.

**M6 — Real player pool.** Sleeper client foundations + Admin `Sync Players` against `/players/nfl`. *Done when:* one click populates real NFL players (upsert — re-running doesn't duplicate), sync time is recorded and displayed, synthetic sample players can coexist or be cleanly replaced (document the choice).

**M7 — Live draft sync.** Poller + `/api/draft/state`. *Done when:* entering a real draft ID makes Draft Day follow it; exactly one poller runs when enabled+configured; changing ID/interval or disabling cleanly restarts/stops it; new picks remove players without reload; repeated polling never duplicates picks; unknown IDs don't crash sync; Sleeper outage falls back to last known state with a stale flag; polling is toggleable and the interval changes without recompiling. **A Sleeper CPU mock draft is the end-to-end test target — run one before the real draft.**

**M8 — Manual fallback.** Mark-drafted + undo. *Done when:* manual marking updates availability instantly, survives refresh, and official sync never produces contradictory duplicates.

**M9 — Polish.** Layout, error/loading states, table density, chart legibility, stale-status UX, README, naming, dead code, unnecessary abstractions. No new major features.

---

## Deferred, on purpose

Single-binary build (Go serving `frontend/dist` via `embed.FS` — the Vite proxy setup makes this a drop-in later), replay mode, fantasy roster caching, real data ingestion, drafted-by-roster display. Revisit after the season, if ever.
