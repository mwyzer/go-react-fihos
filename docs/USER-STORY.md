# User Stories

Stories grouped by persona (PRD §5) and feature area (SRS §1), with acceptance criteria and a priority marker: **[M]** MVP (must), **[S]** next (should), **[C]** could. Stories map to the phases in PHASES.md.

## Platform Admin

### Manage tenants
- **US-1 [M]** As a platform admin, I want to create tenants so that new businesses can be onboarded as workspaces.
  - Accept: only admin can create tenants; new tenant is `active` by default; tenant name is unique.
- **US-2 [M]** As a platform admin, I want to suspend and close tenant workspaces so that I can stop a problematic business.
  - Accept: suspending blocks all tenant API access (403) immediately; suspending does not destroy data; closed tenants can be offboarded.

### Oversight
- **US-3 [S]** As a platform admin, I want a cross-tenant overview (tenants, routers, revenue) so that I can monitor the whole platform from one place.
  - Accept: aggregate KPI cards and per-tenant drill-down; admin-only route.

## Tenant Owner

### Router setup
- **US-4 [M]** As a tenant owner, I want to add my MikroTik router (name/IP/credentials) so that the platform can manage it.
  - Accept: router is registered; platform verifies connectivity and marks `online`/`offline`; credentials are stored encrypted and never shown.
- **US-5 [M]** As a tenant owner, I want to give my technician/IT access rights so that only people I choose touch networking.
  - Accept: owner can create staff users scoped to the tenant; staff cannot change tenant branding or billing.

### Hotspot provisioning
- **US-6 [M]** As a tenant owner, I want the platform to push hotspot config to my router so that I do not configure RouterOS by hand.
  - Accept: create hotspot + assign profile triggers an idempotent, retryable config push; push failure surfaces on the hotspot as `error` with a retry action.

### Branding
- **US-7 [M]** As a tenant owner, I want to set my logo, colors, and name so that the captive portal looks like my brand.
  - Accept: saved in tenant settings; captive portal pages render the tenant branding on every hotspot.

### Vouchers & revenue
- **US-8 [M]** As a tenant owner, I want to generate batches of vouchers (duration/bandwidth/quantity/price) so that I can sell access without spreadsheets.
  - Accept: batch + rows inserted atomically; codes are unique and non-guessable (e.g. `XXXX-XXXX`); prices recorded.
- **US-9 [M]** As a tenant owner, I want to print a voucher sheet or PDF so that staff can sell physical tickets.
  - Accept: print-friendly view; PDF export; batch and single-voucher modes.
- **US-10 [S]** As a tenant owner, I want to see voucher sales/redemption so that I understand how much Wi-Fi I am selling.
  - Accept: dashboard shows issued vs redeemed vs expired/revoked; filter by period.

### Reporting
- **US-11 [S]** As a tenant owner, I want a month-end usage + sales report so that I can reconcile my books.
  - Accept: report aggregates sessions, traffic, and revenue per hotspot; CSV export; values come from persisted usage records.

## Tenant Staff

### Selling vouchers
- **US-12 [M]** As a staff member, I want to generate a single voucher on the spot so that I can serve a walk-in customer fast.
  - Accept: generates one redeemable code under the selected profile; fits the storefront interaction in a few clicks.

### Session support
- **US-13 [M]** As a staff member, I want to see live sessions so that I can answer a customer "how is my connection?"
  - Accept: session list with user, MAC/IP, uptime, and traffic; updates push via WebSocket within 5 s.
- **US-14 [M]** As a staff member, I want to force-disconnect a specific session so that I can stop a stalled or troublemaking device.
  - Accept: authorized action targets a single session; disconnect is persisted and reflected live.

## Technician

### Router health
- **US-15 [M]** As a technician, I want a live router status view so that I can spot offline devices remotely.
  - Accept: per-router status from worker probe; status changes pushed via WebSocket; historical downtime noted.
- **US-16 [S]** As a technician, I want to test connectivity to a router on demand so that I can check a device during setup.
  - Accept: manual probe button returns current reachability + latency/error.

## Guest

### Redeem voucher
- **US-17 [M]** As a guest, I want to enter my voucher code on a simple branded page so that I can get online immediately.
  - Accept: valid code → session starts → redirect to success page; invalid/expired/revoked/already-used codes show a clear error; redeem endpoint is rate-limited.

## Cross-cutting stories (hardening)

- **US-18 [S]** As an auditor, I want privileged actions logged so that changes to tenants, routers, and vouchers are traceable.
  - Accept: audit entries for tenant changes, router config, voucher revocation (who/what/when); log is queryable by admin.
- **US-19 [S]** As a platform operator, I want rate limiting on login and redeem so that brute-force attempts are blocked.
  - Accept: configurable lockout after N failed logins; rate limit on portal redemption per IP.
- **US-20 [C]** As a guest, I want to buy a voucher online so that I can self-serve instead of asking staff. *(post-MVP)*
  - Accept: payment gateway records linked to vouchers; voucher auto-issued on successful payment.

## Story map → delivery

| Persona/area | Sprint/phase |
|---|---|
| Guest redeems (US-17) | Phase 3 |
| Staff live sessions (US-13, US-14) | Phase 4 |
| All others | their feature phase (PHASES.md) |

Priorities: **[M]** stories form the MVP (PRD §8). **[S]** ship within the related feature phase. **[C]** deferred.