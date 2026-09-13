# System Architecture

## 1. High-level architecture

```
                    ┌──────────────────────────────┐
 Browser (React) ──▶│       Load Balancer (TLS)     │
                    └──────────────┬───────────────┘
                                   │
        ┌──────────────────────────┼──────────────────────────┐
        │                          ▼                          │
        │            ┌─────────────────────────┐              │
        │            │      Go / Gin API        │  REST + WS  │
        │            └──────┬──────────┬───────┘              │
        │                   │          │                      │
        │              enqueues     reads                      │
        │                   ▼          ▼                      │
        │            ┌──────────┐┌───────────┐                │
        │            │  Worker   ││ PostgreSQL│                │
        │            └────┬─────┘└───────────┘                │
        │                  │        ▲                         │
        │                  │        │ cache / sessions         │
        │                  │   ┌────┴─────────┐                │
        │                  │   │    Redis      │                │
        │                  │   └──────────────┘                │
        │                  ▼                                   │
        │        ┌────────────────────────────────┐            │
        │        │    MikroTik routers (API)        │            │
        │        └────────────────────────────────┘            │
        └──────────────────────────────────────────────────────┘
```

- Single deployment serves many tenants.
- API handles all client requests (REST + WebSocket).
- Worker performs router synchronisation and background jobs out-of-band so the API stays responsive.

## 2. Frontend

- **Stack:** React + TypeScript, Vite build, single-page application served statically.
- **Routing:** public routes (login), tenant routes, and admin routes (guarded).
- **Data:** typed API client; React Query-style caching for server state; WebSocket hook for live updates (router/session status).
- **Key screens:** Login, Dashboard, Routers, Hotspots, Vouchers, Sessions, Analytics, Tenant Settings, Admin space (users/tenants).
- **Captive portal:** separate minimal React bundle optimized for public hotspot login pages.

## 3. Go / Gin API

- **Framework:** Go with the Gin HTTP router; middleware chain for auth, tenant context, validation, rate limiting, logging, error envelope.
- **Layers:** handler → service → repository (PostgreSQL via pgx/sqlc or GORM, per project choice) + Redis client.
- **WebSocket:** hub module broadcasting router/session events to subscribed clients.
- **Jobs:** publishes router/config tasks to the worker queue (Redis-based).
- **Health:** `/healthz` (liveness) and `/readyz` (readiness) endpoints.

## 4. PostgreSQL

- Primary relational store: tenants, users, routers, hotspots, profiles, vouchers, sessions, usage records, audit logs.
- Migrations maintained under `migrations/` (versioned, applied by CI/deploy).
- Aggregated analytics read from digest/report tables to keep dashboards fast.
- One database shared across tenants; isolation at query level via `tenant_id` (see DATABASE-DESIGN).

## 5. Redis

Used for:

- Session/token cache and revocation lists.
- Rate limiting counters.
- Worker job queue (list/zset of router sync & config-push tasks).
- Realtime fan-out metadata if needed.
- Short-lived key/value configuration and distributed locks.

## 6. Worker

- Separate process; consumes queues from Redis.
- **Router sync:** polls registered routers for session/usage data and stores it.
- **Config push:** applies hotspot/profile/voucher-scope changes to MikroTik idempotently.
- **Status probing:** updates router online/offline status and emits WebSocket events.
- **Housekeeping:** expiry of vouchers and sessions, report digest generation.
- Scales horizontally; workers coordinate via Redis locks to avoid duplicate pushes.

## 7. MikroTik integration

- Communicates with MikroTik RouterOS over the RouterOS API (port 8728/8729).
- Commands executed: fetch hotspot active users, apply hotspot server/profile config, add/remove users (voucher accounts), disconnect sessions.
- All mutation jobs are queued and retried with backoff; results recorded on the resource.
- Credentials stored encrypted; API-only access; direct CLI/SSH not used.

## 8. WebSocket

- Maintenance endpoint `/ws` upgraded to WebSocket after authentication.
- Events: `router.status`, `session.active`, `session.closed`, `voucher.redeemed`, `hotspot.status`.
- Clients subscribe to tenant-scoped channels; the hub pushes only authorised scopes.
- Heartbeat (ping/pong) and reconnection with last-event cursor (fallback to polled sync).

## 9. Deployment architecture

- Docker Compose for dev: `backend`, `worker`, `frontend`, `postgres`, `redis`.
- Production: containers behind a TLS-terminating load balancer/reverse proxy (e.g. Nginx/Traefik) with sticky sessions for WebSocket.
- PostgreSQL and Redis as managed or dedicated services; volumes for persistence.
- Migrations run as a deploy step against PostgreSQL before new API releases.
- Health checks gate load-balancer membership; structured logs (JSON) shipped to a collector.
- Env-driven configuration: secrets via env/secret store; never committed.