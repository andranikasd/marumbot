// Package model holds the value types the engine reasons about: what a
// contract says, what the lender reported, what the borrower reported, and
// what Marum derived from the three.
//
// Nothing here performs I/O, reads a clock, or logs. Validation is total: a
// value that fails Validate cannot reach the engine.
package model

import (
	"fmt"

	"github.com/andranikasd/marumbot/pkg/core/money"
)

// Bucket names a component of what a borrower owes. They are settled in a
// lender-defined order, and the order is a contract fact rather than a
// convention (see package allocation).
type Bucket uint8

const (
	Penalties Bucket = iota
	OverdueFees
	CurrentFees
	UnpaidInterest
	AccruedInterest
	OverduePrincipal
	Principal
	AdvanceCredit // a credit held by the lender, not a debt
)

var bucketNames = map[Bucket]string{
	Penalties: "penalties", OverdueFees: "overdue_fees", CurrentFees: "current_fees",
	UnpaidInterest: "unpaid_interest", AccruedInterest: "accrued_interest",
	OverduePrincipal: "overdue_principal", Principal: "principal",
	AdvanceCredit: "advance_credit",
}

func (b Bucket) String() string {
	if n, ok := bucketNames[b]; ok {
		return n
	}
	return "unknown"
}

// ParseBucket reads a persisted bucket name.
func ParseBucket(s string) (Bucket, error) {
	for b, n := range bucketNames {
		if n == s {
			return b, nil
		}
	}
	return 0, fmt.Errorf("unknown bucket %q", s)
}

// Buckets is the full financial position of a loan at a moment in time.
// Every field is a non-negative amount in the loan's currency.
//
// The design keeps these separate rather than collapsing to one balance
// because a payment is allocated across them in a lender-defined order, and
// because "you owe 1,840,000" is a different statement from "you owe
// 1,800,000 principal, 38,000 accrued interest and 2,000 in fees".
type Buckets struct {
	Principal        money.Amount
	AccruedInterest  money.Amount
	UnpaidInterest   money.Amount
	CurrentFees      money.Amount
	OverdueFees      money.Amount
	Penalties        money.Amount
	OverduePrincipal money.Amount
	AdvanceCredit    money.Amount
}

// NewBuckets returns an all-zero position in cur.
func NewBuckets(cur money.Currency) Buckets {
	z := money.Zero(cur)
	return Buckets{z, z, z, z, z, z, z, z}
}

// Currency returns the currency the position is denominated in.
func (b Buckets) Currency() money.Currency { return b.Principal.Currency() }

// Get returns the amount held in one bucket.
func (b Buckets) Get(k Bucket) money.Amount {
	switch k {
	case Penalties:
		return b.Penalties
	case OverdueFees:
		return b.OverdueFees
	case CurrentFees:
		return b.CurrentFees
	case UnpaidInterest:
		return b.UnpaidInterest
	case AccruedInterest:
		return b.AccruedInterest
	case OverduePrincipal:
		return b.OverduePrincipal
	case AdvanceCredit:
		return b.AdvanceCredit
	default:
		return b.Principal
	}
}

// With returns a copy of b with one bucket replaced.
func (b Buckets) With(k Bucket, v money.Amount) Buckets {
	switch k {
	case Penalties:
		b.Penalties = v
	case OverdueFees:
		b.OverdueFees = v
	case CurrentFees:
		b.CurrentFees = v
	case UnpaidInterest:
		b.UnpaidInterest = v
	case AccruedInterest:
		b.AccruedInterest = v
	case OverduePrincipal:
		b.OverduePrincipal = v
	case AdvanceCredit:
		b.AdvanceCredit = v
	default:
		b.Principal = v
	}
	return b
}

// TotalOwed is everything the borrower would pay to close the loan today,
// before any early-repayment fee, less any credit the lender is holding.
func (b Buckets) TotalOwed() (money.Amount, error) {
	sum := money.Zero(b.Currency())
	var err error
	for _, a := range []money.Amount{
		b.Principal, b.OverduePrincipal, b.AccruedInterest, b.UnpaidInterest,
		b.CurrentFees, b.OverdueFees, b.Penalties,
	} {
		if sum, err = sum.Add(a); err != nil {
			return money.Amount{}, err
		}
	}
	return sum.Sub(b.AdvanceCredit)
}

// IsClosed reports whether nothing remains to pay.
func (b Buckets) IsClosed() bool {
	t, err := b.TotalOwed()
	return err == nil && t.Sign() <= 0
}

// HasArrears reports whether the loan carries anything the MVP engine refuses
// to plan around: penalties, overdue principal, overdue fees or unpaid
// interest. Reminders continue for such a loan; projections do not.
func (b Buckets) HasArrears() bool {
	return b.Penalties.Sign() > 0 || b.OverduePrincipal.Sign() > 0 ||
		b.OverdueFees.Sign() > 0 || b.UnpaidInterest.Sign() > 0
}

// Validate rejects a position that cannot exist.
func (b Buckets) Validate() error {
	cur := b.Principal.Currency().Code
	for k, a := range map[string]money.Amount{
		"principal": b.Principal, "accrued_interest": b.AccruedInterest,
		"unpaid_interest": b.UnpaidInterest, "current_fees": b.CurrentFees,
		"overdue_fees": b.OverdueFees, "penalties": b.Penalties,
		"overdue_principal": b.OverduePrincipal, "advance_credit": b.AdvanceCredit,
	} {
		if a.Sign() < 0 {
			return fmt.Errorf("bucket %s is negative", k)
		}
		if a.Currency().Code != cur {
			return fmt.Errorf("bucket %s is %s, expected %s", k, a.Currency().Code, cur)
		}
	}
	return nil
}
