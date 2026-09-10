# Production hardening audit resolution

Prepared locally on 2026-09-09 for the requested 6-vCPU / 48-GB / 300-GB VPS.
No production deployment, DNS/bot cutover, or external backup account was changed.
See [deployment and recovery](vps.md) and [verification evidence](vps-readiness.md).

| Audit concern | Resolution |
| --- | --- |
| Ledger mutations | Migration 24 rejects event UPDATE/DELETE/TRUNCATE; app privileges narrowed. Requested-account erasure remains guarded. |
| Float money / negative balances | Exact decimal input and output; precision/range rejection. Legacy zero sentinel retained to avoid zeroing existing balances; snapshot_minor supports explicit zero. |
| Paused accounts | Mini App lookup and required reminders filter suspension. |
| Failed reminder starvation | Durable exponential retry delays, snooze guards, cancellation-independent retry persistence. |
| Shared scheduler deadline | Inbox, reminder and metric budgets separated; required reminders, confirmations and generation also have separate stages. |
| Unbounded planner work | Aggregate search admission, per-key coalescing, cancellation between candidates; API admission and deadlines. Successful financial results and manifest hashes remain unchanged. |
| Sensitive traces | Exporter sanitizes error details/status/sensitive attributes; SQL labels use declared names. |
| Compatibility gate skipped DB | Release workflow supplies TEST_DATABASE_URL; race-enabled previous-release store run verified against expanded schema. |
| Erasure resurrection | Durable independent filesystem intent journal, fail-closed production configuration, startup restore reconciliation; authoritative journal recovery is required. |
| First-contact race | Losing statement rolls back; fresh snapshot resolves winner without orphan accounts. |
| Serial polling ingestion | Durable ingestion wakes owned worker; slow command replies no longer hold the polling loop. |
| Create response waits for Telegram | Transactional confirmation outbox and default reminder rules; HTTP returns durable receipt. Lease tokens, retry delays and idempotent creation tested. |
| Reminder/shadow scan cadence | Cursors resume interrupted pages; cooldown begins only on completed sweep. |
| Startup menu sweep | Interrupted work resumes; transient failures prevent falsely completing a sweep. |
| Replay under transaction locks | Immutable proposal replay happens before account/source locks; receipt replay still takes precedence on retry. |
| Arbitrary proposal eviction | Predictable FIFO eviction; replacing existing key does not evict another proposal. |
| Heavy history lists | 50-item metadata API pages and load-more UI; full manifests retained for explicit replay/internal consumers. |
| Historical engine coupling | Existing plan/5 schema and fingerprints deliberately preserved, with pinned replay fixtures. A future engine/DTO migration still needs an explicit versioned compatibility design; old hashes must not be rewritten. |
| Wrong delivery queue metrics | Metrics include scheduled occurrences and the actual confirmation outbox. |
| False-green status/readiness | HTTP 503 on dependency failures, explicit unavailable queue state; schema 26 required before listeners start. |
| Possible lease-index optimization | Not blindly changed: no measured regression justified another index. Benchmark representative queue histories before changing lease SQL. |
| Cache memory assumptions | Serialized-size budget 64 MiB plus entry cap; this is an estimate, not a hard retained-heap guarantee. Actual heap/latency measurement remains a deployment check. |
| VPS resource settings | Adjustable 12 GiB PG / 8 GiB app limits, 3 GB shared buffers,16-connection app pool, CPU and Go limits. |
| Backup retention | Verified local 14-day retention; successful offsite snapshots retain 14 daily / 8 weekly / 12 monthly copies. |
| Weak restore rehearsal | Checksum, isolated restore, restricted grants and ledger-guard checks; -verify-restore checks identity decryption and original plan replay without starting listeners. |
| Plaintext TOTP | Purpose-derived AES-GCM encryption bound to admin identity; restartable CAS legacy migration. Original master key must be preserved. |
| Query trace overhead | Once-built SQL-to-name lookup; bounded labels, no statement tokenization. |
| Mutable deployment artifacts | PG/Caddy defaults pinned to validated digests; application/migrator registry-digest overrides supported. Publish immutable release artifacts before live rollout. |
| Configuration gaps | Environment, currency, URL and production journal path validated at startup. |
| Stale readiness documentation | Replaced obsolete 500-account limit and small-VPS defaults with implemented behavior and remaining launch checks. |

## First-transition caution

This release encrypts legacy TOTP credentials. A pre-hardening binary cannot read
that ciphertext; it is not an automatic rollback target. Keep the original
identity master key and roll forward with a compatible image on this transition.
Future rollback images must support the same encryption/journal formats.

## Conditions for live launch

Server access/OS, production secrets, domain/TLS, bot cutover, offsite credentials,
monitoring destinations and representative traffic are not supplied. Complete the
[VPS checklist](vps.md) on the actual host. Daily backup RPO does not guarantee
recovery of the latest deletion intents after total host loss: recover the latest
authoritative journal, or keep restored borrower access disabled until reconciled.
The code changes alone do not establish a supported user-count or p95/p99 latency.
