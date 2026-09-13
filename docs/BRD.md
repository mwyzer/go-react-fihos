# Business Requirements Document (BRD)

## 1. Background

Businesses running MikroTik routers want to monetize Wi-Fi but today depend on manual processes: per-device CLI configuration, spreadsheet-managed voucher codes, and no live visibility of who is connected or how much they used. This manual approach makes Wi-Fi monetization viable only for large operators, leaving small venues and WISPs stuck giving away a revenue channel.

FIHOS addresses the gap between heavyweight ISP AAA/billing platforms and single-site tools, targeting the middle market: multi-location hotspots with voucher-driven access.

## 2. Business objectives

| Objective | Success measure | Target (v0.1+ ramp) |
|---|---|---|
| Reduce venue onboarding time | From router unboxed to selling the first voucher | < 10 minutes (from 30+ min manual setup) |
| Create a new revenue line for venues | Share of venues selling prepaid access | Every onboarded tenant enabled to sell |
| Accurate, billable usage data | Ratio of ended sessions to matching usage records | 100% (no orphan records) |
| One platform serving many tenants | Tenants onboarded per quarter | Defined by platform capacity/tiers |
| Run as a SaaS + licensing business | Recurring revenue per tenant/location | Monthly subscription + optional self-host license |

## 3. Stakeholders

| Stakeholder | Interest | Benefit |
|---|---|---|
| Platform operators / ISPs | Run hotspot monetization as a service | Recurring SaaS/licensing revenue, low-touch onboarding |
| Tenant owners (cafe, hotel, office) | Sell Wi-Fi, brand the portal, understand usage | Fast voucher sales, quantified Wi-Fi value |
| Tenant staff | Sell/issue vouchers quickly, fix stalled sessions | Simple UI, no router CLIs |
| Technicians | Set up and maintain router fleets | Central status, reliable config push |
| Guests/customers | Convenient paid / time-limited internet | Simple captive portal, instant redemption |

## 4. Business scope (in)

1. Multi-tenant platform with strict data isolation (SaaS-ready).
2. Router (MikroTik) registration, status monitoring, and automated hotspot provisioning.
3. Voucher lifecycle: batch generation, print/PDF, search, revocation — redeemed once, single-use.
4. Branded captive portal per tenant/hotspot with code redemption.
5. Live session monitoring via WebSocket with force-disconnect.
6. Session/usage logging as the billing source of truth.
7. Sales and usage analytics with exportable reports.

## 5. Business scope (out)

- Wi-Fi hardware/provisioning and firmware management.
- Custom per-session non-voucher tariffed billing.
- Social login / identity federation for guests in v1.
- Online payments for self-service voucher purchase (post-MVP).

## 6. Business rules

- BR-1 A tenant owns its data; no cross-tenant access is possible.
- BR-2 A tenant lifecycle is `active` → `suspended`/`closed`; suspended tenants are blocked platform-wide.
- BR-3 A voucher is single-use; redeemed/expired/revoked codes are never accepted again.
- BR-4 Router credentials are encrypted at rest and never returned to any client.
- BR-5 Every ended session produces a persisted usage record (billing source of truth).
- BR-6 Voucher sales and usage are reportable per tenant, router, and hotspot.
- BR-7 Venue branding (logo, colors, name) is configurable per tenant; portal renders it on every hotspot page.
- BR-8 Privileged actions (tenant changes, router config, voucher revocation) are audited.

## 7. Success metrics (business KPIs)

- Time-to-onboard a new tenant/router: < 10 minutes.
- Voucher redemption rate (issued vs. used) — tracked per batch/period.
- Billing accuracy: 100% of sessions end with a matching usage record.
- Dashboard freshness: live status/session updates reflect < 5 s (WebSocket).
- Concurrency capacity: active sessions and per-hotspot peaks within documented limits.
- Platform availability: 99.9% for API and router-sync.

## 8. Constraints & assumptions

- **Hardware:** MikroTik RouterOS boxes (per venue). Rest API surface must be validated against RouterOS versions in the field (device lab).
- **Connectivity:** On-premises routers are reachable from the platform (public IP or VPN/tunnel).
- **Compliance:** Where vouchers imply resale of internet access, local telecom rules may apply; customer can leverage the audit + usage records to demonstrate compliance.
- **Payments:** Payment collection is out of scope initially; voucher prices are recorded but settled offline.

## 9. Dependencies

- MikroTik RouterOS API for provisioning and session control.
- PostgreSQL for transactional data and usage records; Redis for sessions, rate limiting, and caching.
- WebSocket transport for realtime dashboard updates.
- (Post-MVP) Payment gateway integration for self-service top-ups.

## 10. Risks

| Risk | Impact | Mitigation |
|---|---|---|
| MikroTik API variance across RouterOS versions | Config push failures | Mock/test device lab; idempotent, retryable pushes (buffer sprint) |
| Router offline during redemption | Guests cannot get online | Clear errors + retry path; worker probe detects offline routers |
| Multi-tenant isolation breach | Data leakage | Tenant-context enforcement on every route; integration tests incl. cross-tenant cases |
| Voucher misuse/brute force | Revenue loss | Cryptographically random codes; rate limiting on redeem |
| Manual billing reconciliation persists | Stale revenue data | Usage records as single source of truth; exportable reports |

## 11. Business model summary

- **SaaS subscription** per tenant/location/month (basis: active hotspots + voucher volume tier).
- **Platform licensing** for large ISP deployments (self-hosted).
- **Future transaction fee** once online payments land (post-MVP). Voucher price levels remain a tenant business decision.

See PRD.md for product-level details, SRS.md for functional/non-functional specifics, and PHASES.md for delivery sequence.