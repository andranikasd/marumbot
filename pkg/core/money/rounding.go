package money

// Mode selects how a value exactly between two representable results is
// resolved.
type Mode uint8

// The rounding modes a lender may specify.
const (
	HalfUp   Mode = iota // 0.5 rounds away from zero — the Armenian default
	HalfEven             // 0.5 rounds to the even neighbour
	Down                 // truncate toward zero
	Up                   // away from zero whenever there is any remainder
)

func (m Mode) String() string {
	switch m {
	case HalfUp:
		return "half_up"
	case HalfEven:
		return "half_even"
	case Down:
		return "down"
	case Up:
		return "up"
	}
	return "unknown"
}

// Policy is a per-loan property because lenders differ, and a one-dram
// divergence compounded over 360 periods is what turns "your schedule is
// wrong" into a support ticket.
type Policy struct {
	Mode Mode
	// Unit is the settlement granularity in minor units, taken from the
	// currency unless a contract states otherwise. It is a per-lender fact: no
	// Armenian regulation prescribes one, and the corpus records what each
	// lender's paperwork actually does.
	Unit int64
}

// DefaultAMDPolicy is the home-market default, derived from the registry rather
// than restated.
//
// It used to carry its own literal, and when the registry's AMD settlement unit
// was corrected from 100 to 10 against a real loan agreement, this disagreed
// with it silently. Two constants for one fact is one too many: deriving it
// makes that impossible rather than merely unlikely.
//
// Any lender-specific difference must still be confirmed against a real
// repayment schedule before it is encoded.
var DefaultAMDPolicy = DefaultPolicy(AMD)

func (p Policy) unit() int64 {
	if p.Unit < 1 {
		return 1
	}
	return p.Unit
}

// roundQuotient resolves quo + rem/div under the policy's mode, where
// 0 <= rem < div and div > 0. It returns the adjusted quotient.
func roundQuotient(quo, rem, div int64, mode Mode) int64 {
	if rem == 0 {
		return quo
	}
	switch mode {
	case Down:
		return quo
	case Up:
		return quo + 1
	case HalfEven:
		twice := rem * 2
		switch {
		case twice > div:
			return quo + 1
		case twice < div:
			return quo
		default:
			if quo%2 != 0 {
				return quo + 1
			}
			return quo
		}
	default: // HalfUp
		if rem*2 >= div {
			return quo + 1
		}
		return quo
	}
}
