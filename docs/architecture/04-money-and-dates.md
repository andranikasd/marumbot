# Money, currencies and dates

The two packages where a subtle bug becomes a wrong number in front of a
borrower.

## Money is an integer, always

```go
type Amount struct {
    minor int64      // unexported: no arithmetic outside this package
    cur   Currency
}
```

No `float64` anywhere on the path from a stored rate to an accrued amount. The
only package permitted to use one at all is `rates`, which produces a *rate*,
not an amount, and rounds before returning.

Currency mismatch **panics**. Mixing currencies is a bug in the caller, not a
condition a user can cause, and a panic surfaces it in a test long before it
reaches anyone.

## Exponent and settlement unit are different facts

```go
type Currency struct {
    Code           string
    Exponent       uint8   // decimal places in the minor unit
    SettlementUnit int64   // minor units a payable amount rounds to
}
```

| | Exponent | Settlement | Why |
| --- | --- | --- | --- |
| AMD | 2 | **10** | ISO gives two decimals; a real loan agreement settles to a tenth of a dram |
| USD, EUR | 2 | 1 | Cents circulate |
| JPY, KRW | **0** | 1 | Assuming two decimals inflates every amount 100× |
| KWD, BHD | **3** | 1 | Assuming two decimals *loses* a digit |

34 currencies are registered. An unknown code is **rejected**, never assumed to
have two decimal places.

AMD settled to 100 — whole drams — until the corpus arrived, on the reasoning
that the luma stopped circulating decades ago and so nobody can pay one. The
Inecobank agreement refutes it: the instalment is 125,079.60, the first month's
interest is 73,018.20, and not one figure in its 60 rows is a whole dram. The
coin is gone; the accounting unit is not. Ten minor units is the only unit that
reproduces the schedule, so that is what AMD now settles to.

It remains a fixture-backed default, not a lender-wide rule. A contract
carries its own `Rounding` policy; the corpus records the tested policy per
fixture. The source does not establish every lender’s settlement behavior.

Display follows the settlement unit: a whole-dram amount prints as
`1,740,927 AMD` rather than `1740927.00 AMD`, and an amount that is not a whole
dram keeps its digits — `125,079.60 AMD` — because hiding them would hide a
rounding bug.

## Accrual must not run in 64-bit arithmetic

```
interest = principal × rate × days / (denominator × 10⁹)
```

With money in minor units and rates in parts per billion, the intermediate
product **overflows `int64` at ordinary Armenian loan sizes**:

| Loan @ 18% | × rate | × 31 days | int64 |
| --- | ---: | ---: | --- |
| 4,000,000 AMD | 7.2 × 10¹⁶ | 2.2 × 10¹⁸ | fits |
| **16,529,340 AMD** | — | 9.22 × 10¹⁸ | **last value that fits** |
| 30,000,000 AMD | 5.4 × 10¹⁷ | 1.7 × 10¹⁹ | overflow |
| 80,000,000 AMD | 1.44 × 10¹⁸ | 4.5 × 10¹⁹ | overflow — 66 bits |

At a card rate of 26% the ceiling falls to about 11,443,389 AMD. Mortgages here
exceed both, and the failure mode of the naive version is a **silent wrap**.

So `money.Accrue` accumulates the numerator in 128 bits via `math/bits.Mul64`
and divides once. Operands are grouped as `(principal × days) × rate` purely to
keep the first product inside `int64` — there is still exactly one division and
one rounding step, so the result matches the mathematical definition.

Reordering into `(principal × days) / denominator × rate` would dodge the range
problem by rounding twice, and silently change the schedule. Don't.

Overflow is a typed error, never a wrap. `marum_accrual_overflow_total` is
declared but has no current application increment call; a zero or absent series
does not prove that accrual cannot fail. Inspect returned errors and reproduce
the affected operation. See [runbooks](../operations/runbooks.md).

## Day count is a contract term

`Actual365`, `Actual360`, `Thirty360`, and `ActualActual` are declared.
The money package has an `AccrueBetween` helper for year-specific spans, but
current amortisation and ledger paths call day-count-only `Accrue`; the enum
alone does not prove calendar-year-aware Actual/Actual support. See
[accrual](../../pkg/core/money/accrual.go) and
[schedule construction](../../pkg/core/amortisation/schedule.go). A 31-day month accrues more than a
30-day month under Actual/365 — visible on a real statement, and the reason
naive `rate/12` calculators disagree with banks.

Actual/365 is the Armenian consumer default in the weak sense that the one
contract in the corpus states it outright: clause 2.1.3 of the Inecobank
agreement (M26/029210) says interest is computed on the declining balance of
the loan, daily, on a 365-day year. That is quoted rather than inferred, and it
is one lender. The day count therefore stays a contract term, read from the
paperwork loan by loan.

The size of the choice, on that contract — 5,013,002.00 AMD at 17.15% nominal
over 60 monthly instalments:

| Level instalment | Method |
| ---: | --- |
| 124,990.69 | Constant-period annuity at `rate/12` — 88.91 short, every month |
| 125,079.63 | Dated: actual days, 365-day year, which is 125,079.60 at the contract's own quantum |
| **125,079.60** | **What the agreement prints** |

Eighty-nine drams a month is 5,334.60 over the five years, from a formula that
looks entirely reasonable and is used by most calculators. A borrower does not
report that as a rounding difference. They report that the app is wrong.

## Interest is quantised, and the unit is inferred

The tested agreement uses **0.10 AMD, half-up** before splitting the
instalment into interest and principal. This policy was inferred from the
published figures and row comparisons; it is evidence for this fixture, not
a legal conclusion or a default that proves support for another lender.

That is why `money.Policy` carries the mode and the unit per contract instead
of per currency. The currency's settlement unit is only the default a loan
starts from, and it is a guess until a real schedule confirms it.

## The corpus, and what it currently proves

[internal/corpus](../../internal/corpus) compares lender fixtures row by row.
It lives outside `pkg/core` because reading fixtures is I/O. The committed
[manifest](../../testdata/golden/MANIFEST.json) ratchets fixture hashes and exact
row counts; discrepancies are recorded rather than presented as exact coverage.

| Fixture | Total rows | Exact rows | Recorded support |
| --- | ---: | ---: | --- |
| Inecobank M26/029210 agreement | 60 | 59 | provisional |
| Inecobank M26/029210 reissued schedule | 55 | 52 | experimental |
| Unibank annuity worked example | 12 | 8 | experimental |
| CBA Regulation 8/01 example | 0 | 0 | regulatory example, not a repayment-row proof |

The agreement’s 59 non-final rows match; the last absorbs lender drift. The
reissue differs on two non-final rows and the final row. Those differences do
not establish an undocumented internal rounding rule. One profile matching
one agreement is not lender-wide support; the agreement remains provisional
pending its boundary matrix. Unknown allocation remains `unknown/v0` and
unverified clauses, posting calendars and fee maxima remain unknown/refused.

The [development acceptance bounds](../design/v3/development-acceptance.md)
separate fixture evidence, reduced-domain planner proofs and remaining physical
Telegram/usability/bank-reconciliation validation. The corpus is not a general
optimality certificate or production acceptance claim.

## Showing the arithmetic

`amortisation.Explain` renders one row as the calculation that produced it: the
opening balance, the number of days, the rate, `balance × rate × days / 365`,
the unit the result was rounded to, and the split into principal and interest.
The bot prints it on request.

"Trust the number" is not an argument a borrower can check, and a schedule that
shows only the answer is indistinguishable from a schedule that is wrong. A row
laid out this way can be followed against their own paperwork line by line, and
a disagreement becomes a specific one.

## Dates carry no clock

A due date is a business fact. Attaching a time to it invites an off-by-one
every time a value crosses a zone boundary, so `date.Date` has no time and no
zone. `AtLocal` is the single sanctioned crossing into an instant.

### Anchored schedules — the trap

```go
AddMonths chain:      31 Jan → 28 Feb → 28 Mar   // drifted
Occurrence(31, n):    31 Jan → 28 Feb → 31 Mar   // what lenders do
```

Chaining month arithmetic loses the contractual day the first time it meets a
short month, and then *every remaining due date on the loan is wrong*.
`Occurrence` carries the anchor day instead of the clamped result.

Both behaviours are pinned by tests so neither gets "fixed" into the other.
