# Marum v3 implementation review

Status: design for review; no application changes in this pass.
Baseline inspected: de1e0ab, 2 September 2026.
Source: specification.md, supplied by the user. External product claims in that document have not been independently reverified.

## Proposed experience

Home opens on the next action, followed by this period's budget, attention items and the active plan. Root navigation: Home, Plan, Loans, Activity, More. Budget opens from Home and Plan. Financial forms open as sub-screens with back navigation and unsaved-change protection.

Dark mode revision 2 uses neutral charcoal (#101113), surface (#191b1e), raised controls (#22252a), off-white text (#f1f2f4), and restrained green actions (#256345). Backgrounds and fields have no green tint. The new specification supersedes the old brass Approve treatment: activation uses green. The preview uses system typography with Armenian fallbacks; production can self-host Inter and Noto Sans Armenian. Body text is at least 16px and controls at least 44px.

Quick Record collects amount and transaction date, then posting state and optional bank snapshot. A pending record says "Pending bank posting"; tapping a reminder never says "Payment recorded" before a fact is saved. A projected payoff is distinct from a bank-confirmed closure.

Admin opens on correctness work. Role enforcement belongs in authenticated application commands, not hidden buttons. No operator can overwrite loan or billing history.

## Repository verification requested by specification §16

| Check | Current evidence | v3 consequence |
| --- | --- | --- |
| Budget/CashPlan | internal/app/loans.go: Budget; pkg/core/plan/input.go: CashPlan. Monthly supplies income on PayDay in sim.go. | Separate period spending permission from dated funding. Existing amounts must not silently become declared salary. |
| Overrides | MonthlyOverrides explicitly replaces Monthly, keyed YYYY-MM. | Retain replacement meaning; add explicit delta intent and checked normalization. |
| Payment intent/actual | internal/app/paid.go asks for a balance; the prompt already says Payment recorded. No amount/posting capture in that flow. | Introduce a distinct record workflow and ledger admission rules. |
| Expected funds | CashEvent only has On and Amount. | Certainty is absent; exclude expected cash from base feasibility until scenarios are explicit. |
| Excluded loans | Position has no exposure/guard/allocation flags; queries/loans.sql excludes archived loans. | Archival cannot implement tracking-only debts with required cash protection. |
| Search truncation | search.go drops permutations above 4096 candidate combinations; loop order is order/effect/timing/batch. | Recheck the post-fallback bound and report actual counts; do not describe this as dynamic exhaustive search. |
| Inverse budget | BudgetFor bisects for a supplied fixed policy; failures become false and monotonicity is assumed under carry. | Not yet a globally minimal budget over all strategies. Add typed domain/refusal checks and boundary proof. |
| Cache | internal/app/plancache.go hashes EngineVersion plus formatted Input and Goal. Its no-maps comment is stale: overrides are a map. | Define explicit canonical versioned encoding; prove sensitivity to every financial input. This inspection does not establish a cache collision. |
| Reports | Sheet exposes currency, goal, summary and monthly rows. ApprovedPlan is a goal commitment and current sheet is recomputed. | Add immutable activation provenance and historical replay; the current sheet is not a frozen historical plan. |
| Routes/deep links | Mini App uses /api routes, screen and id query parameters. | Preserve old links during migration; introduce entity-aware return paths and new versioned routes. |
| Admin | server.go has one password/session guard and operational pages. | Multi-role authorization, purpose audit, two-person publication and step-up are new work. |
| Edge assets | Worker forwards /app/api/*; otherwise prefers ASSETS. It strips version prefix and labels responses immutable. | Route new /v1 APIs explicitly. Verify edge token packaging separately from Go's stylesheet concatenation; an immutable URL must never serve different bytes. |

## Semantics to settle in the implementation

- The worked example calls opening cash "usable" then subtracts a reserve later. Specify that opening cash is gross available cash and reserve is a separate floor; do not subtract it twice.
- Future projections may assume released payments after a projected closure. Actual household permission changes only on bank-confirmed closure/reissue. Without this distinction future rollover cannot be simulated meaningfully.
- Apply the reserve/due-reserve inequality to optional actions. A required-payment failure must report the contractual shortfall, not silently skip the payment.
- Period permission and cash are independent: additional cash cannot automatically increase the approved spending limit. Changes to either need an explicit user intent.
- Never cap monetary formatting at two decimal places if the currency's settlement quantum needs greater precision.
- Unknown lower bounds/gaps stay unknown; no fabricated zero gap.
- Store source facts, user intent and activation provenance. Derived results should remain reproducible; decide whether immutable report snapshots are justified before adding every proposed table. The repository explicitly requires human review for persisting recomputable data.
- A replacement/void must preserve the existing append-only reversal invariant; do not invent mutable history to match a screen label.

## Delivery sequence and release gates

1. **Budget foundation:** effective-dated budget versions, separate funded cash/period permission, overrides, certainty and required-payment guards. Tests: independent golden cases for insufficient cash despite sufficient limit, insufficient limit despite sufficient cash, negative delta, two cash events, reserve, already-paid instalments and increased minimum. Legacy adapter preserves old behavior until users explicitly configure new funding.
2. **Record and Easy Mode:** amount/date/posting capture, snapshot commands, bootstrap and Home, five root tabs, freshness, alert resolution. Tests: duplicate submissions, stale versions, ownership, future intent excluded from actual state and pending posting labels.
3. **Comparison/scenarios:** shared normalized input, named baselines, isolated what-if versions, inverse-budget domain checks, historical activation. Test one-quantum boundary and unchanged active state before activation.
4. **Admin correctness:** roles and purpose audit before support access, policy review, reconciliation classifications, historical replay and scoped flags. Test authorization server-side for every command.
5. **Advanced engine:** growth, supported rate events/refusals, dynamic search and stress cases. Require independent reduced oracle and honest search certificates before exposure.
6. **Field validation:** real participant setup and two months of bank reconciliation require participants, lender evidence and elapsed time. Automated tests cannot fulfill these acceptance criteria.

No UI should expose controls backed by placeholder calculations. Each phase ships as a complete supported slice with golden/race/lint, edge bundle and deployment smoke gates. Changing routes or splitting screen modules must update smoke assertions in the same change.

## Approval boundary

The earlier request requires a design before application code changes. Review preview.html and the semantics above first. This package does not claim that v3 is implemented or that the field-validation criteria have passed.

## Dark-mode revision 2

Preview coverage: 18 Mini App screen concepts, 13 admin page concepts, 14 bot conversation examples, and 98 editable field examples. These are review layouts, not connected features or completed production screens. The supplied v3 specification remains the functional scope.

Persistent labels, 48px input heights, neutral borders, visible keyboard focus, and text-backed validation states are shared across forms. Telegram owns native chat colors; message structure and inline keyboard order are the implementable bot changes. The preview defaults to dark and has a light-mode comparison. No network requests or account writes are made by the preview.

Home's corrective action is placed immediately after the next-action section, ahead of budget and progress. Setup forms in the preview now use progressive steps, with entries retained when navigating backward.

## Card-based revision 3

User direction: less text, separate task cards, simple everyday use. Home now shows one next-action card, Budget and Plan tiles, and a loan card. Other page sections use bordered cards with two visible facts and additional rows under Details. Financial forms show small groups of fields with Continue, Back and Review; entered values remain in preview memory while navigating. Bot examples show short copy and at most two immediate actions; full explanations are secondary. The preview makes no account writes.

## Icon revision 4

User preference: recognizable icons should help users find actions quickly. The preview now uses local SVG line icons for Mini App/admin navigation, task cards and form-section headings: wallet for budget, bank for loans, chart for plan, calendar for dates, receipt for recording, shield for privacy. Icons share one stroke and retain visible text labels; decorative SVGs are hidden from assistive technology. No external icon assets are fetched. Bot previews use one native emoji in message headings because Telegram cannot render custom SVG icons inline. This supersedes the earlier blanket no-emoji design preference for those headings only.

## Three-loan example

`three-loans.html` is a separate populated preview linked from the main design. It uses the synthetic Car/Home/Phone portfolio in `internal/app/advice_render_test.go:fixedReport`, valued 15 January 2026, calculated with the current plan/2 engine. `three-loans-data.json` preserves the calculated minor-unit values. These are demonstration terms, not verified bank products. The page shows first-cycle required/extra payments, savings versus minimum-only, starting balances, payoff milestones and a cycle slider for projected remaining principal. Every cycle's loan balances and payments were checked against its totals; the final total balance is zero. No actual repayment progress is implied.

## Populated Mini App revision

The three-loan page now uses the mobile Mini App layout, with Home, Plan, Loans, Activity and More navigation. Budget opens from Home or More; each loan opens a detail screen. Loan icons default to Bank and are explicitly chosen in a dropdown; names do not determine icons. The dropdown changes preview memory only.

The Plan screen shares one chart style for projected principal, monthly total payments versus their required portion, and monthly interest versus required-only interest. A common cycle slider updates all chart markers and exact amounts. Required-only data comes from the engine's minimum baseline using matching calendar months. Its interest chart covers the active plan horizon, not the entire longer baseline. Actual interest paid is not claimed because the demo has no posted payment records.

## Compact layers and editing

User preference: keep routine views near one screen; use layers instead of long scrolling. The populated Mini App now has one chart selected by a dropdown (balance, monthly payments, monthly interest), with one cycle slider. Payments, milestones and calculation details are separate views. Home and Loans use compact loan rows. Bottom navigation remains visible, with overflow available on smaller screens rather than clipping content.

Edit budget is visible in Budget; Edit loan is visible on loan rows and details; Update balance opens a separate snapshot form. Multi-block edit forms use tabs. Demo saves update in-memory fields only and explicitly mark the original calculation outdated. Existing charts never pretend to reflect edited terms. Browser interaction checks covered chart switching, budget saving, loan name/icon saving and snapshot saving.
