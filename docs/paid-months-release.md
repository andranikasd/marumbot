# Paid months and planning start

In **Loans → open a loan → Paid months**, confirm the last completed month,
remaining principal from the bank, and the next bank instalment. The saved
balance must already include those payments. This records a bank statement;
it does not invent a transfer or deduct the payment from your cash again.
For payments recorded in Marum but not yet reconciled, use **Reconcile** first.
A zero remaining principal closes the repayment schedule.

Then choose **Plan → Plan start → Next month** to defer optional payments.
This requires a budget with dated funding and no outstanding required payment
before that month. Marking one loan paid does not hide payments on other loans.
The pause preserves cash, income dates, actual spending, reserve, and approved
limits. It does not move the valuation date or skip accruing interest.

The shortcut supports the current completed month and, when its next due is
still in the future, the preceding month. A stale or overdue bank statement,
changed contract schedule, or partially paid instalment needs reconciliation;
Marum must not assume those obligations disappeared.

## Release verification

Automated coverage includes:

- First use without loans/budget, fully repaid loans, and unreconciled balances.
- Current-month versus confirmed next-month dues; overdue dates remain unknown.
- September/October, December/January, February 28/29, month-end and account timezone.
- Budget pauses with custom cycles, actual spending, reserves, and monthly overrides.
- Exact decimal input, malformed separators, zero balances, and amount bounds.
- Concurrent edits, repeated requests, uncertain network outcomes, ownership,
  and a retry after midnight or archival.
- Balance edits preserving confirmed due dates, changed terms clearing old dates,
  and paid reminders not returning through snooze.
- Language changes during navigation, cold-load translations, button cleanup,
  and clear loading, empty, reconciliation, authentication, and retry states.

The checks are deterministic regression tests, HTTP tests, browser-script UI
harnesses, and real PostgreSQL tests. They do not replace a real Telegram-client
smoke test through the production Cloudflare/Nginx route.

After deployment, launch from the bot's authenticated Mini App button, switch
both languages, check empty and populated accounts, save a bank-confirmed paid
month, choose next-month planning, inspect the resulting schedule, and sign
into admin. Do not enter fabricated payments on a real financial account for
testing. Confirm `/readyz` reports the release SHA. Use a dedicated test account
for a production smoke test.
