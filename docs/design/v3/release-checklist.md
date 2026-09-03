# Full v3 release checklist

## Deployed build — 3 September 2026

Development **2.0.0** is live from commit
`219cac1a89dd00a829a28cd683c8447d89f4a36e` on the branch below.
This deployment verifies the implemented build; **the full v3 specification
is not complete**. The unchecked functional requirements remain open.

- [CI passed](https://github.com/andranikasd/marumbot/actions/runs/33733922684),
  including race tests, database tests, migration reapplication, both CPU
  architectures, bundles, image build, lint, vulnerability and secret checks.
- [Development deployment passed](https://github.com/andranikasd/marumbot/actions/runs/33734213874),
  including authenticated Telegram-menu smoke. No production deployment,
  release tag, or merge into main was performed for this build.
- Live health and readiness return `2.0.0`, database ready, schema version 11.
  Mini App and admin sign-in render `2.0.0`; the dev bot's global launch URL
  is `https://dev.marum.loan/app/?v=2.0.0`.
- Profile-name throttling is isolated from menu publication. The smoke check
  now gives menu rollout its own startup deadline and compares exact versions.
- Grafana's optional deployment annotation returned HTTP 503 while its instance
  was loading; this did not affect application deployment or smoke verification.

The original full-scope acceptance gates follow. Deployment does not imply
that payment recording, advanced planning, role-scoped admin workflows,
remaining setup/settings/alerts, or real-participant field validation are done.

Implementation and release branch: `feature/v1.1.0-complete-ui-redesign`.
Release target: **v2.0.0 — DEVELOPMENT ONLY**, after full implementation.
No production environment exists; this release has no production promotion step.
The user approved implementation of the design and the full functional scope,
including backend, engine, bot and admin work. Publishing these design files
does not establish that the application implements the specification.

The source of acceptance criteria is `specification.md` sections 14–15.
`implementation-review.md` records the inspected gaps and semantic decisions.
Existing functionality must be verified against these criteria rather than
counted as complete from the presence of a screen or endpoint.

## Required implementation and evidence

- [ ] Budget foundation: effective-dated versions; independent cash and spending
  limits; positive/negative overrides; dated confirmed/expected cash; reserve,
  carry and released-payment rules; mid-period and increased-minimum guards.
- [ ] Payment recording: amount, transaction/value date, posting state and trust;
  separate future intentions; append-only corrections; duplicate protection;
  dated snapshots; actual-versus-plan variance and freshness.
- [ ] Planning: named strategy comparison on identical normalized inputs;
  isolated scenarios and activation history; inverse-budget boundary proof;
  progressive budgets; supported rate events or typed refusal; dynamic search
  checked against an independent oracle; honest bounds and stress results.
- [ ] Data and API: immutable source versions and activation provenance;
  ownership and aggregate-version checks; idempotent mutations; currency,
  quantum, as-of date and reliability in financial responses; compatible links.
- [ ] Mini App: approved compact layers and neutral dark theme; five root tabs;
  visible editing; persisted loan icon picker defaulting to Bank; one selectable
  chart with principal, monthly payments and interest comparisons; activity,
  alerts, setup and settings connected to real commands and reports.
- [ ] Bot: concise labeled actions; required/optional reminders; amount/posting
  capture; exact resolution links; private-chat protection; stale-plan handling.
- [ ] Admin: server-side roles and purpose audit; classified reconciliation;
  independently verified policy publication; golden corpus and historical
  replay; support, notifications, entitlements and scoped feature controls.
- [ ] Verification: independent financial golden fixtures, race tests, lint,
  authorization and concurrency checks, browser flows, edge assets and smoke.

## Development v2.0.0 promotion gates

- [ ] Complete all required implementation and evidence above before promotion.
- [ ] Commit and push the completed implementation on the requested branch.
- [ ] Pass CI and review the complete development v2.0.0 release diff.
- [ ] Promote the verified branch commit to **dev only** using the manual route below.
- [ ] Verify the complete application in dev, including schema and module smoke,
  and confirm its actual version is `2.0.0` and its health checks pass.

Intended route, only after full implementation and the gates above: GitHub
Actions → **CD · dev** (`.github/workflows/cd-dev.yml`) → **Run workflow** →
select `feature/v1.1.0-complete-ui-redesign` at the verified commit → set
`version` to `2.0.0` (no `v` prefix) → **Run workflow**.
The existing `workflow_dispatch` version validation accepts `2.0.0`; it stamps
and publishes the development image, then calls the reusable `deploy.yml` with
`environment: dev`, `wrangler_env: dev`, and `version: 2.0.0`.

These are future promotion instructions, not a deployment request now. Do not
create or push a Git tag or publish a GitHub Release for this route. Do not run
**CD · prod**; it is retained as manual-only for future separate authorization.

## Field evidence

The specification additionally requires participant usability checks and two
months of bank reconciliation. These require real participants, bank evidence
and elapsed time; automated tests or synthetic three-loan demonstrations cannot
be substituted for them. Report this status explicitly with any software release.

## Implementation checkpoint — 2 September 2026

Implemented in this branch, not released:

- Opt-in separation of monthly funding and calendar-month spending permission,
  expected receipt exclusion, spent-this-month accounting, required-payment and
  reserve guards. Legacy funded-budget input retains its interpretation.
- Budget declaration history, version conflict rejection, funding persistence;
  budget/funding/month tabs with actual save operations.
- Explicit stored loan icons (Bank default), optional-allocation exclusion,
  dated zero-capable balance statements, retained same-day snapshot history.
- Home/Plan/Loans/Activity/More navigation; one selectable principal/payment/
  interest chart; separate plan detail layers; real source-history activity.
- Shared normalized input hashes, minimum timeline, currency precision and
  search evidence in plan responses; bounded attempted/feasible policy counts.
- Neutral charcoal/green design tokens; edge stylesheet includes shared tokens;
  expired edge asset versions refuse rather than serve different immutable bytes.
- Bot balance prompt no longer claims a payment was recorded before a write.

Still required before marking the full specification complete:
Quick Record and posted-payment lifecycle, idempotency on every mutation,
future-effective budget/scenario activation and replay, additional named
strategies and inverse-budget domain proof, configurable carry/growth and
cash earmarking, actual-versus-plan reconciliation, role-scoped admin workflows,
policy publication controls, advanced rate/search/stress support and the
remaining setup/settings/alert screens. Existing goal approval is not an
immutable historical plan and must not be described as one.

Verification so far includes independent zero-interest budget cases, the
existing lender corpus, local migration/database regressions, and browser
interaction checks using the synthetic three-loan fixture. A release remains
blocked on the unchecked implementation gates above, not on design approval.

The required-only baseline now forbids optional early closures in legacy mode
as well. For the deterministic three-loan fixture this changes minimum interest
from 312703040 to 312703070 minor AMD units; the optimized plan is unchanged.
The revised baseline has zero optional outflow and its required payments equal
550000000 principal plus 312703070 interest. The report fingerprint records the
intentional correction and the plan/3 label.

Checkpoint verification completed: `make fmt`, `make lint` (zero issues),
`make test` (race enabled), local PostgreSQL integration tests with migrations
10–11, TypeScript checking, all Mini App module syntax checks, and Wrangler's
`deploy --dry-run --env dev` including its container build. Chromium at 390×844
passed five-tab navigation, chart switching, separate payment view, icon
prefill, budget field retention and save using the synthetic fixture.

## Implementation checkpoint — 3 September, payment source facts

**Not deployed. Full v3 remains incomplete.** These changes extend the requested
feature branch; the development deployment recorded at the top is unchanged.

Implemented and exercised:

- Quick Record opens from a loan or the bot's paid callback. It captures amount,
  transaction date, unknown/known bank value date, and required/extra intent.
  Acknowledging a reminder alone never creates a payment. Entered facts remain
  `user_entered`; selecting posted never upgrades trust to bank-confirmed.
- Payment writes lock the owned loan, compare its ledger version, detect likely
  duplicates, and preserve idempotent retries. Corrections atomically append a
  void and replacement. Pending entries have no invented value date.
- Activity includes statements, payments and voids, with correction/posting and
  void actions. Immutable cursors keep older records accessible beyond 100 rows.
- Unreconciled facts block new plans and projected repayment figures across the
  Mini App, bot and reminders. Required reminders retain their date with a review
  message instead of asserting an unverified amount.
- Payment forms and validation share the account's business date. Uncertain
  saves freeze the submitted fields and retry the same key and payload.
- Telegram inbound financial commands require matching private sender/chat IDs;
  outbound messages also reject non-private destinations left in old bindings.
- Inverse-budget bisection refuses unproven domains with `NonMonotoneError`.
  Its currently proven domain is one zero-interest loan with aligned supplied
  instalments, immediate credit, constant funding and due-calendar payments.
  The existing multi-loan numerical smoke test now requires that refusal;
  an independent fixture proves the supported minimum and one-quantum boundary.

Evidence: real PostgreSQL tests cover same-key concurrent retries, conflicting
writers, ownership, duplicate detection, atomic rollback after a failed
replacement, immutable history, and cursor pagination under new inserts.
Migration 12 reapplies after its preservation-only rollback. Mobile browser
checks at 390×844 cover pending entry, posting fields and correction prefill;
these browser records are synthetic, never live account data.
The lost-response browser check returns one record after retry. A CI screen
lifecycle test covers navigation while a save is pending, restoration of two
independent retry identities, and editing after a declined duplicate warning.
The full race suite, store integration suite, lint and module syntax checks pass.

**Still blocking release:** Quick Record has no bank-allocation/balance
reconciliation command yet, so a genuine recorded payment pauses further
planning. Completing that lifecycle and current-period cash/spending accounting
is the next dependency; voiding a genuine payment is not a reconciliation fix.
Actual-versus-plan attribution, activation provenance, remaining budget and
strategy capabilities, role-scoped admin, setup/settings/alerts and the original
field evidence gates remain open. Do not mark the broader payment, planning,
admin or release checkboxes complete from this checkpoint.

The next development build must use a distinct version for changed immutable
assets; do not publish different application bytes under the existing `2.0.0`
asset paths. No production promotion or release tag is part of this checkpoint.

## UI corrections — 3 September, screenshot review

Not deployed. The screenshot fixes remain on the same feature branch:

- Chart values no longer inherit the narrow summary-value column. Currency
  precision is consistent, and solid/dashed legends identify the plotted series.
- Home and Loans separate icons, names, balances and payment details. Input
  units participate in layout rather than overlapping fields.
- A visible `Հայ / EN` header button opens language settings. Authenticated
  Mini App settings share the bot's stored locale, including on reopening;
  the bot keyboard also exposes its language action. Deep-linked forms resolve
  that setting before mounting.
- Concurrent reads share one request; opening Home no longer eagerly computes
  a plan. This removes redundant work, not a measured end-to-end latency claim.

Local evidence: app and Mini App race tests, lint, transport invalidation and
payment retry regressions pass. A synthetic three-loan preview at 390×844
verified chart labels, unbroken amounts, cleaner loan cards and Armenian/English
switching with persistence on reopening. A physical Telegram/iOS check remains.
Live readiness still reports `2.0.0`, schema 11. These corrections do not close
the payment reconciliation or broader full-v3 release gates above.

## Reconciliation implementation — 3 September

User authorized completion of the software gates and development deployment;
the final Telegram/iPhone check will be performed by the user afterwards.

The reconciliation command now records a user-reported current balance, next
unpaid contractual due date/amount, and explicit after-payment cash and total
period spending. It atomically appends a snapshot and event coverage with a
budget revision. It requires known posting, checks both aggregate versions,
rejects understated reported spending, and preserves retries across day changes.
Correcting a covered payment requires a new statement. No reported amount is
silently subtracted from principal or cash, and trust remains user-entered.

The planner respects the bank-reported first unpaid date and supplied annuity
instalment, and does not credit receipts already included in stated cash.
A declining schedule that contradicts the supplied instalment refuses rather
than dropping the debt from a portfolio. Contractual day changes require a
contract revision; this command does not invent a replacement schedule.

Evidence: PostgreSQL lifecycle and race tests; independent zero-interest
already-paid-instalment and payday double-counting regressions; authenticated
handler/application tests; lost-response/two-loan form retry test; synthetic
browser traversal of both reconciliation steps and save. Migration 13 preserves
source statements on rollback. Smoke now checks schema 13 and the new module.

Still open: actual allocation/variance reporting, broader budget/scenario/
planning requirements and admin integration. Admin capability rules and budget
growth normalization are being implemented separately and are not connected
product features merely because their isolated tests pass. No new dev deployment
has been performed at this checkpoint.
