package app

import (
	"context"
	"encoding/json"
	"errors"
	"math/big"
	"sort"
	"time"

	"github.com/andranikasd/marumbot/pkg/core/date"
)

var ErrPlanActualsCoverage = errors.New("plan_actuals_coverage_unavailable")

// PlanActualsStore selects active activation events at read time. The selected
// immutable version is then replayed; no approval or plan is changed.
type PlanActualsStore interface {
	ActiveActualBaselines(context.Context, string) ([]ActualBaseline, error)
	PlanActualFacts(context.Context, string, ActualBaseline, string) ([]PlanActualFact, error)
}
type ActualBaseline struct {
	PlanID, Currency string
	ActivatedAt      time.Time
}
type PlanActualFact struct {
	ID, LoanID, TransactionDate, ValueDate string
	AmountMinor                            int64
	Allocation                             *PaymentAllocation
	RecordedAfterActivation                bool
}

type VarianceCause string

const (
	VarianceAmount     VarianceCause = "amount"
	VarianceDate       VarianceCause = "date"
	VarianceFee        VarianceCause = "fee"
	VarianceAllocation VarianceCause = "allocation"
	VarianceMissing    VarianceCause = "missing"
	VarianceSchedule   VarianceCause = "schedule"
)

// PlanActualRow describes observable aggregate differences or gaps, never
// inferred bank behaviour. Empty causes do not certify reconciliation or a match.
type PlanActualRow struct {
	LoanID                 string             `json:"loan_id"`
	Loan                   string             `json:"loan"`
	PlannedMinor           string             `json:"planned_minor"`
	PlannedFeeMinor        string             `json:"planned_fee_minor"`
	PostedMinor            *string            `json:"posted_minor"`
	AmountDeltaMinor       *string            `json:"amount_delta_minor"`
	KnownPrincipalMinor    *string            `json:"known_principal_minor"`
	KnownInterestMinor     *string            `json:"known_interest_minor"`
	KnownFeeMinor          *string            `json:"known_fee_minor"`
	FeeDeltaMinor          *string            `json:"fee_delta_minor"`
	UnknownAllocationMinor string             `json:"unknown_allocation_minor"`
	PlannedCount           int                `json:"planned_count"`
	PostedCount            int                `json:"posted_count"`
	MissingAllocationCount int                `json:"missing_allocation_count"`
	PlannedDates           []ActualDateAmount `json:"planned_dates"`
	PostedDates            []ActualDateAmount `json:"posted_dates"`
	Causes                 []VarianceCause    `json:"causes"`
}
type ActualDateAmount struct {
	On          string `json:"on"`
	AmountMinor string `json:"amount_minor"`
}
type PlanActualComparison struct {
	PlanID                        string          `json:"plan_id"`
	Currency                      string          `json:"currency"`
	CurrencyExponent              uint8           `json:"currency_exponent"`
	InputHash                     string          `json:"input_hash"`
	ActivatedOn                   string          `json:"activated_on"`
	From                          string          `json:"from"`
	Through                       string          `json:"through"`
	EmptyWindow                   bool            `json:"empty_window"`
	PendingCount                  int             `json:"pending_count"`
	ExcludedBeforeActivationCount int             `json:"excluded_before_activation_count"`
	OutsideBaselineCount          int             `json:"outside_baseline_count"`
	Rows                          []PlanActualRow `json:"rows"`
}

func (w *Worker) ActivePlanActuals(ctx context.Context, user, month string) ([]PlanActualComparison, error) {
	if err := ValidatePaymentMonth(month); err != nil {
		return nil, err
	}
	reader, ok := w.History.(PlanActualsStore)
	if !ok {
		return nil, ErrPlanActualsCoverage
	}
	today, err := (PaymentService{Clock: w.Clock, Users: w.Users}).BusinessDate(ctx, user)
	if err != nil {
		return nil, err
	}
	baselines, err := reader.ActiveActualBaselines(ctx, user)
	if err != nil {
		return nil, err
	}
	out := make([]PlanActualComparison, 0, len(baselines))
	for _, baseline := range baselines {
		version, err := w.History.PlanVersion(ctx, user, baseline.PlanID)
		if err != nil {
			return nil, err
		}
		// Use the timezone captured with the approval sources, never today's setting.
		var sources struct {
			Timezone string `json:"timezone"`
		}
		if json.Unmarshal([]byte(version.Manifest.Sources), &sources) != nil || sources.Timezone == "" || baseline.ActivatedAt.IsZero() {
			return nil, ErrPlanActualsCoverage
		}
		zone, err := time.LoadLocation(sources.Timezone)
		if err != nil {
			return nil, ErrPlanActualsCoverage
		}
		activated := date.From(baseline.ActivatedAt, zone)
		timeline, err := w.PaymentTimeline(ctx, user, "", baseline.PlanID)
		if err != nil {
			return nil, err
		}
		if timeline.Currency != baseline.Currency {
			return nil, ErrConflict
		}
		facts, err := reader.PlanActualFacts(ctx, user, baseline, month)
		if err != nil {
			return nil, err
		}
		loans := make(map[string]string, len(version.Manifest.Input.Loans))
		for _, loan := range version.Manifest.Input.Loans {
			loans[loan.ID] = loan.Name
		}
		comparison, err := comparePlanActuals(baseline, timeline, loans, facts, month, activated, today)
		if err != nil {
			return nil, err
		}
		out = append(out, comparison)
	}
	return out, nil
}

type actualAccumulator struct {
	row                                                             PlanActualRow
	planned, plannedFee, posted, principal, interest, fees, unknown big.Int
	plannedDates, postedDates                                       map[string]*big.Int
}

func comparePlanActuals(baseline ActualBaseline, timeline PlanTimeline, loans map[string]string, facts []PlanActualFact, month string, activated, today date.Date) (PlanActualComparison, error) {
	start, err := date.Parse(month + "-01")
	if err != nil {
		return PlanActualComparison{}, ErrPaymentInvalid
	}
	end := start.EndOfMonth()
	if today.Before(end) {
		end = today
	}
	// Same-day attribution is unknowable from dates alone. Even a later value
	// date does not make a transfer from before approval part of the new plan.
	if !activated.Before(start) {
		start = date.AddDays(activated, 1)
	}
	out := PlanActualComparison{PlanID: baseline.PlanID, Currency: timeline.Currency, CurrencyExponent: timeline.Exponent, InputHash: timeline.InputHash, ActivatedOn: activated.String(), From: start.String(), Through: end.String(), EmptyWindow: start.After(end), Rows: []PlanActualRow{}}
	if out.EmptyWindow {
		return out, nil
	}
	acc := map[string]*actualAccumulator{}
	get := func(id string) *actualAccumulator {
		a := acc[id]
		if a == nil {
			a = &actualAccumulator{row: PlanActualRow{LoanID: id, Loan: loans[id], Causes: []VarianceCause{}}, plannedDates: map[string]*big.Int{}, postedDates: map[string]*big.Int{}}
			acc[id] = a
		}
		return a
	}
	for _, p := range timeline.Payments {
		if p.On < start.String() || p.On > end.String() {
			continue
		}
		if _, ok := loans[p.LoanID]; !ok || p.AmountMinor < 0 || p.FeeMinor < 0 || p.FeeMinor > p.AmountMinor {
			return out, ErrConflict
		}
		a := get(p.LoanID)
		a.row.PlannedCount++
		a.planned.Add(&a.planned, big.NewInt(p.AmountMinor))
		a.plannedFee.Add(&a.plannedFee, big.NewInt(p.FeeMinor))
		addActualDate(a.plannedDates, p.On, p.AmountMinor)
	}
	for _, f := range facts {
		if _, ok := loans[f.LoanID]; !ok {
			out.OutsideBaselineCount++
			continue
		}
		// Preserve evidence gaps rather than accepting malformed or unknown dates.
		transaction, err := date.Parse(f.TransactionDate)
		if err != nil {
			return out, ErrPlanActualsCoverage
		}
		if !f.RecordedAfterActivation || !transaction.After(activated) {
			out.ExcludedBeforeActivationCount++
			continue
		}
		if f.ValueDate == "" {
			if f.TransactionDate >= start.String() && f.TransactionDate <= end.String() {
				out.PendingCount++
			}
			continue
		}
		posted, err := date.Parse(f.ValueDate)
		if err != nil || posted.Before(transaction) || f.AmountMinor <= 0 {
			return out, ErrPlanActualsCoverage
		}
		if !posted.After(activated) {
			out.ExcludedBeforeActivationCount++
			continue
		}
		if posted.Before(start) || posted.After(end) {
			continue
		}
		a := get(f.LoanID)
		a.row.PostedCount++
		a.posted.Add(&a.posted, big.NewInt(f.AmountMinor))
		addActualDate(a.postedDates, f.ValueDate, f.AmountMinor)
		if f.Allocation == nil {
			a.row.MissingAllocationCount++
			a.unknown.Add(&a.unknown, big.NewInt(f.AmountMinor))
			continue
		}
		// Stored facts must still satisfy the same all-or-nothing contract.
		c := PaymentCommand{LoanID: f.LoanID, Key: "allocation-check", AmountMinor: f.AmountMinor, TransactionDate: f.TransactionDate, ValueDate: f.ValueDate, Allocation: f.Allocation}
		if c.validate(today) != nil {
			return out, ErrPlanActualsCoverage
		}
		a.principal.Add(&a.principal, big.NewInt(*f.Allocation.PrincipalMinor))
		a.interest.Add(&a.interest, big.NewInt(*f.Allocation.InterestMinor))
		a.fees.Add(&a.fees, big.NewInt(*f.Allocation.FeesMinor))
	}
	ids := make([]string, 0, len(acc))
	for id := range acc {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	for _, id := range ids {
		a := acc[id]
		r := a.row
		r.PlannedMinor = a.planned.String()
		r.PlannedFeeMinor = a.plannedFee.String()
		r.UnknownAllocationMinor = a.unknown.String()
		r.PlannedDates = actualDates(a.plannedDates)
		r.PostedDates = actualDates(a.postedDates)
		if r.PostedCount > 0 {
			r.PostedMinor = actualString(&a.posted)
			r.AmountDeltaMinor = actualString(new(big.Int).Sub(&a.posted, &a.planned))
			if r.PlannedCount == 0 {
				r.Causes = append(r.Causes, VarianceSchedule)
			} else {
				if a.posted.Cmp(&a.planned) != 0 {
					r.Causes = append(r.Causes, VarianceAmount)
				}
				// Only compare the sets of observed dates; never pair facts to actions.
				if !sameActualDates(r.PlannedDates, r.PostedDates, a.posted.Cmp(&a.planned) == 0) {
					r.Causes = append(r.Causes, VarianceDate)
				}
			}
			if r.MissingAllocationCount < r.PostedCount {
				r.KnownPrincipalMinor = actualString(&a.principal)
				r.KnownInterestMinor = actualString(&a.interest)
				r.KnownFeeMinor = actualString(&a.fees)
			}
			if r.MissingAllocationCount > 0 {
				r.Causes = append(r.Causes, VarianceAllocation)
			} else {
				r.FeeDeltaMinor = actualString(new(big.Int).Sub(&a.fees, &a.plannedFee))
				if a.fees.Cmp(&a.plannedFee) != 0 {
					r.Causes = append(r.Causes, VarianceFee)
				}
			}
		} else if r.PlannedCount > 0 {
			r.Causes = append(r.Causes, VarianceMissing)
		}
		out.Rows = append(out.Rows, r)
	}
	return out, nil
}
func actualString(n *big.Int) *string { s := n.String(); return &s }
func addActualDate(days map[string]*big.Int, on string, minor int64) {
	if days[on] == nil {
		days[on] = new(big.Int)
	}
	days[on].Add(days[on], big.NewInt(minor))
}

func actualDates(days map[string]*big.Int) []ActualDateAmount {
	out := make([]ActualDateAmount, 0, len(days))
	for on, amount := range days {
		out = append(out, ActualDateAmount{on, amount.String()})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].On < out[j].On })
	return out
}

func sameActualDates(a, b []ActualDateAmount, compareAmounts bool) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i].On != b[i].On || (compareAmounts && a[i].AmountMinor != b[i].AmountMinor) {
			return false
		}
	}
	return true
}
