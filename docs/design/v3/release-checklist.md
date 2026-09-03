# Development release checklist

## Current status

Target: **2.0.2 — development only** on
`feature/v1.1.0-complete-ui-redesign`.

The first expanded rollout (`2.0.1`, commit `7c5062d`) reached schema 22,
then failed a malformed smoke probe: GET requests incorrectly carried a JSON
body, which the edge Fetch proxy rejected. The rollback step ran; do not infer
complete container rollback from the Worker rollback alone. A normal unsigned
GET returned 401. The probe has a reproducing regression and is corrected. Rollback now pins
the pre-release Worker version before deployment and secret synchronization,
rather than selecting the immediately previous revision implicitly.
The fresh `2.0.2` rollout remains pending. The last fully verified deployment
before this attempt was `2.0.0`, commit `219cac1a89dd00a829a28cd683c8447d89f4a36e`.
Changed assets must use a new version; existing `2.0.0` URLs remain immutable.
No production deployment, merge to main, Git tag or GitHub Release is requested.

Previous live evidence:
[CI](https://github.com/andranikasd/marumbot/actions/runs/33733922684) and
[development deployment](https://github.com/andranikasd/marumbot/actions/runs/33734213874).

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
- [ ] Commit and push the verified implementation on the requested branch.
- [ ] CI passes for that exact commit, including both architecture reports.
- [ ] Deploy that branch with manual **CD · dev**, version `2.0.2`.
- [ ] Verify live version, schema, assets, unsigned-call rejection and bot menu.

## User-owned field check

The user will perform point 4 afterwards: reopen Telegram on iPhone and check
language, aligned numbers, chart labels, Loans, editing, payments and reminders.
Participant usability trials and two months of bank reconciliation additionally
require real people, source statements and elapsed time. Automated tests and the
synthetic three-loan example do not replace that field evidence.
