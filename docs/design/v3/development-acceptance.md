# Development acceptance evidence

Branch: `feature/v1.1.0-complete-ui-redesign`.
Target: development `2.0.1`, a new immutable asset version after `2.0.0`.
This document describes software evidence. Deployment status and run links belong
in `release-checklist.md`; this file does not claim a deployment happened.

## Payment lifecycle and current-period accounting

- Reported, pending and posted payments retain original transaction/value dates.
  Optional bank-reported principal/interest/fee allocation must sum exactly to the
  payment; an absent allocation remains unknown. User entry never becomes
  bank-confirmed merely because the user selects posted.
- Corrections append reversals and replacement facts. Reconciliation atomically
  records a dated statement, event coverage and declared after-payment cash and
  spending, guarded by loan and budget versions. Covered-payment corrections
  require a fresh statement. Required dates are preserved; no inferred principal
  subtraction or missing-loan omission repairs a contradiction.
- Spending periods are independent of income and reporting periods. Regressions
  cover payday 10, cycle start 15, payment 20 and policy change 25, including a
  later payday within the same spending period.
- Activity shows known allocation coverage and actual-versus-approved-plan
  variance, with typed causes and original activation boundaries.

Evidence: `payments_test.go`, `reconciliation_test.go`, `plan_actuals_test.go`,
`budget_policy_acceptance_test.go`, core spending/carry tests, PostgreSQL
payment/reconciliation/actuals tests, and Mini App payment/reconciliation tests.

## Planning and budget workflows

- New calculations require explicitly declared cash funding. Spending permission,
  confirmed receipts, expected receipts, reserve and already-spent amounts are
  separate. Historical manifests retain their original interpretation.
- Effective-dated budget declarations support fixed/percentage growth with caps,
  positive/negative changes and whole-period replacements, carry and confirmed
  released-payment policies. Editing future receipts preserves reconciled cash,
  spent totals, reserve and approved permission rules.
- Cash routing supports pooled allocation, whole-event splits and earmarks,
  date/threshold holds and explicitly reconfirmed retained cash. Past routing is
  not silently erased or inferred as cash still available.
- Strategy comparison uses one normalized source identity. Activation preserves
  the exact displayed policy; a named baseline does not inherit the optimizer's
  optimality claim. Bounded results disclose search coverage and unknown bounds.
- Scenarios preserve original inputs and changes; preview/save do not activate.
  Activation applies the budget declaration and plan together with source and
  version checks. Historical exports/replay retain original versions; stale
  current proposals cannot export fresh-looking payment instructions.
- Loan and budget commands retain durable retry identities. A lost response does
  not duplicate facts, restore old data, or consume a second aggregate revision.
  Statement dates are pinned across midnight. Ownership and concurrent-writer
  tests exercise the real database.

Supported-domain boundaries remain explicit: inverse budget has a proven
fee-free zero-interest domain; unproven inverse cases refuse. Dynamic search is
verified against a reduced independent oracle, not advertised as a full-domain
guarantee. Unverified lender clauses, posting calendars and fee maxima are not
invented. Goal-dependent released-payment behavior without a defined confirmed
target is refused. Stress results distinguish tested failures from unknown
rules and never replace base assumptions.

## Mini App, bot and admin

- Five compact root tabs; editing and secondary workflows use separate layers.
  Loan icons are explicit selections with Bank as the default. Chart modes share
  legends and aligned values. Balances show statement dates, including zero.
- Armenian/English selection is visible, persisted and shared with the bot.
  Concurrent reads are shared. Cached financial data stays labelled stale until
  the affected foreground data refreshes; connectivity alone cannot clear it.
- Optional reminders refer to approved dated actions and stop when approval or
  source freshness changes. Required reminders retain priority and distinct
  labels. Snooze changes delivery time, never the contractual due date.
- Admin pages enforce individual identities, TOTP, server-side roles, purpose
  audit and step-up. Policy publication requires independent review of the exact
  payload. Cases require classified evidence; no financial-fact overwrite API
  exists. Profile flags act independently, and historical replay uses original manifests with the available engine. An
  unavailable engine version returns a visible refusal; it never substitutes
  the latest rules. This is the first deployment of immutable plan history.
- Public readiness and queue summaries use a separate non-personal service;
  administrative authorization is not weakened to make health checks pass.

Admin operator enhancements outside the six section-14 acceptance checks remain
listed in `internal/adapter/in/admin/README.md`, including authenticator recovery,
key rotation, export/pagination and additional billing/support operations.

## Evidence that software tests cannot supply

The user's point 4 is the final physical Telegram/iPhone check. Participant
usability trials and two months of real bank reconciliation remain field
validation, not outcomes proved by synthetic examples or automated tests.
