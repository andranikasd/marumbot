# Current state

Verified on **2026-09-03**, against application release **v2.0.3**. This is a
release snapshot; future changes must update it alongside the affected guides.

| Item | Verified state |
| --- | --- |
| Environment | Development only; no production environment |
| Application | Mini App, bot/backend and admin deployed as 2.0.3 |
| Source | `main` at tag `v2.0.3`, commit `8d34606852f5d88ef31b8b32df757e37f0cce203` |
| Database | Schema 22 |
| Planning engine | `plan/5` |
| Public endpoint | https://dev.marum.loan |
| Operator endpoint | https://admin-dev.marum.loan |
| Bot | `marum_dev_bot` |

## Implemented scope

- Five Mini App tabs with compact cards and child screens, explicit loan icons
  (bank by default), loan/budget editing, selectable charts with series legends,
  aligned amounts and English/Armenian language controls.
- Separate spending permission and cash funding, budget policies/overrides,
  carry/release choices, strategy comparisons, scenarios, activations and
  original-manifest history/replay/export.
- Payment records and immutable corrections; bank statement reconciliation;
  current-period cash/spending accounting and actual-versus-plan attribution.
- Required reminders and approved optional actions, snooze, preferences,
  payment deep links and private-chat handling.
- Operator identity/TOTP, roles, purpose audit, classified cases, independent
  policy review/publication and scoped profile controls.

The [acceptance record](design/v3/development-acceptance.md) contains detailed
behavior and test evidence. The [budget guide](product/budgeting.md) explains
what each declaration means without changing the engine's rules.

## What v2.0.3 fixes

The release addresses iPhone feedback: native date/time field overflow,
compressed Retry text, checkbox and save-button styling, and difficult budget
wording. Budget and Money have collapsed English/Armenian guidance; language
selection precedes collapsed reminder settings.

Reads and response-body waits are bounded. Screen-specific stale warnings,
loading/error/retry states and lazy strategy comparison prevent unrelated
failures from leaving a blank-looking screen or old comparison figures.
Financial writes are not automatically retried.

## Limits and remaining evidence

- This is a development release, not production readiness certification.
- A supported profile is scoped to its fixture evidence, not an entire lender.
  [Corpus support states](../testdata/golden/MANIFEST.json) remain provisional
  or experimental where indicated.
- Inverse-budget proof is limited to the supported fee-free zero-interest
  domain. Unproven cases refuse; bounded searches disclose coverage and unknown
  bounds. The original v3 specification includes broader aspirations.
- Expected receipts are excluded from base-plan funding. A newly assumed
  scenario receipt must be confirmed before activation.
- A recorded payment is not bank confirmation. Missing allocation is Unknown,
  not zero. Historical replay depends on the available compatible engine.
- Synthetic three-loan/browser tests do not replace Telegram/iPhone field
  validation, participant usability studies or real statement reconciliation.
- The release compatibility job omits `TEST_DATABASE_URL`, so its success does
  not establish previous-release database compatibility. Current-release store
  integration CI is separate evidence.
- The backup bucket exists, but scheduled dump/upload and a validated restore
  procedure are not implemented in this repository.
- The Grafana deployment annotation returned HTTP 401 due to an invalid API key.
  This nonblocking integration needs credential repair; application smoke passed.

## Evidence and document authority

[Release checklist](design/v3/release-checklist.md) links the merged PR, CI,
tagged release and successful development rollout. Operational steps live in
[releases](operations/releases.md) and [deployment](operations/deployment.md).

Current guides describe inspected implementation; migrations and source define
runtime behavior. The original MVP/v3 proposals, decision-engine review, PDF,
HTML mockups and diagram exports remain historical design references. Their
presence does not mean all proposed features shipped. The five invariants in
[AGENTS.md](../AGENTS.md) remain policy; known enforcement/documentation gaps
are called out in the [engineering guide](engineering-guide.md).
