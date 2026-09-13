# Document Plan

## 1. Phase 1 tasks (MVP)

| # | Task | Docs | Flags |
|---|---|---|---|
| P1 | Repo scaffold: backend (Go/Gin), frontend (React+Vite), Docker compose, CI skeleton | PROJECT-STRUCTURE, SYSTEM-ARCHITECTURE | Foundation |
| P2 | DB migrations + schema (tenants, users, routers, hotspots, profiles, batches, vouchers, sessions, usage, audit) | DATABASE-DESIGN | Foundation |
| P3 | Auth core: login, refresh, guard middleware, RBAC helper | SRS §4, §3 · API: Authentication | M1 |
| P4 | Tenant + user CRUD (admin/owner) with suspension | SRS §5 · API: Tenant | M1 |
| P5 | Router CRUD + probe + status polling in worker + WebSocket events | SRS §6 · API: Router | M2 |
| P6 | Hotspot CRUD + profile CRUD + config push to MikroTik | SRS §7 · API: Hotspot | M2 |
| P7 | Voucher batch generation, print PDF, search, revoke | SRS §8 · API: Voucher | M2 |
| P8 | Captive portal (branding + redeem flow) | SRS §9 · API: Portal | M3 |
| P9 | Session sync (worker → DB), listing, disconnect; usage logs | SRS §10 · API: Session | M3 |
| P10 | Dashboard + analytics basics; CSV export | SRS §11 · API: Analytics/Billing | M4 |
| P11 | Testing strategy pass + hardening (rate limits, audit, error envelope) | SRS §12, §13 | M4 |

Milestones: **M1** — auth+tenancy solid · **M2** — routers+hotspots+vouchers end-to-end · **M3** — portal+live sessions · **M4** — reporting+hardening release candidate.

## 2. Sprint breakdown (2-week sprints, 1 dev)

- **Sprint 1:** P1, P2 — scaffold, migrations, compose, CI green.
- **Sprint 2:** P3, P4 — auth + tenant/user management.
- **Sprint 3:** P5 — routers + probe + WS events + worker skeleton.
- **Sprint 4:** P6 — hotspots + profiles + MikroTik config push.
- **Sprint 5:** P7 — vouchers (generate, print, manage).
- **Sprint 6:** P8 — captive portal + redemption.
- **Sprint 7:** P9 — session sync + live dashboard + disconnect + usage logs.
- **Sprint 8:** P10, P11 — analytics, exports, hardening, release.

Buffer: +1 sprint for the MikroTik integration unknowns (device lab/testing).

## 3. Definition of Done

A task/item is **done** when:

1. Code committed with a clear message and no unrelated changes.
2. Backend: unit tests for services/middleware pass; `go vet`/lint clean.
3. Frontend: type-check + lint + build pass; key screens render against local API.
4. Migration forward + rollback verified against a clean database.
5. API matches API-SPECIFICATION (endpoints, envelope, status codes) and is covered by integration tests.
6. DB access respects tenant scoping — cross-tenant test present.
7. Worker tasks idempotent / retryable; failure visible on the affected resource.
8. Docs touched by the change updated (PRD/SRS/API/DB as needed).
9. Manual smoke: happy path via API + UI; error paths return proper envelope.
10. No secrets committed; `.env*` and credentials excluded via `.gitignore`.

## 4. Testing strategy

**Layers:**
- **Unit (Go):** auth/RBAC, voucher code logic, redemption state machine, billing calc, MikroTik command builder (mock RouterOS).
- **Integration (Go, against testcontainers Postgres+Redis):** repository + service flows, multi-tenant isolation cases, voucher batch transaction, reversions on router failure.
- **Contract:** OpenAPI-defined routes asserted by API tests (request/response shapes).
- **Frontend (Vitest + Testing Library):** components & hooks; voucher print flow; WebSocket event handling with a mocked hub.
- **E2E (Playwright):** login → create tenant → add router (mock) → hotspot → voucher → redeem → session in dashboard.

**Key risk tests:**
- Suspended tenant receives 403 everywhere.
- Voucher can never be redeemed twice.
- Router credentials never leak via API responses/logs.
- Config-push retry converges (idempotency).
- WS event ordering on reconnect (cursor fallback).

**Coverage target:** ≥ 70% backend line coverage; 100% on voucher/usage state machines.

## 5. CI/CD

**CI (per push / PR):**
- Backend: `golangci-lint`, `go vet`, `go test ./...` (unit + integration with testcontainers).
- Frontend: `npm ci` → `npm run lint` → `npm run typecheck` → `npm run test` → `npm run build`.
- Migrations: `golang-migrate` up/down run against throwaway Postgres.
- Docker images built and `docker-compose up` smoke-tested.

**CD (on merge to main / tag):**
- Backend+worker image tagged and pushed (semver or commit SHA).
- Frontend static build + portal bundle pushed to registry/CDN.
- Migrations applied with `golang-migrate` (up + down verification) before API rollout.
- Rolling deploy behind the load balancer; health checks gate traffic.
- Post-deploy: run smoke E2E (Playwright) against staging, then production.

**Environments:** `dev` (compose on dev machine) → `staging` (CI e2e gate) → `production` (tagged release, DB backup + migrations).
**Observability:** structured JSON logs, `/healthz` `/readyz` liveness/readiness, metrics endpoint; alert on router-sync lag and failed job queue depth.