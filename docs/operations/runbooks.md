# Runbooks

Current dev release evidence: [releases.md](releases.md). No production exists.

## Reminders stopped

1. Check `/healthz`, `/readyz` and `/status`. The latter exposes
   `commands_pending`, `deliveries_pending`, `oldest_pending_command_s` and
   `oldest_pending_delivery_s`; a 200 alone does not prove database health.
2. Inspect the `tick` log and scheduler spans. Cloudflare cron runs every five
   minutes; local polling uses `MARUM_TICK_INTERVAL` (default 60 seconds).
   `TickReminders` gates a successful reminder walk to once per hour per process.
3. Look for `tick failed`, `reminder tick failed`, `scheduling reminders failed`
   and Telegram send errors. Inspect reminder rules, occurrences and user
   preferences, including quiet hours/snoozes, before assuming a missed delivery.
4. For a local diagnostic tick, with the local service token unset:

   ```bash
   curl -fsS -X POST http://127.0.0.1:8080/internal/tick
   ```

   The container endpoint requires `X-Marum-Service-Token` when configured.
   The public Worker does not forward `/internal/tick`; do not use the dev public
   URL as a manual tick endpoint. A tick can send due messages.

`marum_tick_last_success_timestamp` and `marum_sender_is_leader` are not emitted
by current code. Do not diagnose the scheduler from those absent series. The
reminder path sends directly and marks `reminder_occurrences` delivered (satisfied in the legacy path); the
notification outbox alone is not a complete reminder-delivery view.

## A user reports a wrong number

1. Read the loan's reliability and reasons in the admin interface.
2. For `needs_reconciliation` or `unsupported`, inspect the reason and obtain a
   bank-confirmed balance where required; do not substitute a confident estimate.
3. Reproduce using the stored inputs, application revision and recorded engine
   metadata. Current planning certificates/manifests use `plan/5`, independently
   of the application version `2.0.3`.
4. Use authorized plan-history replay where available; check its engine/version
   compatibility result. It is not a generic loan-state rebuild command.
5. If inputs are wrong, correct them through supported user/admin flows. If the
   engine is wrong, add the case to `testdata/golden` before changing arithmetic.

## Reminders sent twice

Telegram acceptance and the database mark are separate operations, so a crash
or failed mark can cause at-least-once delivery. Inspect `reminder_occurrences`
for the direct reminder path and `notification_deliveries` for outbox records;
there is no `notifications` table.

Compare the logical occurrence and send/mark errors. Different valid
occurrences can generate separate messages; two rows alone do not prove a bug.
Repeated sends of one occurrence warrant checking failed marks and retries.
Do not erase ledger or billing events while investigating duplicates.

## Reconciliation mismatch

Inspect the bank snapshot, append-only event history, allocation policy and
`loan_state` provenance together. Compare replay for the same inputs and engine
version, including `event_set_hash`; different input sets legitimately produce
different hashes. The admin reconciliation page is an inspection surface, not
an automatic cache repair command.

There is no general operator CLI to rebuild loan caches. Reproduce drift in a
fixture and use the supported reconciliation flow or a reviewed repair. If an
allocation policy is unsupported or suspect, obtain a bank-reported balance
and review the lender contract before enabling confident planning.

## Accrual overflow

`money.Accrue` uses a checked 128-bit intermediate; overflow does not by itself
prove that path was bypassed. Capture the exact inputs privately and reproduce
with a test before changing arithmetic. Do not log amounts or user identifiers.

`marum_accrual_overflow_total` is declared in `internal/obs/metrics.go`, but the
current application has no increment call. An absent/zero series is not proof
that accrual cannot fail; inspect the returned errors and affected operation.

## Bot token compromised

1. Revoke the affected bot token through BotFather.
2. Update `MARUM_BOT_TOKEN` in the GitHub dev environment, and rotate
   `MARUM_WEBHOOK_SECRET` and `WEBHOOK_PATH` there as well.
3. Redeploy from `main` so CD syncs the replacement runtime secrets. Editing only
   Cloudflare would be overwritten by the next GitHub-driven sync.
4. Register the webhook with the new path/header secret as described in
   [deployment.md](deployment.md); CD does not register it automatically.
5. Verify `getWebhookInfo`, the versioned bot menu and authenticated bot behavior.
   Record the exposure window and remediation in an incident review.

No separate alert-bot deployment or synthetic watchdog is configured in this
repository. Do not rely on one to detect or report this incident.

## Rollout failed

Use the failed step to distinguish infrastructure, migration, deploy, secret
sync and smoke errors. Smoke failure rolls back only when a prior Worker version
was captured, targeting that explicit pre-release version. Schema expansion
remains applied. A Grafana annotation 401 is optional and does not undo a
successful deployment. See [deployment.md](deployment.md) for exact sequencing.

## Restore drill

Terraform provisions `marum-dev-backups` and retention rules, but no scheduled
dump/upload or automated restore drill exists here. First establish that a
usable dump and the required identity keys exist; a bucket alone is not a backup.

Restore only into an isolated database. Preserve and apply erasure records
(`deletion_tombstones`) so a restore cannot resurrect erased identities. Verify
schema/constraints, application startup, identity decryption and replay against
the restored inputs; record restore duration and discrepancies. A complete
restore and erasure-reapplication procedure still needs implementation and
validation before it can be treated as operational coverage.
