# Phase 5 — Analytics, Reporting & Hardening

Extends PHASES.md Phase 5. Sprint scope: DOCUMENT-PLAN **P10** (dashboard + analytics basics; CSV export) + **P11** (testing strategy pass + hardening: rate limits, audit trail, error envelope).

- **Goal:** a hardened release candidate with owner-grade reporting and trustworthy KPIs.
- **Milestone:** M4 — reporting + hardening release candidate.

## 1. Scope (in)

1. Dashboard KPIs: active sessions, sessions today, vouchers sold (issued/redeemed/expired/revoked), traffic totals (rx/tx), platform uptime.
2. Trends per day/week/month for sessions, revenue, traffic, vouchers; per-router and per-hotspot breakdowns.
3. Cross-tenant admin space aggregates (top tenants by sessions/revenue).
4. Billing summary + CSV reports derived exclusively from persisted usage records (FR-BILLING-1).
5. Sales report per voucher batch (issued/sold/redeemed metrics).
6. Hardening pass: rate limiting on login/redeem/voucher generation (SEC-4), audit trail for all privileged actions (SEC-7), consistent error envelope everywhere (SRS §12), input validation + parameterized SQL audit (SEC-6), HTTPS/secure headers/CORS (SEC-5).

## 2. Scope (out)

- Online payments and self-service top-ups (post-MVP).
- Advanced analytics (cohorts, heatmaps, export pipelines) and PDF reports (post-MVP).
- Custom RBAC roles per tenant (fixed roles only).

## 3. Functional requirements (from SRS)

| ID | Requirement |
|---|---|
| FR-BILLING-1 | Usage records are the source of truth for billing |
| FR-BILLING-2 | Report sales and usage per tenant/hotspot periods |
| FR-BILLING-3 | Billing amounts derived from voucher price/type and session usage |
| FR-ANALYTICS-1 | Dashboards show active sessions, sessions today, vouchers sold, traffic totals |
| FR-ANALYTICS-2 | Trends per day/week/month |
| FR-ANALYTICS-3 | Reports filterable by tenant, router, hotspot (admin space) |
| SEC-4 | Rate limiting (Redis) on login, redeem, voucher generation |
| SEC-7 | Audit log for privileged actions (tenant changes, router config, voucher revocation) |

## 4. Stories (from USER-STORY.md)

- **US-3** admin: cross-tenant overview (tenants, routers, revenue).
- **US-10** owner: voucher sales/redemption view.
- **US-11** owner: month-end usage + sales report (CSV).
- **US-18** auditor: privileged actions logged.
- **US-19** operator: rate limiting on login and redeem.

New stories owned by this phase:

- **US-AN1** As an owner, I want KPI cards that refresh live so that I can eyeball the business at a glance.
  - Accept: active sessions/sessions today/vouchers today/traffic within 5 s freshness; per-period trends render correctly.
- **US-AN2** As an owner, I want usage vs. sales numbers that reconcile so that my books match the platform.
  - Accept: sales report + usage report for the same period reconcile to the same voucher/session sets.
- **US-AUD1** As a platform admin, I want every sensitive change traceable so that incidents can be investigated.
  - Accept: audit queries with who/what/when/entity; admin-only access.

## 5. API surface (from API-SPECIFICATION)

| Method | Path | Role | Notes |
|---|---|---|---|
| GET | `/api/v1/analytics/dashboard` | owner/staff | RT KPI cards |
| GET | `/api/v1/analytics/trends` | owner/staff | `metric`, `granularity` (day/week/month), `from`, `to` |
| GET | `/api/v1/analytics/by-router`·`by-hotspot` | owner/staff | breakdowns |
| GET | `/api/v1/billing/summary` | owner | `from`, `to`, `hotspot_id?` → sessions/bytes/amount |
| GET | `/api/v1/billing/reports` | owner | CSV export (line-item usage per period) |
| GET | `/api/v1/voucher-batches/{id}/sales` | admin | issued/sold/redeemed per batch |
| GET | `/api/v1/analytics/tenants` | admin | cross-tenant snapshot |
| GET | `/api/v1/audit-logs` | admin | queryable audit trail |

All tenant-scoped; admin endpoints aggregate tenant-scoped data only in admin space.

## 6. Data & implementation notes

**Digest strategy (SRS §11):** prefer worker-maintained digest tables keyed per tenant for trend/aggregate reads; fall back to computed queries on `usage_logs`/`sessions`/`vouchers`. Choose digest for anything crossing the day-by-day growth curve; keep raw tables as the canonical store (billing source of truth).

- KPIs source: `sessions` (active), `usage_logs` (traffic/duration), `vouchers` (sales), `voucher_batches` + `price` (revenue).
- Amounts: from voucher `price` (sold) and, where applicable, tariff math on usage (FR-BILLING-3).
- CSV export: consistent column set (period, tenant, hotspot, session, started/ended, duration, bytes_rx/tx, amount, voucher code truncated).

## 7. Implementation tasks

### Backend (Go/Gin)
- [ ] Analytics handlers: dashboard, trends, by-router/by-hotspot; tenant-scoped; date-range + granularity validation (SEC-6).
- [ ] Billing handlers: summary, CSV report builder, batch sales report; amounts from persisted records.
- [ ] Admin-space endpoints: cross-tenant aggregates, audit-logs query.
- [ ] Audit middleware/hook: write on tenant change, router config, voucher revocation, user admin actions (skip non-privileged reads).
- [ ] Rate limiting: extend Phase 1 helper to voucher generation; verify login/redeem covered; configurable thresholds in `config`.
- [ ] Error envelope audit: every handler returns SRS §12 shape; missing code paths flagged in tests.
- [ ] Worker: digest aggregation job (incremental, idempotent) to feed trend tables.

### Frontend (React)
- [ ] Dashboard: KPI cards + trend charts (day/week/month), router/hotspot breakdown tables; live refresh via existing WS events where applicable.
- [ ] Reports: date-range report builder + CSV download; sales view per batch (owner), cross-tenant view (admin).
- [ ] Audit log screen (admin): filters + detail.

### Hardening / ops
- [ ] Security pass: HTTPS-only config in nginx, secure headers, CORS allow-list, input validation audit, parameterized SQL check.
- [ ] Observability: structured JSON logs, `/healthz`/`/readyz` retained, metrics endpoint wired (DOCUMENT-PLAN §5).
- [ ] CI gate run: `go vet`, golangci-lint, `go test ./...`, frontend lint/typecheck/build, migrations up/down on throwaway DB, compose smoke.

### Tests
- [ ] Unit: billing math (amount derivation), digest aggregation idempotency, rate-limit thresholds, error-envelope shape for all handlers.
- [ ] Integration: KPI correctness vs. seeded data; trends grouping; CSV shape; cross-tenant isolation on admin aggregates; audit entries present for each privilege class.
- [ ] E2E (Playwright, DOCUMENT-PLAN §4): login → tenant → router (mock) → hotspot → voucher → redeem → session → dashboard reflects → report exports.
- [ ] Coverage gate ≥70% backend line coverage; 100% on voucher/usage state machines.

## 8. Definition of Done (Phase 5 exit criteria)

1. Dashboard + trends + breakdowns correct against seeded data; freshness ≤5 s.
2. Billing summary/report reconcile with usage records (no orphans, FR-BILLING-1).
3. Rate limiting active on login, redeem, voucher generation; brute-force lockout proven (US-19).
4. Audit trail complete for privileged actions and queryable by admin (US-18).
5. Error envelope uniform across the API (SRS §12); confirmed by tests.
6. Full Playwright E2E green; coverage ≥70%; DOCUMENT-PLAN §3 DoD checklist fully met.
7. CI/CD pipeline from DOCUMENT-PLAN §5 in place and green on this release.

## 9. Out-of-scope reminders

- Payments/self-service (post-MVP).
- PDF reports, cohorts/heatmaps, export pipelines (post-MVP).
- Custom per-tenant roles.

Reference: SRS §11–§13, API-SPECIFICATION Billing/Analytics + error envelope, DATABASE-DESIGN §2 usage_logs/batches/vouchers/digests, US-3/10/11/18/19/AN1/AN2/AUD1, PRD §7.4, DOCUMENT-PLAN §3–§5.