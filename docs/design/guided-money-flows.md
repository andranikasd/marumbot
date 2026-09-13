# Guided money and payment flows

The starting screen gives an incomplete account a short checklist: add a loan,
set money available, then view the plan. Failed reads never imply missing data.

Budget setup asks for the amount set aside for loans each month once. An explicit
checkbox uses the same amount as the spending limit; a different limit remains
available and existing differences are preserved. Next come money left today
and payments already made in the current budget period. Both need an answer,
including zero. Reserve, extra income and unusual months are optional. A review
shows the actual submitted values before saving. Saving does not record a bank
transfer or change a loan balance.

Loan actions distinguish a new payment from payments completed before using
Marum. Bank-balance-only corrections are under Other changes. If payment records
need checking, the direct next action is Check bank balances.

Record a payment asks whether the bank has processed it. Bank interest/principal
breakdown is optional. A processed payment leads directly to Check bank balances;
a pending payment explains how to update its status later.

Check bank balances asks for loan facts, then money remaining and total loan
spending for the displayed budget period, then a review and confirmation.
The total already includes the payment: no repeated cash deduction. Custom
budget periods use the server's current projected period. Missing statements,
pending payments and mismatched currencies need correction, never guessed data.

Activity shows payment records first. Monthly totals and plan comparison are
optional and load only when opened. The budget editor loads only when visited.

Both languages must cover initial load, generated review text, errors and retry
states. Navigating or changing language must preserve unfinished answers and
must not change the identity of an uncertain financial request.

Verification uses HTTP boundary tests, exact calculation fixtures, browser-script
workflow tests and database integration tests. No live browser is connected to
this workspace; Telegram device layout and production routing still require a
real-client smoke test before public rollout.
