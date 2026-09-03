package plan

import (
	"errors"
	"fmt"

	"github.com/andranikasd/marumbot/pkg/core/date"
)

// StressHealth describes coverage as well as feasibility. Unknown cases prevent
// Healthy; a known failed case takes precedence and makes a feasible base Tight.
type StressHealth string

// Public health identifiers are stable API values.
const (
	StressHealthy    StressHealth = "healthy"
	StressTight      StressHealth = "tight"
	StressInfeasible StressHealth = "infeasible"
	StressUnknown    StressHealth = "unknown"
)

// StressStatus is the evidence available for one requested perturbation.
type StressStatus string

// Cases distinguish tested success from an explicitly irrelevant rule.
const (
	StressPassed        StressStatus = "passed"
	StressFailed        StressStatus = "failed"
	StressNotApplicable StressStatus = "not_applicable"
	StressCaseUnknown   StressStatus = "unknown"
)

// StressReason is machine-readable; callers must not turn an unknown into zero.
type StressReason string

// Reason codes document the minimal exact domain and its unsupported seams.
const (
	StressExactReplay           StressReason = "exact_selected_policy_replay"
	StressLateIncome            StressReason = "scheduled_receipts_shifted_three_calendar_days"
	StressExpectedExcluded      StressReason = "expected_cash_already_excluded_from_base"
	StressNoExpected            StressReason = "no_expected_cash"
	StressPaydayUnknown         StressReason = "income_date_unknown"
	StressRequiredConfigMissing StressReason = "required_increase_percentage_missing"
	StressRequiredTermsMissing  StressReason = "required_adjustment_terms_unverified"
	StressCalendarMissing       StressReason = "business_calendar_and_posting_rule_unverified"
	StressNoExtras              StressReason = "no_extra_payments"
	StressNoFees                StressReason = "no_fee_rules"
	StressFeeRuleMissing        StressReason = "verified_maximum_fee_stress_rule_missing"
	StressFixedFees             StressReason = "fixed_contract_fees_already_applied"
	StressNoDebt                StressReason = "no_remaining_obligations"
	StressHorizonUnknown        StressReason = "payoff_not_completed_within_horizon"
	StressDomainUnknown         StressReason = "unsupported_simulation_domain"
)

// StressOptions deliberately has no default percentage. A configured increase
// remains unknown until verified contractual adjustment terms are representable.
type StressOptions struct{ RequiredIncreaseBP int64 }

// StressCase contains only replay evidence, not an optimized or stored plan.
type StressCase struct {
	ID         string
	Status     StressStatus
	Reason     StressReason
	Failure    *InfeasibleError
	PayoffDate date.Date
}

// StressReport contains base evidence and five cases for the selected policy.
// A completed replay proves feasibility through payoff; a horizon refusal is
// unknown, not proof that a required instalment cannot be met.
type StressReport struct {
	Health             StressHealth
	Base               StressCase
	Cases              []StressCase
	RequiredIncreaseBP int64
}

// StressCases accepts the original NORMALIZED input and the selected policy.
// It performs no search, mutation, clock read, persistence or term inference.
// Exact initial support: delay future scheduled receipts by three calendar days
// while preserving their nominal-month amounts; expected cash is already absent
// in conservative runs. Variable required terms and posting calendars are not
// represented, so those cases stay explicitly unknown. Deterministic fixed fees
// already apply in the base; variable maximum-fee stress remains unknown.
func StressCases(in Input, pol Policy, options StressOptions) (StressReport, error) {
	out := StressReport{Health: StressUnknown, RequiredIncreaseBP: options.RequiredIncreaseBP}
	if options.RequiredIncreaseBP < 0 || options.RequiredIncreaseBP > 10000 {
		return out, fmt.Errorf("plan: stress percentage must be between zero and 10000 basis points")
	}
	if err := in.Validate(); err != nil {
		return out, err
	}
	baseResult, baseErr := run(in, pol, cache{})
	base, err := stressOutcome("base", StressExactReplay, baseResult, baseErr)
	if err != nil {
		return out, err
	}
	out.Base = base
	income := StressCase{ID: "income_three_days_late", Status: StressCaseUnknown, Reason: StressPaydayUnknown}
	if in.Cash.PayDay > 0 {
		result, runErr := runConfigured(in, pol, cache{}, nil, 3)
		income, err = stressOutcome(income.ID, StressLateIncome, result, runErr)
		if err != nil {
			return out, err
		}
	}
	expected := base
	expected.ID = "missing_expected_cash"
	expected.Reason = StressExpectedExcluded
	hasExpected := false
	for _, event := range in.Cash.Lumps {
		hasExpected = hasExpected || event.Expected
	}
	if !hasExpected {
		expected.Status = StressNotApplicable
		expected.Reason = StressNoExpected
		expected.Failure = nil
		expected.PayoffDate = date.Date{}
	}
	required := StressCase{ID: "required_payment_increase", Status: StressCaseUnknown, Reason: StressRequiredConfigMissing}
	if options.RequiredIncreaseBP > 0 {
		required.Reason = StressRequiredTermsMissing
	}
	posting := StressCase{ID: "next_business_day_credit", Status: StressCaseUnknown, Reason: StressCalendarMissing}
	if baseErr == nil && baseResult.Prepayments == 0 {
		posting.Status = StressNotApplicable
		posting.Reason = StressNoExtras
	}
	fee := StressCase{ID: "maximum_verified_fee", Status: StressNotApplicable, Reason: StressNoFees}
	for _, loan := range in.Loans {
		if loan.Balance.Sign() <= 0 {
			continue
		}
		pp := loan.Contract.Prepayment
		if pp.FeeBP != 0 {
			fee.Status = StressCaseUnknown
			fee.Reason = StressFeeRuleMissing
			break
		}
		for _, charge := range pp.Charges {
			if charge.PercentBP != 0 {
				fee.Status = StressCaseUnknown
				fee.Reason = StressFeeRuleMissing
				break
			}
			if fee.Status != StressCaseUnknown {
				fee.Status = base.Status
				fee.Reason = StressFixedFees
				fee.Failure = base.Failure
			}
		}
		if fee.Status == StressCaseUnknown {
			break
		}
	}
	debt := false
	for _, loan := range in.Loans {
		debt = debt || loan.Balance.Sign() > 0
	}
	if !debt {
		required.Status = StressNotApplicable
		required.Reason = StressNoDebt
		income.Status = StressNotApplicable
		income.Reason = StressNoDebt
	}
	out.Cases = []StressCase{income, expected, required, posting, fee}
	switch base.Status {
	case StressFailed:
		out.Health = StressInfeasible
	case StressCaseUnknown:
		out.Health = StressUnknown
	default:
		out.Health = StressHealthy
		for _, c := range out.Cases {
			if c.Status == StressFailed {
				out.Health = StressTight
				break
			}
			if c.Status == StressCaseUnknown {
				out.Health = StressUnknown
			}
		}
	}
	return out, nil
}

func stressOutcome(id string, reason StressReason, result Result, err error) (StressCase, error) {
	c := StressCase{ID: id, Reason: reason, Status: StressPassed, PayoffDate: result.PayoffDate}
	if err == nil {
		return c, nil
	}
	var infeasible *InfeasibleError
	switch {
	case errors.As(err, &infeasible):
		c.Status = StressFailed
		c.Failure = infeasible
	case errors.Is(err, ErrHorizon):
		c.Status = StressCaseUnknown
		c.Reason = StressHorizonUnknown
	default:
		var unsupported *UnsupportedError
		if !errors.As(err, &unsupported) {
			return StressCase{}, err
		}
		c.Status = StressCaseUnknown
		c.Reason = StressDomainUnknown
	}
	return c, nil
}
