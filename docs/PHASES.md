# FIHOS — Development Phases

Phased breakdown of the MVP (DOCUMENT-PLAN tasks P1–P11). Each phase is independently shippable; the platform is only monetizable once Phase 3 lands (vouchers + MikroTik). Phases map to the milestones in DOCUMENT-PLAN.

## Phase 0 — Foundations

- **Goal:** runnable skeleton in Docker, shared DB schema, green CI.
- **Tasks:** P1 (repo scaffold: Go backend, React frontend, Docker compose, CI skeleton), P2 (DB migrations + schema: tenants, users, routers, hotspots, profiles, batches, vouchers, sessions, usage, audit).
- **Key docs:** PROJECT-STRUCTURE, SYSTEM-ARCHITECTURE, DATABASE-DESIGN.
- **Deliverables:** `docker compose up` brings up web/api/worker/migrate/postgres/redis; all 6 migrations apply cleanly; `/healthz` + `/readyz` green; CI runs lint/typecheck/build/tests/migrations.
- **Exit criteria:** fresh `down -v` + `up --build` is reproducible; `0001`–`0006` up **and** down verified; README quick-start accurate.

## Phase 1 — Auth & Tenancy

- **Goal:** secure login, refresh, RBAC, and multi-tenant user/admin management.
- **Tasks:** P3 (auth core: login, refresh, guard middleware, RBAC helper), P4 (tenant + user CRUD for admin/owner with suspension).
- **Key docs:** SRS §3–§5; API-SPECIFICATION §Authentication, §Tenant.
- **Deliverables:** `/api/v1/auth/*` (login/refresh/logout), auth middleware on all protected routes, seed owner/admin accounts; tenant & user CRUD with suspension; suspended tenant returns 403 everywhere.
- **Exit criteria:** integration tests for login/refresh/suspension pass; cross-tenant access test present; no credentials in API responses or logs.

## Phase 2 — Routers, Hotspots & MikroTik

- **Goal:** central router inventory + automated config push so hotspot setup "just happens".
- **Tasks:** P5 (router CRUD + probe + status polling in worker + WebSocket events), P6 (hotspot CRUD + profile CRUD + config push to MikroTik).
- **Key docs:** SRS §6–§7; API-SPECIFICATION §Router, §Hotspot.
- **Deliverables:** Router model + credentials vault; worker probes routers on a schedule and publishes WS events; hotspot/profile CRUD; config push builds RouterOS commands (profiles, wallet, banner) and applies them; push is idempotent and retryable.
- **Exit criteria:** mock RouterOS test covers push success/failure/revert; WS event ordering on reconnect (cursor fallback); router credentials never leak.

## Phase 3 — Vouchers & Captive Portal

- **Goal:** revenue — generate, print, and redeem vouchers end-to-end.
- **Tasks:** P7 (voucher batch generation, print PDF, search, revoke), P8 (captive portal: branding + redeem flow).
- **Key docs:** SRS §8–§9; API-SPECIFICATION §Voucher, §Portal; PRD "success looks like" #1–#2.
- **Deliverables:** Batch generator (code format, profile-bound, expiry), printable voucher sheets (PDF), revoke/search; portal accepts redemption, activates session, logs usage; voucher can never be redeemed twice.
- **Exit criteria:** 100% test coverage on voucher/redemption state machine; end-to-end flow: login → voucher batch → print → redeem → session live in dashboard.

## Phase 4 — Live Sessions & Usage

- **Goal:** realtime monitoring and accurate usage-based billing data.
- **Tasks:** P9 (session sync from worker → DB, listing, disconnect; usage logs).
- **Key docs:** SRS §10; API-SPECIFICATION §Session; IDEA "multi-site live view".
- **Deliverables:** Worker syncs active sessions/usage into DB on a schedule; session list with live status (WS); admin disconnect action; usage log rows per session.
- **Exit criteria:** disconnect converges/resyncs correctly on worker restart (idempotency); session listing paginated and tenant-scoped.

## Phase 5 — Analytics, Reporting & Hardening

- **Goal:** release candidate — owner-grade reporting and a hardened platform.
- **Tasks:** P10 (dashboard + analytics basics; CSV export), P11 (testing strategy pass + hardening: rate limits, audit trail, error envelope).
- **Key docs:** SRS §11–§13; API-SPECIFICATION §Analytics/Billing; DOCUMENT-PLAN §4 testing strategy.
- **Deliverables:** dashboard KPIs (sales, usage, active sessions), CSV export, audit log for sensitive actions, rate limiting on auth/portal, consistent error envelope.
- **Exit criteria:** ≥70% backend line coverage; 100% on voucher/usage state machines; Playwright E2E smoke green; Definition of Done (DOCUMENT-PLAN §3) fully met.

---

**Sequencing notes**

- Phase 0 must precede everything (CI can't gate an unrunnable repo).
- Phase 3 is the first user-facing value; Phase 2 must ship before it (portals/vouchers need router push + profiles).
- Phase 4 depends on Phase 3 sessions existing to sync.
- Phase 5 is a hardening pass — plan a buffer sprint for MikroTik device-lab unknowns (DOCUMENT-PLAN §2).