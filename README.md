# Fantasy Football Draft App

A personal, local-first fantasy football draft-day dashboard. Milestone M0 provides the two-process scaffold: a Go API and a Vite/React frontend.

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

In another terminal, start the frontend:

```bash
cd frontend && npm run dev
```

Open `http://localhost:5173`. The Vite development server proxies relative `/api` requests to `http://localhost:8080`; the Go server does not enable CORS.

The seed command (`cd backend && go run ./cmd/seed`) is introduced in Milestone M1, when SQLite storage is added. There is no database or seed data in M0.

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
