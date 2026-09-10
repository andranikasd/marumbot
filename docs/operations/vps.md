# VPS deployment

This is a single-instance Docker Compose deployment. It serves embedded Mini App
assets through Caddy HTTPS and uses Telegram long polling plus the application's
local reminder scheduler. Cloudflare Workers and their cron are not required.
Do not run a second instance with the same production bot token.

The target server is **8 vCPU, 24 GB RAM, 200 GB NVMe, Ubuntu 26.04**.
The starting limits cap PostgreSQL at 8 GiB / 2 CPU and Go at 6 GiB / 4 CPU.
Go uses a 4 GiB soft memory limit and four scheduler threads, leaving room inside
its container for non-Go memory. Caddy and the one-shot migrator each have a
256 MiB limit. The remaining host resources provide filesystem cache, monitoring
and maintenance headroom. Limits are ceilings, not preallocated memory.

PostgreSQL starts with 2 GB shared buffers, 8 MB work memory and 60 connections;
the app pool remains bounded at 16. `effective_cache_size=6GB` is a query-planner
estimate, not a separate memory allocation. Work memory can multiply across
query operations and concurrent connections. These are initial settings, not
measured capacity. Build release images away from production traffic.

Existing `.env` values override these defaults: when moving from the earlier
48-GB configuration, update the memory, shared-buffer and cache-size settings
from `.env.example` before recreating containers. Keep all existing secrets.

## First deployment

1. Check out an immutable reviewed release under `/opt/marum`. Install Docker
   from its official distribution instructions. Keep SSH key authentication and
   host security updates enabled. Allow inbound SSH from operator addresses and
   TCP 80/443 (UDP 443 is optional HTTP/3). Do not expose 5432, 8080 or 8081.
2. Point the public domain's A record, and AAAA only when IPv6 actually works,
   to the VPS. Ensure no other service owns ports 80/443. Caddy obtains and
   renews the certificate; its data volume must survive deployments.
3. Copy `deploy/vps/.env.example` to `deploy/vps/.env`, then `chmod 600` it.
   Fill every required value. Generate separate hex passwords/tokens using
   `openssl rand -hex 32`, and an identity key using `openssl rand -base64 32`.
   Back up the identity key independently: database dumps cannot recover it.
   Use a separate production bot token. For an existing bot, explicitly remove
   its webhook at cutover **without dropping pending updates** and stop its old
   deployment before starting this one. The app does not remove webhooks itself.
4. Optionally generate the admin password hash using `make admin-password`.
   Single-quote the hash in `.env`. The admin listener stays disabled if empty.
5. Provision the independent erasure journal **outside the database volume**:

   ```sh
   sudo install -d -m 0700 -o 65532 -g 65532 /var/lib/marum/erasures
   ```

   Set `MARUM_ERASURE_JOURNAL_DIR` if using another host directory. Compose refuses
   a missing bind source instead of creating an empty directory automatically. For a restore,
   recover the latest authoritative journal here; never substitute an empty
   directory. Every production runtime requires this setting. The app reconciles
   recorded deletion intents before opening listeners. Erasure fails closed if
   it cannot durably record the intent. Include this directory in independent
   backups; a database dump alone is insufficient.
6. From `/opt/marum`, validate, build, and start:

   ```sh
   docker compose --env-file deploy/vps/.env -f deploy/vps/compose.yml config --quiet
   docker compose --env-file deploy/vps/.env -f deploy/vps/compose.yml build
   docker compose --env-file deploy/vps/.env -f deploy/vps/compose.yml up -d --wait
   curl --fail --max-time 5 http://127.0.0.1:8080/readyz
   curl --fail --max-time 10 https://YOUR_DOMAIN/app/
   ```

   Readiness must report the intended release and current migration version
   (26 at this checkout). Open the production Mini App from Telegram and verify
   a fixture-backed journey, inbox processing and a due reminder before launch.
   Do not use a real borrower to generate synthetic test payments.

The initial database script creates `marum_app` without superuser, role creation
or database creation privileges. Migrations run as `marum_owner`; default grants
allow the app to use newly migrated tables and sequences. Migration 24 restricts
ledger privileges and adds mutation guards while retaining requested erasure. Initialization runs
only on an empty volume. Editing password variables does **not** rotate an
existing database role's password. Apply coordinated password rotation through
an authenticated owner session, then recreate the app. Never delete the volume
to resolve an authentication failure.

Database/admin ports are not public. Admin access uses an SSH tunnel:

```sh
ssh -L 8081:127.0.0.1:8081 operator@YOUR_VPS
```

Open `http://127.0.0.1:8081` locally. Public Caddy routing permits `/app/` only;
health, status, tick, webhook and admin routes stay private. PostgreSQL and migrations use the internal `database` network. Caddy uses only
`public`; Marum joins both to reach PostgreSQL, Telegram and telemetry endpoints.
PostgreSQL has no published port or direct external route. Application database
traffic is unencrypted only on the internal single-host Docker network. Use
certificate-verified TLS if you later move the database off-host.

## Backups and recovery

Create a custom-format dump with owner/ACL portability and a checksum:

```sh
sh /opt/marum/deploy/vps/backup.sh
sh /opt/marum/deploy/vps/restore-check.sh /absolute/path/database.dump
# To include the actual binary in this isolated rehearsal, export the original
# MARUM_IDENTITY_KEY and latest MARUM_ERASURE_JOURNAL_DIR, then set:
# MARUM_VERIFY_IMAGE=your-registry/marum@sha256:... sh deploy/vps/restore-check.sh ...
```

The restore checker creates a disposable PostgreSQL container with no network or
published ports, fails on any restore error, then removes only that container
and its temporary volume. It verifies the checksum, role grants, mutation guards and requested-erasure
cascade in addition to the restore. This is not a complete business recovery. A full recovery rehearsal must also start the matching app
with the preserved identity key and verify historical replay and authentication:

```sh
docker compose --env-file deploy/vps/.env -f deploy/vps/compose.yml run --rm --no-deps marum -verify-restore
```

Run this before enabling ingress. It reconciles erasure intents and encrypts any
legacy admin credentials, then validates borrower identity decryption, every admin TOTP credential
(including disabled administrators), and stored plan replay without starting HTTP listeners or sending Telegram messages. An unavailable
historical engine fails the check and requires the matching preserved engine.

For scheduled encrypted offsite copies, install restic and util-linux (`flock`), initialize a repository
in independent storage, and create `/etc/marum/backup.env` (root-readable, mode
600) with `RESTIC_REPOSITORY`, `RESTIC_PASSWORD_FILE`, and any provider credential
environment variables. Keep the restic password and identity key in independent
secure storage. Then install the timer:

```sh
sudo cp deploy/vps/marum-backup.service deploy/vps/marum-backup.timer /etc/systemd/system/
sudo systemctl daemon-reload
sudo systemctl start marum-backup.service
sudo systemctl enable --now marum-backup.timer
sudo systemctl list-timers marum-backup.timer
```

Manual and scheduled offsite runs share `/run/lock/marum-backup.lock` across
dump creation, upload and pruning. A concurrent invocation fails before making
a dump. Run manual offsite backups with the same environment and user as the
service, preferably `sudo systemctl start marum-backup.service`.
If overriding `MARUM_BACKUP_LOCK`, use the same host-local path for every run;
do not delete the lock file while a backup is active. The lock is released when
the processes exit, including after an upload failure.

Verify the first offsite snapshot, download it and run a restore rehearsal.
Alert on service failures and a newest successful backup older than 26 hours.
The daily schedule implies up to roughly one day of data loss; use WAL archiving
or managed PostgreSQL PITR if that is unacceptable. Local verified dumps retain
14 days by default (`MARUM_BACKUP_RETENTION_DAYS`); restic keeps 14 daily,
8 weekly and 12 monthly snapshots (`MARUM_RESTIC_KEEP_*`). Retention runs only
after a successful replacement. Export these settings in `/etc/marum/backup.env`.
The offsite archive includes the erasure journal resolved from the application's
actual bind mount (including stopped application containers). A missing or ambiguous
container or journal directory aborts the backup. `MARUM_COMPOSE_ENV` in the backup
service selects the same Compose environment file as the application; a separately
exported journal path cannot redirect this backup to a stale directory. An older journal is unsafe for
recovery: reconcile against the latest independent journal before enabling
traffic. If the host is lost and journal freshness cannot be established, keep
the restored service offline until deletion requests are reconciled.

For a real recovery, keep the app stopped, provision a fresh volume through the
same role bootstrap, and restore with `pg_restore --exit-on-error --single-transaction
--no-owner --no-acl` as `marum_owner` into the empty `marum` database **before**
running migrations. Apply pending expand-only migrations, then run
`queries/provisioning/restrict-ledgers.sql` as the owner (ACL-free restores do
not retain privilege restrictions). Restore the latest independent erasure
journal, retain the original identity key, and verify readiness and replay
before enabling ingress. Never
restore over a running application or use `down -v` on the live project.

## Releases and rollback

Take and verify a backup before each release. Build a new immutable version and
keep the previous image. A brief maintenance window is intentional here:

```sh
docker compose --env-file deploy/vps/.env -f deploy/vps/compose.yml build
docker compose --env-file deploy/vps/.env -f deploy/vps/compose.yml stop marum
docker compose --env-file deploy/vps/.env -f deploy/vps/compose.yml run --rm migrate
docker compose --env-file deploy/vps/.env -f deploy/vps/compose.yml up -d --no-build --wait
```

If migration fails, leave the app stopped and inspect the failure; do not bypass
the migration gate. For subsequent releases, select a retained image supporting the current
credential and journal formats and start it with `up -d --no-build --no-deps marum`.
**This transition encrypts legacy TOTP secrets at startup. Pre-hardening binaries
cannot authenticate those secrets and must not be used as an automatic rollback.**
Keep the identity key: it now also derives a separate, row-bound admin encryption
key. Roll forward with a compatible image on the first deployment of this change.
Keep the expanded schema. Test previous-binary compatibility before deployment;
never automatically migrate down. DNS/bot cutover and deployment to the actual
VPS have not been performed by preparing these files.

## Performance and operations

- The app pool starts at 16 maximum / 2 minimum connections. PostgreSQL permits
  60 total connections, leaving headroom for maintenance. Do not increase pool
  size until acquisition wait, database CPU and request latency justify it.
- App sessions bound connection establishment (5s), statements (15s), lock waits
  (5s), and idle transactions (15s). Migration/backup owner sessions do not inherit
  app statement limits. Investigate timeouts rather than repeatedly retrying
  financial writes whose outcome may be unknown.
- PostgreSQL starts with 2 GB shared buffers and 8 MB work memory; work memory
  applies per operation, not once per database. Keep fsync, WAL durability and
  autovacuum enabled. Do not enable SQL/parameter logging for diagnosis: financial
  values and identifiers must remain out of telemetry.
- Logs rotate (three 10 MiB files per container). No local Grafana/Loki/Tempo stack
  is started. Set `OTEL_EXPORTER_OTLP_ENDPOINT` and `OTEL_EXPORTER_OTLP_HEADERS` in the
  Compose `.env` to enable the existing HTTP traces/metrics/logs exporters. Optional
  `PYROSCOPE_SERVER_ADDRESS`, `PYROSCOPE_BASIC_AUTH_USER` and
  `PYROSCOPE_BASIC_AUTH_PASSWORD` configure profiling. Keep credentials in the
  private environment file and confirm signals reach the destination before launch.
  Wire external monitoring before launch: HTTPS availability,
  private `/readyz`, queue age, database pool waits, CPU/memory, disk/inodes,
  restarts and backup age. Docker health status alone does not restart a hung
  process; `restart: unless-stopped` handles exited processes.
- Reminder generation uses resumable 500-account pages. It continues on following
  ticks and starts its hourly cooldown only after completing the account sweep.
  Required reminders and confirmation notifications have independent send budgets
  and durable retry delays. Startup menu refresh resumes interrupted passes. Polling also runs the shadow
  evidence sweep with an independent eight-second budget.
  Keep a single application instance until distributed sender coordination exists.
- Plan searches have cancellation and aggregate admission limits; duplicate cache
  misses coalesce. API contexts are bounded to 30 seconds and 32 concurrent
  requests. History lists use 50-item pages. Watch timeout rates and queue ages;
  larger hardware cannot compensate for Telegram delivery limits.
- Use `MARUM_IMAGE`, `MARUM_MIGRATE_IMAGE`, `MARUM_POSTGRES_IMAGE` and
  `MARUM_CADDY_IMAGE` with reviewed registry digests for reproducible deployments.
  Application local-build tags remain available for validation; use registry
  digests for releases. PostgreSQL/Caddy defaults are pinned digests.
- Measure authenticated Mini App latency with representative loan histories on
  staging, including concurrent reads/writes. Existing synthetic tests and pool
  defaults do not establish a production p95/p99 or concurrent-user capacity.

## Disk and host maintenance

The 200-GB disk holds PostgreSQL data/WAL, container images, logs and local dumps.
Keep at least 20% free and alert before it falls below that threshold. The local
14-day backup retention is time-based, not a size cap: review database size and
backup growth before extending retention. Put `MARUM_BACKUP_DIR` outside the
release checkout in `/etc/marum/backup.env`, for example `/var/backups/marum`.
Retain the compatible rollback image; remove only known obsolete images after a
successful release. Never prune database or Caddy volumes as routine maintenance.
Provider VM snapshots supplement the tested database/journal backups.

Start with the bounded app stack and external telemetry rather than adding an
unbudgeted full local observability stack. Track planner latency, pool waits,
container memory/OOMs, host memory, disk/inodes and backup age on the actual VM.
Tune after representative authenticated load tests; this preset establishes no
maximum user count. A single VM remains a single failure domain.

Operational references: [Docker Compose production](https://docs.docker.com/compose/how-tos/production/),
[PostgreSQL 17 memory settings](https://www.postgresql.org/docs/17/runtime-config-resource.html),
and [restic backups](https://restic.readthedocs.io/en/stable/040_backup.html).
