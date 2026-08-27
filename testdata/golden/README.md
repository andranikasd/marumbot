# The correctness corpus

Every file here is a repayment schedule a real lender actually issued, with the
figures they actually printed. A test replays each one row by row and fails if
this engine disagrees by a single minor unit.

This is the asset. The engine is replaceable; the evidence that it matches real
paperwork is not.

## Why it exists

A loan calculator is easy to write and hard to trust. The arithmetic is
elementary, and every plausible-looking implementation produces a different
answer, because the answer depends on choices nobody writes down:

- whether interest accrues over **actual days** or over twelve equal twelfths
- which **day-count** divides them — 365, 360, or 30/360
- what the intermediate figures are **rounded to**, and when
- whether the **last payment** absorbs the residue

Get any one wrong and the schedule is close, plausible, and not the lender's.
For this Inecobank loan the textbook constant-period formula is out by 88.91 AMD
an instalment — 5,335 AMD over five years — while looking entirely reasonable.

A borrower does not report that as a rounding difference. They report that the
app is wrong, and stop believing the rest of it.

## What a fixture records

Each file carries the source document, the terms, the convention that
reproduces it, and every published row. The convention is **per lender**,
because it is a property of their system rather than of Armenian law: nothing
in the Civil Code, the Consumer Lending Law, the Mortgage Law or CBA
Regulations 8/01 and 8/05 prescribes a rounding rule for payments.

## Adding one

Any real schedule is worth adding, especially one that disagrees. A fixture
that fails is more valuable than one that passes: it names a convention the
engine does not yet know.

1. Copy a lender's schedule into a new `*.json` here.
2. Run `make test`. If it fails, the difference is the finding.
3. Record the convention that reproduces it. Do not adjust the published
   figures to fit the engine — the paperwork is the authority.

## Coverage

| Fixture | Rows | Exact | Instalment | Accrual |
| --- | --- | --- | --- | --- |
| Inecobank M26/029210, loan agreement | 60 | **59 / 59** | dated solve | ACT/365, 0.10 AMD |
| Inecobank M26/029210, re-issued schedule | 55 | 52 / 54 | dated solve | ACT/365, 0.10 AMD |
| Unibank, published annuity example | 12 | 8 / 11 | **rate / 12** | ACT/365, 0.01 AMD |
| CBA Reg 8/01, worked example | 12 | — | — | ACT/365, 0.01 AMD |

Two lenders, and already two conventions for the instalment. Both accrue
interest daily on the declining balance over a 365-day year — as does every
one of the ten Armenian banks whose terms state a rule. They differ in how the
level payment is set: Inecobank solves it over the actual dated schedule, and
Unibank uses the textbook formula with r = annual/12 (90,258.31 reproduces
exactly that way; the dated solve gives 90,269.17).

That is why a contract carries the lender's stated instalment rather than
always deriving one. The accrual is the engine's; the instalment is the bank's.

The first two are the same loan, and they disagree with each other. The
agreement prints 69,045.40 for 24/09/2026; the schedule the bank re-issued five
months later prints 69,045.30. The engine matches the agreement.

One lender, one loan, two documents, a tenth of a dram apart. That is the whole
argument for measuring against real paperwork instead of against a formula:
no amount of reasoning about conventions would have predicted it.

Phase 1 does not close until ten schedules from four lenders reproduce.
