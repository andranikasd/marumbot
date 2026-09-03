package app

import (
	"context"
	"io"
	"log/slog"
	"strings"
	"testing"
	"time"

	"github.com/andranikasd/marumbot/internal/i18n"
	"github.com/andranikasd/marumbot/pkg/core/allocation"
	"github.com/andranikasd/marumbot/pkg/core/date"
	"github.com/andranikasd/marumbot/pkg/core/model"
	"github.com/andranikasd/marumbot/pkg/core/money"
)

type paidFakes struct {
	loan     UserLoan
	loanErr  error
	recorded []struct {
		loanID, userID, asOf string
		minor                int64
	}
	state    string
	messages []string
}

func (f *paidFakes) LoanForUser(context.Context, string, string) (UserLoan, error) {
	return f.loan, f.loanErr
}
func (f *paidFakes) UpdateLoan(context.Context, string, string, string, string) error { return nil }
func (f *paidFakes) ArchiveLoan(context.Context, string, string) error                { return nil }

func (f *paidFakes) RecordBalance(_ context.Context, loanID, userID string, minor int64, asOf string) error {
	f.recorded = append(f.recorded, struct {
		loanID, userID, asOf string
		minor                int64
	}{loanID, userID, asOf, minor})
	return nil
}

func (f *paidFakes) SetState(_ context.Context, _ string, s string) error { f.state = s; return nil }
func (f *paidFakes) State(context.Context, string) (string, error)        { return f.state, nil }
func (f *paidFakes) ClearState(context.Context, string) error             { f.state = ""; return nil }

func (f *paidFakes) SendMessage(_ context.Context, _ int64, text string, _ any) error {
	f.messages = append(f.messages, text)
	return nil
}
func (f *paidFakes) SendChatAction(context.Context, int64, string) error { return nil }

func (f *paidFakes) SetChatMenuButtonFor(context.Context, int64, string, string) error { return nil }

// Budget/loans reads reached through withTip on the confirmation path.
func (f *paidFakes) LoansForUser(context.Context, string, int32) ([]UserLoan, error) {
	return []UserLoan{f.loan}, nil
}
func (f *paidFakes) Budget(context.Context, string) (Budget, error) { return Budget{}, nil }
func (f *paidFakes) SetBudget(context.Context, string, string, int64, int) error {
	return nil
}

func paidLoan(t *testing.T) UserLoan {
	t.Helper()
	amd := money.MustLookup("AMD")
	v := date.MustNew(2026, 1, 15)
	return UserLoan{
		ID: "loan-a", Name: "Car",
		Contract: model.Contract{
			LoanID: "loan-a", Version: 1, Currency: amd, EffectiveFrom: v,
			NominalRate: money.RateFromPercent(18, 0), DayCount: money.Actual365,
			Type: model.Annuity, StartDate: v, MaturityDate: date.MustNew(2028, 1, 15),
			PaymentDay: 15, Rounding: money.DefaultPolicy(amd),
		},
		Balance: money.FromMinor(100_000_000, amd),
		AsOf:    v,
		Excess:  allocation.ExcessReducePrincipal,
		Trust:   "user_entered",
	}
}

func paidWorker(t *testing.T, f *paidFakes) *Worker {
	t.Helper()
	return &Worker{
		Editor: f, Balances: f, Convos: f, Send: f, Loans: f, Budgets: f,
		Clock:           &fixedPaidClock{},
		Log:             slog.New(slog.NewTextHandler(io.Discard, nil)),
		DefaultCurrency: money.MustLookup("AMD"),
	}
}

type fixedPaidClock struct{}

func (fixedPaidClock) Now() time.Time { return time.Date(2026, 9, 1, 10, 0, 0, 0, time.UTC) }

func TestPaidFlowRecordsTheStatedBalance(t *testing.T) {
	f := &paidFakes{loan: paidLoan(t)}
	w := paidWorker(t, f)
	ctx := context.Background()

	if err := w.askPaidBalance(ctx, "user-1", 42, i18n.EN, "loan-a"); err != nil {
		t.Fatalf("ask: %v", err)
	}
	if f.state != StateAwaitingBalance+":loan-a" {
		t.Fatalf("state = %q, want the loan id remembered", f.state)
	}

	taken, err := w.takeBalance(ctx, "user-1", 42, i18n.EN, f.state, "950 000")
	if err != nil || !taken {
		t.Fatalf("take: taken=%v err=%v", taken, err)
	}
	if len(f.recorded) != 1 {
		t.Fatalf("want 1 snapshot recorded, got %d", len(f.recorded))
	}
	r := f.recorded[0]
	if r.loanID != "loan-a" || r.userID != "user-1" || r.minor != 95_000_000 || r.asOf != "2026-09-01" {
		t.Errorf("recorded wrong: %+v", r)
	}
	if f.state != "" {
		t.Errorf("state not cleared: %q", f.state)
	}
	last := f.messages[len(f.messages)-1]
	if !strings.Contains(last, "Car") {
		t.Errorf("confirmation does not name the loan: %q", last)
	}
}

func TestPaidFlowRefusesAnotherCurrency(t *testing.T) {
	f := &paidFakes{loan: paidLoan(t)}
	w := paidWorker(t, f)

	taken, err := w.takeBalance(context.Background(), "user-1", 42, i18n.EN,
		StateAwaitingBalance+":loan-a", "500 USD")
	if err != nil || !taken {
		t.Fatalf("take: taken=%v err=%v", taken, err)
	}
	if len(f.recorded) != 0 {
		t.Fatal("a dollar figure anchored a dram loan")
	}
}

func TestPaidFlowLetsOrdinaryTextThrough(t *testing.T) {
	f := &paidFakes{loan: paidLoan(t)}
	w := paidWorker(t, f)

	taken, err := w.takeBalance(context.Background(), "user-1", 42, i18n.EN,
		StateAwaitingBalance+":loan-a", "thanks!")
	if err != nil {
		t.Fatalf("take: %v", err)
	}
	if taken {
		t.Fatal("ordinary text was swallowed as an answer")
	}
	if len(f.recorded) != 0 {
		t.Fatal("ordinary text produced a snapshot")
	}
}

func TestPaidFlowZeroMeansSettled(t *testing.T) {
	f := &paidFakes{loan: paidLoan(t)}
	w := paidWorker(t, f)

	taken, err := w.takeBalance(context.Background(), "user-1", 42, i18n.EN,
		StateAwaitingBalance+":loan-a", "0")
	if err != nil || !taken {
		t.Fatalf("take: taken=%v err=%v", taken, err)
	}
	if len(f.recorded) != 1 || f.recorded[0].minor != 0 {
		t.Fatalf("a zero balance was not recorded as stated: %+v", f.recorded)
	}
}

func TestPaidOpensQuickRecordWithoutInventingPayment(t *testing.T) {
	f := &paidFakes{loan: paidLoan(t)}
	w := paidWorker(t, f)
	w.MiniApp = "https://dev.example/app/"
	if err := w.askPaidBalance(t.Context(), "owner", 1, i18n.Locale("en"), "loan-a"); err != nil {
		t.Fatal(err)
	}
	if len(f.recorded) != 0 || f.state != "" {
		t.Fatal("acknowledgement created a fact or balance prompt")
	}
	if len(f.messages) != 1 || !strings.Contains(f.messages[0], "posting status") {
		t.Fatal("payment prompt missing")
	}
}
