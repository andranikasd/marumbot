# Runbooks

## Reminders stopped

The failure most likely to go unnoticed, because nothing errors — it simply
goes quiet.

1. Check `marum_tick_last_success_timestamp`. Older than 5 minutes means the
   scheduler is not ticking.
2. Check the sender leader: `sum(marum_sender_is_leader)` must equal exactly 1.
   Zero means nothing is sending; more than one means the rate limit is not
   global.
3. Check the outbox backlog and Telegram error codes. If 429-dominated, lower
   the global rate and let it drain.
4. Invoke a tick by hand against `/internal/tick` and watch the backlog.

## A user reports a wrong number

**Do not change any code first.**

1. Find the loan in the admin interface. Read its reliability and reasons.
2. If it is `needs_reconciliation` or `unsupported`, the product is behaving
   correctly and the answer is a bank-confirmed balance.
3. If it is `confirmed`, reproduce from the stored inputs and the recorded
   `EngineVersion`.
4. If the engine is right, the loan data is wrong and the fix is a bot flow.
5. If the engine is wrong, **add the case to `testdata/golden` before touching
   any code.** The corpus is the asset.

## Reminders sent twice

Expected, rarely. Delivery is at-least-once and the gap between Telegram
accepting and Marum marking cannot be closed.

Confirm which kind it is from `notifications`:

- **Two rows** → a scheduling bug. Real defect.
- **One row, sent twice** → the accepted at-least-once behaviour, unless the
  rate is rising, which would mean the mark is failing.

## Reconciliation mismatch

The cache and the ledger disagree, which means one is wrong and it is not the
ledger.

1. Rebuild the cache by replaying.
2. Compare the `event_set_hash` before and after.
3. If drift is concentrated on one allocation policy version, **disable
   confident planning for that policy** and re-read the contract. Repeated
   drift means the policy is wrong, not the arithmetic.

## Accrual overflow

`marum_accrual_overflow_total` incremented. This must never happen.

1. Stop showing projections for the affected loans.
2. The 128-bit path in `money.Accrue` was bypassed or a principal exceeded even
   that range.
3. Add the exact principal, rate and period as a test case first.

## Bot token compromised

1. Revoke via BotFather.
2. Set the new token as a Cloudflare secret and redeploy.
3. Re-register the webhook with a **fresh secret and a fresh path**.
4. Verify with the synthetic check.
5. Post-mortem within 48 hours.

Alerting uses a **separate bot token**, so a compromise of the product's token
cannot silence the alarm about it.

## Restore drill — monthly

1. Restore the newest dump into an isolated database.
2. Apply every deletion-journal entry included in or newer than the backup.
3. Verify erased subject HMACs are absent.
4. Run migration verification, constraint checks, application startup, identity
   decryption for each active key version, and `replay == loan_state`.
5. Record the wall-clock restore time.

Row counts prove nothing. An untested backup is not a backup.
