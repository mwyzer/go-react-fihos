# Phase 4 — Live Sessions & Usage

Extends PHASES.md Phase 4. Sprint scope: DOCUMENT-PLAN **P9** (session sync worker → DB, listing, disconnect, usage logs).

- **Goal:** realtime session monitoring and accurate usage data so billing has a trustworthy source of truth.
- **Milestone:** M3 — live sessions visible in the dashboard and disconnect works.

## 1. Scope (in)

1. Worker polls routers for active hotspot sessions (username, MAC, IP, uptime, traffic) on a cadence and upserts them to `sessions`.
2. Live session list per tenant/hotspot/router with a `state`: `active`/`closing`/`closed`; WebSocket push of `session.active` / `session.closed`.
3. Force-disconnect of a single session (router command + persisted close).
4. Every ended session persisted as a `usage_logs` record (start, end, duration, bytes up/down, voucher ref) — the billing source of truth.
5. Search/pagination + filtering on sessions and usage logs; date-range queries on usage.
6. Reconcile: worker detects sessions ended between polls (no orphans).

## 2. Scope (out)

- Dashboard analytics/aggregates and CSV reporting (Phase 5).
- Payments, portal self-service (post-MVP).

## 3. Functional requirements (from SRS)

| ID | Requirement |
|---|---|
| FR-SESSION-1 | Track active sessions per hotspot: username, MAC, IP, uptime, traffic |
| FR-SESSION-2 | Sync session data from routers on a polling cadence |
| FR-SESSION-3 | Live session updates pushed to the dashboard via WebSocket |
| FR-SESSION-4 | Authorized users disconnect individual sessions |
| FR-SESSION-5 | Every ended session persisted as a usage record (start, end, bytes up/down, duration) |

## 4. Stories (from USER-STORY.md)

- **US-13** staff: see live sessions (user, MAC/IP, uptime, traffic), updates within 5 s.
- **US-14** staff: force-disconnect a specific session.

New stories owned by this phase:

- **US-S1** As a technician, I want the dashboard to reflect reality even if a router was unreachable, so that I trust session counts.
  - Accept: sessions missing after successive poll failures are reconciled to `closed` with `ended_at` = last_seen; worker reconnects and resumes.
- **US-S2** As an owner, I want historical usage per hotspot so that I can reconcile with voucher sales later.
  - Accept: usage logs queryable by hotspot + date range; sorted by `ended_at` desc.
- **US-S3** As an owner, I want a disconnect to leave an accurate record so that billing stays clean.
  - Accept: forced close writes a complete usage record (not an orphan); `closing` state during the transition.

## 5. API surface (from API-SPECIFICATION)

| Method | Path | Role | Notes |
|---|---|---|---|
| GET | `/api/v1/sessions` | owner/staff | filters `hotspot_id`, `router_id`, `state`; live from last sync |
| GET | `/api/v1/sessions/{id}` | owner/staff | detail + live counters |
| POST | `/api/v1/sessions/{id}/disconnect` | owner/staff | router disconnect → `session.closed` event |
| GET | `/api/v1/usage-logs` | owner/staff | `hotspot_id`, `from`/`to`, pagination; sort `ended_at` desc |
| GET | `/api/v1/usage-logs/{id}` | owner | single record (billing input) |

**WebSocket events added this phase:** `session.active` `{ session: {...} }`, `session.closed` `{ session_id, usage: {...} }` (existing hub + tenant scoping from Phase 2). Heartbeat ping/pong every 30 s; cursor fallback on reconnect for event ordering.

## 6. Data model (from DATABASE-DESIGN)

- **sessions** — `id`, `tenant_id`, `hotspot_id` FK, `router_id` FK, `mac_address`, `ip_address`, `username`, `voucher_id` FK (nullable, set at redemption in Phase 3), `started_at`, `last_seen_at`, `state` (`active|closing|closed`).
- **usage_logs** — `id`, `tenant_id`, `session_id` FK, `voucher_id` FK (nullable), `started_at`/`ended_at`, `duration_sec`, `bytes_rx`/`bytes_tx`, `amount` (nullable), `billing_status`.
- Composite tenant FKs enforce isolation (§4/§6). Indexes on `sessions(state)`, `sessions(last_seen_at)`, `usage_logs` date range for reporting (§5).
- Closing a session writes usage in the same transaction as the state flip → no orphans (FR-SESSION-5).

## 7. Implementation tasks

### Backend (Go/Gin)
- [ ] Extend `mikrotik.Client`: list active hotspot sessions, query per-session traffic counters, remote-disconnect command (idempotent).
- [ ] `session.Service`: diff-last-sync upsert (create new / update `last_seen_at` + counters / close missing), reconciliation rules + row-lock guards against double-close.
- [ ] `usage.Service`: write usage_log row on close (atomic with state transition), guard against duplicate closes (unique `session_id`).
- [ ] Session handlers: list/detail/`disconnect` (enqueue router command → emit `session.closed`).
- [ ] Usage handlers: list + date-range filter + single detail; tenant-scoped.
- [ ] Worker: session poll job on cadence (extend Phase 2 probe loop); reconcile short-lived/churned sessions; handle router-offline gaps (US-S1).
- [ ] WS hub: broadcast session events; reconnect cursor fallback.

### Frontend (React)
- [ ] Sessions screen: live table (WS updates), filters, disclaimers on sync staleness; force-disconnect action with confirm.
- [ ] Usage screen: paginated log list + date-range picker; detail view.
- [ ] Keep Vite/nginx `/ws` proxy working for the dashboard.

### Router/mock testing
- [ ] Mock RouterOS: session listing, counter deltas, disconnect returns concurrent-close safety (double-disconnect no effect).
- [ ] Worker restart test: sessions resumed/closed correctly after downtime (no duplicates, no orphans).

### Tests
- [ ] Unit: diff/reconcile math (counters delta, duration), close race (two goroutines close once), state guards.
- [ ] Integration: poll → upsert; session list reflects; disconnect → `session.closed` + usage row complete; router offline → sessions reconciled to `closed`; usage date-range + pagination correct.
- [ ] WS ordering test on reconnect (cursor fallback).

## 8. Definition of Done (Phase 4 exit criteria)

1. Worker sync cycle updates sessions and usage automatically; dashboard reflects within 5 s (NFR-PERF-2 / NFR-REAL-1).
2. Force-disconnect works from UI and writes a complete usage record (no orphans).
3. Sessions that vanish between polls are reconciled correctly; worker restart is safe (idempotent).
4. Usage logs queryable per hotspot/date-range with pagination; tenant-scoped.
5. 100% of ended sessions produce a matching usage record (billing-accuracy KPI).
6. `go vet` + frontend typecheck/lint/build clean; compose stack green.

## 9. Out-of-scope reminders

- Analytics dashboards/CSV (Phase 5) — simple session list is in scope.
- Amounts/billing_status logic beyond recording (Phase 5 reports + reconciliation may touch `amount`).
- Self-service/payments.

Reference: SRS §10, API-SPECIFICATION Session + WebSocket sections, DATABASE-DESIGN §2 sessions/usage_logs, US-13/14/S1–S3, IDEA "multi-site live view".