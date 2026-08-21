# AGENTS.md

Personal, local-first fantasy football **draft-day** web app for one Sleeper draft this season (12-team, 1-QB, 0.5-PPR). Single user, runs on a laptop, not deployed. Product requirements, schema direction, and milestones live in `SPEC.md` — read it before implementing features. This file is the durable working agreement.

Priorities, in order:

1. Correctness of the features that exist
2. Readability and hackability
3. A cohesive, useful draft-day UI
4. Simple local development
5. Easy extension later
6. Performance only where it affects draft-day responsiveness

Prefer obvious code over clever code. Prefer a little duplication over an abstraction that makes the project harder to understand. This should feel like a useful personal tool, not an architecture demonstration. The end state is a dashboard that can be understood, modified, and debugged quickly *while a real draft is happening*.

---

## 1. Working agreement

- Inspect the existing repo and reuse its patterns before inventing new ones.
- If a requirement is materially unclear or would change the architecture/UX significantly, ask a concise grouped list of questions first. Do not re-ask things answered here or in `SPEC.md`.
- If ambiguity is minor and the simplest choice is obvious, make it and mention it.
- For substantial work, state a short plan before editing.
- Work incrementally; keep the app runnable at logical checkpoints. No large unrelated refactors mid-feature.
- Add a dependency only when it clearly simplifies the implementation.
- Update `README.md` when setup steps, commands, env vars, or user-facing behavior change.
- If a user instruction conflicts with this file, follow the instruction, then update this file if the new behavior should become a lasting rule.

**Before declaring any task or milestone done, run and pass:**

```bash
# backend/
go test ./... && go vet ./...

# frontend/
npm run lint && npm run build
```

If a command doesn't exist yet, add the simplest equivalent.

---

## 2. Commands and ports

The entire app is two processes and one file. No Docker.

```bash
cd backend && go run ./cmd/server    # Go API on :8080 (runs migrations automatically at startup)
cd backend && go run ./cmd/seed      # loads synthetic sample data (idempotent; safe to re-run)
cd frontend && npm run dev           # Vite dev server on :5173
```

Keep these exact commands accurate in `README.md`. Fixed ports: frontend **5173**, backend **8080**. Don't change them silently.

**No CORS anywhere.** The frontend calls relative paths (`/api/...`); `vite.config.ts` proxies `/api` to `http://localhost:8080`. The Go server never sets CORS headers, and the frontend never hardcodes a backend origin. This also means a future single-binary build (Go serving the built frontend via `embed.FS`) works with zero code changes — that build is explicitly deferred, not for now.

**Configuration:** environment variables with sensible defaults so zero setup is required — `DB_PATH` (default `./data/draft.db`), `BACKEND_PORT` (default `8080`). No `.env` machinery unless a third variable ever appears. Sleeper username / league ID / draft ID / polling settings are **application settings stored in SQLite** (`app_settings`, singleton row `id = 1`), editable on the Admin page — never compile-time constants, never env vars.

---

## 3. Technology stack

**Frontend:** Vite + React + TypeScript — a plain client-side SPA. There is no server-side rendering, no server components, no meta-framework. React Router (library mode) for the three routes. Tailwind CSS, shadcn/ui (Vite setup), TanStack Query (server state + polling), TanStack Table (tables), Recharts (charts, preferably via shadcn chart components). Use the stable versions selected at project init; don't churn versions without a reason.

**Backend:** Go, `net/http`, `chi` for routing, standard `database/sql` with `modernc.org/sqlite` (pure Go — the build must never require cgo), handwritten SQL, standard `encoding/json`, standard `net/http` client for Sleeper. No ORM, no `sqlc` initially — the query surface is small and readability wins; revisit only if query maintenance becomes painful.

**Database:** a single SQLite file at `DB_PATH`. It durably stores players, historical stats, ADP, tiers, odds, settings, and draft/pick history so nothing must be re-fetched on every start. Backing up the database = copying the file. `data/` is gitignored.

**Migrations:** plain SQL files in `backend/migrations/`, embedded with `embed.FS` and applied automatically at server startup via the `goose` library (`sqlite3` dialect). No migration CLI to install, no separate migrate step.

---

## 4. SQLite rules

These are the non-negotiable mechanics of using SQLite correctly here:

1. **Open the database exactly like this** (WAL for concurrent reads during writes, FKs enforced, writers wait instead of erroring):

```go
db, err := sql.Open("sqlite",
    "file:"+dbPath+"?_pragma=journal_mode(WAL)&_pragma=foreign_keys(1)&_pragma=busy_timeout(5000)")
```

2. Placeholders are `?`, not `$1`. Upserts use `INSERT ... ON CONFLICT (...) DO UPDATE`.
3. **Timestamps are TEXT, ISO-8601, UTC** — write with `time.Now().UTC().Format(time.RFC3339)`, parse with `time.Parse(time.RFC3339, s)`. Never store local time; never rely on SQLite datetime functions for correctness.
4. Types: `INTEGER PRIMARY KEY` for internal ids; `TEXT` for Sleeper IDs and timestamps; `REAL` for ADP, lines, and fantasy points; `INTEGER` 0/1 for booleans.
5. This is a single-process, single-user app: the API handlers and the one poller share one `*sql.DB`. With WAL + busy_timeout that is the whole concurrency story — do not add connection-pool tuning, mutexes around the DB, or a write queue.
6. Keep transactions short and per-operation (e.g., one transaction per poll upsert batch).

---

## 5. Architecture

```text
Browser ──/api (Vite proxy)──> Go backend ──> SQLite file
                                   │
                                   └──HTTPS──> Sleeper public API
```

The browser never calls Sleeper directly.

**Backend owns:** Sleeper integration, DB access, settings, draft-state sync, manual-pick fallback, API response shaping (explicit DTOs, not raw rows).

**Frontend owns:** rendering, filters, selected-player state, charts, polling cadence, local presentation state.

**Two separate loops — never merge them:**

```text
Backend loop:   Sleeper → Go poller → SQLite        (default every 2000 ms)
Frontend loop:  Browser → Go API → SQLite            (TanStack Query, ~1000 ms)
```

A frontend refresh must never cause a Sleeper call. The frontend reads the latest *synchronized* state from the backend.

### The poller

Exactly **one** background goroutine polls the configured active draft. This is the only sanctioned application-level concurrency. Never one poller per request, page, or tab.

- Reads draft ID / interval / enabled from current settings.
- Runs an immediate sync on start, then ticks on the interval.
- Each iteration is synchronous and linear: fetch picks → parse → map Sleeper IDs to local player IDs where possible → upsert → update last-sync status.
- Stops/restarts cleanly (store a cancel func, `context.Context`, ticker) when polling is disabled, the draft ID changes, the interval changes, or the app shuts down.
- No configured draft ID is a **normal state**: no poller runs, and Overview / player detail / Admin / manual mode all still work. Draft Day shows a clean "not configured" state, not an error.

**Sleeper failure behavior:** never crash; keep serving the last persisted local draft state; mark it `stale: true` with `last_synced_at` and a short user-displayable message; keep manual actions usable.

---

## 6. Sleeper API reference

Public, read-only, **no API key or token exists — never add a credential field anywhere.** Base URL `https://api.sleeper.app/v1`.

```text
GET /user/{username}            → user object (user_id)
GET /league/{league_id}         → league metadata
GET /league/{league_id}/drafts  → drafts for the league
GET /draft/{draft_id}           → draft metadata (status, settings, draft_order, slot_to_roster_id)
GET /draft/{draft_id}/picks     → array of pick objects (the endpoint the poller lives on)
GET /players/nfl                → full NFL player map, keyed by sleeper player id (~5 MB)
```

Pick objects include: `round`, `pick_no`, `draft_slot`, `roster_id`, `picked_by`, `player_id` (string), and a `metadata` object with `first_name`, `last_name`, `position`, `team`. Persist `pick_no` ordering and the raw `player_id` always.

`/players/nfl` is large and Sleeper asks that it be fetched **at most once per day**. Fetch it only from an explicit Admin "Sync Players" action, upsert into the local `players` table, and record the sync time. Never fetch it per request, on boot, or from the poller.

Sleeper IDs (player, league, draft, roster owner) are **strings** everywhere — DB, Go, JSON. Do not parse them as integers.

---

## 7. Data ownership rules (load-bearing — do not violate)

1. **Sleeper is authoritative for live picks.** During live mode the latest successful Sleeper response is truth for Sleeper-reported picks. The backend persists them for restart resilience, debugging, and fallback.
2. **SQLite is authoritative for local/reference data:** players, NFL teams, stats, ADP, tiers, odds, settings, persisted pick history.
3. **"Taken" is never a player property.** No mutable `players.taken` column, ever. A player is taken only in the context of a specific draft; availability is derived from the active draft's picks. The frontend may hold a derived in-memory `Set` of taken IDs for rendering.
4. **Sync is idempotent.** Re-fetching the same Sleeper picks must not create duplicate rows. Key pick identity on (draft, sleeper_player_id / pick_no) as appropriate. A player manually marked drafted who later appears in official Sleeper picks must not produce contradictory duplicate availability.
5. **Unknown Sleeper player IDs never break sync.** Persist the Sleeper player ID with a nullable local `player_id` mapping, surface it for debugging (and render the name from pick `metadata` when available), and keep syncing.
6. Manual picks are a fallback, not the primary mechanism. They live in the same `draft_picks` table with `source = 'manual'`, are undoable, and never mutate `players` rows.

---

## 8. Engineering style

**Prefer:** plain Go; plain SQL; straightforward React components; small functions with descriptive names; explicit data flow, structs, and DTOs; direct HTTP; simple REST; simple queries; local component state; comments only where the "why" isn't obvious.

**Avoid:** clever generics; premature interfaces; framework-like internal layering (`ports/adapters/usecases/repositories/factories`); reflection-heavy code; channels/goroutines beyond the one poller (Go's HTTP server using concurrency internally is fine); Redux/Zustand or mirroring server state into Context (revisit only if local state becomes genuinely unmanageable); React effects that derived state or TanStack Query already handles; frontend virtualization, server-side pagination, or caching layers before an observed need. Never introduce SSR, server components, or a meta-framework — this is a client-side SPA by design.

**Backend layout** (guideline — don't create packages or files the code doesn't need yet):

```text
backend/
  cmd/
    server/main.go
    seed/main.go
  internal/
    api/        handlers, routes (split by subject when handlers.go grows)
    database/   db.go (open + migrate), players.go, settings.go, drafts.go
    sleeper/    client.go, types.go
    draft/      state.go, poller.go
  migrations/   001_initial.sql, ...   (embedded, applied at startup)
  data/         draft.db               (gitignored)
```

**Frontend layout:**

```text
frontend/
  src/
    main.tsx            router + providers
    App.tsx             layout + top nav
    pages/              overview.tsx, draft.tsx, admin.tsx
    components/         extracted as they earn a name
    lib/                api.ts, types.ts
  vite.config.ts        /api proxy → :8080
```

Extract a component when it's reused, a page becomes hard to read, or a distinct UI concept deserves a name — not before, and not per table cell.

---

## 9. Conventions

**Terminology (use consistently in code, UI, and API):**

- **NFL team** = Texans, Bills, Lions. Never call an NFL team a "roster".
- **fantasy roster / fantasy team** = a participant's Sleeper roster.
- **draft** = one Sleeper draft event; **pick** = one player selected in it.
- **taken / available** = derived per-draft state.
- **Aggregate ADP** = FantasyPros Aggregate ADP specifically. Never a computed cross-provider average.
- **tier** = externally supplied/sample projected position tier — never an app-generated recommendation.

**Data conventions:** ISO-8601 UTC timestamps in JSON and in the DB (per §4); integer seasons (`2025`) and weeks (`1`); numbers are JSON numbers (only inherently string-like external IDs are strings). Missing values render as `—`, never as `0`, and sort after real values. Season-specific values live in season/source tables, never as `2025_*` columns on `players`.

**Migrations:** never edit an applied migration; add a new one. Keep them plain SQL and readable.

**Errors/logging:** parameterized SQL always; generic errors to the browser (no stack traces or raw SQL); log server-side what's needed to debug locally — startup, migration results, Sleeper failures, sync summaries, unknown Sleeper IDs, settings updates, manual actions. Don't log every successful request.

---

## 10. Testing philosophy

Protect behavior that can break quietly, without turning the project into a testing exercise.

**Backend (focused Go tests, table-driven where it helps):** median/average/high/low calculation; taken-player derivation; Sleeper pick parsing; idempotent pick merge; manual/Sleeper duplicate handling. Use `httptest.Server` for Sleeper-client tests, and an in-memory or temp-file SQLite database for storage tests — a real database in tests is cheap now; use it instead of mocks. No mock frameworks.

**Frontend:** TypeScript clean, lint clean, build clean. Component tests only if UI logic becomes genuinely complex later.

---

## 11. Non-goals

Do not build unless explicitly requested later: auth, accounts, multi-tenancy, cloud deployment, billing; recommendation algorithms, ML, "best pick" logic, positional-scarcity math, availability prediction, Monte Carlo simulation; WebSockets, SSE, GraphQL, microservices, message queues, Redis, caching layers, background job systems beyond the one poller; DI frameworks, plugin architectures; Docker, Postgres, or any database server; SSR or meta-frameworks; provider ingestion pipelines (FantasyPros / Underdog / sportsbooks) before a provider is chosen; scoring-rule engines, league-rules engines, identity-matching engines; mobile-first design or native apps; production secret management, OAuth, sessions, CSRF machinery. The single-binary `embed.FS` build is a known future step, not a current one.

Do not build speculative features just because the schema could support them. If the project is ever deployed publicly, revisit security before treating it as safe.

**Unresolved decisions — use sample data and clean extension points; ask before choosing:** real stats provider, ADP ingestion mechanisms, odds provider/consensus methodology, tier source and methodology, extra scoring settings, deployment.

---

## 12. Definition of "simple"

A solution is simple when a developer can answer these quickly: Where does this data come from? Which function calls Sleeper? Which SQL query loads this page? What determines whether a player is taken? Where are settings stored? Where does the poller start and stop? What causes the page to refresh? How do I add another ADP or odds source? How do I seed another player? How do I run this locally?

If answering any of those requires navigating six abstraction layers, simplify the design.
