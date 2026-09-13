# Phase 2 — Routers, Hotspots & MikroTik

Extends PHASES.md Phase 2. Sprint scope: DOCUMENT-PLAN **P5** (router CRUD, probe, status polling, WS events) + **P6** (hotspot CRUD, profile CRUD, MikroTik config push).

- **Goal:** central router inventory with live status and API-driven hotspot provisioning — "the router config just happens".
- **Milestone:** M2 (front half) — routers + hotspots reach end-to-end against a (mock) MikroTik device.

## 1. Scope (in)

1. Router registration/lifecycle + encrypted credential storage (`routers`).
2. On-demand probe + worker-polled status (`unverified`/`online`/`offline`) with WebSocket push of `router.status`.
3. Hotspot CRUD under a router, with profile assignment.
4. Hotspot profile CRUD (bandwidth caps, session uptime limit, keepalive, MAC binding).
5. Config push: worker builds RouterOS commands and applies them via the MikroTik API; idempotent + retryable; hotspot status `configuring` → `active`/`error`.
6. Hotspot enable/disable (blocks new sessions).
7. WebSocket hub + tenant-scoped subscriptions pushing `router.status` and `hotspot.status` (first WS delivery — required by NFR-REAL-1).

## 2. Scope (out)

- Voucher/portal features (Phase 3).
- Live session sync + disconnect (Phase 4) — session row creation on redemption only.
- Non-MikroTik router vendors.

## 3. Functional requirements (from SRS)

| ID | Requirement |
|---|---|
| FR-ROUTER-1 | Register routers: name, IP, API port, credentials |
| FR-ROUTER-2 | Test router connectivity and report status |
| FR-ROUTER-3 | Periodically probe and mark `online`/`offline` |
| FR-ROUTER-4 | Credentials encrypted at rest, never returned |
| FR-ROUTER-5 | Status changes pushed to clients over WebSocket |
| FR-HOTSPOT-1 | Create hotspots under a router |
| FR-HOTSPOT-2 | Assign a hotspot profile (bandwidth, session limits) |
| FR-HOTSPOT-3 | Push hotspot config to router on create/update |
| FR-HOTSPOT-4 | Enable/disable hotspot; disabled stops new sessions |

## 4. Stories (from USER-STORY.md)

- **US-4** owner: add MikroTik router (name/IP/credentials).
- **US-6** owner: platform pushes hotspot config (no manual RouterOS).
- **US-15** technician: live router status view.
- **US-16** technician: on-demand connectivity test.

New stories owned by this phase:

- **US-R1** As a technician, I want CRUD for router credentials so that I can replace passwords without a site visit.
  - Accept: `PATCH` updates encrypted creds; API never returns them; probe re-runs after update.
- **US-R2** As a technician, I want to delete a router so that retired hardware is removed cleanly.
  - Accept: `409` if active hotspots exist; otherwise remove (soft or hard per policy).
- **US-H1** As an owner, I want hotspot profiles (speed caps, session limits) so that I can sell tiered access later.
  - Accept: profile CRUD, per-tenant unique name.
- **US-H2** As an owner, I want to enable/disable a hotspot so that I can control when access is sold.
  - Accept: disable stops new sessions only, existing sessions unaffected; server blocks further push if disabled.
- **US-H3** As a technician, I want to see push errors on the hotspot so that I know when provisioning failed.
  - Accept: hotspot enters `error` with message; retry endpoint re-enqueues config.

## 5. API surface (from API-SPECIFICATION)

| Method | Path | Role | Notes |
|---|---|---|---|
| POST | `/api/v1/routers` | owner | triggers initial probe |
| GET | `/api/v1/routers` | owner/staff | list + status filter |
| GET | `/api/v1/routers/{id}` | owner/staff | detail + `last_seen_at`/`last_sync_at` |
| PATCH | `/api/v1/routers/{id}` | owner | update name/ip/port/creds |
| DELETE | `/api/v1/routers/{id}` | owner | 409 if active hotspots |
| POST | `/api/v1/routers/{id}/probe` | owner/staff | enqueue check → `router.status` WS |
| GET | `/api/v1/hotspot-profiles` | owner/staff | list |
| POST | `/api/v1/hotspot-profiles` | owner | create |
| POST | `/api/v1/hotspots` | owner/staff | → `configuring`, worker pushes |
| GET | `/api/v1/hotspots` | owner/staff | filter `router_id`/`status` |
| GET | `/api/v1/hotspots/{id}` | owner/staff | incl. profile + router |
| PATCH | `/api/v1/hotspots/{id}` | owner | update → reconfig |
| POST | `/api/v1/hotspots/{id}/enable`·`/disable` | owner/staff | toggle |
| DELETE | `/api/v1/hotspots/{id}` | owner | 409 if active sessions |
| GET | `/api/v1/hotspots/{id}/reconfig` | owner | retry failed push (US-H3) |

**WebSocket:** `GET /ws` (short-lived ticket). Events this phase: `router.status` `{ router_id, status, last_seen_at }`, `hotspot.status` `{ hotspot_id, status, error? }`. Client subscribes to tenant scope; 30s ping/pong heartbeat.

## 6. Data model (from DATABASE-DESIGN)

- **routers** — `id`, `tenant_id` FK, `name`, `ip_address` inet, `api_port` (default 8728), `username`, `password_enc`, `status` (`unverified|online|offline`), `last_seen_at`, `last_sync_at`.
- **hotspot_profiles** — `id`, `tenant_id` FK, `name` (unique per tenant), `rx_rate`/`tx_rate` (bps), `session_uptime_limit` (s), `keepalive_timeout` (s), `created_by`.
- **hotspots** — `id`, `tenant_id` FK, `router_id` FK, `profile_id` FK, `name`, `mikrotik_id`, `ip_range`, `status` (`configuring|active|disabled|error`), `last_config_at`.
- Composite `(tenant_id, ...)` FKs prevent cross-tenant references (DATABASE-DESIGN §4/§6).
- Encrypted router password at rest; plaintext never leaves the worker push path.

## 7. Implementation tasks

### Backend (Go/Gin)
- [ ] `mikrotik.Client`: RouterOS API connection (login, execute commands), timeouts; injectable/mockable interface for tests.
- [ ] `repositories`: `routers`, `hotspot_profiles`, `hotspots` (tenant-scoped), `config_jobs` queue table or Redis job list.
- [ ] Router handlers: create/list/detail/update/delete, `probe` enqueue.
- [ ] Hotspot + profile handlers per §5; enable/disable gating new sessions.
- [ ] Config builder: translate hotspot + profile rows into RouterOS commands (interface, wallet, server, profile, HTTP enabled) — idempotent command set keyed by `mikrotik_id`.
- [ ] Config push uses a queue worker (Redis) for retry/backoff; `error` state + retry endpoint.
- [ ] `worker`: probe job (every N s update `routers.status`, emit WS), config-push consumer (idempotent, retryable), reconcile.
- [ ] `ws.Hub`: tenant-scoped subscribe, broadcast `router.status`/`hotspot.status`, heartbeat ping/pong.

### Frontend (React)
- [ ] Routers screen: list with live status badge, add/edit/delete, probe button; WS-triggered status flip.
- [ ] Hotspots screen: create/edit/disable/delete, status + error display, retry push.
- [ ] Profiles screen: CRUD with rate/limit inputs.
- [ ] Vite proxy already forwards `/ws` (dev) and nginx proxies it in Docker — verify.

### Data / migrations
- [ ] Schema for routers/hotspot_profiles/hotspots (align `0001`–`0006` or `0008_*` migration) up/down verified.

### MikroTik test surface
- [ ] Mock RouterOS server (testcontainer or stub) for: successful push, auth failure, command error, offline router, retry convergence.
- [ ] Idempotency proof: re-run config push twice → same end state, no duplicates.

### Tests
- [ ] Unit: config-builder output (golden commands), RBAC for router/hotspot routes.
- [ ] Integration: create router → probe → status reflected; create hotspot → push → `active`; push failure → `error` → retry → `active`; disabled hotspot blocks config push.
- [ ] WS: client subscribed to tenant A does not receive tenant B events.

## 8. Definition of Done (Phase 2 exit criteria)

1. Router CRUD + encrypted storage; credentials never appear in API responses or logs (tested).
2. Worker probes on cadence; `router.status` events pushed over WS within 5 s (NFR-PERF-2).
3. Hotspot + profile CRUD works; enable/disable correctly gates new sessions.
4. Config push end-to-end against mock RouterOS: success, failure→`error`, retry→`active`; idempotent.
5. Cross-tenant isolation verified for routers/hotspots (integration tests).
6. WS tenant-scoping verified; heartbeat works.
7. `go vet` clean; frontend typecheck/lint/build clean; compose stack green.

## 9. Out-of-scope reminders

- No voucher/portal endpoints yet (Phase 3).
- No session sync/disconnect/usage (Phase 4).
- No realtime analytics (Phase 5).

Reference: SRS §6–§7, API-SPECIFICATION Router/Hotspot/WebSocket sections, DATABASE-DESIGN §2 routers/hotspots/hotspot_profiles, US-4/6/15/16/R1/R2/H1–H3, mkdocs and SSH/API port assumptions per BRD §9.