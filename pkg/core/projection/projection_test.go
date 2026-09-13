package projection_test

import (
	"errors"
	"reflect"
	"testing"

	"github.com/andranikasd/marumbot/pkg/core/date"
	"github.com/andranikasd/marumbot/pkg/core/model"
	"github.com/andranikasd/marumbot/pkg/core/money"
	"github.com/andranikasd/marumbot/pkg/core/projection"
)

func day(s string) date.Date {
	d, err := date.Parse(s)
	if err != nil {
		panic(err)
	}
	return d
}

func loan(id string, balance, payment int64) projection.Loan {
	cur := money.MustLookup("USD")
	return projection.Loan{ID: id, Name: id, Balance: money.FromMinor(balance, cur), AsOf: day("2026-09-01"), Contract: model.Contract{LoanID: model.ID(id), Version: 1, Currency: cur, EffectiveFrom: day("2026-01-15"), StartDate: day("2026-01-15"), MaturityDate: day("2027-09-15"), PaymentDay: 15, HasScheduled: true, ScheduledPayment: money.FromMinor(payment, cur), NotBeforeDue: day("2026-09-15"), Rounding: money.DefaultPolicy(cur)}}
}

func build(t *testing.T, loans []projection.Loan, x projection.Extra) projection.Result {
	t.Helper()
	r, e := projection.Build(day("2026-09-14"), money.MustLookup("USD"), loans, x)
	if e != nil {
		t.Fatal(e)
	}
	return r
}

func TestMonthlyOverridesReplacePauseAndReset(t *testing.T) {
	l := loan("a", 90000, 7500)
	x := projection.Extra{Minor: 2500, Overrides: map[string]int64{"2026-10": 0, "2026-11": 5000}}
	r := build(t, []projection.Loan{l}, x)
	if r.Months[0].Extra != 2500 || r.Months[1].Extra != 0 || r.Months[2].Extra != 5000 || r.Months[3].Extra != 2500 || r.Finish != "2027-05" {
		t.Fatalf("replacement/proration/carry error %+v", r)
	}
	delete(x.Overrides, "2026-10")
	r = build(t, []projection.Loan{l}, x)
	if r.Months[1].Extra != 2500 {
		t.Fatal("reset did not restore default")
	}
}

func TestRequiredFallsAndExtraDoesNotGrow(t *testing.T) {
	r := build(t, []projection.Loan{loan("a", 7500, 7500), loan("b", 30000, 7500)}, projection.Extra{Minor: 2500})
	if r.Months[0].Required != 15000 || r.Months[1].Required != 7500 || r.Months[1].Extra != 2500 {
		t.Fatalf("freed instalment recycled %+v", r)
	}
	if r.Months[len(r.Months)-1].Extra != 2500 {
		t.Fatal("extra changed without a source edit")
	}
	r = build(t, []projection.Loan{loan("a", 7500, 7500)}, projection.Extra{Minor: 50000})
	if r.Months[0].Extra != 0 || r.Months[0].UnallocatedExtra != 50000 {
		t.Fatal("extra charged after required settled debt")
	}
}

func TestIncompleteBankDatesRemainVisible(t *testing.T) {
	a, b := loan("a", 90000, 7500), loan("b", 90000, 8500)
	a.Reason = "interest_needed"
	a.Contract.NotBeforeDue = day("2026-08-15")
	b.Reason = "early_payment_rules_needed"
	b.Contract.NotBeforeDue = day("2026-10-15")
	r := build(t, []projection.Loan{a, b}, projection.Extra{Minor: 2000})
	if r.Complete || r.Finish != "" || r.StartMonth != "2026-09" || len(r.Months) != 2 || r.Months[0].Required != 7500 || r.Months[1].Required != 8500 || r.Months[0].Extra != 0 || r.Months[0].UnallocatedExtra != 2000 {
		t.Fatalf("invented/hid obligation %+v", r)
	}
	if r.Months[0].Loans[0].Due != "2026-08-15" {
		t.Fatal("overdue date rolled away")
	}
}

func TestMonthEndAndInterestTimingGolden(t *testing.T) {
	// Actual/360 at36%: 14 days on100000=1400; Sept15 required10100
	// leaves91300. 15 days toSep30=>1370 rounded; extra20000 leaves71300.
	// 15 days toOct15=>1070; Octrequired10100 subtracts7660 principal.
	l := loan("a", 100000, 10100)
	l.OpeningInterestKnown = true
	l.Contract.NominalRate = money.RateFromPercent(36, 0)
	l.Contract.DayCount = money.Actual360
	r := build(t, []projection.Loan{l}, projection.Extra{Minor: 20000})
	if r.Months[0].Required != 10100 || r.Months[0].Extra != 20000 || r.Months[1].Required != 10100 {
		t.Fatalf("timing golden %+v", r)
	}
	// Remaining events settle exactly: Sep30100, Oct30100, Nov30100,
	// Dec required10100 + final extra5963 (sum106363, interest6363).
	// Oct31 interest1018; Nov15 interest655; Nov30 interest528;
	// Dec15 interest228; Dec31 settlement interest94.
	var total int64
	for _, m := range r.Months {
		total += m.Total
	}
	if total != 106363 || r.Finish != "2026-12" {
		t.Fatalf("interest golden total=%d result=%+v", total, r)
	}
}

func TestCalendarBoundaryAndDeterminism(t *testing.T) {
	l := loan("a", 30000, 10000)
	l.AsOf = day("2027-12-31")
	l.Contract.StartDate = day("2027-01-31")
	l.Contract.MaturityDate = day("2028-03-31")
	l.Contract.PaymentDay = 31
	l.Contract.NotBeforeDue = day("2028-01-31")
	r, e := projection.Build(day("2027-12-31"), l.Balance.Currency(), []projection.Loan{l}, projection.Extra{})
	if e != nil {
		t.Fatal(e)
	}
	for i, d := range []string{"2028-01-31", "2028-02-29", "2028-03-31"} {
		if r.Months[i].Loans[0].Due != d {
			t.Fatalf("boundary drift %+v", r)
		}
	}
	a, b := loan("a", 20000, 7500), loan("b", 20000, 7500)
	x := projection.Extra{Minor: 5000}
	if !reflect.DeepEqual(build(t, []projection.Loan{a, b}, x), build(t, []projection.Loan{b, a}, x)) {
		t.Fatal("input order changed recommendation")
	}
}

func TestProjectionRejectsUnsafeSources(t *testing.T) {
	for _, x := range []projection.Extra{{Minor: -1}, {Minor: projection.MaxMinor + 1}, {Overrides: map[string]int64{"2026-13": 0}}, {Overrides: map[string]int64{"2026-09": -1}}} {
		if !errors.Is(projection.ValidateExtra(x), projection.ErrInvalid) {
			t.Fatal("invalid extra accepted")
		}
	}
	l := loan("a", 10000, 7500)
	if _, err := projection.Build(day("2026-09-14"), l.Balance.Currency(), []projection.Loan{l, l}, projection.Extra{}); err == nil {
		t.Fatal("duplicate principal")
	}
	l.AsOf = day("2026-10-01")
	if _, err := projection.Build(day("2026-09-14"), l.Balance.Currency(), []projection.Loan{l}, projection.Extra{}); err == nil {
		t.Fatal("future anchor")
	}
}

func BenchmarkMonthlyProjection100Loans(b *testing.B) {
	loans := make([]projection.Loan, 100)
	for i := range loans {
		loans[i] = loan(string(rune(1000+i)), 1000000, 10000)
	}
	for b.Loop() {
		_, _ = projection.Build(day("2026-09-14"), money.MustLookup("USD"), loans, projection.Extra{Minor: 100000})
	}
}

func TestMidlifePrincipalDoesNotAssertZeroAccruedInterest(t *testing.T) {
	l := loan("a", 100000, 10100)
	l.AsOf = day("2026-09-14")
	l.Contract.NominalRate = money.RateFromPercent(36, 0)
	r := build(t, []projection.Loan{l}, projection.Extra{Minor: 20000})
	if r.Complete || r.Finish != "" || r.Reason != "accrued_interest_needed" || len(r.Months) != 1 || r.Months[0].Required != 10100 || r.Months[0].Extra != 0 {
		t.Fatalf("unverified initialinterest became payoff %+v", r)
	}
}

func TestKnownOpeningInterestSettlementGolden(t *testing.T) {
	// Principal100000 atSep14; bank accruedinterest2000;36%Actual360.
	// Sep15 accrues100: required10100 credits8000 principal, leaves92000.
	// Sep30 adds1380 interest; settlement93380. Total103480, no double debit.
	l := loan("a", 100000, 10100)
	l.AsOf = day("2026-09-14")
	l.Contract.NominalRate = money.RateFromPercent(36, 0)
	l.Contract.DayCount = money.Actual360
	l.OpeningInterestKnown = true
	l.OpeningInterestMinor = 2000
	r := build(t, []projection.Loan{l}, projection.Extra{Minor: 200000})
	if !r.Complete || r.Finish != "2026-09" || r.Months[0].Required != 10100 || r.Months[0].Extra != 93380 || r.Months[0].Total != 103480 {
		t.Fatalf("opening accruedinterest golden %+v", r)
	}
}

func TestFirstBankPaymentIsNeverSilentlyReplaced(t *testing.T) {
	for _, l := range []projection.Loan{loan("a", 100, 150), func() projection.Loan {
		l := loan("a", 10000, 100)
		l.Contract.MaturityDate = day("2026-09-15")
		return l
	}()} {
		r := build(t, []projection.Loan{l}, projection.Extra{})
		if r.Complete || r.Reason != "bank_payment_mismatch" || r.Months[0].Required != l.Contract.ScheduledPayment.Minor() {
			t.Fatalf("bank instalment silently changed %+v", r)
		}
	}
}
