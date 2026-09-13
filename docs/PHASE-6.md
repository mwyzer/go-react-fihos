# Phase 6 — Payments & Self-Service (Post-MVP)

Extends PHASES.md (post-MVP, release-candidate-era additions). Scope from PRD §9.1 and SRS FR-BILLING-4. Ships after Phase 5 is stable.

- **Goal:** unassisted revenue — guests buy vouchers online, venues collect via integrated payments, platform takes a small transaction fee — plus usage anomaly detection with a summarizable alert backlog and scheduled rate boosts.
- **Milestone:** post-MVP — payments live + self-service redemption, anomaly alert backlog + summary, rate boost windows.

## 1. Scope (in)

1. Online payments for voucher purchase (card / mobile money); payment records linked to vouchers (FR-BILLING-4).
2. Payment initiation + webhook verification endpoints (API-SPECIFICATION `POST /payments`, `/payments/webhook/*`).
3. Self-service account: guest can see own voucher balance/expiry (`customer self-service`, PRD §9.1#5).
4. Automatic voucher issuance on successful payment; double-spend safety via idempotent webhooks.
5. Voucher sales/threshold alerts (PRD §9.1#3) — notify owner on low stock, big sales, or failed payment spikes.
6. Full RBAC UI: custom roles per tenant (PRD §9.1#4) — replaces the fixed role set where enabled.
7. Platform transaction-fee ledger (BRD §11) for reconciliation.
8. Usage anomaly detection: detect + flag abnormal usage, keep an alert **backlog** (`anomaly_alerts`) and an aggregated **summary** (`anomaly_summaries`) so alerts can be concluded at a glance without reading every row.
9. Scheduled **rate boost** windows (per-hotspot, multiplier rule): temporarily widen bandwidth caps on chosen days/hours (e.g. school-exam days), auto-applied and auto-reverted by the worker.

## 2. Scope (out)

- Multiple router vendor support beyond MikroTik (PRD §9.1#6 — kept as a separate initiative; see §9).
- Hardware/SFP provisioning and firmware management.
- Social login / identity federation for guests.
- ML-based anomaly detection (isolation forest etc.) — v1 stays rule-based + statistical baseline in the worker; an ML pass is only reconsidered once ≥2 months of labeled data exists.
- National vs international traffic differentiation (explicitly out of product scope).

## 3. Functional requirements mapping

| ID | Requirement | This phase |
|---|---|---|
| FR-BILLING-4 | Payment gateway records linked to vouchers | Payment + webhook + voucher issuance |
| FR-VOUCHER-2 | Voucher params (price) | Price now drives payable amount |
| NFR-PERF-1/2 | API latency + WS freshness | Carry-forward regression gates |
| SEC-2/4/5 | Central auth, rate limiting, HTTPS-only | Webhook auth + HMAC/secret signing; stricter rate limits on payment endpoints |
| FR-SESSION-5 | Every ended session persisted as a usage record | Usage records feed anomaly detection + rate-boost accounting |
| FR-ANALYTICS-1/2 | Dashboard KPIs + trends | Anomaly summary digests follow the same pattern |
| SEC-7 | Audit log for privileged actions | Rate-window create/update/delete audited |

*Note: rate boost has no SRS row yet; it extends FR-HOTSPOT-3 (config push) by making the pushed profile rates time-dependent.*

## 4. Stories (from USER-STORY.md)

- **US-20** guest: buy a voucher online (self-service).

New stories owned by this phase:

- **US-PA1** As a guest, I want to pay for a voucher with card/mobile money so that I never need to find staff.
  - Accept: choose voucher (price/package) → pay → voucher issued automatically; payment state tracked (`pending`/`succeeded`/`failed`/`refunded`).
- **US-PA2** As a guest, I want my voucher history and balances so that I can check what I still have.
  - Accept: self-service view by voucher code or email lookup; shows status + expiry.
- **US-PA3** As an owner, I want the platform to take its fee out of ticket sales so that my books are clean.
  - Accept: fee ledger per transaction (amount, fee, net); export for owner reconciliation.
- **US-PA4** As an owner, I want low-stock alerts so that I can reprint before running dry.
  - Accept: threshold configurable per batch/Hotspot; alert via UI + email/webhook.

### Usage anomaly detection
- **US-ANM1** As an owner, I want abnormal usage flagged automatically so that I can catch shared vouchers or blown traffic caps.
  - Accept: 6 anomaly types detected; alert stores observed value + baseline + severity; alert appears in backlog within the worker cadence.
- **US-ANM2** As an owner, I want a **summary** of alerts so that I can conclude what's wrong without reading every row.
  - Accept: rollup per hotspot × type × day/week/month (open counts, first/last seen, severity mix).
- **US-ANM3** As an owner, I want to acknowledge/resolve alerts so that my team tracks what still needs action.
  - Accept: `ack`/`resolve` transition only from valid state; resolved history retained for the retention window.
- **US-ANM4** As an owner, I want repeated identical alerts grouped so that I'm not flooded.
  - Accept: same fingerprint within the dedupe window increments a counter instead of creating new rows.
- **US-ANM5** As an owner, I want scheduled boosts not flagged as anomalies so that exam-day traffic isn't a false alarm.
  - Accept: sessions/usage in a `rate_windows` period carry `rate_window_id`; spike baseline excludes/tags those periods (expected = normal × multiplier).

### Scheduled rate boost
- **US-RB1** As an owner, I want to widen a hotspot's bandwidth on chosen days/hours (e.g. school-exam days) so that guests get a more comfortable tunnel then.
  - Accept: create a rate window (one-off or repeating weekly) with a multiplier; worker applies the boost at `effective_from` and reverts at `effective_until`.
- **US-RB2** As an owner, I want the boost to apply to active sessions so that a burst mid-exam is still upgraded.
  - Accept: RouterOS profile rates updated live; no session kick on window boundaries.
- **US-RB3** As an owner, I want a failed boost to be visible and retried, and a guaranteed revert so that caps always come back to normal.
  - Accept: hotspot `error` + retry if push fails at window start; revert enqueued unconditionally after `effective_until`.

## 5. API surface (additions to API-SPECIFICATION)

| Method | Path | Auth | Notes |
|---|---|---|---|
| POST | `/api/v1/payments` | public (guest) | `{ tenant, package_id, payment_method, return_url }` → `payment_id`, pay URL/instructions |
| POST | `/api/v1/payments/webhook/*` | signed | provider callbacks; idempotent (by provider event id) |
| GET | `/api/v1/payments/{id}` | guest/owner | state + link to voucher when paid |
| GET | `/api/v1/me/vouchers` | public (by email/code) | self-service list |
| POST | `/api/v1/alerts` | owner | configure low-stock/threshold alerts |
| GET | `/api/v1/billing/fees` | owner | transaction-fee ledger + export |
| GET/POST | `/api/v1/roles` | admin | custom RBAC role CRUD (fixed roles unchanged by default) |

**Anomaly detection:**

| Method | Path | Auth | Notes |
|---|---|---|---|
| GET | `/api/v1/anomalies` | owner/staff | backlog; filters `status`, `severity`, `type`, `hotspot_id`, `from`/`to` |
| GET | `/api/v1/anomalies/summary` | owner/staff | rollup per `granularity` (day/week/month) × hotspot × type |
| PATCH | `/api/v1/anomalies/{id}/ack` | owner/staff | → `acknowledged` (from `open`/`resolved`) |
| PATCH | `/api/v1/anomalies/{id}/resolve` | owner | → `resolved` (from `open`/`acknowledged`) |

**Rate boost:**

| Method | Path | Auth | Notes |
|---|---|---|---|
| GET | `/api/v1/rate-windows` | owner/staff | active/scheduled windows per hotspot |
| POST | `/api/v1/rate-windows` | owner | body `{ hotspot_id, boost_multiplier, effective_from, effective_until, repeat?, note? }` (audited) |
| PATCH | `/api/v1/rate-windows/{id}` | owner | update schedule/multiplier (audited) |
| DELETE | `/api/v1/rate-windows/{id}` | owner | cancel; treats boundary as revert point (audited) |

## 6. Data model additions

- **payments** — `id`, `tenant_id`, `voucher_id` FK (issued on success), `external_ref` (provider), `amount`, `fee`, `net_amount`, `status` (`pending|succeeded|failed|refunded`), `payment_method`, timestamps; unique `external_ref` for idempotency.
- **alerts** — `id`, `tenant_id`, `type`, `threshold`, `channel`, `enabled`.
- **tenant_roles** (custom RBAC) — `id`, `tenant_id`, `name`, `permissions` (jsonb) + `users.role` extended to allow tenant-defined roles via FK; fixed `admin/owner/staff` remain the default.
- Fee ledger derived from `payments` (fee + net) — reportable per tenant/period.

**Anomaly detection:**

- **anomaly_alerts** (raw backlog) — `id`, `tenant_id`, `hotspot_id`, `router_id`, `session_id?`, `type` (`excess_traffic|voucher_shared|traffic_spike|concurrency_gap|over_limit_session|mac_hop`), `severity` (`info|warning|critical`), `status` (`open|acknowledged|resolved|dismissed`), `value`, `baseline`, `fingerprint`, `occurrences`, `rate_window_id?`, `acked_at`/`resolved_at`/`created_at`. Index `(tenant_id, status, severity, created_at)`; dedupe key = `fingerprint`.
- **anomaly_summaries** (digest, worker-maintained) — `(tenant_id, hotspot_id, type, bucket_start, severity)` → `open_count`, `total_count`, `resolved_count`, `value_sum`, `first_seen`, `last_seen`. Mirrors the Phase 5 analytics digest pattern so summaries stay cheap with a long backlog.
- Retention: backlog kept 90 days on the hot path, older rows archived in the same table set (housekeeping worker).

**Rate boost:**

- **rate_windows** — `id`, `tenant_id`, `hotspot_id` FK, `boost_multiplier` (>1, e.g. 2.0), `effective_from`, `effective_until`, `repeat` (jsonb: weekly day-of-week + time pattern), `note`, `created_by`, `created_at`; audited on create/update/delete.
- Effective cap = active `hotspot_profiles.rx_rate/tx_rate` × `boost_multiplier`, computed at push time (no stored absolute values → stays proportional to profile changes).
- **Sessions/usage link:** `sessions.rate_window_id` / `usage_logs.rate_window_id?` (nullable FK) so anomaly baselines can scale expected traffic by the multiplier during windows.

## 7. Implementation tasks

### Backend (Go/Gin)
- [ ] Payment provider abstraction (interface): initiate, verify webhook signature, refund; one concrete adapter first (card or mobile money per market).
- [ ] Idempotent webhook consumer: upsert on `external_ref`; transition `pending → succeeded` and issue voucher atomically (no double issuing).
- [ ] Payment handlers + self-service lookup (`me/vouchers` by code/email).
- [ ] Fee computation + `billing/fees` report (BRD §11 fee).
- [ ] Alerts service + worker check on batch thresholds (extend Phase 5 digest job).
- [ ] Custom RBAC: role model, permission checks generalized in auth middleware (default = SRS §3 matrix; tenants may add roles).
- [ ] Stricter auth on webhook endpoint (signed/HMAC), rate limiting additions (SEC-4).

### Backend — Usage anomaly detection (Go/Gin)
- [ ] `anomaly.Service`: Level-1 rule evaluation per anomaly type (explicit thresholds) at session close.
- [ ] Level-2 statistical baseline: rolling median/IQR per `(hotspot, hour-of-week)` computed from `usage_logs`; flag on z-score breach.
- [ ] Alert lifecycle helpers: open → ack → resolve → dismiss; dedupe window (1 h) by `fingerprint` (increment `occurrences`, slide `last_seen`).
- [ ] Auto-resolve open alerts that don't recur within the follow-up window; housekeeping/archive job (>90 days).
- [ ] `rate_window_id` context on sessions/usage so spike baselines scale by `boost_multiplier` during windows (no false flags).
- [ ] Handlers: `GET /anomalies`, `GET /anomalies/summary`, `PATCH .../ack`, `PATCH .../resolve`; tenant-scoped.

### Backend — Scheduled rate boost (Go/Gin)
- [ ] `rate_windows` repository + handlers (GET/POST/PATCH/DELETE, audited).
- [ ] Worker scheduler job: evaluate current windows per hotspot (one-off + weekly repeat); at boundary enqueue config push (boost) / revert (normal) via the Phase 2 push queue.
- [ ] Config builder: compute effective `rx_rate/tx_rate = profile × multiplier`; emit same idempotent RouterOS command set; apply live to active sessions (profile rate change).
- [ ] Guaranteed revert: after `effective_until` push normal caps; if window start push failed, hotspot stays `error` + retry until converged; delete mid-window reverts immediately.

### Frontend (React)
- [ ] Guest payment flow: package selection → pay → success/failure states; self-service voucher page.
- [ ] Owner: payment history, fee report, alert configuration; custom-role management UI (admin).
- [ ] Keep portal bundle consistent with new package/payment branding.
- [ ] Anomalies tab: open backlog table, summary cards/charts per hotspot × type, ack/resolve actions, severity + quiet-window filters (WS optional).
- [ ] Hotspot page: rate-window form (one-off / repeating), active-window indicator, boost note (e.g. "ujian sekolah"), cancel action.

### Compliance & ops
- [ ] Provider sandbox + production credentials outside the repo (`.env`, secret store); webhook secret rotation support.
- [ ] Payment reconciliation job: mark stale `pending` as `failed`, reconcile provider report vs. local (periodic).
- [ ] Observable: payment events in structured logs; fee ledger integrity in CI.

### Tests
- [ ] Unit: idempotent webhook double-delivery, state machine (`pending→succeeded/failed/refunded`), fee math, custom-role permission checks.
- [ ] Integration: initiate → webhook success → voucher issued once; refund → voucher revocable; failed payment never issues a voucher; self-service lookup tenant-scoped.
- [ ] E2E: guest pays (provider mock) → voucher issued → redeems on portal → owner sees payment + fee in reports.

### Anomaly tests
- [ ] Unit: alert state machine (open/ack/resolve/dismiss), dedupe window (same fingerprint + counter, no flood), z-score/IQR math, expiry + archive job.
- [ ] Integration: rule + baseline detection produce correct alerts; `anomaly_summaries` rollups correct for seeded data; cross-tenant isolation on backlog/summary; ack/resolve transitions guarded.
- [ ] Anti-false-positive: traffic during an active `rate_windows` period is not flagged as `traffic_spike` (baseline scales by multiplier).

### Rate-boost tests
- [ ] Unit: window boundary math (one-off + weekly repeat), multiplier cap computation, revert scheduling.
- [ ] Integration: window start → push applied (active sessions upgraded); window end → revert pushed; push failure at start → `error` + retry → converged; delete mid-window reverts.
- [ ] E2E: owner schedules a boost (one-off day) → worker applies → UI shows active window → auto-reverts after expiry.

## 8. Definition of Done (Phase 6 exit criteria)

1. End-to-end paid voucher: initiate → pay (sandbox) → voucher issued atomically → redeem → fee recorded; no double-issue on webhook replay.
2. Webhook security verified (HMAC/signing, secret rotation); sandbox tests green.
3. Self-service voucher lookup works with correct tenant isolation.
4. Alerts fire at configured thresholds.
5. Custom RBAC roles function; default matrix unchanged (backward compatible).
6. Fee + net amounts reconcile against provider report; reports exportable.
7. Full test suite green (unit/integration/E2E); security regressions (SEC-4/5) pass.
8. Anomaly backlog + summary: alerts detected, deduplicated, ackable/resolvable, and **summarizable** (summary derived from digests, not raw scans); no alert flood (dedupe proven); cross-tenant isolation verified.
9. Rate boost end-to-end: per-hotspot window applies the multiplier at start and reliably reverts at end (incl. failure/retry); active sessions upgraded without kick; boosts not flagged as anomalies.
10. Ops: archive/housekeeping >90 days behaves; `/readyz` + digests run on the worker cadence.

## 9. Out-of-scope / parked

- Second router vendor: design the `mikrotik.RouterOS`-style interface to allow a future `vendor.Client` interface — no implementation now.
- Social login, advanced analytics pipelines, self-care app.
- ML-based anomaly pass (isolation forest etc.) and national/international traffic split.

Reference: PRD §9.1, SRS §2 (FR-BILLING-4)/§13, API-SPECIFICATION Payments + WebSocket, BRD §11 fee model, US-20/PA1–PA4/ANM1–ANM5/RB1–RB3.