# API Specification

Base path: `/api/v1` · Content-Type: `application/json` · Auth: `Authorization: Bearer <token>` unless noted.

Error envelope:

```json
{ "error": { "code": "validation_failed", "message": "Invalid voucher code", "details": { "code": "required" } } }
```

## Authentication API

### POST /auth/login
Public. Body: `{ "email", "password" }`.
→ `200` `{ "access_token", "refresh_token", "user": { "id", "email", "full_name", "role", "tenant_id" } }`
Errors: `401 unauthorized`, `429 too_many_requests` (rate limited).

### POST /auth/refresh
Body: `{ "refresh_token" }` → rotated `tokens` pair.

### POST /auth/logout
Revokes current token(s). → `204`.

### POST /auth/change-password
Body: `{ "current_password", "new_password" }` → `204`.

### POST /users
Create user (owner for own tenant, admin any). Body: `{ "email", "password", "role", "full_name", "tenant_id?" }`.
→ `201` user. Errors: `409 conflict` (email exists), `403 forbidden`.

### PATCH /users/{id}
Update role/name/active state. Owner only manages own tenant staff.

## Tenant API

### POST /tenants *(admin)*
Body: `{ "name", "slug", "branding?" }` → `201` tenant.

### GET /tenants *(admin)*
List with pagination + status filter → `{ "items": [...], "total", "page", "size" }`.

### PATCH /tenants/{id} *(admin)*
Update name / branding. Body fields optional.

### POST /tenants/{id}/suspend · POST /tenants/{id}/activate *(admin)*
Transitions tenant status. → `204`.
Suspended tenants receive `403 tenant_suspended` on all tenant-scoped calls.

## Router API

### POST /routers
Body: `{ "name", "ip_address", "api_port", "username", "password" }` → `201` router.
Triggers an initial connectivity probe (`status: unverified` until first probe).

### GET /routers
List tenant routers (optional `status` filter) with live status.

### GET /routers/{id}
Detail including `last_seen_at`, `last_sync_at`, `status`.

### PATCH /routers/{id}
Update name/ip/port/credentials. Credentials never returned.

### DELETE /routers/{id}
Remove; returns `409` if it has active hotspots.

### POST /routers/{id}/probe
Enqueue immediate status check; worker replies via WebSocket event `router.status`.

## Hotspot API

### POST /hotspots
Body: `{ "name", "router_id", "profile_id", "ip_range" }` → `201` hotspot (`status: configuring`), then worker pushes to router → WebSocket `hotspot.status`.

### GET /hotspots
List (filter by `router_id`, `status`).

### GET /hotspots/{id}
Detail incl. profile + router info.

### PATCH /hotspots/{id}
Update name/ip/profile; enqueues reconfig.

### POST /hotspots/{id}/enable · POST /hotspots/{id}/disable
Toggle live hotspot. Disable blocks new sessions only.

### DELETE /hotspots/{id}
Return `409` if active sessions exist.

### GET /hotspot-profiles · POST /hotspot-profiles
List / create bandwidth profiles. Create body: `{ "name", "rx_rate", "tx_rate", "session_uptime_limit", "keepalive_timeout" }`.

## Voucher API

### POST /voucher-batches
Body: `{ "name", "quantity", "duration", "price?", "profile_id", "valid_from?", "valid_to?" }` → `201` batch with `quantity` voucher rows.

### GET /voucher-batches
List with summary counts by status.

### POST /voucher-batches/{id}/print
→ PDF (byte stream) printable voucher sheet for unredeemed vouchers.

### GET /vouchers
List; filters: `batch_id`, `status`, `q` (code partial), pagination.

### PATCH /vouchers/{id}/revoke
Revoke if `unused`; `409 conflict` if already redeemed/expired.

### POST /vouchers
Single quick voucher generation (same params as batch, quantity=1).

## Captive Portal API

Public (no auth; rate-limited per IP).

### GET /portal/settings
Query params: `tenant={slug}`, `hotspot={id}` → `{ "branding": {...}, "hotspot": { "name", "status" } }`.
Errors: `404 not_found`, hotspot disabled → `409`.

### POST /portal/redeem
Body: `{ "tenant": "slug", "hotspot_id", "code" }`.
→ `200` `{ "session": {...} }` on success (worker instructs router to authorize client).
Errors: `invalid_code`, `already_used`, `expired`, `revoked`, `hotspot_disabled`, `router_offline` (503-ish semantics with `409`/`422` mapping).

## Session API

### GET /sessions
Active sessions for tenant; filters: `hotspot_id`, `router_id`, `state`; synchronously fresh from last router sync.

### GET /sessions/{id}
Detail with live counters.

### POST /sessions/{id}/disconnect
Instructs router to disconnect; emits `session.closed` event.

### GET /usage-logs
Closed sessions with usage; filters: `hotspot_id`, date range `from`/`to`, pagination. Sorted by `ended_at` desc.

### GET /usage-logs/{id}
Single usage record (billing input).

## Billing API

### GET /billing/summary
Query: `from`, `to`, `hotspot_id?` → `{ "sessions", "active_sessions", "bytes_rx", "bytes_tx", "amount" }`.

### GET /billing/reports
Export CSV of line-item usage per period (owner/admin).

### GET /voucher-batches/{id}/sales *(admin)*
Per-batch sales report (issued / sold / redeemed metrics).

### POST /voucher-batches/{id}/sales *(admin)*
Per-batch sales report (issued / sold / redeemed metrics).

### Customer wallet (prepaid balance)

Customers carry a prepaid balance (`customers.balance`). Top-ups go through the payment gateway (mock settles instantly; sandbox completes via webhook) and credit the wallet net of the gateway fee. Paying a monthly billing window can be debited from the wallet. Balance-mutating actions are owner/admin; reads are any tenant role.

### POST /customers/{id}/topup *(owner/admin)*
Body: `{ "amount": 200000 }` → `201` `{ "async": false, "payment_url": "...", "payment": { "id", "status", "external_ref", "customer_id", "net_amount" } }`.
Credits the wallet with `net_amount` (`amount × (1 − fee)`); emits audit `customer.topup`.
Errors: `409 insufficient_balance` (never here), `404 not_found`, invalid amount → `422`.

### GET /customers/{id}/wallet
→ `200` `{ "customer_id", "balance", "transactions": { "items": [ { "id", "type": "topup|bill_payment|adjustment", "amount", "balance_after", "note", "created_at" } ], "total", "page", "size" } }`.

### POST /customers/{id}/wallet/adjust *(owner/admin)*
Body: `{ "amount": 50000, "note": "cash deposit" }` (signed) → `200` `{ "id", "adjusted", "balance" }`.
Audit `customer.wallet_adjust`. Negative adjustments are limited to available balance.
Errors: `409 insufficient_balance`, `404 not_found`.

### POST /billing/{id}/pay
Body: `{ "method": "wallet" | "manual" }` (default `wallet`) → `200` `{ "window": {...}, "payment": {...} }`.
`method=wallet` debits the customer balance atomically; `method=manual` bypasses the wallet (cash/walk-in). Emits audit `billing.paid`.
Errors: `409 insufficient_balance`, `409 already_paid`, `404 not_found`.

### Payments
`payments` is polymorphic: `vouchers` (portal voucher purchase) vs `wallet_topup`, selected by `entity_type` (`customer_id` set for wallet top-ups). Gateway providers: mock (instant settle), sandbox (async, completed via webhook).

### GET /payments *(owner/admin)*
Query: `status=`, `kind=wallet_topup|voucher`, `page=`, `size=` → paginated list (payments screen / financial ledger).

## Analytics API

### GET /analytics/dashboard
RT stats: active sessions, today sessions, today vouchers, today traffic, uptime.

### GET /analytics/trends
Query: `metric` (`sessions` | `revenue` | `traffic` | `vouchers`), `granularity` (`day` | `week` | `month`), `from`, `to`.

### GET /analytics/by-router · GET /analytics/by-hotspot
Breakdown tables for the same metrics.

*(Admin space only)*
### GET /analytics/tenants
Cross-tenant aggregate snapshot (top tenants by sessions/revenue).

## WebSocket

Endpoint: `GET /ws` (authenticated via `?token=` or header, short-lived ticket).

Events pushed:
| Event | Payload |
|---|---|
| `router.status` | `{ "router_id", "status", "last_seen_at" }` |
| `hotspot.status` | `{ "hotspot_id", "status", "error?" }` |
| `session.active` | `{ "session": {...} }` |
| `session.closed` | `{ "session_id", "usage": {...} }` |
| `voucher.redeemed` | `{ "voucher_id", "code_truncated", "hotspot_id" }` |

Client subscribes to tenant scope; `heartbeat: { "type": "ping" }` every 30s, `pong` reply.

Conventions:
- Pagination: `?page=&size=`, headers `X-Total-Count`.
- Dates: RFC3339. IDs: int64. Money: numeric as string.
- All scoped calls validated by tenant middleware; scoping filters are never client-supplied.