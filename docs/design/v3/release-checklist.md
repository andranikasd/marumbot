# Full v3 release checklist

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
