# The correctness corpus

This directory contains lender-issued schedules, a lender's published example
and a regulatory reference example. It is scoped evidence, not certification of
all products from those lenders. The [manifest](MANIFEST.json) records source
hashes, row counts, exact matches and support states.

The corpus tests preserve known discrepancies and prevent coverage regressions.
They do not assert that every row of every fixture currently matches exactly.

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

Each fixture records its source and the terms/rows available from that source.
The reproduction convention is profile-specific; do not generalize one loan's
payment rounding or final-row treatment to an entire lender. Regulatory examples
are reference inputs, not borrower-issued repayment schedules.

## Adding one

Any real schedule is worth adding, especially one that disagrees. A fixture
that fails is more valuable than one that passes: it names a convention the
engine does not yet know.

1. Copy a lender's schedule into a new `*.json` here.
2. Run `make test`. If it fails, the difference is the finding.
3. Update `MANIFEST.json` provenance/support evidence in the same reviewed change.
   Record the convention that reproduces it. Do not adjust the published
   figures to fit the engine — the paperwork is the authority.

## Coverage

| Fixture | Rows | Exact | Instalment | Accrual |
| --- | --- | --- | --- | --- |
| Inecobank M26/029210, loan agreement | 60 | **59 / 59** | dated solve | ACT/365, 0.10 AMD |
| Inecobank M26/029210, re-issued schedule | 55 | 52 / 54 | dated solve | ACT/365, 0.10 AMD |
| Unibank, published annuity example | 12 | 8 / 11 | **rate / 12** | ACT/365, 0.01 AMD |
| CBA Reg 8/01, worked example | 0 schedule rows in manifest | — | regulatory reference | ACT/365, 0.01 AMD |

Two lenders, and already two conventions for the instalment. Both accrue
interest daily on the declining balance over a 365-day year in these fixtures. They differ in how the
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

The original research target was ten schedules from four lenders. Current
coverage is narrower: the original Inecobank profile is provisional and the
reissued/Unibank examples are experimental in the manifest. That research goal
is not evidence of coverage already achieved by the development release.
See [current acceptance](../../docs/design/v3/development-acceptance.md).
