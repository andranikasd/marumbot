# Cloudflare alternative — deferred

Assessed on 2026-09-10: Workers for webhook/API, Workers static assets for the
Mini App, Cron Triggers for reminders, D1 for data, and scheduled exports to R2.
The user subsequently selected a VM with PostgreSQL instead. This assessment
is retained as a deferred alternative, not the current deployment plan. It is **not yet an
implemented or verified deployment**. Existing Go/PostgreSQL and VPS changes are
retained for development and comparison; no live resources have been changed.

## Alignment audit

| Requested component | Current implementation | Required change |
| --- | --- | --- |
| Worker webhook/API | `deploy/cloudflare/worker/index.ts` authenticates and forwards to `MarumApp`, a Go container | Run use cases in the Workers runtime; preserve authentication, command receipts and all API contracts |
| Static Mini App | Existing `ASSETS` binding and build pipeline | Reuse assets; verify API routing and immutable asset versions without the container |
| Cron reminders | Scheduled Worker calls the container's `/internal/tick` | Execute durable inbox, retries, reminders and shadow work against D1; no process-local locks or cursors as the correctness boundary |
| D1 | Only PostgreSQL adapter and PostgreSQL migrations exist | New D1 schema/adapter, transaction design, migration and parity evidence |
| D1 exports to R2 | VPS `pg_dump`/restic scripts; no D1 export implementation | Durable export polling, streamed R2 upload, checksums, retention, alerts and isolated restore rehearsal |
| Planning engine | Deterministic Go `pkg/core`, engine `plan/5` | Evaluate unchanged engine via WebAssembly before porting financial logic |
| Admin and deletion recovery | Go admin/TOTP and private filesystem erasure journal | Keep role/TOTP protections; independent durable deletion intents in R2, reconciled before restored accounts are exposed |

The current `env.prod` Wrangler section has no complete runtime/database binding
configuration. Adding a D1 binding alone will not make its container calls work.
Production mode also requires a durable erasure journal; an ephemeral container
filesystem is not an equivalent replacement.

## Runtime and budget feasibility

Cloudflare supports Go through WebAssembly, but Workers execute on one thread.
The free plan permits 10 ms CPU per HTTP request and Cron invocation, with a
128 MB isolate memory limit. Paid Workers start at $5/month plus usage; paid CPU
allowances differ by trigger. These are platform allowances, not Marum capacity
claims. See [runtime support](https://developers.cloudflare.com/workers/runtime-apis/webassembly/),
[limits](https://developers.cloudflare.com/workers/platform/limits/) and
[pricing](https://developers.cloudflare.com/workers/platform/pricing/).

A local native, single-thread `BenchmarkSearchFive` run on 2026-09-10 took
1.062 seconds and allocated 71,373,048 bytes (145,574 allocations). Command:
`GOMAXPROCS=1 go test ./pkg/core/plan -run '^$' -bench BenchmarkSearchFive -benchtime=1x -benchmem`.
This is one diagnostic on a Ryzen 7730U, not Worker CPU time or peak memory.
It strongly argues against promising the full planner on the free tier without
an actual Workers benchmark. Allocated bytes are not retained or peak memory.

The unchanged planner also compiled to `js/wasm`. Under Node, the zero-interest
inverse-budget fixture, stress-income golden and independent released-budget
fixtures passed; the five-loan benchmark took 3.350 seconds and allocated
71,371,480 bytes. This establishes a limited Wasm reuse signal, not Workers
compatibility, startup limits, peak memory or production performance. The temporary
probe lives under `/tmp/marum-cloudflare-feasibility` and is not a deployed Worker.

D1's free read/write/storage allowances are usage limits, not unlimited service.
R2 export costs also depend on retained data and operations. No fixed $0 bill is
established by this table. See [D1 pricing](https://developers.cloudflare.com/d1/platform/pricing/).

## Preservation requirements

1. Preserve every current product capability unless an explicit scope decision
   says otherwise. Keep Go integer arithmetic and fixture values unchanged.
2. Prove exact signed int64 money transport. D1 stores 64-bit integers, but its
   JavaScript binding does not support BigInt and reads integers as Number.
   Evaluate validated decimal-string bindings with explicit INTEGER casts and
   TEXT reads; prove boundary and overflow behavior. Do not route money through
   JavaScript floating-point arithmetic or silently narrow its range.
   [D1 conversion rules](https://developers.cloudflare.com/d1/worker-api/).
3. Replace PostgreSQL `FOR UPDATE`, `SKIP LOCKED`, advisory locks, JSONB and
   transaction-specific SQL deliberately. D1 `batch()` provides transactional
   rollback; it does not provide the current Go transaction port's interactive
   read/compute/write API. Encode freshness checks, financial writes and command
   receipts into atomic batches and prove conflicts roll back all effects.
   [D1 batches](https://developers.cloudflare.com/d1/worker-api/d1-database/).
4. Preserve append-only financial facts, optimistic versions, idempotency,
   retry fencing, suspension and requested-account erasure. No network send
   inside a financial write. Duplicate cron/webhook delivery must be harmless.
5. Preserve original manifests, input/result hashes and encrypted identities.
   Avoid JSON reserialization that changes historical hashes. Retain original
   master keys and verify borrower and administrator decryption after import.
6. Back up deletion intents independently of database snapshots. Recovery must
   reconcile the latest authoritative intents before serving borrowers. A D1
   dump alone cannot establish this. Use the export REST API and resumable
   orchestration; `D1Database.dump()` is restricted to legacy alpha databases.
   Cloudflare's [D1-to-R2 example](https://developers.cloudflare.com/workflows/examples/backup-d1/)
   uses Workflows, an additional service; select it explicitly or implement
   bounded Cron-driven polling within the requested stack.

## Ordered delivery gates

1. **Runtime proof:** run the unchanged engine and pinned hashes through a
   Worker-compatible Wasm wrapper. Measure cold start, CPU, bundle and peak
   memory with representative inputs. Select an affordable supported plan.
2. **One complete write path:** create/edit a loan and record/reverse a payment
   on local D1 with exact int64 transport, atomic conflict handling, durable
   receipts and immutable history. Reuse current fixtures and API contracts.
3. **Full parity:** port budgets, plans/replay/scenarios, preferences, admin,
   encrypted identities, reminders and outbox processing. Run the current
   golden/API/UI corpus and add D1 concurrency/retry/erasure regressions.
4. **Recovery proof:** export a populated D1 database, restore to a separate D1
   database, reconcile R2 intents, decrypt all credentials and replay manifests.
   Prove alerting, retention and interrupted-export resumption.
5. **Cutover:** freeze writes, export and preserve the PostgreSQL source,
   import to D1, compare facts/versions/hashes, then switch the one bot webhook
   and API route. Retain the original source read-only. Before D1 accepts new
   writes, rollback can return to it; after new writes, rollback requires a
   verified reverse migration or forward repair. Never start both writers.

No migration, domain cutover, paid subscription, or production deployment has
been performed. The paid-plan versus strict-free requirement remains open.
