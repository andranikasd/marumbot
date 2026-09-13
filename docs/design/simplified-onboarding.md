# Guided onboarding and monthly plans

Implemented from the approved September 2026 flow proposal. This guide describes
repository behavior; it is not evidence that this version is deployed.

## Standard journey

1. A Telegram Main Mini App or authenticated menu/inline button opens Marum.
   Verified Telegram identity creates the account if necessary, before private
   reads. Opening a normal bot chat cannot forcibly open the Mini App; `/start`
   offers one launch button.
2. First use shows three short explanations and **Add my first loan**.
3. The loan wizard asks for original and remaining principal, dates, the bank's
   required payment and next unpaid due date, then a review. Interest and bank
   prepayment rules and accrued interest are optional bank details. Unknown interest
   remains unknown.
4. The loan list offers another loan or **Continue**.
5. Each currency shows required payments and asks only for the usual monthly
   extra. Zero is valid. Existing budgets are not imported as extras.
6. The plan appears immediately. It separates required payments from the extra,
   shows per-loan suggestions, and offers reminders after the plan is visible.

The three primary tabs are **Plan**, **Loans**, **Settings**. Language and
reminders live in Settings. Previous cash budgets, plans and payment history
remain accessible as secondary tools. No standard setup step asks when income
arrives, when a budget is available, or for a reserve or opening cash balance.

## Calculation contract

This is a conditional monthly projection, separate from the existing cash
engine and its immutable manifests. Required payments occur on bank due dates;
extras occur at month-end. The first partial month's extra means money still to
pay, without prorating. When all next unpaid obligations start in a future month,
the projection starts in that month.

The usual extra is added above each month's actual required amount. A completed
loan's former required payment is not silently rolled into extra. A month override
replaces the usual extra for that month; zero pauses extra only. Reset removes
the override. Currency groups are never converted or totalled together.

Supported estimates require known interest, fixed instalments, a supported
Actual/365 or Actual/360 basis and explicit compatible early-payment terms
(immediate principal reduction, no prepayment fee, no minimum). Suggestions
allocate extra by highest nominal rate first, with deterministic balance/ID ties.
This is not a claim of global optimality across arbitrary bank policies.

A positive-rate loan needs a bank-reported accrued-interest amount for its current
statement, or a verified drawdown anchor, before a full estimate is possible.
Principal alone never implies zero accrued interest.

Missing facts, unsupported terms and overdue obligations produce a limited
bank-required view with a reason. They never invent a payoff date or savings.
Unallocated extra remains explicit. Declared bank balances remain authoritative;
no new source choice is treated as a transfer or a cash receipt.

## Paid months and corrections

During setup, the next unpaid due date identifies where obligations resume.
The entered remaining principal already includes completed payments. Saving
these facts never subtracts those payments a second time.

**Mark month paid** is the primary loan action. It asks which month is complete
and for current bank figures, then refreshes the monthly plan. Future projected
months cannot be marked paid from the plan. Detailed payment recording and bank
reconciliation remain secondary tools. Unreconciled recorded payments must be
resolved before a new paid-through statement.

Unknown-rate balance-only edits retain unknown interest. Adding a rate later
requires an explicit full terms edit; bank-rule confirmation is a separate source
choice. Overdue dates are retained with a warning, not shifted forward. Unsupported
future-start or balance-above-original declarations receive a specific explanation.

## Reliability and compatibility

- Every financial command retains its exact retry identity. Unknown outcomes
  require an explicit identical retry; version conflicts require reloading.
- Setup saves original and remaining figures atomically. Its bank snapshot has
  deterministic precedence over the opening snapshot in that transaction.
- Empty accounts, incomplete plans, expired authentication and storage failures
  are separate states. Private reads wait for verified session establishment.
- English and Armenian cover eager and lazy screens. Drafts survive language
  changes and uncertain saves; late responses do not take over navigation.
- Schema 27 adds loan source metadata, 28 append-only monthly source preferences,
  and 29 reminder choices. Readiness requires schema 29. Down migrations preserve
  source data; older binaries can continue using the expanded schema.
- Existing cash budgets, payment events and plan manifests are retained. No
  derived monthly projection is persisted.
- New accounts start with reminders off. Opt-in asks Telegram for write access
  when necessary. Existing accounts retain their previous reminder preference
  and timing until changed. Rules follow the chosen local time and quiet hours.

## Release validation

Automated coverage includes the independent approved zero-interest fixture,
interest/month-end arithmetic, zero/reset overrides, freed required payments,
mixed currencies, overdue/unknown terms, exact retries and concurrent settings,
new-user authentication, translations, and real PostgreSQL integration.

Release operators must still verify the actual Telegram launch URL and bot token,
Main Mini App profile entry, notification permission, and mobile rendering on the
production origin. Configure Main Mini App in BotFather; the application's chat
menu and `/start` launch button provide the supported fallback. Deploy migrations
before starting this binary. Do not downgrade or delete database volumes to roll
back the application.

### Local verification record — 2026-09-14

- `make test`: all Go packages passed with the race detector.
- `make lint`: zero issues.
- All 28 Mini App UI suites passed, including dynamic projection-reason and
  Armenian/English translation coverage.
- Real PostgreSQL tests passed with the restricted `marum_app` role. Coverage
  includes source snapshots, paid-month updates, settings concurrency/retry,
  reminder replacement/opt-out and deletion of private projection history.
- Migrations 27–29 were reapplied after their expand-only down steps with source
  choices retained.
- The production Docker image built with 97,197 bytes of initial minified
  JavaScript across five files; secondary screens remain lazy.
- That image reached schema-29 readiness and passed a synthetic HTTP flow:
  verified session → empty loans → language save → loan save and identical retry
  → extra configuration and retry → nine-month zero-interest projection → zero
  monthly override → paid-month bank statement → reminder enable/disable.
- Chromium mobile renders checked English/light Plan, Armenian/dark Extra,
  Armenian/light Reminders and the expanded loan-payment step. This does not
  replace verification in actual Telegram iOS/Android clients after deployment.

The runtime smoke used synthetic credentials and disposable local containers;
those containers were removed. No production account or database was changed.
