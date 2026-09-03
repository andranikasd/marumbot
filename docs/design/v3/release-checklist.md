# Development release checklist

## Current release

**v2.0.3 is deployed to development**, verified 2026-09-03. The implementation
from `feature/v1.1.0-complete-ui-redesign` was merged through
[PR #99](https://github.com/andranikasd/marumbot/pull/99).
Tag `v2.0.3` and the released `main` commit are
`8d34606852f5d88ef31b8b32df757e37f0cce203`.

The live API, Mini App and admin report **2.0.3**. Readiness reports schema **22**;
the engine is **`plan/5`**. The smoke verified that the Telegram menu opens the
Mini App at 2.0.3. There is no production environment or production rollout.

## Evidence

| Gate | Evidence |
| --- | --- |
| PR checks | [CI 33772749526](https://github.com/andranikasd/marumbot/actions/runs/33772749526) |
| Merged source checks | [CI 33773084763](https://github.com/andranikasd/marumbot/actions/runs/33773084763) |
| Tagged release | [Release 33773187762](https://github.com/andranikasd/marumbot/actions/runs/33773187762) |
| Published release | [v2.0.3](https://github.com/andranikasd/marumbot/releases/tag/v2.0.3) |
| Final development rollout | [CD 33774632460](https://github.com/andranikasd/marumbot/actions/runs/33774632460) |

- [x] Application/core race tests, PostgreSQL integration and migration checks.
- [x] Lint, frontend behavior, bundle and rollout-smoke regressions.
- [x] Release compatibility job completed and artifacts/SBOM/provenance published.
  This job does not prove previous-release database compatibility: it omits
  `TEST_DATABASE_URL`, so database-dependent tests skip.
- [x] PR merged, tag published and exact source deployed to development.
- [x] Live version, readiness/schema, assets, unsigned-call rejection and bot
  menu verified; independent live smoke also passed.

Release and development jobs build their own images. Matching source commits
are verified; this does not claim their image digests are identical.

## Deployment path and recovery

Development protection rejects tag refs. The attempted tag-ref run
[33774376583](https://github.com/andranikasd/marumbot/actions/runs/33774376583)
was rejected before deployment. The successful run dispatched `cd-dev.yml` from
`main` with explicit version `2.0.3`, after verifying main matched the tag commit.
No protection rules were changed. See [release procedure](../../operations/releases.md).

The earlier 2.0.1 rollout exposed a smoke bug: GET requests carried a JSON body
that the edge proxy rejected. The regression is fixed. Rollback now captures the
pre-release Worker version before deployment and secret synchronization; it must
not silently choose the immediately previous revision. Worker rollback alone is
not proof of container or database rollback. 2.0.2 then deployed successfully,
before the merged/tagged 2.0.3 release above.

## Changes validated in v2.0.3

The [acceptance record](development-acceptance.md) covers payments, reconciliation,
budget/funding policies, planning/scenarios/history, reminders and admin controls.
The iPhone feedback follow-up fixes native date/time overflow, compressed Retry
text, checkbox/save styling and confusing budget wording. It adds collapsed
English/Armenian budget guidance and puts language ahead of reminder settings.

Read/body waits are bounded with explicit loading/error/retry states. Stale
warnings follow the visible screen; hidden stale data stays marked until read.
Strategy comparisons load on demand and clear obsolete figures. Financial
writes are not automatically retried.

## Remaining field and operations checks

- Previous-release store compatibility needs a run with `TEST_DATABASE_URL` set.
  Current-release PostgreSQL CI passes, but is a different guarantee.
- The backup bucket is provisioned, but a scheduled dump/upload and verified
  restore procedure are not implemented. See [runbooks](../../operations/runbooks.md).

- The optional Grafana deployment annotation returned HTTP 401 (invalid API key).
  Replace its annotation credential separately; deployment and application health
  checks succeeded despite this warning.
- Reopen the Mini App in Telegram on iPhone to confirm layout, language, editing,
  payment and reminder flows. Automated synthetic browser checks are evidence of
  those fixtures, not certification of the device webview.
- Participant usability trials and extended bank-statement reconciliation still
  need real users, source statements and elapsed time.
- Unsupported financial domains remain explicit refusals/Unknown; this release
  does not certify every lender or the complete exploratory v3 specification.
