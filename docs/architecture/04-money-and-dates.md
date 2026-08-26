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
| AMD | 2 | **100** | ISO gives two decimals, but the luma is obsolete — lenders settle in whole drams |
| USD, EUR | 2 | 1 | Cents circulate |
| JPY, KRW | **0** | 1 | Assuming two decimals inflates every amount 100× |
| KWD, BHD | **3** | 1 | Assuming two decimals *loses* a digit |

31 currencies are registered. An unknown code is **rejected**, never assumed to
have two decimal places.

Display follows the settlement unit: a borrower sees `1,740,927 AMD`, not
`1740927.00 AMD`. An amount that is *not* a whole settlement unit keeps its
digits, because hiding them would hide a rounding bug.

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

Overflow is a typed error, never a wrap. `marum_accrual_overflow_total` must
stay at zero; any increment means this path was bypassed.

## Day count is a contract term

`Actual365` (the Armenian consumer default), `Actual360`, `Thirty360`. A 31-day
month accrues more than a 30-day month under Actual/365 — visible on a real
statement, and the reason naive `rate/12` calculators disagree with banks.

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
