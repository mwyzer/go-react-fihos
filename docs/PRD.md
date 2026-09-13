# Product Requirements Document (PRD)

## 1. Product overview

**FIHOS (Free Internet Hotspot Operating System)** is a multi-tenant, cloud-based hotspot management platform for MikroTik routers. It lets internet service providers (ISPs), cafe owners, hotels, and small businesses provision and sell prepaid Wi-Fi access through voucher-based tickets and a captive portal — all from a single web dashboard.

The platform is a full-stack web application:

- **Frontend:** React (web dashboard for admins/tenants)
- **Backend:** Go/Gin REST API plus a background worker
- **Data:** PostgreSQL (primary), Redis (caching, sessions, rate limiting)
- **Integration:** MikroTik RouterOS API for router/hotspot provisioning
- **Realtime:** WebSocket for live session status and connectivity monitoring

## 2. Problem statement

Small and medium businesses rely on MikroTik hotspots to monetize Wi-Fi, but managing them today requires:

- Manual router-by-router configuration and no central overview.
- Printing/issuing voucher codes in spreadsheets, error-prone and slow.
- No way to see live who is connected, how long, or how much bandwidth they used.
- Billing that is disconnected from actual usage, forcing manual reconciliation.
- A captive portal that is generic or hard to brand per location.

There is no lightweight, multi-tenant tool that combines router provisioning, voucher sales, captive portal branding, billing, and analytics for operators of many independent hotspot locations.

## 3. Goals & objectives

- **Centralize:** Manage all routers and hotspots of a tenant from one dashboard.
- **Automate provisioning:** Push hotspot and firewall config to MikroTik routers via API instead of manual steps.
- **Simplify Vouchers:** Role-based vouchers (time limit, bandwidth limit, etc.) generated, printed, sold, and revoked from the UI.
- **Brand the captive portal:** Per-tenant, per-hotspot branded login pages.
- **Monetize usage:** Accurate usage accounting so billing is straight-forward.
- **Scale tenants:** One deployment serves many independent tenants with strict data isolation (multi-tenancy).

## 4. Target users

- **Platform operators / ISPs** running Wi-Fi monetization as a service.
- **Coffee shops, restaurants, hotels** selling prepaid Wi-Fi to customers.
- **Small offices / campuses** issuing time-limited access to guests.
- **Field technicians** configuring routers at tenant sites.

## 5. User personas

| Persona | Role | Goals | Pain points |
|---|---|---|---|
| **Platform Admin** | Super admin of the whole deployment | Manage tenants, oversee usage, configure global settings | No cross-tenant visibility, manual tenant onboarding |
| **Tenant Owner** (e.g. cafe owner) | Runs one business and the hotspot hardware | Sell fast, brand portal, understand usage | Spreadsheet vouchers, unquantified Wi-Fi value |
| **Tenant Staff** | Frontline staff at a venue | Issue / sell vouchers quickly, reset a stalled session | Slow workflows, CLI-only router access |
| **Technician** | Sets up and maintains MikroTik routers | Add/remove routers, apply configs reliably | Manual CLI config, no central status |

## 6. Core features

### 6.1 Authentication & roles
- Secure login for admins, tenants, and tenant staff.
- Role-based access control (see SRS: Roles & permissions).
- Token-based sessions backed by the Go API + Redis.

### 6.2 Multi-tenancy
- Dedicated tenant workspaces with full data isolation.
- Tenant lifecycle: onboarding, suspension, offboarding.

### 6.3 Router management
- Register and manage MikroTik routers per tenant.
- Push hotspot configuration to routers.
- Live status of each router (online/offline) via WebSocket.
- Display connection and hotspot status.

### 6.4 Hotspot management
- Create and configure multiple hotspots per router.
- Configure captive portal, user profile limits, up/down bandwidth.
- Assign hotspot profiles (speed caps) to tenants.

### 6.5 Voucher management
- Generate voucher batches with configurable parameters (duration, bandwidth, validity, quantity).
- Print-friendly voucher sheets and PDF export.
- Search, deactivate, or invalidate vouchers individually or in bulk.
- Sales tracking of issued vs. redeemed vouchers.

### 6.6 Captive portal
- Branded, per-tenant HTML login pages.
- Voucher code redemption and automatic session creation on the router.
- Extensible theming (colors, logos, terms).

### 6.7 Sessions & realtime monitoring
- Live list of active sessions per router/hotspot via WebSocket.
- Session lifecycle: login, keepalive, logout, force-disconnect.
- Usage counters (bytes/up-down, duration) synced from routers.

### 6.8 Billing
- Usage-based accounting derived from voucher types and session usage.
- Reports for sales and usage per tenant/hotspot.
- (Future) online payment gateway integration. See 9.1.

### 6.9 Analytics
- Dashboards: active sessions, revenue by period, voucher redemption rate.
- Per-router and per-hotspot usage breakdown.
- Exportable reports.

## 7. User journeys

### 7.1 Technician sets up a new venue (Multi-tenant hotspots)
1. Platform admin creates the tenant and assigns a hotspot profile.
2. Technician adds the MikroTik router in the dashboard (name, IP, credentials).
3. System verifies connectivity and pushes the hotspot configuration.
4. Technician creates a hotspot and marks it active; status flips to online.

### 7.2 Staff sells a voucher (Vouchers + sales)
1. Staff opens the Vouchers screen and generates a batch (e.g. 20 × 1 hour).
2. A printable sheet is produced; or a single voucher is generated on the spot.
3. Customer redeems the code on the captive portal and gets online.

### 7.3 Customer connects (Captive portal + sessions)
1. Guest opens Wi-Fi, is redirected to the branded captive portal.
2. Guest enters the voucher code; API verifies validity.
3. Router starts the session; guest is online with the assigned profile.
4. Staff sees the session live in the dashboard; on expiry it is terminated.

### 7.4 Owner reviews the month (Analytics + billing)
1. Owner opens the dashboard, views revenue and usage for the period.
2. Export the report for reconciliation and planning.

## 8. MVP scope

For the first release (v0.1):

1. Authentication core + roles (admin, tenant, tenant staff).
2. Basic multi-tenant onboarding (admins create tenants).
3. Router registration and status (ping/live check via WebSocket status flag).
4. Hotspot creation + MikroTik config push for the primary use cases.
5. Voucher generation in batches, voucher print view, deactivation.
6. Captive portal with voucher redemption and branded tenant page.
7. Active session listing with polled/WebSocket updates and force-disconnect.
8. Dashboard with simple usage metrics (vouchers sold, sessions, traffic).
9. Session log for accurate billing records.

## 9. Non-MVP scope

### 9.1 Post-MVP (later release)
1. Online payments (card / mobile money) for self-service voucher purchase.
2. Advanced analytics (cohorts, heatmaps, export pipelines).
3. Automatic voucher sales alerts / threshold notifications.
4. Full RBAC UI (custom roles per tenant) — start with fixed roles.
5. Customer self-service portal (check own balance/expiry).
6. Multiple router vendor support (beyond MikroTik).

### 9.2 Explicitly out of scope
1. WiFi hardware / SFP provisioning and firmware management.
2. Radically custom per-session pricing (non-voucher tariffed billing).
3. Social login/identity federation for guests in v1.

## 10. Business model

Proposed (adjustable by ownership):

- **SaaS subscription** per tenant/location per month.
- Basis: number of active hotspots and voucher volume tiers.
- Optional **platform licensing** for large ISP deployments (self-hosted).
- Voucher resale at tenant level is a tenant business; the platform can take a small transaction fee once payments land (9.1).

## 11. Success metrics

- **Time to onboard a new tenant/router:** to < 10 minutes from 30+ min manual setup.
- **Voucher redemption rate:** % of generated vouchers actually used.
- **Active daily sessions per hotspot** and concurrency peaks (capacity).
- **Billing accuracy:** 100% of sessions end with a matching usage record (no orphans).
- **Dashboard data freshness:** session/status updates reflected in < 5 seconds via WebSocket.
- **Uptime:** platform API/router sync 99.9% availability target.