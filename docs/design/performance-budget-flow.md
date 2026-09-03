# Responsive app and clearer budget flow

Implementation notes for the changes after development v2.0.3. This document
records the patch, not a claim that a new release has already been deployed.

## Borrower flow

The budget editor groups existing declarations by when they matter:

- **Each month:** spending limit, regular money set aside for loans and its
  arrival day. A Next button leads to the current cash statement.
- **Today:** cash already available, protected reserve and payments already made.
- **Extras:** extra receipts, collapsed month overrides and advanced policy rules.

These are the same financial declarations and save endpoints. A limit does not
create cash, and no amount is inferred from another field. Existing policy locks,
statement dates, aggregate versions and retry identities remain in force.

English and Armenian use shorter labels. The amount and currency remain beside
one another on narrow phones. The summary distinguishes monthly funding from
spending permission; room in the limit is not labeled as spendable cash.
Expected receipts remain outside base-plan funding until confirmed.

## Responsiveness

Previously, opening any screen waited for account language settings. Boot now
opens the requested screen immediately and synchronizes language separately.
A late language response must not override an explicit newer choice, navigate
back to the original deep link, or recreate a dirty form.

Budget, policy and loan context reads start together. The form remains disabled
until the complete required context is available; no partial document is saved.
Financial writes are not automatically retried.

Backend changes remove repeated work within a request. They do not cache
financial state across requests, infer lender behavior or change allocations.
See the associated regression tests for lookup counts and response equivalence.

## Evidence and limits

- Deferred-response boot regression: navigation works while settings is unresolved;
  language refresh preserves edits and pending-write locks.
- Budget regression: all context reads start before any response resolves;
  saving waits for complete context and retains exact retry payloads.
- Existing Mini App behavior suites and full module-graph bundle check.
- Synthetic English/Armenian browser checks at 320px and 390px. Physical Telegram
  and real network conditions remain distinct from this local fixture evidence.

These checks establish eliminated waits and repeated work. They are not a
production latency percentile, cold-start benchmark or promise that every
calculation finishes instantly. Container/database latency and the size of a
supported planner search still affect response time.

## Deployment and audit coverage

- Production Docker and Wrangler builds share pinned esbuild 0.28.1. Secondary
  forms/tools load on demand; initial shared chunks are preloaded. The build
  check caps initial JS at 230 KB and gzip at 65 KB. Current payload: about
  168 KB / 43 KB gzip, versus roughly 328 KB of eager source modules.
- Go precompresses public JS/CSS once, including design tokens, negotiates gzip,
  and preserves HEAD/range fallback behavior. Private API data is not cached.
- Ordinary commands no longer update per-chat Telegram menus. Startup, /start
  and language changes retain menu updates. Sender admission is paced globally
  and per chat, honors retry-after cooldown, and never silently replays a send.
- PostgreSQL defaults to eight connections but honors DSN `pool_max_conns` and
  `pool_min_conns`; e.g. append `&pool_max_conns=16&pool_min_conns=2` to an existing
  query string. Size these against the database’s capacity, not an arbitrary
  target. Eight connections limits concurrent DB checkouts, not all HTTP work.
- Polling is implemented through the same durable inbox. The earlier documented
  default had no getUpdates loop. It never silently removes a hosted webhook.
- Reminder dates batch per loan. Generation remains hourly; due delivery runs
  each tick, before generation. Generation has its own 20-second deadline so a
  stalled walk cannot starve already-due deliveries. Newly generated occurrences
  are eligible on the following tick. Hosted ticks have a 45-second deadline.
  Startup menu work already had a 30-second deadline; it remains bounded.
- One container remains deliberate. Shared scheduler coordination and distributed
  sender pacing are prerequisites for horizontal replicas. The full-account menu
  sweep is still sequential; deadline/resumption and the 500-account reminder
  walk need a separate paginated background-work design for larger deployments.

The five invariant statements are unchanged. `pkg/core/rates` remains a
historical exception name with no corresponding package; see the engineering
guide. No fictitious implementation is inferred from that wording.

## Focused CPU evidence

`BenchmarkBudgetReadPolicy` exercises the authenticated budget handler with
in-memory reads and an existing policy fixture. Before/after median local time
was 21.17 ms → 7.98 ms, with allocations 211,974 → 106,045. This isolates removal
of duplicate policy timeline construction; it excludes database/network delays
and is not an end-to-end development latency measurement. Plan source reads
fall from four to two while retaining both activation metadata reads and the
final source-change conflict guard. Profile flags deduplicate only within one
request; no cross-request financial freshness is relaxed.
