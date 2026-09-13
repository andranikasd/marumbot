package miniapp

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/andranikasd/marumbot/internal/app"
	"github.com/andranikasd/marumbot/pkg/core/model"
	"github.com/andranikasd/marumbot/pkg/core/money"

	"github.com/andranikasd/marumbot/pkg/core/date"
)

func TestSetupLoanBankStatementValidation(t *testing.T) {
	today, _ := date.Parse("2026-09-14")
	base := setupLoanRequest{Currency: "USD", Principal: "1000", Remaining: "700", Start: "2026-01-15", End: "2027-01-15", NextDue: "2026-10-15", Payment: "100", Confirmed: true}
	statement, err := base.validate(today)
	if err != nil {
		t.Fatal(err)
	}
	if statement.Draft.Principal.Minor() != 100000 || statement.Draft.Balance.Minor() != 70000 || statement.PaymentMinor != 10000 || statement.NextDue.String() != "2026-10-15" || statement.InterestKnown {
		t.Fatal("bank statement changed or unknown rate treated as known")
	}
	zero := json.Number("0")
	base.Rate = &zero
	statement, err = base.validate(today)
	if err != nil || !statement.InterestKnown {
		t.Fatal("explicit zero interest must differ from unknown", err)
	}
	for _, change := range []func(*setupLoanRequest){
		func(r *setupLoanRequest) { r.Remaining = "" }, func(r *setupLoanRequest) { r.Principal = "" }, func(r *setupLoanRequest) { r.Remaining = "1001" },
		func(r *setupLoanRequest) { r.Payment = "" }, func(r *setupLoanRequest) { r.Payment = "0" }, func(r *setupLoanRequest) { r.NextDue = "2027-02-15" },
		func(r *setupLoanRequest) { r.Remaining = "1.001" }, func(r *setupLoanRequest) { r.Confirmed = false }, func(r *setupLoanRequest) { r.Start = r.End },
	} {
		r := base
		change(&r)
		if _, err := r.validate(today); err == nil {
			t.Fatalf("accepted invalid setup: %+v", r)
		}
	}
	base.Remaining = "0"
	base.Payment = "0"
	base.NextDue = ""
	statement, err = base.validate(today)
	if err != nil || statement.Draft.Balance.Minor() != 0 {
		t.Fatal("zero balance restored original principal", err)
	}
}

type setupLoansReader struct{ loan app.UserLoan }

func (f setupLoansReader) LoansForUser(context.Context, string, int32) ([]app.UserLoan, error) {
	return []app.UserLoan{f.loan}, nil
}

func TestLoanListShowsBankPaymentWithoutInventingUnknownRate(t *testing.T) {
	currency := money.MustLookup("USD")
	due, _ := date.Parse("2026-10-15")
	asof, _ := date.Parse("2026-09-14")
	loan := app.UserLoan{InterestUnknown: true, AsOf: asof, Balance: money.FromMinor(70000, currency), Contract: model.Contract{Currency: currency, HasScheduled: true, NotBeforeDue: due, ScheduledPayment: money.FromMinor(10000, currency)}}
	server := budgetTestServer(nil)
	server.Reader = setupLoansReader{loan}
	request := httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/api/loans", nil)
	request.Header.Set("X-Telegram-Init-Data", knownInitData())
	response := httptest.NewRecorder()
	server.Handler().ServeHTTP(response, request)
	if response.Code != http.StatusOK || !strings.Contains(response.Body.String(), `"rate_percent":null`) || !strings.Contains(response.Body.String(), `"next_due":"2026-10-15"`) || !strings.Contains(response.Body.String(), `"payment_source":"bank_statement"`) {
		t.Fatal("bank payment missing or unknown rate invented", response.Body.String())
	}
	loan.UnreconciledPayments = true
	server.Reader = setupLoansReader{loan}
	response = httptest.NewRecorder()
	server.Handler().ServeHTTP(response, request)
	if strings.Contains(response.Body.String(), `"next_due"`) {
		t.Fatal("stale bank amount published after payment")
	}
}

func TestSetupPreservesOverdueDate(t *testing.T) {
	today, _ := date.Parse("2026-09-14")
	statement, err := (setupLoanRequest{Currency: "USD", Principal: "1000", Remaining: "700", Start: "2026-01-15", End: "2027-01-15", NextDue: "2026-08-15", Payment: "100", Confirmed: true}).validate(today)
	if err != nil || statement.NextDue.String() != "2026-08-15" {
		t.Fatal("overdue date moved or rejected", err)
	}
	loan := app.UserLoan{Balance: statement.Draft.Balance, AsOf: today, Contract: statement.Draft.Contract}
	loan.Contract.NotBeforeDue = statement.NextDue
	if _, err = loan.Schedule(); !errors.Is(err, app.ErrLoanOverdueNeedsReview) {
		t.Fatal("overdue obligation forecast as ordinary current loan", err)
	}
}

func TestUnknownRateBalanceEditDoesNotAssertZeroInterest(t *testing.T) {
	balance := int64(50000)
	request := LoanEditRequest{Name: "Loan", StartDate: "2026-01-15", MaturityDate: "2027-01-15", PaymentDay: 15, SnapshotMinor: &balance, SnapshotAsOf: "2026-09-14"}
	edit, err := request.Validate(money.MustLookup("USD"))
	if err != nil || !edit.BalanceOnly {
		t.Fatal("unknown-rate snapshot coerced into full zero-rate edit", err)
	}
	request.SnapshotMinor = nil
	if _, err = request.Validate(money.MustLookup("USD")); err == nil {
		t.Fatal("missing rate accepted as explicit zero in full edit")
	}
}

func TestSetupAccruedInterestSource(t *testing.T) {
	today, _ := date.Parse("2026-09-14")
	input := setupLoanRequest{Currency: "USD", Principal: "1000", Remaining: "700", Start: "2026-01-15", End: "2027-01-15", NextDue: "2026-10-15", Payment: "100", Confirmed: true}
	for _, raw := range []json.Number{"0", "20.01"} {
		input.AccruedInterest = &raw
		statement, err := input.validate(today)
		if err != nil || statement.AccruedInterestMinor == nil {
			t.Fatal("explicit bank interest lost", err)
		}
	}
	for _, raw := range []json.Number{"-1", "1.001", "90071992547409.92"} {
		input.AccruedInterest = &raw
		if _, err := input.validate(today); err == nil {
			t.Fatal("invalid accrued interest accepted")
		}
	}
	input.Remaining = "0"
	input.Payment = "0"
	input.NextDue = ""
	positive := json.Number("1")
	input.AccruedInterest = &positive
	if _, err := input.validate(today); err == nil {
		t.Fatal("interest-only debt shown as fully paid")
	}
}
