# Product Idea & Vision

## 1. Elevator pitch

**FIHOS** turns any business running a MikroTik router into a paid Wi-Fi hotspot in minutes — with branded captive portals, printable voucher tickets, realtime session monitoring, and accurate usage-based billing. One dashboard, many locations, fully multi-tenant.

## 2. The one-liner

> "Serve and sell Wi-Fi on MikroTik from a browser — no CLI scripts, no spreadsheets."

## 3. Why this product

Large ISPs have full-blown AAA/billing platforms (too heavy), while single-site tools exist for standalone use (no multi-location management). The gap is the **middle market**:

- **Mom-and-pop venues** that buy a MikroTik to monetize Wi-Fi but don't want to learn RouterOS.
- **Small ISPs / WISPs** running many independent client locations and needing a per-location revenue picture.

They all share the same pain: prepaid access = vouchers, and vouchers today are manual everywhere.

## 4. Core belief (hypothesis)

> If a small business can generate, print, and sell vouchers from one dashboard — while the router config "just happens" — they will monetize Wi-Fi they currently give away for free.

The platform's job is to turn the most manual jobs (provisioning, voucher issuing, usage accounting) into zero-config automation.

## 5. What makes this different

| Dimension | Typical approach | FIHOS |
|---|---|---|
| Router setup | Manual CLI / vendor GUI per device | API-driven config push, central |
| Vouchers | Notebook / Excel / hand-written codes | Batched generation + printable sheets |
| Multi-site | Separate installs per site | One multi-tenant platform |
| Live view | Router console | WebSocket-driven dashboard |
| Billing | Estimated / disconnected from usage | Sessions logged end-to-end |

## 6. Inspiration & reference points

- MikroTik's native Hotspot feature (proven concept, terrible management UX).
- Managed-hotspot SaaS offerings for enterprise (feature-rich, over-priced for small venues).
- The model of simple prepaid/scratch-card utilities in emerging markets where voucher Wi-Fi is standard.

## 7. Problem validation checklist (open questions)

- [ ] Confirm target segment: cafe/hotel operators vs. small WISPs (different onboarding needs).
- [ ] Validate willingness to pay for a standalone dashboard vs. free RouterOS webfig.
- [ ] Confirm the primary monetization driver: voucher sales reporting vs. session control.
- [ ] Validate minimum router API surface against the MikroTik versions in the field (AWS test/dev device).

## 8. Vision statement (3 years)

FIHOS is the default Wi-Fi monetization layer for small-to-mid venues and WISPs: ten minutes from router unboxed to selling first voucher, with payments, self-service top-ups, and cross-location analytics — a single platform their staff can run without training.

## 9. Success looks like

A venue owner can stand up that day:
1. Create account → add router → push config → hotspot live (under 10 minutes).
2. Print a voucher sheet on a plain printer and start selling.
3. Open the dashboard and see every connected session, live.
4. At month-end, pull the revenue/usage report for the books.