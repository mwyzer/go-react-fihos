# Phase 3 — Vouchers & Captive Portal

Extends PHASES.md Phase 3. Sprint scope: DOCUMENT-PLAN **P7** (voucher batch generation, print PDF, search, revoke) + **P8** (captive portal: branding + redeem flow).

- **Goal:** the first revenue-generating capability — generate, sell, print, and redeem vouchers end-to-end.
- **Milestone:** M2 — vouchers + portal reach end-to-end (mock RouterOS). First customer-facing value: PRD §7.2 / §7.3 happy paths.

## 1. Scope (in)

1. Voucher batch generation with configurable params (quantity, duration, price, profile, validity window) — transactional insert.
2. Cryptographically secure, non-guessable codes in human-readable form (`XXXX-XXXX`).
3. Single-voucher quick generation.
4. Voucher states: `unused`, `redeemed`, `expired`, `revoked`; search with filters; revoke unused codes (single + bulk).
5. Print-friendly voucher sheet + PDF export.
6. Branded captive portal per tenant/hotspot: tenant logo/colors/name + hotspot context.
7. Redemption flow: validate code → mark redeemed → start session at router (worker push) → success page; clear error taxonomy for bad codes.
8. Rate limiting on the redeem endpoint.

## 2. Scope (out)

- Live session listing/disconnect/usage sync from the worker (Phase 4) — redemption may create a session row, but monitoring is Phase 4.
- Online payments / self-service purchase (post-MVP).
- Advanced theming (beyond colors/logo/name/terms).

## 3. Functional requirements (from SRS)

| ID | Requirement |
|---|---|
| FR-VOUCHER-1 | Generate vouchers individually or in batches |
| FR-VOUCHER-2 | Params: quantity, duration, bandwidth profile, validity window, optional price |
| FR-VOUCHER-3 | Unique, non-guessable codes |
| FR-VOUCHER-4 | Printable sheet + PDF export |
| FR-VOUCHER-5 | Search + status view (`unused`/`redeemed`/`expired`/`revoked`) |
| FR-VOUCHER-6 | Revoke unused vouchers |
| FR-VOUCHER-7 | Redeemed voucher is single-use |
| FR-PORTAL-1 | Branded captive portal per hotspot (logo/colors/name) |
| FR-PORTAL-2 | Portal accepts and validates a voucher code |
| FR-PORTAL-3 | On success: instruct router to start session + mark voucher redeemed (same transaction) |
| FR-PORTAL-4 | Clear error for invalid/expired/revoked/already-redeemed codes |

## 4. Stories (from USER-STORY.md)

- **US-8** owner: generate voucher batches (duration/bandwidth/quantity/price).
- **US-9** owner: print a voucher sheet or PDF.
- **US-12** staff: generate a single voucher on the spot.
- **US-17** guest: enter a code on a simple branded page and get online.

New stories owned by this phase:

- **US-V1** As an owner, I want to see each voucher's status so that I can chase redemptions.
  - Accept: list shows status + expiry; filters by batch/status/code search.
- **US-V2** As an owner, I want to revoke an unused voucher (single or a batch slice) so that I can stop abuse or misprints.
  - Accept: `unused` only; `409` if redeemed/expired; revocation audited.
- **US-V3** As an owner, I want expired vouchers surfaced so that I know what's stale.
  - Accept: `expired` derived from validity window; shown in filters/status summary.
- **US-P1** As a guest, I want to see which hotspot I'm on so that I trust the page is the venue's.
  - Accept: portal settings render tenant branding + hotspot name.
- **US-P2** As an owner, I want the redeem error to be friendly so that guests aren't confused.
  - Accept: distinct messages for invalid/already-used/expired/revoked/offline; portal-localized wording, machine-readable code retained in API.

## 5. API surface (from API-SPECIFICATION)

| Method | Path | Auth | Notes |
|---|---|---|---|
| POST | `/api/v1/voucher-batches` | owner/staff | body `{name, quantity, duration, price?, profile_id, valid_from?, valid_to?}` → batch + rows atomically |
| GET | `/api/v1/voucher-batches` | owner/staff | list + status summary counts |
| POST | `/api/v1/voucher-batches/{id}/print` | owner/staff | PDF byte stream (unredeemed) |
| GET | `/api/v1/vouchers` | owner/staff | filters `batch_id`, `status`, `q`; pagination |
| PATCH | `/api/v1/vouchers/{id}/revoke` | owner/staff | `unused` only; `409` otherwise |
| POST | `/api/v1/vouchers` | owner/staff | single quick voucher |
| GET | `/api/v1/portal/settings` | public | `?tenant={slug}&hotspot={id}` → branding + hotspot |
| POST | `/api/v1/portal/redeem` | public (rate-limited) | `{tenant, hotspot_id, code}` → session |

**Redeem error taxonomy** (API-SPECIFICATION §Captive Portal): `invalid_code`, `already_used`, `expired`, `revoked`, `hotspot_disabled`, `router_offline`.

## 6. Data model (from DATABASE-DESIGN)

- **voucher_batches** — `id`, `tenant_id`, `name`, `quantity`, `price` (nullable), `duration` (s), `profile_id` FK, `valid_from`/`valid_to`, `created_by`, `created_at`.
- **vouchers** — `id`, `tenant_id`, `batch_id` FK, `code` (unique), `status` (`unused|redeemed|expired|revoked`), `redeemed_at`, `redeemed_by`, `expires_at` (from batch window).
- **sessions** — redemption inserts a session row (FK `voucher_id`) **in the same transaction** as marking the voucher `redeemed` (DATABASE-DESIGN §6 integrity rule).
- Partial indexes for active entities per DATABASE-DESIGN §5.
- Code = cryptographically-secure random (base32), formatted `XXXX-XXXX`.

## 7. Implementation tasks

### Backend (Go/Gin)
- [ ] `voucher.Codec`: secure code generation (format, chars excluded to avoid ambiguity), validation.
- [ ] `voucher.Service`: batch generation (single transaction, unique-code collision retry), single voucher, status transitions, expiry derivation, revoke (guard `unused`), search/pagination.
- [ ] Voucher handlers per §5; audit write on revoke (SEC-7).
- [ ] PDF printing: printable HTML/PDF template (voucher sheet, per-sheet unredeemed only); byte-stream response.
- [ ] Portal handlers: `settings` (tenant+hotspot branding, 404/409 rules), `redeem` (validate → tx: redeem voucher + create session → enqueue router authorization).
- [ ] Redeem rate limiting per IP (Redis) — extend Phase 1 lockout helper.
- [ ] Worker/queue: consumer to authorize client at router on redemption (`mikrotik.Client` from Phase 2); surface `router_offline` when unreachable.

### Frontend (React)
- [ ] Vouchers screen: batch wizard, single quick voucher, list + filters + status chips, revoke (single/multi), print/PDF open.
- [ ] Portal bundle: minimal branded page (tenant settings → branding), code entry, success/error states; served from the web build path (`frontend/portal`).
- [ ] Wire redeem flow to Vite/nginx proxy.

### Client-facing (router)
- [ ] Redeem → instruct router to authorize MAC/IP with profile; idempotent (no double session if retried).
- [ ] Mock RouterOS coverage for authorization command + failure paths.

### Tests
- [ ] Unit: codec (format, charset, uniqueness under batch of 100k), expiry derivation, state-machine guards.
- [ ] Integration: batch atomicity (failure rolls back all rows), single-use guarantee (concurrent redeems — one wins), revoke guards, search filters, PDF returns non-empty valid stream.
- [ ] Portal E2E (Playwright): generate batch → print PDF → portal redeem valid code → success; invalid/already-used → correct error.
- [ ] 100% coverage on voucher/redemption state machine (DOCUMENT-PLAN §4).

## 8. Definition of Done (Phase 3 exit criteria)

1. Batch + single generation works end-to-end from the UI; codes unique and secure.
2. Printable sheet + PDF verified (opens, correct codes).
3. Revoke/search with status filters correct; revocation audited.
4. Portal renders tenant branding; redeem happy path starts a session at (mock) router; voucher marked `redeemed` atomically.
5. Every bad-code path returns the correct taxonomy entry; redeem rate-limited.
6. Voucher can never be redeemed twice (concurrency test green).
7. `go vet` + frontend typecheck/lint/build clean; compose stack green.

## 9. Out-of-scope reminders

- Session monitoring/disconnect/usage sync (Phase 4).
- Analytics/reporting (Phase 5) — simple status summaries on the vouchers screen are in scope.
- Payments/self-service (post-MVP).

Reference: SRS §8–§9, API-SPECIFICATION Voucher/Captive Portal sections, DATABASE-DESIGN §2 voucher_batches/vouchers/sessions, US-8/9/12/17/V1–V3/P1–P2, IDEA "success looks like" #1–#2.