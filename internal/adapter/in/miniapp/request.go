package miniapp

import (
	"fmt"
	"math"

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
	title := trimTo(r.Title, 60)
	if title == "" {
		return app.LoanDraft{}, fmt.Errorf("%w: no title", ErrInvalid)
	}
	description := trimTo(r.Description, 200)

	cur, err := money.Lookup(r.Currency)
	if err != nil {
		return app.LoanDraft{}, fmt.Errorf("%w: currency %q", ErrInvalid, r.Currency)
	}

	// The form sends major units as a JSON number. Converting through float is
	// unavoidable at this boundary, so it happens exactly once, here, and
	// everything past this line is integer minor units.
	if math.IsNaN(r.PrincipalMajor) || math.IsInf(r.PrincipalMajor, 0) || r.PrincipalMajor <= 0 {
		return app.LoanDraft{}, fmt.Errorf("%w: principal must be above zero", ErrInvalid)
	}
	scale := math.Pow10(int(cur.Exponent))
	minor := math.Round(r.PrincipalMajor * scale)
	if minor > float64(math.MaxInt64/1000) {
		return app.LoanDraft{}, fmt.Errorf("%w: principal too large", ErrInvalid)
	}
	principal := money.FromMinor(int64(minor), cur)

	// A loan that has been running is filed with what is owed now, not what
	// was borrowed. That figure becomes the anchor the schedule projects from;
	// the contract keeps its original dates so the instalment still solves
	// correctly. Zero means "not yet paid down" and falls back to principal.
	balance := principal
	if r.BalanceMajor > 0 {
		if math.IsNaN(r.BalanceMajor) || math.IsInf(r.BalanceMajor, 0) {
			return app.LoanDraft{}, fmt.Errorf("%w: balance", ErrInvalid)
		}
		bm := math.Round(r.BalanceMajor * scale)
		if bm > minor {
			return app.LoanDraft{}, fmt.Errorf("%w: balance cannot exceed the principal", ErrInvalid)
		}
		balance = money.FromMinor(int64(bm), cur)
	}

	if math.IsNaN(r.RatePercent) || r.RatePercent < 0 || r.RatePercent > 200 {
		return app.LoanDraft{}, fmt.Errorf("%w: rate must be between 0 and 200", ErrInvalid)
	}
	whole := int64(r.RatePercent)
	micro := int64(math.Round((r.RatePercent - float64(whole)) * 1_000_000))
	rate := money.RateFromPercent(whole, micro)

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
		Title:       title,
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

// BudgetRequest is what the budget form posts.
type BudgetRequest struct {
	MonthlyMajor float64 `json:"monthly_major"`
	Currency     string  `json:"currency"`
	// PayDay is the day of the month the money arrives; 0 means not stated.
	PayDay int `json:"pay_day"`
}

// Validate turns a posted budget into minor units and a pay day.
func (r BudgetRequest) Validate() (string, int64, int, error) {
	cur, err := money.Lookup(r.Currency)
	if err != nil {
		return "", 0, 0, fmt.Errorf("%w: currency %q", ErrInvalid, r.Currency)
	}
	if math.IsNaN(r.MonthlyMajor) || math.IsInf(r.MonthlyMajor, 0) || r.MonthlyMajor < 0 {
		return "", 0, 0, fmt.Errorf("%w: a budget cannot be negative", ErrInvalid)
	}
	minor := math.Round(r.MonthlyMajor * math.Pow10(int(cur.Exponent)))
	if minor > float64(math.MaxInt64/1000) {
		return "", 0, 0, fmt.Errorf("%w: budget too large", ErrInvalid)
	}
	if r.PayDay < 0 || r.PayDay > 31 {
		return "", 0, 0, fmt.Errorf("%w: pay day %d out of range", ErrInvalid, r.PayDay)
	}
	return cur.Code, int64(minor), r.PayDay, nil
}
