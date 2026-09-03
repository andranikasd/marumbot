package plan

import (
	"fmt"
	"math/big"

	"github.com/andranikasd/marumbot/pkg/core/money"
)

// ReleasedReduction is permission removed after a confirmed required-payment
// reduction. Retained amounts apply per released payment; percentages round
// half-up to the currency settlement unit. A projection is never a source.
func ReleasedReduction(before, after money.Amount, rule string, retain money.Amount, percentPPB int64, confirmed bool) (money.Amount, error) {
	cur := before.Currency()
	known, err := money.Lookup(cur.Code)
	if err != nil || known != cur {
		return money.Amount{}, fmt.Errorf("plan: invalid released-payment currency")
	}
	if before.Currency() != after.Currency() || before.Sign() < 0 || after.Sign() < 0 {
		return money.Amount{}, fmt.Errorf("plan: invalid released-payment facts")
	}
	zero := money.Zero(cur)
	if !confirmed || after.Cmp(before) >= 0 || rule == RollAll {
		return zero, nil
	}
	released, err := before.Sub(after)
	if err != nil {
		return money.Amount{}, err
	}
	kept := zero
	switch rule {
	case ReleaseAll:
	case RollAmount:
		if retain.Currency() != cur || retain.Sign() < 0 {
			return zero, fmt.Errorf("plan: invalid retained payment")
		}
		kept = retain
	case RollPercent:
		if percentPPB < 0 || percentPPB > 1_000_000_000 {
			return zero, fmt.Errorf("plan: invalid retained percentage")
		}
		numerator := new(big.Int).Mul(big.NewInt(released.Minor()), big.NewInt(percentPPB))
		unit := money.DefaultPolicy(cur).Unit
		denominator := new(big.Int).Mul(big.NewInt(1_000_000_000), big.NewInt(unit))
		units, remainder := new(big.Int), new(big.Int)
		units.QuoRem(numerator, denominator, remainder)
		if remainder.Lsh(remainder, 1).Cmp(denominator) >= 0 {
			units.Add(units, big.NewInt(1))
		}
		units.Mul(units, big.NewInt(unit))
		switch {
		case units.Cmp(big.NewInt(released.Minor())) > 0:
			kept = released
		case !units.IsInt64():
			return zero, money.ErrOverflow
		default:
			kept = money.FromMinor(units.Int64(), cur)
		}
	default:
		return zero, &UnsupportedError{Feature: "released-payment goal condition"}
	}
	if kept.Cmp(released) > 0 {
		kept = released
	}
	return released.Sub(kept)
}
