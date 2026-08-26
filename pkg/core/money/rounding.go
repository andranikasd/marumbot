package money

// Mode selects how a value exactly between two representable results is
// resolved.
type Mode uint8

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
	// Unit is the settlement granularity in minor units. AMD defaults to 100:
	// two decimal places exist in ISO 4217, but banks settle in whole drams.
	Unit int64
}

// DefaultAMDPolicy is half-up to the whole dram. Any lender-specific
// difference must be confirmed against a real repayment schedule first.
// For other currencies use DefaultPolicy, which reads the settlement unit
// from the currency registry.
var DefaultAMDPolicy = Policy{Mode: HalfUp, Unit: 100}

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
