# Messaging

Telegram communication is private-chat only. The
[normalizer](../../internal/adapter/in/telegram/update.go) rejects group/channel
messages, bots, invalid IDs and mismatched sender/chat ownership, including
callbacks. The outbound [client](../../internal/adapter/out/telegramclient/client.go)
also refuses non-positive chat IDs so legacy group bindings cannot receive
financial messages.

## Inbound durability and retries

```mermaid
sequenceDiagram
    participant T as Telegram
    participant W as Webhook
    participant D as PostgreSQL
    participant C as app.Worker
    W->>W: Verify configured secrets; normalize private update
    W->>D: Persist command, unique telegram_update_id
    D-->>W: Accepted or duplicate
    W->>T: Acknowledge callback where applicable
    opt Newly accepted command
        W->>C: HandleOne, bounded to 10 seconds
        C->>D: Lease the named command
        C->>C: Apply use case
        C->>T: Send reply
        C->>D: Complete/fail with fencing token
    end
    W-->>T: HTTP 200 after durable acceptance
```

The [webhook](../../internal/adapter/in/telegram/webhook.go) attempts immediate
handling after persistence. Failure of that attempt does not turn an accepted
command into HTTP 500: the internal tick drains pending work later. Persistence
failure does return 500. Unsupported or malformed updates are acknowledged
without creating a financial command.

The durable inbox holds payload and processing state, not just a deduplication
ID. Leases carry fencing tokens; a stale worker cannot complete a reclaimed
lease. [Inbox policy](../../internal/app/inbox.go) sets five attempts with
backoff; the hourly reminder walk purges completed rows older than seven days.
Telegram update deduplication is therefore not permanent history.

Loan/budget/payment/preference/activation mutations have their own durable
command identities and transaction boundaries. Their receipts prevent a lost
response from appending the same fact or consuming another aggregate revision;
see [database](05-database.md). The bot's whole apply-and-reply path is not one
transaction, and a financial receipt does not guarantee exactly-once messaging.

## Outbound delivery

[app.Worker](../../internal/app/worker.go) and
[reminders](../../internal/app/reminders.go) call the Telegram client directly.
The schema retains notification delivery/item tables and admin views, but the
current wiring does not implement a single-leader, globally rate-limited outbox
sender, cross-loan reminder bundles or editing a previous reminder in place.

Delivery is at-least-once: Telegram may accept a message before Marum can record
success, so retry can duplicate it. Reminder copy uses explicit dates and must
remain understandable on repeat. The client classifies throttling, server and
transport failures; this is not a guarantee of distributed throughput or a
measured delivery SLO.

## Language and preferences

English (`en`) and Armenian (`hy`) are supported. Locale is persisted on the
user and shared by Mini App and bot; changing language updates the bot's menu
and subsequent messages. See [worker language handling](../../internal/app/worker.go)
and [user storage](../../queries/telegram.sql).

[Preferences](../../internal/app/user_preferences.go) persist an IANA timezone,
optional quiet window and version, with durable retry receipts. Quiet windows
may cross midnight. The schema defaults quiet hours to disabled (stored window
22:00–08:00); there is no fixed universal 09:00–21:00 sending rule.

## Required and optional reminders

| Rule | Current behavior |
| --- | --- |
| Required defaults | Three days before and on the due date, at 10:00 local; see [SQL](../../queries/loans.sql). Preferences cannot disable required reminders. |
| Scheduling | Generate within a fourteen-day horizon. The internal tick invokes a reminder walk gated to hourly within the process. |
| Priority | Required reminders fill the delivery batch first; optional reminders use remaining capacity. |
| Quiet hours | Filter before the batch limit and recheck before sending. |
| Snooze | Versioned, owned occurrence command with retry receipt; move delivery to a future instant within seven days, preserving contractual due date. |
| Send-time checks | Re-read occurrence status and target time; guarded completion preserves a snooze arriving during the send. |
| Optional extras | Refer to a positive extra-payment action in the approved dated timeline, identified by plan and action index; explicitly labelled optional. |
| Optional freshness | Recheck active approval and original source identity before sending; cancel obsolete/stale actions. Midnight alone does not invalidate an approved timeline, but a past action is not sent. |

Optional reminders store notification identity/state, not evidence of payment or
a copied authoritative projection. Amounts come from replay of the approved
manifest. Previewing or saving a scenario does not approve reminders.

Evidence: [optional reminder use case](../../internal/app/optional_reminders.go),
[preference SQL](../../queries/user_preferences.sql),
[optional reminder SQL](../../queries/optional_reminders.sql),
[freshness regressions](../../internal/app/optional_reminders_freshness_test.go)
and [development acceptance](../design/v3/development-acceptance.md).
