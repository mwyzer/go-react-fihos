# FIHOS

Wi-Fi hotspot management — Go/Gin backend + React/Vite frontend, fully containerized.

## Stack

| Service | Image | External port | Notes |
|---|---|---|---|
| `web` | `fihos-web` (nginx) | `8081` | Serves the React app, proxies `/api`, `/ws`, `/healthz` to the API |
| `api` | `fihos-backend` | `8080` | Go/Gin API on its own port |
| `worker` | `fihos-backend` | — | Background worker (router sync/probe) |
| `migrate` | `fihos-backend` | — | Runs once, then exits (golang-migrate) |
| `postgres` | `postgres:17-alpine` | `55432` | pg data persisted in the `pgdata` volume |
| `redis` | `redis:7-alpine` | `6380` | Cache/session store |

## Requirements

- Docker Engine with Docker Compose v2 (plugin)
- Nothing else — images are built in containers

## Quick start

```bash
docker compose -f docker/docker-compose.yml up -d --build
```

First run builds the Go and frontend images (a few minutes) and applies migrations. After that:

- Frontend: http://localhost:8081
- API base: http://localhost:8080 (`/healthz`, `/readyz`, `/api/v1/...`)

### Verify it's healthy

```bash
curl http://localhost:8080/readyz          # {"db":"up","redis":"up","status":"ok"}
curl http://localhost:8080/healthz         # {"status":"ok"}
curl http://localhost:8080/healthz         # also reachable via the web proxy
```

## Common commands

Run from the repository root.

| Task | Command |
|---|---|
| Start the stack | `docker compose -f docker/docker-compose.yml up -d` |
| Rebuild after code changes | `docker compose -f docker/docker-compose.yml up -d --build` |
| Show status | `docker compose -f docker/docker-compose.yml ps` |
| Follow all logs | `docker compose -f docker/docker-compose.yml logs -f` |
| Logs for one service | `docker compose -f docker/docker-compose.yml logs -f api` |
| Stop (keep data) | `docker compose -f docker/docker-compose.yml down` |
| Stop and delete DB volume | `docker compose -f docker/docker-compose.yml down -v` |
| Reset database | `docker compose -f docker/docker-compose.yml down -v && docker compose -f docker/docker-compose.yml up -d --build` |

## Configuration

Environment is defined in `docker/docker-compose.yml`. Overrides via a `.env` file next to the compose file (or environment variables), e.g.:

```bash
JWT_SECRET=change-me
```

Defaults (dev only):

- Postgres: `fihos` / `fihos` @ db `fihos`
- Redis: no auth
- `JWT_SECRET`: `dev-secret-change-me`

## Running locally without Docker

Backend (Go 1.23+):

```bash
cd backend
go run ./cmd/migrate   # applies migrations
go run ./cmd/api       # API on :8080
go run ./cmd/worker    # background worker
```

Set `DATABASE_URL` / `REDIS_ADDR` to `fihos:fihos@localhost:55432/fihos` and `localhost:6380` if the compose services are running.

Frontend (Node 22+):

```bash
cd frontend
npm install
npm run dev             # Vite dev server, proxy -> localhost:8080
```

## Project layout

```
backend/    Go API, worker, migrate entrypoints (cmd/) + internals
frontend/   React + Vite SPA
migrations/ SQL migration files (canonical source)
docker/     Dockerfiles, nginx conf, docker-compose.yml
docs/       PRD, SRS, API spec, DB design, plans
```

## Troubleshooting

- **Port already in use** (`bind: Only one usage of each socket address`): stop whatever is on `8080`/`8081` (e.g. a local `go run ./cmd/api` dev process), or change the `ports:` mapping in the compose file.
- **Migrations not applied**: check `docker compose logs migrate`; run `migrate` again with `docker compose -f docker/docker-compose.yml up migrate`.
- **Frontend cannot reach the API**: nginx proxies to the internal `api` service by name over the compose network; don't change service names in the compose file unless you update `docker/nginx.conf` too.