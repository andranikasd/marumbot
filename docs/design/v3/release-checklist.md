# Development release checklist

## Current status

Deployed: **2.0.2 — development only** on
`feature/v1.1.0-complete-ui-redesign`.

The first expanded rollout (`2.0.1`, commit `7c5062d`) reached schema 22,
then failed a malformed smoke probe: GET requests incorrectly carried a JSON
body, which the edge Fetch proxy rejected. The rollback step ran; do not infer
complete container rollback from the Worker rollback alone. A normal unsigned
GET returned 401. The probe has a reproducing regression and is corrected. Rollback now pins
the pre-release Worker version before deployment and secret synchronization,
rather than selecting the immediately previous revision implicitly.
The fresh `2.0.2` rollout succeeded on 2026-09-03 from application commit
`45169b620ba143ff2e7bd1714792a5cc535e3df5`. Live API and Mini App version
checks report `2.0.2`; readiness reports schema 22; admin login reports `2.0.2`.
The deployment smoke verified that the Telegram menu opens version `2.0.2`.
Changed assets must use a new version; existing `2.0.0` URLs remain immutable.
No production deployment, merge to main, Git tag or GitHub Release is requested.

Release evidence:
[exact-commit CI](https://github.com/andranikasd/marumbot/actions/runs/33766922851) and
[successful development deployment](https://github.com/andranikasd/marumbot/actions/runs/33767241888).
An independent live smoke run also passed. The optional Grafana deployment
annotation returned HTTP 401 (invalid API key); its annotation token needs
replacement. This did not block deployment or application health checks.

## Software scope

[Development acceptance evidence](development-acceptance.md) records the
implemented workflows and supported-domain boundaries. The implementation covers:

- Payment recording, immutable corrections, statement reconciliation, exact
  current-period cash/spending accounting and actual-versus-plan attribution.
- Effective budget policies, growth, overrides, carry/release choices, explicit
  funding, cash routing, strategy comparisons, isolated scenarios, activation
  history, exact replay/export, inverse-budget domain proof and stress reports.
- Compact Mini App layers, editable loans/budgets, explicit icons, chart legends,
  aligned amounts, statement dates, Armenian/English and persistent stale labels.
- Required/optional reminders, exact payment links, snooze, account settings and
  private-chat protection. Unchanged approved future actions survive midnight;
  stale or superseded optional reminders are suppressed.
- Individual admin identities/TOTP, roles, purpose audit, classified cases,
  independent policy review/publication, fixture evidence, original-manifest replay with the available engine and
  scoped profile controls.
- Durable retry receipts and aggregate-version checks for financial commands;
  application-owned transactions and compatible source-history locking.

Unsupported financial domains remain explicit refusals or Unknown results,
never guessed calculations. See the acceptance document and admin README for
specific limitations; this release does not certify every lender or every
advanced engine domain described in the exploratory specification.

## Release gates

- [x] Final race-enabled application/core tests.
- [x] Final PostgreSQL integration tests through migration 22.
- [x] Lint, frontend behavioral tests, bundle and rollout-smoke regressions.
- [x] Preservation-only rollback/reapplication of the latest migration.
- [x] Commit and push the verified implementation on the requested branch.
- [x] CI passes for that exact commit, including both architecture reports.
- [x] Deploy that branch with manual **CD · dev**, version `2.0.2`.
- [x] Verify live version, schema, assets, unsigned-call rejection and bot menu.

## User-owned field check

The user will perform point 4 afterwards: reopen Telegram on iPhone and check
language, aligned numbers, chart labels, Loans, editing, payments and reminders.
Participant usability trials and two months of bank reconciliation additionally
require real people, source statements and elapsed time. Automated tests and the
synthetic three-loan example do not replace that field evidence.

## v2.0.3 iPhone feedback follow-up

The next tagged development release addresses the screenshots from the 2.0.2
field check: native date/time field overflow, compressed Retry text, oversized
checkboxes and the reminder save button. Budget and Money now have a collapsed
English/Armenian explanation of spending limits, available cash and payments
already made. Reminder settings are collapsed below the language selector.

Read and response-body waits are bounded, with explicit loading/error/retry
states. No financial write is automatically retried. Stale warnings follow the
visible screen, while hidden snapshots remain marked stale until reread. Plan
strategy comparisons load on demand and cannot retain old figures on reopening.
Budget view and save retries have distinct element IDs.

Validation: all Mini App behavioral suites, adapter race tests and synthetic
320px/390px English/Armenian browser checks. These checks do not replace testing
inside Telegram's iPhone webview. The user has authorized merging PR #99 and
publishing a new tag to development; production remains out of scope.
