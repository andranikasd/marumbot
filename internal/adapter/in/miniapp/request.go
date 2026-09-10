package miniapp

import (
	"encoding/json"
	"fmt"
	"regexp"

	"github.com/andranikasd/marumbot/internal/app"
	"github.com/andranikasd/marumbot/pkg/core/date"
	"github.com/andranikasd/marumbot/pkg/core/model"
	"github.com/andranikasd/marumbot/pkg/core/money"
)

// maxTermYears bounds a loan. Forty years covers any Armenian mortgage; beyond
// that is a typo or an attempt to make the engine project half a million rows.
const maxTermYears = 40

// Validate turns a posted form into a draft, or explains why it cannot.
//
// Every rule here is also in the browser. The browser's copy exists to tell the
// user quickly; this one exists because the browser belongs to whoever is using
// it and its checks can simply be deleted.
func (r LoanRequest) Validate(today date.Date) (app.LoanDraft, error) {
	icon, iconErr := app.LoanIcon(r.Icon)
	if iconErr != nil {
		return app.LoanDraft{}, fmt.Errorf("%w: loan icon", ErrInvalid)
	}
	title := trimTo(r.Title, 60)
	if title == "" {
		return app.LoanDraft{}, fmt.Errorf("%w: no title", ErrInvalid)
	}
	description := trimTo(r.Description, 200)

	cur, err := money.Lookup(r.Currency)
	if err != nil {
		return app.LoanDraft{}, fmt.Errorf("%w: currency %q", ErrInvalid, r.Currency)
	}

	minor, err := budgetMinor(r.PrincipalMajor, cur, false)
	if err != nil {
		return app.LoanDraft{}, err
	}
	principal := money.FromMinor(minor, cur)
	balance := principal
	if r.BalanceMajor != "" {
		bm, err := budgetMinor(r.BalanceMajor, cur, true)
		if err != nil || bm > minor {
			return app.LoanDraft{}, fmt.Errorf("%w: balance", ErrInvalid)
		}
		// Legacy clients send zero when the balance is omitted.
		if bm > 0 {
			balance = money.FromMinor(bm, cur)
		}
	}
	rateMinor, err := scaledDecimal(r.RatePercent, 6, 200_000_000, true)
	if err != nil {
		return app.LoanDraft{}, err
	}
	rate := money.RateFromPercent(rateMinor/1_000_000, rateMinor%1_000_000)

	start, err := date.Parse(r.StartDate)
	if err != nil {
		return app.LoanDraft{}, fmt.Errorf("%w: start date", ErrInvalid)
	}
	maturity, err := date.Parse(r.MaturityDate)
	if err != nil {
		return app.LoanDraft{}, fmt.Errorf("%w: maturity date", ErrInvalid)
	}
	if !maturity.After(start) {
		return app.LoanDraft{}, fmt.Errorf("%w: maturity must follow the start", ErrInvalid)
	}
	if maturity.Year()-start.Year() > maxTermYears {
		return app.LoanDraft{}, fmt.Errorf("%w: term exceeds %d years", ErrInvalid, maxTermYears)
	}
	if r.PaymentDay < 1 || r.PaymentDay > 31 {
		return app.LoanDraft{}, fmt.Errorf("%w: payment day must be 1 to 31", ErrInvalid)
	}

	typ := model.Annuity
	switch r.Method {
	case "declining":
		typ = model.DecliningPrincipal
	case "annuity", "":
	default:
		return app.LoanDraft{}, fmt.Errorf("%w: unknown method %q", ErrInvalid, r.Method)
	}
	prepay, err := model.ParsePrepaymentEffect(r.PrepayEffect)
	if err != nil {
		return app.LoanDraft{}, fmt.Errorf("%w: prepayment effect", ErrInvalid)
	}

	return app.LoanDraft{
		Title: title, Icon: icon, OptionalExcluded: r.OptionalExcluded,
		Description: description,
		Principal:   principal,
		Balance:     balance,
		AsOf:        today,
		Contract: model.Contract{
			Version:     1,
			Currency:    cur,
			NominalRate: rate,
			// ACT/365 on the actual outstanding balance. Nothing in Armenian
			// law prescribes a convention, but it is what the Central Bank's own
			// worked examples use and what lenders publish, so it is the default
			// and a per-loan override rather than an assumption.
			DayCount:     money.Actual365,
			Type:         typ,
			StartDate:    start,
			MaturityDate: maturity,
			PaymentDay:   r.PaymentDay,
			Rounding:     money.DefaultPolicy(cur),
			Prepayment:   model.Prepayment{Effect: prepay},
		},
	}, nil
}

// LoanEditRequest is what the edit form patches. The currency and the
// original principal are deliberately absent: the first cannot change
// without re-denominating the ledger, the second is history.
type LoanEditRequest struct {
	SnapshotMinor    *int64      `json:"snapshot_minor"`
	SnapshotAsOf     string      `json:"snapshot_as_of"`
	Icon             *string     `json:"icon"`
	OptionalExcluded *bool       `json:"optional_excluded"`
	Name             string      `json:"name"`
	Description      string      `json:"description"`
	RatePercent      json.Number `json:"rate_percent"`
	Method           string      `json:"method"`
	PrepayEffect     string      `json:"prepay_effect"`
	StartDate        string      `json:"start_date"`
	MaturityDate     string      `json:"maturity_date"`
	PaymentDay       int         `json:"payment_day"`
	// BalanceMajor restates what is owed today; zero means unchanged.
	BalanceMajor json.Number `json:"balance_major"`
}

// FullEdit reports whether the patch carries contract terms, or only the
// borrower's own words. The old client sends name and description alone; a
// missing start date is how the two shapes are told apart.
func (r LoanEditRequest) FullEdit() bool { return r.StartDate != "" }

// Validate turns the patch into an edit, against the currency the loan
// already has. Same discipline as LoanRequest.Validate: the browser's checks
// exist to be quick, these exist because the browser can be lied about.
func (r LoanEditRequest) Validate(cur money.Currency) (app.LoanEdit, error) {
	if r.Icon != nil {
		if _, err := app.LoanIcon(*r.Icon); err != nil {
			return app.LoanEdit{}, fmt.Errorf("%w: icon", ErrInvalid)
		}
	}
	name := trimTo(r.Name, 60)
	if name == "" {
		return app.LoanEdit{}, fmt.Errorf("%w: no title", ErrInvalid)
	}
	rateMinor, err := scaledDecimal(r.RatePercent, 6, 200_000_000, true)
	if err != nil {
		return app.LoanEdit{}, err
	}
	whole, micro := rateMinor/1_000_000, rateMinor%1_000_000

	start, err := date.Parse(r.StartDate)
	if err != nil {
		return app.LoanEdit{}, fmt.Errorf("%w: start date", ErrInvalid)
	}
	maturity, err := date.Parse(r.MaturityDate)
	if err != nil {
		return app.LoanEdit{}, fmt.Errorf("%w: maturity date", ErrInvalid)
	}
	if !maturity.After(start) {
		return app.LoanEdit{}, fmt.Errorf("%w: maturity must follow the start", ErrInvalid)
	}
	if maturity.Year()-start.Year() > maxTermYears {
		return app.LoanEdit{}, fmt.Errorf("%w: term exceeds %d years", ErrInvalid, maxTermYears)
	}
	if r.PaymentDay < 1 || r.PaymentDay > 31 {
		return app.LoanEdit{}, fmt.Errorf("%w: payment day must be 1 to 31", ErrInvalid)
	}
	typ := model.Annuity
	switch r.Method {
	case "declining":
		typ = model.DecliningPrincipal
	case "annuity", "":
	default:
		return app.LoanEdit{}, fmt.Errorf("%w: unknown method %q", ErrInvalid, r.Method)
	}
	prepay, err := model.ParsePrepaymentEffect(r.PrepayEffect)
	if err != nil {
		return app.LoanEdit{}, fmt.Errorf("%w: prepayment effect", ErrInvalid)
	}

	e := app.LoanEdit{
		Name:        name,
		Description: trimTo(r.Description, 200), Icon: r.Icon, OptionalExcluded: r.OptionalExcluded,
		NominalRate:  money.RateFromPercent(whole, micro),
		Type:         typ,
		StartDate:    start,
		MaturityDate: maturity,
		PaymentDay:   r.PaymentDay,
		PrepayEffect: prepay,
	}
	if r.SnapshotMinor != nil {
		if *r.SnapshotMinor < 0 || *r.SnapshotMinor > maxSafeMinor {
			return app.LoanEdit{}, fmt.Errorf("%w: balance", ErrInvalid)
		}
		asOf, err := date.Parse(r.SnapshotAsOf)
		if err != nil || asOf.Before(start) {
			return app.LoanEdit{}, fmt.Errorf("%w: balance date", ErrInvalid)
		}
		e.BalanceMinor = r.SnapshotMinor
		e.BalanceAsOf = asOf
	}
	if r.BalanceMajor != "" {
		minor, err := budgetMinor(r.BalanceMajor, cur, true)
		if err != nil {
			return app.LoanEdit{}, fmt.Errorf("%w: balance", err)
		}
		// Zero is the legacy unchanged sentinel; SnapshotMinor supports explicit zero.
		if minor > 0 {
			e.BalanceMinor = &minor
		}
	}
	return e, nil
}

// BudgetRequest is what the budget form posts.
type BudgetRequest struct {
	AsOf            string             `json:"as_of"`
	Key             string             `json:"idempotency_key"`
	Funding         *app.BudgetFunding `json:"funding"`
	ExpectedVersion *int64             `json:"expected_version"`
	MonthlyMajor    json.Number        `json:"monthly_major"`
	Currency        string             `json:"currency"`
	// PayDay is the day of the month the money arrives; 0 means not stated.
	PayDay int `json:"pay_day"`
	// OpeningMajor is cash on hand for loans today. Zero or absent withdraws
	// the statement because this endpoint receives the complete form.
	OpeningMajor *json.Number `json:"opening_major"`
	// ReserveMajor is the part of opening cash the plan must not spend.
	ReserveMajor *json.Number `json:"reserve_major"`
	// Overrides are whole-month figures keyed "2006-01", in major units.
	// An absent or empty object clears every stated month.
	Overrides map[string]json.Number `json:"overrides"`
}

// maxOverrideMonths bounds the document. Three years of stated months is a
// plan horizon, not a budget; beyond it is a client bug.
const maxOverrideMonths = 36

var monthKeyRe = regexp.MustCompile(`^\d{4}-(0[1-9]|1[0-2])$`)

// Validate turns a posted budget into minor units and a pay day.
func (r BudgetRequest) Validate() (string, int64, int, error) {
	cur, err := money.Lookup(r.Currency)
	if err != nil {
		return "", 0, 0, fmt.Errorf("%w: currency %q", ErrInvalid, r.Currency)
	}
	minor, err := budgetMinor(r.MonthlyMajor, cur, false)
	if err != nil {
		return "", 0, 0, fmt.Errorf("%w: monthly budget", err)
	}
	if r.PayDay < 0 || r.PayDay > 31 {
		return "", 0, 0, fmt.Errorf("%w: pay day %d out of range", ErrInvalid, r.PayDay)
	}
	return cur.Code, minor, r.PayDay, nil
}

// ValidateOpening converts the stated cash on hand. Only called when the
// field is present.
func (r BudgetRequest) ValidateOpening(cur money.Currency) (int64, error) {
	minor, err := budgetMinor(*r.OpeningMajor, cur, true)
	if err != nil {
		return 0, fmt.Errorf("%w: cash on hand", err)
	}
	return minor, nil
}

// ValidateReserve converts the protected cash floor. Only called when present.
func (r BudgetRequest) ValidateReserve(cur money.Currency) (int64, error) {
	minor, err := budgetMinor(*r.ReserveMajor, cur, true)
	if err != nil {
		return 0, fmt.Errorf("%w: protected reserve", err)
	}
	return minor, nil
}

// ValidateOverrides converts the per-month document to minor units. Only
// called when the field is present.
func (r BudgetRequest) ValidateOverrides(cur money.Currency) (map[string]int64, error) {
	if len(r.Overrides) > maxOverrideMonths {
		return nil, fmt.Errorf("%w: more than %d stated months", ErrInvalid, maxOverrideMonths)
	}
	out := make(map[string]int64, len(r.Overrides))
	for k, v := range r.Overrides {
		if !monthKeyRe.MatchString(k) {
			return nil, fmt.Errorf("%w: %q is not a month", ErrInvalid, k)
		}
		minor, err := budgetMinor(v, cur, true)
		if err != nil {
			return nil, fmt.Errorf("%w: the %s budget", err, k)
		}
		out[k] = minor
	}
	return out, nil
}

// budgetMinor is the single conversion rule for every amount on the budget
// form. Monthly must be positive; cash on hand and a stated month may be zero
// because zero explicitly clears cash or says that month has nothing to give.
func budgetMinor(major json.Number, cur money.Currency, allowZero bool) (int64, error) {
	return scaledDecimal(major, cur.Exponent, maxSafeMinor, allowZero)
}

// ValidateFunding checks explicitly declared minor-unit funding without using
// floating-point conversions. Future expected receipts remain assumptions.
func (r BudgetRequest) ValidateFunding(today date.Date) error {
	if r.ExpectedVersion != nil && *r.ExpectedVersion < 0 {
		return fmt.Errorf("%w: version", ErrInvalid)
	}
	if r.Funding == nil {
		return nil
	}
	f := r.Funding
	if f.SpentPeriodStart != "" {
		d, err := date.Parse(f.SpentPeriodStart)
		if err != nil || d.After(today) {
			return fmt.Errorf("%w: spending period start", ErrInvalid)
		}
	}
	if f.CashThrough != "" {
		d, err := date.Parse(f.CashThrough)
		if err != nil || d.After(today) {
			return fmt.Errorf("%w: cash statement date", ErrInvalid)
		}
	}
	if r.PayDay < 1 || r.PayDay > 31 || f.MonthlyMinor < 0 || f.SpentMinor < 0 || f.MonthlyMinor > maxSafeMinor || f.SpentMinor > maxSafeMinor || len(f.Events) > maxOverrideMonths {
		return fmt.Errorf("%w: funding", ErrInvalid)
	}
	if err := app.ValidateBudgetCashEvents(f.Events, r.Currency, today); err != nil {
		return fmt.Errorf("%w: %w", ErrInvalid, err)
	}
	return nil
}
