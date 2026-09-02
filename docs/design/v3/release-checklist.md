# Full v3 release checklist

Implementation and release branch: `feature/v1.1.0-complete-ui-redesign`.
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

## Publication gates

- [ ] Commit and push the implementation on the requested branch.
- [ ] Pass CI and review the complete release diff.
- [ ] Verify the complete application in dev, including schema and module smoke.
- [ ] Publish the release tag only for the verified implementation commit.
- [ ] Complete the production deployment and verify its actual version/health.

## Field evidence

The specification additionally requires participant usability checks and two
months of bank reconciliation. These require real participants, bank evidence
and elapsed time; automated tests or synthetic three-loan demonstrations cannot
be substituted for them. Report this status explicitly with any software release.
