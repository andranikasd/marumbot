# How budgeting works

Marum needs two things before it can plan a payment: **money available on the
payment date**, and **permission to spend it within your budget**. Required
payments come first. Extra payments use what remains under both constraints.

## What to enter

| Field or concept | Meaning |
| --- | --- |
| Monthly spending limit | Your loan spending limit for the budget period, including required payments |
| Money set aside each month | Regular money set aside for loans; its availability date matters |
| Payday | When the regular money becomes available, not a reset of spending |
| Cash available now | Money already held today; avoid also declaring it as a future receipt |
| Keep untouched | Cash you want to keep untouched |
| Already paid / spent | Loan payments already made in the current spending period; not new cash |
| Additional receipt | A separate incoming amount and date; distinguish confirmed from expected |
| Specific month | A replacement limit for that month, not extra cash or an addition to the normal limit |

Budget periods, receipt dates and reporting months serve different purposes.
A later payday does not erase spending already counted in the current period.
All declarations are currency-specific; Marum does not invent exchange rates.

## A practical order

1. Check your loan balances, required payments and statement dates.
2. Choose the total loan spending limit you can commit to, including required
   payments. Raising this limit alone does not fund the plan.
3. Declare the regular money and when it arrives. Enter cash already on hand
   separately and protect the reserve you need.
4. Check payments already recorded in the current period. Do not count paid
   money as still available or reset spending merely because income arrived.
5. Add genuine additional receipts. Mark uncertain receipts as expected.
6. Use a month override only when that month's spending limit should differ.
7. Review the plan's funding assumptions and warnings before activation.

## Why can a payment be blocked?

| Situation | What it means |
| --- | --- |
| Enough budget, too little cash | The spending limit allows the payment, but usable money has not arrived |
| Enough cash, too little budget | Cash is available, but the spending limit or already-spent total constrains it |
| Receipt arrives after a due date | Later income cannot fund an earlier payment |
| Money is reserved or held by a routing rule | It is not automatically available for every loan |
| A receipt is only expected | It is excluded from base-plan funding until confirmed |

A newly added expected one-time receipt can be assumed in a scenario preview,
but that scenario cannot be activated until the receipt is confirmed. Existing
expected receipts do not become funded merely by opening a scenario.

Changing carry, release or cash-routing policies is an explicit declaration;
do not assume unused permission means cash exists. The plan shows unsupported
or inconsistent situations rather than inventing missing funds.

## Editing and checking actual payments

Open **Budget** from Home or **Edit budget** from More. The editor has three groups:

- **Each month:** your spending limit, money set aside regularly, and arrival day.
- **Today:** cash you still have, your protected reserve, and payments already made.
- **Extras:** additional receipts, different limits for specific months, and advanced rules.

Use **Next: today’s money** after the monthly questions, then **Save budget**.
The overview shows regular money and the spending limit separately. Collapsed
help remains available in English and Armenian.

Record actual payments with their dates. If the lender provides an allocation
between principal, interest and fees, enter it; otherwise the allocation remains
unknown. Use statement reconciliation to anchor the bank balance and declare
cash/spending after the covered payments. Editing a payment already covered by a
statement requires a fresh statement rather than silently rewriting the anchor.

[Development acceptance](../design/v3/development-acceptance.md) describes
supported policies and accounting regressions. [Domain model](../architecture/02-domain-model.md)
describes the source facts behind these screens.
