package app

import (
	"context"
	"io"
	"log/slog"
	"testing"
	"time"

	"github.com/andranikasd/marumbot/pkg/core/model"
	"github.com/andranikasd/marumbot/pkg/core/money"
)

type reviseFakes struct {
	paidFakes // Editor, Balances, Send, Convos, Loans, Budgets

	revised   []model.Contract
	renamed   int
	cancelled int
	ensured   int
}

func (f *reviseFakes) ApplyLoanRevision(_ context.Context, _, _ string, r LoanRevision) error {
	if r.Contract != nil {
		f.revised = append(f.revised, *r.Contract)
	}
	if r.Rename {
		f.renamed++
	}
	if r.BalanceMinor != nil {
		f.recorded = append(f.recorded, struct {
			loanID, userID, asOf string
			minor                int64
		}{minor: *r.BalanceMinor})
	}
	return nil
}

func (f *reviseFakes) EnsureDefaultReminders(context.Context, string) error {
	f.ensured++
	return nil
}

func (f *reviseFakes) ScheduleReminders(context.Context, time.Time, string) error { return nil }

func (f *reviseFakes) DueReminders(context.Context, int32) ([]DueReminder, error) { return nil, nil }

func (f *reviseFakes) MarkReminderSatisfied(context.Context, string) error { return nil }

func (f *reviseFakes) CancelRemindersForLoan(context.Context, string) error {
	f.cancelled++
	return nil
}

func reviseWorker(t *testing.T, f *reviseFakes) *Worker {
	t.Helper()
	return &Worker{
		Editor: f, Balances: f, Contracts: f, Convos: f, Send: f,
		Loans: f, Budgets: f, Reminders: f,
		Clock:           &fixedPaidClock{},
		Log:             slog.New(slog.NewTextHandler(io.Discard, nil)),
		DefaultCurrency: money.MustLookup("AMD"),
	}
}

func baseEdit(ln UserLoan) LoanEdit {
	return LoanEdit{
		Name:         ln.Name,
		Description:  ln.Description,
		NominalRate:  ln.Contract.NominalRate,
		Type:         ln.Contract.Type,
		StartDate:    ln.Contract.StartDate,
		MaturityDate: ln.Contract.MaturityDate,
		PaymentDay:   ln.Contract.PaymentDay,
		PrepayEffect: ln.Contract.Prepayment.Effect,
	}
}

// An edit that changes nothing writes nothing: no version churn, no rename,
// no snapshot, no reminder reshuffle.
func TestReviseLoanNoChangeWritesNothing(t *testing.T) {
	f := &reviseFakes{paidFakes: paidFakes{loan: paidLoan(t)}}
	w := reviseWorker(t, f)

	if err := w.ReviseLoan(context.Background(), "loan-a", "user-1", baseEdit(f.loan)); err != nil {
		t.Fatalf("revise: %v", err)
	}
	if len(f.revised) != 0 || f.renamed != 0 || len(f.recorded) != 0 || f.cancelled != 0 {
		t.Errorf("a no-op edit wrote something: revised=%d renamed=%d snapshots=%d cancelled=%d",
			len(f.revised), f.renamed, len(f.recorded), f.cancelled)
	}
}

func TestReviseLoanVersionsChangedTerms(t *testing.T) {
	f := &reviseFakes{paidFakes: paidFakes{loan: paidLoan(t)}}
	w := reviseWorker(t, f)

	e := baseEdit(f.loan)
	e.NominalRate = money.RateFromPercent(15, 0)
	e.PaymentDay = 20
	if err := w.ReviseLoan(context.Background(), "loan-a", "user-1", e); err != nil {
		t.Fatalf("revise: %v", err)
	}
	if len(f.revised) != 1 {
		t.Fatalf("want 1 new contract version, got %d", len(f.revised))
	}
	c := f.revised[0]
	if c.NominalRate != money.RateFromPercent(15, 0) || c.PaymentDay != 20 {
		t.Errorf("revision missed the edit: %+v", c)
	}
	// The terms the form does not carry ride over untouched.
	if c.DayCount != f.loan.Contract.DayCount || c.Rounding != f.loan.Contract.Rounding {
		t.Errorf("revision changed terms the form never edits: %+v", c)
	}
	// A changed payment day reshuffles the reminders.
	if f.cancelled != 1 || f.ensured != 1 {
		t.Errorf("reminders not reshuffled: cancelled=%d ensured=%d", f.cancelled, f.ensured)
	}
}

func TestReviseLoanRestatesTheBalance(t *testing.T) {
	f := &reviseFakes{paidFakes: paidFakes{loan: paidLoan(t)}}
	w := reviseWorker(t, f)

	e := baseEdit(f.loan)
	minor := int64(75_000_000)
	e.BalanceMinor = &minor
	if err := w.ReviseLoan(context.Background(), "loan-a", "user-1", e); err != nil {
		t.Fatalf("revise: %v", err)
	}
	if len(f.revised) != 0 {
		t.Errorf("a balance restatement minted a contract version")
	}
	if len(f.recorded) != 1 || f.recorded[0].minor != 75_000_000 {
		t.Fatalf("balance not recorded: %+v", f.recorded)
	}
}

func TestReviseLoanRenamesWords(t *testing.T) {
	f := &reviseFakes{paidFakes: paidFakes{loan: paidLoan(t)}}
	w := reviseWorker(t, f)

	e := baseEdit(f.loan)
	e.Name = "The blue car"
	if err := w.ReviseLoan(context.Background(), "loan-a", "user-1", e); err != nil {
		t.Fatalf("revise: %v", err)
	}
	if f.renamed != 1 || len(f.revised) != 0 {
		t.Errorf("rename wrote wrong things: renamed=%d revised=%d", f.renamed, len(f.revised))
	}
}
