package app

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"testing"
	"time"

	"github.com/andranikasd/marumbot/pkg/core/date"
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
		Key: "test-revision-key-0001", ExpectedVersion: 1,
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

func TestBackdatedSnapshotCannotUseNewTerms(t *testing.T) {
	f := &reviseFakes{paidFakes: paidFakes{loan: paidLoan(t)}}
	w := reviseWorker(t, f)
	e := baseEdit(f.loan)
	e.NominalRate = money.RateFromPercent(15, 0)
	n := int64(10000)
	e.BalanceMinor = &n
	e.BalanceAsOf = date.AddDays(date.From(w.Clock.Now(), time.UTC), -1)
	if err := w.ReviseLoan(context.Background(), "loan-a", "user-1", e); !errors.Is(err, ErrSnapshotContractDate) {
		t.Fatalf("historical balance accepted: %v", err)
	}
	if len(f.revised) != 0 || len(f.recorded) != 0 {
		t.Fatal("partial mutation before refusal")
	}
}

// A locally current statement must not be rejected during the hours when UTC
// is still on the previous date.
func TestReviseLoanUsesAccountBusinessDate(t *testing.T) {
	f := &reviseFakes{paidFakes: paidFakes{loan: paidLoan(t)}}
	w := reviseWorker(t, f)
	w.Clock = &fixedClock{at: time.Date(2026, 6, 14, 22, 0, 0, 0, time.UTC)}
	w.Users = reminderUsersFake{prefs: UserPreferences{Timezone: "Asia/Yerevan"}}
	e := baseEdit(f.loan)
	e.BalanceAsOf, _ = date.Parse("2026-06-15")
	balance := f.loan.Balance.Minor()
	e.BalanceMinor = &balance
	if err := w.ReviseLoan(context.Background(), "loan-a", "user-1", e); err != nil {
		t.Fatal(err)
	}
	if len(f.recorded) != 1 {
		t.Fatal("current local-date statement was not recorded")
	}
	e.BalanceAsOf, _ = date.Parse("2026-06-16")
	if err := w.ReviseLoan(context.Background(), "loan-a", "user-1", e); err == nil {
		t.Fatal("future local-date statement accepted")
	}
}

// Existing revision assertions also run through the command transaction boundary.
func (f *reviseFakes) BeginLoanCommand(context.Context) (LoanCommandTransaction, error) {
	return &revisionCommandFake{reviseFakes: f}, nil
}

type revisionCommandFake struct{ *reviseFakes }

func (f *revisionCommandFake) LockUser(context.Context, string) error { return nil }

func (f *revisionCommandFake) LockLoan(context.Context, string, string) (int64, error) { return 1, nil }

func (f *revisionCommandFake) Version(context.Context, string, string) (int64, error) { return 1, nil }

func (f *revisionCommandFake) Receipt(context.Context, string, string) (LoanCommandReceipt, string, error) {
	return LoanCommandReceipt{}, "", ErrNotFound
}

func (f *revisionCommandFake) RecordReceipt(context.Context, string, string, string, LoanCommandReceipt) error {
	return nil
}

func (f *revisionCommandFake) CreateLoan(context.Context, LoanDraft) (string, error) {
	return "", errors.New("unexpected create")
}
func (f *revisionCommandFake) Commit(context.Context) error   { return nil }
func (f *revisionCommandFake) Rollback(context.Context) error { return nil }

func TestReviseLoanRejectsMissingCommandIdentity(t *testing.T) {
	f := &reviseFakes{paidFakes: paidFakes{loan: paidLoan(t)}}
	w := reviseWorker(t, f)
	e := baseEdit(f.loan)
	e.Key = ""
	if err := w.ReviseLoan(context.Background(), "loan-a", "user-1", e); !errors.Is(err, ErrPaymentInvalid) {
		t.Fatalf("missing key: %v", err)
	}
	if f.renamed != 0 || len(f.revised) != 0 || len(f.recorded) != 0 {
		t.Fatal("invalid command wrote facts")
	}
}

func (f *reviseFakes) LoanCommandCurrency(context.Context, string, string) (money.Currency, error) {
	return f.loan.Contract.Currency, nil
}
