# VPS verification evidence — 2026-09-09

Local production hardening based on `23ceb20`; no actual VPS or external
infrastructure has been changed. [Audit resolutions](production-fixes.md) and
[deployment/recovery procedure](vps.md) explain the final behavior.

## Verified

- Full Go race suite and all 16 Mini App UI suites passed during hardening.
- Schema 26 applied on disposable PostgreSQL 17. Full store race suite passed
  with the restricted marum_app role.
- Previous release v2.0.4 store race suite passed against schema 26. This proves
  database compatibility, not pre-hardening TOTP ciphertext compatibility.
- Concurrent first-contact regression: 128 calls, one account per identity,
  no orphan users. Suspension/restoration behavior covered.
- Database guards reject ledger mutation, broad deletion and TRUNCATE while
  requested erasure still cascades. Checks execute inside rolled-back fixtures.
- Reminder retries, cancellation cleanup, snooze protection, independent stage
  budgets, cursor continuation and menu retries have focused race regressions.
- Outbox idempotency, lease fencing, retry, completion and suspension verified.
- History paging tested with 57 versions; manifests remain available for replay.
- TOTP encryption/migration, wrong-key/row rejection, durable journal validation,
  restore reconciliation and identity/replay verification have regressions.
- Five-loan search diagnostic at GOMAXPROCS=4: one iteration 443.8 ms,
  73.7MB allocated, 149,935 allocations on the development Ryzen 7730U.
  This is a single local measurement, not VPS capacity or a latency promise.

- Pinned golangci-lint reports 0 issues; ShellCheck, actionlint and Compose checks pass.
- Final production image built. Isolated Compose starts healthy with schema 26;
  readiness/status return 200 and the intended release stamp; unauthenticated loan
  API returns 401. Domain-matched proxy permits /app/ and returns 404 for probes,
  status, tick and Telegram webhook paths. Real-domain ACME remains a host check.
- Production backup.sh created and verified a local archive. A populated archive
  with a valid encrypted identity and approved plan restored successfully; the
  production binary's -verify-restore confirmed identity decryption and original
  replay inside the isolated restored database.
- All three new migrations rolled down to 23 and up to 26 on populated fixtures.
  Digests of users, loans, financial events, snapshots and approved manifests were
  identical before/after. This is schema rollback evidence, not permission to
  remove retry state or encryption protections from a live service.

## Still required on the target host

Production secrets and preserved identity key; private journal directory and
latest authoritative contents; domain/TLS and firewall checks; single-bot cutover;
independent encrypted storage and tested alerts; authenticated representative
load and real Telegram journeys. See the deployment guide. A daily backup alone
cannot establish journal freshness after total host loss: do not expose restored
accounts until deletion intents are reconciled.

## Follow-up review — 2026-09-10

Workers/D1/R2 was assessed during this follow-up, then the user selected
the VM/PostgreSQL deployment as the production target. The Cloudflare
[alignment audit](cloudflare-target.md) remains a deferred alternative. The existing implementation received these local fixes:

- Backups resolve the actual application journal bind mount; four automated
  regressions cover custom paths, missing/ambiguous containers, missing journals
  and failed-upload pruning protection (the two container cases share a test).
- Restore verification now checks all admin credentials, including disabled
  administrators and later pages. Missing keys/verifiers and corrupt secrets fail.
- Polling and webhook scheduling use the same bounded shadow stage.
- VPS configuration passes through existing OTLP and profiling settings.
- Full Go race suite, restricted-role schema-26 store race suite (including the
  105-admin paging/corruption regression), all Mini App UI suites, rollout helper
  tests, lint and ShellCheck passed. Ledger guard checks passed on rolled-back
  fixtures in a disposable PostgreSQL container.

These checks prove the current Go/PostgreSQL fixes. They do not establish D1
compatibility, Worker execution limits, a new release deployment, or production
capacity. After the VM target was reselected, the corrected image
`marum-vps:vm-candidate-20260910` was built with version
`23ceb20-vm-candidate-20260910` and image ID
`sha256:114a3c81734229cec88ab85ed006ca4a73b4ce9216ed02911882e2cb82453990`.
Compose validation passed. A populated fixture dump passed the production
`restore-check.sh`, including checksum, isolated restore, restricted grants,
ledger guards, borrower/admin decryption and original plan replay through this
exact image. This was a local rehearsal; SSH access, live secrets, domain/TLS,
offsite storage and production traffic remain unverified.

## 24-GB VM Compose validation — 2026-09-10

The deployment defaults now target 8 vCPU / 24 GB RAM / 200 GB NVMe:
PostgreSQL 8 GiB and 2 CPU, Marum 6 GiB and 4 CPU, Go soft memory limit
4 GiB, PostgreSQL shared buffers 2 GB and cache estimate 6 GB. Existing `.env`
overrides must be updated explicitly. The database network is internal; only
Marum also joins Caddy's network. A missing journal bind directory fails startup.

A disposable Compose project using the candidate image above passed startup,
migrations and health checks with these limits. Runtime inspection confirmed
the memory/CPU limits and service network membership. Direct readiness/status
returned 200, unauthenticated loans returned 401, and Caddy served `/app/` with
200 while rejecting `/status` and `/internal/tick` with 404. The test override
disabled published ports, external egress and TLS issuance; production Compose
configuration was separately checked for loopback app ports and database
isolation. No real Telegram messages were sent.

The populated restore rehearsal passed again using the reduced Go/container
limits, including credential decryption and plan replay. Four backup regressions
and ShellCheck passed. These are local deployment checks, not a representative
load test or validation of the target host, public TLS or live delivery.

Further backup review added a host-local lock spanning dump, upload and pruning
to prevent overlap between manual and scheduled offsite runs. The new regression
failed before the fix and passed afterward: an already-held lock prevents dump
creation and restic calls. All five backup tests and ShellCheck pass; tests also
check lock release after normal completion and upload failure.
