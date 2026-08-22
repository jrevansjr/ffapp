# Fantasy Football Draft App

A personal, local-first fantasy football draft-day dashboard. Milestone M1 adds persistent SQLite storage, embedded migrations, and fictional sample data to the Go API and Vite/React scaffold.

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

The seed contains 60 clearly fictional players with synthetic stats, ADP, tiers, odds, and draft picks. It is idempotent and safe to run again: existing sample rows are updated rather than duplicated. Seeding is explicit and never runs automatically when the backend starts.

## Available routes

- `/overview`
- `/draft`
- `/admin`
- `/api/health` on the backend, or through the Vite proxy

The frontend header displays the result of `/api/health`, providing a visible check that the two processes are communicating through the proxy.

## Checks

Run the backend checks:

```bash
cd backend && go test ./... && go vet ./...
```

Run the frontend checks:

```bash
cd frontend && npm run lint && npm run build
```
