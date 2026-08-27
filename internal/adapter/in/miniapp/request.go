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
func (r LoanRequest) Validate() (app.LoanDraft, error) {
	lender := trimTo(r.Lender, 60)
	if lender == "" {
		return app.LoanDraft{}, fmt.Errorf("%w: no lender", ErrInvalid)
	}

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

	return app.LoanDraft{
		Lender:    lender,
		Principal: principal,
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
		},
	}, nil
}
