# Database Design

## 1. ERD

```
tenants 1───* users
tenants 1───* routers
tenants 1───* hotspots
tenants 1───* voucher_batches
tenants 1───* sessions
tenants 1───* usage_logs
hotspot_profiles 1───* hotspots
routers 1───* hotspots
hotspots 1───* sessions
voucher_batches 1───* vouchers
vouchers 1───0..1 usage_logs        (redemption binds voucher to usage)
tenants 1───* settings
users 1───* audit_logs
```

Note: a *tenant-local* relationship to a shared `hotspot_profiles` foreign key is resolved by validating the profile belongs to the same tenant (denormalized at app layer). Profiles are per-tenant.

## 2. Tables

### tenants
| Column | Type | Notes |
|---|---|---|
| id | bigint PK | |
| name | varchar | |
| slug | varchar unique | URL/identifier |
| status | enum | `active`, `suspended`, `closed` |
| branding | jsonb | logo/colors/portal text |
| created_at / updated_at | timestamptz | |

### users
| Column | Type | Notes |
|---|---|---|
| id | bigint PK | |
| tenant_id | bigint FK → tenants | null for platform admin |
| email | varchar unique | |
| password_hash | varchar | bcrypt/argon2 |
| role | enum | `admin`, `owner`, `staff` |
| full_name | varchar | |
| is_active | boolean | |
| last_login_at | timestamptz | |

### hotspot_profiles
| Column | Type | Notes |
|---|---|---|
| id | bigint PK | |
| tenant_id | bigint FK | |
| name | varchar | |
| rx_rate | bigint | bps cap down |
| tx_rate | bigint | bps cap up |
| session_uptime_limit | int | seconds, 0 = none |
| keepalive_timeout | int | seconds |
| created_by | bigint FK users | |

### routers
| Column | Type | Notes |
|---|---|---|
| id | bigint PK | |
| tenant_id | bigint FK | |
| name | varchar | |
| ip_address | inet | |
| api_port | int | default 8728 |
| username | varchar | RouterOS login |
| password_enc | text | encrypted at rest |
| status | enum | `unverified`, `online`, `offline` |
| last_seen_at | timestamptz | |
| last_sync_at | timestamptz | |
| created_at / updated_at | timestamptz | |

### hotspots
| Column | Type | Notes |
|---|---|---|
| id | bigint PK | |
| tenant_id | bigint FK | |
| router_id | bigint FK → routers | |
| profile_id | bigint FK → hotspot_profiles | |
| name | varchar | |
| mikrotik_id | varchar | router-local hotspot id |
| ip_range | inet/cidr | |
| status | enum | `configuring`, `active`, `disabled`, `error` |
| last_config_at | timestamptz | |

### voucher_batches
| Column | Type | Notes |
|---|---|---|
| id | bigint PK | |
| tenant_id | bigint FK | |
| name | varchar | |
| quantity | int | |
| price | numeric | per voucher, nullable |
| duration | int | seconds |
| profile_id | bigint FK | bandwidth profile |
| valid_from / valid_to | timestamptz | validity window |
| created_by | bigint FK users | |
| created_at | timestamptz | |

### vouchers
| Column | Type | Notes |
|---|---|---|
| id | bigint PK | |
| tenant_id | bigint FK | |
| batch_id | bigint FK → voucher_batches | |
| code | varchar unique | format `XXXX-XXXX` |
| status | enum | `unused`, `redeemed`, `expired`, `revoked` |
| redeemed_at | timestamptz | |
| redeemed_by | bigint FK users | nullable |
| expires_at | timestamptz | derived from batch window |

### sessions
| Column | Type | Notes |
|---|---|---|
| id | bigint PK | |
| tenant_id | bigint FK | |
| hotspot_id | bigint FK → hotspots | |
| router_id | bigint FK → routers | |
| mac_address | varchar | |
| ip_address | inet | |
| username | varchar | voucher/user name |
| voucher_id | bigint FK → vouchers | nullable |
| started_at | timestamptz | |
| last_seen_at | timestamptz | |
| state | enum | `active`, `closing`, `closed` |

### usage_logs
| Column | Type | Notes |
|---|---|---|
| id | bigint PK | |
| tenant_id | bigint FK | |
| session_id | bigint FK → sessions | |
| voucher_id | bigint FK | nullable |
| started_at / ended_at | timestamptz | |
| duration_sec | int | |
| bytes_rx / bytes_tx | bigint | |
| amount | numeric | billed amount, nullable |
| billing_status | enum | `unpaid`, `paid` | (Post-MVP) |

### settings
| Column | Type | Notes |
|---|---|---|
| id | bigint PK | |
| tenant_id | bigint FK unique | |
| portal_title | varchar | |
| portal_message | text | |
| currencies / payment_config | jsonb | (Post-MVP) |

### audit_logs
| Column | Type | Notes |
|---|---|---|
| id | bigint PK | |
| tenant_id | bigint FK | nullable |
| user_id | bigint FK | |
| action | varchar | e.g. `tenant.create`, `voucher.revoke` |
| entity_type / entity_id | varchar / bigint | polymorphic reference |
| meta | jsonb | before/after payload |
| created_at | timestamptz | |

## 3. Columns

- All tables include `created_at` (and `updated_at` where mutable) as `timestamptz` with defaults.
- IDs are `bigint` PK (identity/serial); codes/tokens use dedicated secure-random strings.
- Enums stored as PostgreSQL `ENUM` or constrained `varchar` + check; prefer check constraints for easy evolution.

## 4. Relationships

- `users.tenant_id → tenants.id`; `routers.tenant_id`, `hotspots.tenant_id`, etc.
- `hotspots.router_id → routers.id`; `hotspots.profile_id → hotspot_profiles.id`.
- `vouchers.batch_id → voucher_batches.id`; `sessions.hotspot_id → hotspots.id`.
- `sessions.voucher_id → vouchers.id` (set at redemption); `usage_logs.session_id → sessions.id`.
- Composite (tenant_id, …) foreign keys enforced so children can never reference another tenant's parent.

## 5. Indexes

- `users(tenant_id)`, `users(email)` unique.
- `routers(tenant_id)`, `routers(ip_address)`.
- `hotspots(tenant_id)`, `hotspots(router_id)`, `hotspots(status)`.
- `vouchers(tenant_id)`, `vouchers(batch_id)`, `vouchers(code)` unique, `vouchers(status)`, `vouchers(expires_at)`.
- `voucher_batches(tenant_id)`, `voucher_batches(created_at)`.
- `sessions(tenant_id)`, `sessions(hotspot_id)`, `sessions(state)`, `sessions(last_seen_at)`.
- `usage_logs(tenant_id)`, `usage_logs(hotspot_id)`, `usage_logs(started_at)` — plus date-range composite for reporting.
- `audit_logs(tenant_id, created_at)`.

## 6. Constraints

- `vouchers.code` unique (secure uniqueness).
- `hotspot_profiles.name` unique per tenant.
- Check `price >= 0`, `duration > 0` on batches; `bytes_rx/tx >= 0` on usage_logs.
- Voucher redemption integrity: a voucher set to `redeemed` via same transaction as session creation.
- FK delete behavior: restrict on financial/log records (usage_logs, vouchers); cascade on children you want to clean up (audit_logs) or soft-delete for tenants.
- Partial unique indexes for active entities where relevant (e.g. one default profile per tenant, if applicable).

## 7. Multi-tenant strategy

**Choice: shared schema (single database/tables) + tenant scoping baked into every query and constraint.**

- `tenant_id` on every tenant-scoped table; composite FKs include tenant to prevent cross-tenant links.
- Application enforces tenant context: middleware injects `tenant_id`; repositories always filter on it. No tenant ID is ever taken from client input.
- Connection pool uses a single service role with least privilege.
- Reads/caches in Redis are namespaced `tenant:{id}:…`.
- Suspended tenants: central check short-circuits all requests (403).
- Future option: move heavy tenants to schema-per-tenant if isolation demands grow; designs should keep `tenant_id` as the seam to make that migration possible.
- Analytics digest tables are keyed by tenant and generated by the worker per tenant.