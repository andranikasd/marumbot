package miniapp

import (
	"errors"
	"testing"

	"github.com/andranikasd/marumbot/pkg/core/date"
	"github.com/andranikasd/marumbot/pkg/core/model"
	"github.com/andranikasd/marumbot/pkg/core/money"
)

func good() LoanRequest {
	return LoanRequest{
		Title: "Car loan", Description: "monthly, 15th", PrincipalMajor: 5_000_000, Currency: "AMD",
		RatePercent: 14.5, Method: "annuity",
		StartDate: "2026-01-15", MaturityDate: "2029-01-15", PaymentDay: 15,
	}
}

func TestValidateAcceptsAWellFormedLoan(t *testing.T) {
	d, err := good().Validate(date.MustNew(2026, 8, 27))
	if err != nil {
		t.Fatalf("Validate: %v", err)
	}
	// 5,000,000 AMD at exponent 2 is 500,000,000 minor units.
	if got := d.Principal.Minor(); got != 500_000_000 {
		t.Errorf("principal = %d minor, want 500000000", got)
	}
	// 14.5% is 145,000,000 parts per billion.
	if got := int64(d.Contract.NominalRate); got != 145_000_000 {
		t.Errorf("rate = %d ppb, want 145000000 (14.5%%)", got)
	}
	if d.Contract.Type != model.Annuity {
		t.Errorf("type = %v, want annuity", d.Contract.Type)
	}
}

func TestValidateAcceptsDeclining(t *testing.T) {
	r := good()
	r.Method = "declining"
	d, err := r.Validate(date.MustNew(2026, 8, 27))
	if err != nil {
		t.Fatal(err)
	}
	if d.Contract.Type != model.DecliningPrincipal {
		t.Errorf("type = %v, want declining", d.Contract.Type)
	}
}

// Everything the browser checks, checked again. The browser belongs to whoever
// is using it and its validation can simply be deleted.
func TestValidateRejects(t *testing.T) {
	cases := map[string]func(*LoanRequest){
		"no title":           func(r *LoanRequest) { r.Title = "   " },
		"zero principal":     func(r *LoanRequest) { r.PrincipalMajor = 0 },
		"negative principal": func(r *LoanRequest) { r.PrincipalMajor = -1 },
		"absurd principal":   func(r *LoanRequest) { r.PrincipalMajor = 1e18 },
		"unknown currency":   func(r *LoanRequest) { r.Currency = "XYZ" },
		"negative rate":      func(r *LoanRequest) { r.RatePercent = -1 },
		"impossible rate":    func(r *LoanRequest) { r.RatePercent = 500 },
		"maturity before":    func(r *LoanRequest) { r.MaturityDate = "2025-01-01" },
		"maturity equals":    func(r *LoanRequest) { r.MaturityDate = r.StartDate },
		"term too long":      func(r *LoanRequest) { r.MaturityDate = "2099-01-15" },
		"payment day zero":   func(r *LoanRequest) { r.PaymentDay = 0 },
		"payment day 32":     func(r *LoanRequest) { r.PaymentDay = 32 },
		"unparseable start":  func(r *LoanRequest) { r.StartDate = "not a date" },
		"unknown method":     func(r *LoanRequest) { r.Method = "balloon" },
	}
	for name, mutate := range cases {
		r := good()
		mutate(&r)
		if _, err := r.Validate(date.MustNew(2026, 8, 27)); !errors.Is(err, ErrInvalid) {
			t.Errorf("%s: got %v, want ErrInvalid", name, err)
		}
	}
}

// NaN and infinity survive JSON decoding as float64 in some encoders and would
// otherwise propagate into money arithmetic as a nonsense int64.
func TestValidateRejectsNonFiniteNumbers(t *testing.T) {
	inf := 1.0
	for i := 0; i < 400; i++ {
		inf *= 10
	}
	for name, v := range map[string]float64{"inf": inf, "-inf": -inf, "nan": inf - inf} {
		r := good()
		r.PrincipalMajor = v
		if _, err := r.Validate(date.MustNew(2026, 8, 27)); !errors.Is(err, ErrInvalid) {
			t.Errorf("principal %s: accepted", name)
		}
		r = good()
		r.RatePercent = v
		if _, err := r.Validate(date.MustNew(2026, 8, 27)); !errors.Is(err, ErrInvalid) {
			t.Errorf("rate %s: accepted", name)
		}
	}
}

// A lender name is displayed. Overlong input is truncated rather than refused,
// because a paste with trailing whitespace is a user error worth absorbing.
func TestTitleIsTruncated(t *testing.T) {
	r := good()
	long := ""
	for i := 0; i < 200; i++ {
		long += "x"
	}
	r.Title = long
	d, err := r.Validate(date.MustNew(2026, 8, 27))
	if err != nil {
		t.Fatal(err)
	}
	if len(d.Title) != 60 {
		t.Errorf("title kept %d characters, want 60", len(d.Title))
	}
}

// The default day count is ACT/365 on the actual balance: not prescribed by
// Armenian law, but what the Central Bank's own worked examples use.
func TestDefaultsMatchArmenianPractice(t *testing.T) {
	d, err := good().Validate(date.MustNew(2026, 8, 27))
	if err != nil {
		t.Fatal(err)
	}
	if d.Contract.DayCount != money.Actual365 {
		t.Errorf("day count = %s, want ACT/365", d.Contract.DayCount)
	}
}
