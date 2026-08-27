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

| Lender | Product | Rows | Convention |
| --- | --- | --- | --- |
| Inecobank | Consumer loan, 60 months | 55 | ACT/365, 0.10 AMD, half-up |
| CBA Reg 8/01 | Regulator's worked example | 12 | ACT/365, 0.01 AMD, half-up |

Phase 1 does not close until ten schedules from four lenders reproduce.
