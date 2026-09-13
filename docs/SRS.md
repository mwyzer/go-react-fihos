# Software Requirements Specification (SRS)

## 1. Functional requirements

### FR-AUTH. Authentication
- **FR-AUTH-1** The system SHALL support username/password authentication for users.
- **FR-AUTH-2** Sessions SHALL be issued as signed tokens and validated by the API middleware.
- **FR-AUTH-3** The system SHALL support password change and administrative password reset.
- **FR-AUTH-4** The API SHALL reject requests with missing, expired, or invalid tokens (HTTP 401).
- **FR-AUTH-5** Failed logins SHALL be subject to a configurable lockout / rate limit via Redis.

### FR-TENANT. Multi-tenancy
- **FR-TENANT-1** Only platform admin SHALL create tenants.
- **FR-TENANT-2** Tenants SHALL support lifecycle states: `active`, `suspended`, `closed`.
- **FR-TENANT-3** Suspended tenants SHALL NOT be able to authenticate or receive API responses beyond status.
- **FR-TENANT-4** All tenant-scoped data SHALL be isolated from other tenants (see DATABASE-DESIGN Multi-tenant strategy).

### FR-USER. Roles & users
- **FR-USER-1** Users SHALL have one role: `admin` (platform), `owner` (tenant admin), `staff` (tenant staff).
- **FR-USER-2** Owners SHALL manage their tenant's staff accounts.
- **FR-USER-3** Staff SHALL only access their tenant workspace.

### FR-ROUTER. Router management
- **FR-ROUTER-1** Users with router rights SHALL register MikroTik routers (name, IP, API port, credentials).
- **FR-ROUTER-2** The system SHALL test connectivity to a router and report status.
- **FR-ROUTER-3** The system SHALL periodically probe router status and mark `online`/`offline`.
- **FR-ROUTER-4** Router credentials SHALL be stored encrypted and never returned by the API.
- **FR-ROUTER-5** Router status changes SHALL be pushed to clients over WebSocket.

### FR-HOTSPOT. Hotspot management
- **FR-HOTSPOT-1** Users SHALL create hotspots under a router.
- **FR-HOTSPOT-2** Users SHALL assign a hotspot profile (bandwidth, session limits) to each hotspot.
- **FR-HOTSPOT-3** The system SHALL push hotspot configuration to the MikroTik router on create/update.
- **FR-HOTSPOT-4** Users SHALL enable/disable a hotspot; disabling it SHALL stop new sessions.

### FR-VOUCHER. Vouchers
- **FR-VOUCHER-1** Users SHALL generate vouchers individually or in batches.
- **FR-VOUCHER-2** Voucher parameters SHALL include: quantity, duration, bandwidth profile, validity window, optional price.
- **FR-VOUCHER-3** The system SHALL generate unique, non-guessable codes.
- **FR-VOUCHER-4** Users SHALL view a print-friendly voucher sheet and export it (PDF).
- **FR-VOUCHER-5** Users SHALL search vouchers and view redemption status (`unused`, `redeemed`, `expired`, `revoked`).
- **FR-VOUCHER-6** Users SHALL be able to revoke unused vouchers.
- **FR-VOUCHER-7** A redeemed voucher SHALL be single-use.

### FR-PORTAL. Captive portal
- **FR-PORTAL-1** Each hotspot SHALL expose a branded captive portal page (tenant logo/colors/name).
- **FR-PORTAL-2** The portal SHALL accept a voucher code and validate it against the API.
- **FR-PORTAL-3** On successful validation, the API SHALL instruct the router to start a session and mark the voucher redeemed.
- **FR-PORTAL-4** The portal SHALL display a clear error for invalid/expired/revoked or already-redeemed codes.

### FR-SESSION. Sessions
- **FR-SESSION-1** The system SHALL track active sessions per hotspot with username/mac/ip/uptime/traffic.
- **FR-SESSION-2** Session data SHALL be synchronized from routers by the worker on a polling cadence.
- **FR-SESSION-3** Live session updates SHALL be pushed to the dashboard via WebSocket.
- **FR-SESSION-4** Authorized users SHALL be able to disconnect individual sessions.
- **FR-SESSION-5** Every ended session SHALL be persisted as a usage record (start, end, bytes up/down, duration).

### FR-BILLING. Billing
- **FR-BILLING-1** Usage records SHALL be the source of truth for billing.
- **FR-BILLING-2** The system SHALL report sales (vouchers sold) and usage per tenant/hotspot periods.
- **FR-BILLING-3** Billing amounts SHALL be derived from voucher price/type and session usage.
- **FR-BILLING-4** (Post-MVP) The system SHALL support payment gateway records linked to vouchers.

### FR-ANALYTICS. Analytics
- **FR-ANALYTICS-1** Dashboards SHALL show: active sessions, sessions today, vouchers sold, traffic totals.
- **FR-ANALYTICS-2** Trends SHALL be available per day/week/month.
- **FR-ANALYTICS-3** Reports SHALL be filterable by tenant, router, and hotspot in admin space.

## 2. Non-functional requirements

- **NFR-PERF-1** API p95 response time under normal load SHALL be < 300ms for read endpoints.
- **NFR-PERF-2** Dashboard session/status updates SHALL reflect within < 5 seconds (WebSocket push).
- **NFR-REAL-1** Realtime data SHALL use WebSocket; polling SHALL be the fallback.
- **NFR-SEC-1** Passwords SHALL be stored hashed (bcrypt/argon2), router credentials SHALL be encrypted at rest.
- **NFR-SEC-2** All API traffic SHALL use HTTPS (TLS) in production.
- **NFR-SEC-3** API SHALL apply multi-tenant access checks on every tenant-scoped route.
- **NFR-AVAIL-1** Target availability of 99.9% for API and router-sync services.
- **NFR-SCAL-1** Architecture SHALL support horizontal scaling of API and worker behind a load balancer.
- **NFR-OBS-1** The system SHALL emit structured logs and expose health endpoints.
- **NFR-I18N-1** UI text SHALL be externalized; initial locale is English.

## 3. Roles & permissions

| Permission | admin | owner | staff |
|---|---|---|---|
| Manage tenants | ✅ | – | – |
| View all tenants stats (admin space) | ✅ | – | – |
| Manage tenant user accounts | ✅ | ✅ | – |
| Register/edit routers | ✅ | ✅ | – |
| Manage hotspots | ✅ | ✅ | ✅ |
| Generate vouchers | ✅ | ✅ | ✅ |
| Print / revoke vouchers | ✅ | ✅ | ✅ |
| Disconnect sessions | ✅ | ✅ | ✅ |
| View analytics | ✅ | ✅ | ✅ (tenant only) |
| Configure tenant branding & portal | – | ✅ | – |
| Billing reports | ✅ | ✅ | – |

## 4. Authentication

- Login flow: `POST /api/v1/auth/login` → validates credentials → returns signed JWT/session token + user profile.
- Middleware extracts the token, verifies signature/expiry, loads user + tenant, injects context into handlers.
- Redis keys sessions/tokens for revocation and rate limiting; lockout after N failed attempts.
- Refresh support: short-lived access token + longer refresh token; refresh rotates.
- Tenant suspension check is applied at authentication and per-request.

## 5. Multi-tenancy

- Every tenant-scoped table carries `tenant_id`.
- API middleware resolves the tenant from the authenticated user and injects a `tenant_id` filter into every query.
- No cross-tenant data access is possible; tenant-local IDs are never trusted globally.
- Tenant suspension is enforced centrally so suspended tenants receive 403 on all API calls.
- Redis caches are keyed with `tenant:{id}:...` namespaces.

## 6. Router management

- Register router: `POST /api/v1/routers` (name, ip, api_port, credentials).
- Probe: API contacts router via MikroTik API; sets status; reported to clients.
- Worker periodic ping job updates status every N seconds and emits events.
- Config push: hotspot/profile update triggers idempotent router commands in the worker queue.

## 7. Hotspot management

- CRUD for hotspots; each belongs to a router and has a profile.
- Profile defines: max shared/individual bandwidth (rx/tx), session uptime hard limit, keepalive timeout, MAC binding policy.
- On create/update, a config-push job targets the router.
- Hotspot status: `configuring`, `active`, `disabled`, `error`.

## 8. Voucher

- Batch generation is transactional: insert batch + rows atomically; codes unique.
- Codes: random cryptographically-secure base32 of ~10 chars, human-readable format `XXXX-XXXX`.
- Voucher states: `unused`, `redeemed`, `expired`, `revoked`.
- Expiry determined by creation + validity window or explicit validity dates.
- Redemption marks the voucher used and initiates an active session at the router.

## 9. Captive portal

- Served statically (React), themed per tenant/hotspot via settings endpoint.
- Flow: GET stage → user submits code → `POST /api/v1/portal/redeem` → API validates → worker/router starts session → redirect to success page.
- Redeem endpoint is rate-limited per IP to prevent brute force.
- Error taxonomy: `invalid_code`, `already_used`, `expired`, `revoked`, `hotspot_disabled`, `router_offline`.

## 10. Billing

- Every session close writes a usage record; worker reconciles usage vs. voucher.
- Reports aggregate by day/week/month with totals (sessions, traffic, revenue).
- Data for billing comes exclusively from persisted usage records, not live state.

## 11. Analytics

- Dashboards read from aggregated digest tables or computed queries on usage/session/voucher data.
- Admin space shows cross-tenant aggregates; tenant space is tenant-scoped only.
- Reports exportable (CSV) for offline processing (Post-MVP: PDF).

## 12. Error handling

- Uniform error envelope: `{ "error": { "code": "...", "message": "...", "details": ... } }`.
- Codes: `invalid_request`, `unauthorized`, `forbidden`, `not_found`, `conflict`, `validation_failed`, `tenant_suspended`, `router_offline`, `quota_exceeded`.
- Validation errors list per-field messages.
- Router/worker errors are surfaced on the affected resource (e.g. hotspot `error` state) with a retry endpoint.

## 13. Security requirements

- **SEC-1** Hashed passwords; encrypted storage of router credentials; no secrets in logs or responses.
- **SEC-2** Centralized auth middleware for all routes except login/portal-redeem/health.
- **SEC-3** Tenant-context enforcement on every tenant-scoped handler.
- **SEC-4** Rate limiting (Redis) on login, redeem, and voucher generation.
- **SEC-5** HTTPS-only, secure cookie/header configuration, CORS allow-list.
- **SEC-6** Input validation on all request bodies; parameterized SQL everywhere.
- **SEC-7** Audit log for privileged actions (tenant changes, router config, voucher revocation).
- **SEC-8** Least-privilege DB service accounts; migrations applied with restricted role.