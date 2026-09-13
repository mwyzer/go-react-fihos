# Phase 1 — Auth & Tenancy

Extends PHASES.md Phase 1. Sprint scope: DOCUMENT-PLAN **P3** (auth core) + **P4** (tenant + user management).

- **Goal:** secure login/refresh/logout, RBAC, and admin-managed tenant lifecycle with suspension.
- **Milestone:** M1 — auth + tenancy solid, cross-tenant isolation proven.

## 1. Scope (in)

1. Login, password change, logout, token refresh (rotating refresh tokens, Redis-backed revocation).
2. RBAC: roles `admin`, `owner`, `staff`; middleware guard on all protected routes.
3. Tenant CRUD lifecycle: create, list, update, suspend, activate; `tenant_suspended` (403) enforcement.
4. User management: platform admin creates tenants + tenant users; owner manages tenant staff (create, update role/name/active).
5. Login lockout / rate limiting via Redis.
6. Audit log wiring for tenant + user privileged actions.

## 2. Scope (out)

- Router/hotspot/voucher features (later phases).
- Custom RBAC roles per tenant (fixed roles only; post-MVP).
- Captive portal, sessions, analytics.

## 3. Functional requirements (from SRS)

| ID | Requirement |
|---|---|
| FR-AUTH-1 | Username/password authentication |
| FR-AUTH-2 | Signed-token sessions validated by API middleware |
| FR-AUTH-3 | Password change + admin password reset |
| FR-AUTH-4 | 401 on missing/expired/invalid token |
| FR-AUTH-5 | Configurable lockout / rate limit via Redis on failed logins |
| FR-TENANT-1 | Only platform admin creates tenants |
| FR-TENANT-2 | Tenant lifecycle states: `active`, `suspended`, `closed` |
| FR-TENANT-3 | Suspended tenants blocked from authenticated API access |
| FR-TENANT-4 | Tenant data isolated from other tenants |
| FR-USER-1 | Users have one role: `admin`, `owner`, or `staff` |
| FR-USER-2 | Owners manage their tenant's staff accounts |
| FR-USER-3 | Staff only access their tenant workspace |

## 4. Stories (from USER-STORY.md)

- **US-1** As a platform admin, I want to create tenants…
- **US-2** As a platform admin, I want to suspend and close tenant workspaces…
- **US-5** As a tenant owner, I want to give my technician/IT access rights…

New stories owned by this phase:

- **US-A1** As a user, I want to log in with my email/password and stay logged in across refreshes so that I use the dashboard without re-authenticating.
  - Accept: `POST /auth/login` returns access + refresh tokens; refresh rotates and revokes the previous; expired/invalid access token → 401 and re-login required after refresh fails.
- **US-A2** As a user, I want to change my password so that I can rotate compromised credentials.
  - Accept: requires current password; returns 204; old tokens invalidated.
- **US-A3** As a user, I want to log out so that sessions are revoked server-side.
  - Accept: `POST /auth/logout` → 204; token no longer accepted.
- **US-A4** As a platform admin, I want to reset a user's password so that they can recover access.
  - Accept: owner for own tenant staff, admin for any; issues temp/reset flow rather than revealing the old password.
- **US-A5** As a platform admin, I want to see all tenants with status so that I can manage the workload.
  - Accept: paginated list, status filter (`active`/`suspended`/`closed`).
- **US-A6** As a platform admin, I want to update a tenant's name and branding settings so that workspace details stay current.
  - Accept: `PATCH /tenants/{id}`; audit logged.

## 5. API surface (from API-SPECIFICATION)

| Method | Path | Role | Notes |
|---|---|---|---|
| POST | `/api/v1/auth/login` | public | rate-limited; returns token pair + user profile |
| POST | `/api/v1/auth/refresh` | public | rotates token pair |
| POST | `/api/v1/auth/logout` | auth | revoke; 204 |
| POST | `/api/v1/auth/change-password` | auth | current + new password |
| POST | `/api/v1/users` | owner(tenant)/admin | create user |
| PATCH | `/api/v1/users/{id}` | owner(own staff)/admin | role/name/active |
| POST | `/api/v1/users/{id}/reset-password` | owner/admin | IE: FR-AUTH-3 |
| POST | `/api/v1/tenants` | admin | create |
| GET | `/api/v1/tenants` | admin | list + status filter |
| PATCH | `/api/v1/tenants/{id}` | admin | update |
| POST | `/api/v1/tenants/{id}/suspend` | admin | status → `suspended` |
| POST | `/api/v1/tenants/{id}/activate` | admin | status → `active` |
| GET | `/api/v1/me` | auth | current user profile (+ tenant) |

**Error envelope:** `{ "error": { "code": "...", "message": "...", "details": ... } }`.
Relevant codes this phase: `unauthorized`, `forbidden`, `tenant_suspended`, `conflict`, `validation_failed`, `too_many_requests`, `not_found`.

## 6. Data model (from DATABASE-DESIGN)

- **tenants** — `id`, `name`, `slug` (unique), `status` (`active|suspended|closed`), `branding` (jsonb), timestamps.
- **users** — `id`, `tenant_id` (FK, null for admin), `email` (unique), `password_hash`, `role` (`admin|owner|staff`), `full_name`, `is_active`, `last_login_at`.
- **audit_logs** — `tenant_id`, `user_id`, `action`, `entity_type`, `entity_id`, `meta`, `created_at`.
- Middleware-scoped querying enforced at repository layer (never from client input).
- Redis namespaces: `tenant:{id}:…`, `ratelimit:{...}`, `revoked:{token_id}`.

New migration in this phase: `0007_auth_tenancy.*` (seed admin account, tenant & user tables if the base schema does not already carry them).

## 7. Implementation tasks

### Backend (Go/Gin)
- [ ] `config`: add `TOKEN_TTL`, `REFRESH_TTL`, login-lockout settings (already partially present in `internal/config`). Verify wiring.
- [ ] `auth.Service`: issue + verify JWT (access), refresh token (rotate + revoke in Redis), hash/verify passwords (bcrypt).
- [ ] `auth` handlers: `login`, `refresh`, `logout`, `change-password`, `reset-password`, `me`.
- [ ] `middleware.Auth`: token parse, active-user + tenant load, suspension check → 403 `tenant_suspended`.
- [ ] `middleware`: login rate-limit / lockout helper (Redis `INCR` + expiry).
- [ ] Repositories: `users` (tenant-scoped CRUD), `tenants` (admin CRUD + states), `audit_logs` (write on privileged actions).
- [ ] Handlers for `users` and `tenants` endpoints per §5; wire routes in `internal/server/server.go`.
- [ ] DB pool: ensure composite tenant FK checks per DATABASE-DESIGN §6.

### Frontend (React)
- [ ] Login screen → stores token pair (secure storage), attach `Authorization: Bearer` on all calls.
- [ ] Auth context + route guard (redirect to login on 401).
- [ ] Tenant management UI (admin): create/list/suspend/activate.
- [ ] User management UI (owner/admin): create staff, toggle active, reset password.
- [ ] Axios/fetch wrapper: 401 → refresh → retry; on refresh failure → logout.

### Data / migration
- [ ] Migration up **and** down verified on a clean DB (mirror `0001`–`0006` style).

### Tests
- [ ] Unit: auth service, RBAC matrix (admin/owner/staff × permission table in SRS §3), lockout logic.
- [ ] Integration: login → token → protected call → refresh → old token rejected; suspended tenant gets 403 on every route; cross-tenant access attempt denied.
- [ ] Frontend: typecheck + lint + build pass; login guard renders against local API.

## 8. Definition of Done (Phase 1 exit criteria)

1. `docker compose` stack boots with Phase 1 migrations applied; `/readyz` green.
2. Login flow works end-to-end (UI → API → JWT → guarded route).
3. Admin can create/suspend/activate tenants; suspended tenant 403 everywhere immediately.
4. Owner can add/disable staff; staff is strictly tenant-scoped; RBAC matrix tests green.
5. Refresh rotation + logout revocation tests green; brute-force lockout verified (Redis).
6. Passwords hashed; no secrets in responses or logs; audit entries written for tenant + privileged user actions.
7. Backend `go vet` clean; frontend typecheck/lint/build clean.

## 9. Out-of-scope reminders

- No router/hotspot/voucher endpoints yet.
- WebSocket hub is not required for Phase 1 (status events arrive in Phase 2).
- No payment, portal, or analytics work.

Reference: SRS §4–§5, API-SPECIFICATION Authentication/Tenant sections, DATABASE-DESIGN §2 tenants/users/audit_logs, US-1/2/5/A1–A6.