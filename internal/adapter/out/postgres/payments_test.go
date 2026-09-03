package postgres_test

import (
	"context"
	"errors"
	"sync"
	"testing"

	"github.com/google/uuid"

	"github.com/andranikasd/marumbot/internal/app"
)

func TestPaymentLifecycle(t *testing.T) {
	s := testStore(t)
	ctx := context.Background()
	owner := newUser(t, s)
	loan, err := s.CreateLoan(ctx, draft(owner, t))
	if err != nil {
		t.Fatal(err)
	}
	service := app.PaymentService{Store: s, Clock: clock}
	command := app.PaymentCommand{LoanID: loan, Key: uuid.NewString(), AmountMinor: 12345, TransactionDate: "2026-08-02"}
	receipt, err := service.Record(ctx, owner, command)
	if err != nil {
		t.Fatal(err)
	}
	if receipt.Version != 1 || receipt.Status != "pending_bank_posting" {
		t.Fatalf("wrong pending receipt: %+v", receipt)
	}
	repeat, err := service.Record(ctx, owner, command)
	if err != nil || repeat != receipt {
		t.Fatalf("retry: %+v %v", repeat, err)
	}
	changed := command
	changed.AmountMinor++
	if _, err := service.Record(ctx, owner, changed); !errors.Is(err, app.ErrConflict) {
		t.Fatalf("key reuse: %v", err)
	}
	if _, err := service.Record(ctx, newUser(t, s), command); !errors.Is(err, app.ErrNotFound) {
		t.Fatalf("ownership: %v", err)
	}
	duplicate := command
	duplicate.Key = uuid.NewString()
	duplicate.ExpectedVersion = 1
	if _, err := service.Record(ctx, owner, duplicate); !errors.Is(err, app.ErrPaymentDuplicate) {
		t.Fatalf("duplicate: %v", err)
	}
	correction := duplicate
	correction.Replaces = receipt.ID
	correction.ValueDate = "2026-08-03"
	posted, err := service.Record(ctx, owner, correction)
	if err != nil {
		t.Fatal(err)
	}
	if posted.Version != 3 || posted.Status != "needs_reconciliation" {
		t.Fatalf("wrong posted correction: %+v", posted)
	}
	facts, err := s.BorrowerActivity(ctx, owner)
	if err != nil {
		t.Fatal(err)
	}
	payments, voids := 0, 0
	for _, fact := range facts {
		if fact.Kind == "payment_reported" {
			payments++
			if fact.ID == receipt.ID && (!fact.Voided || fact.ValueDate != "") {
				t.Fatal("original pending fact was rewritten")
			}
		}
		if fact.Kind == "entry_voided" {
			voids++
		}
	}
	if payments != 2 || voids != 1 {
		t.Fatalf("history payments=%d voids=%d", payments, voids)
	}
	context, err := s.PaymentContext(ctx, loan, owner)
	if err != nil || context.Version != 3 {
		t.Fatalf("metadata: %+v %v", context, err)
	}
	read, err := s.LoanForUser(ctx, loan, owner)
	if err != nil || !read.UnreconciledPayments {
		t.Fatalf("unreconciled payment hidden: %v", err)
	}
	// A second void of an already corrected entry cannot create another fact.
	invalid := app.PaymentCommand{LoanID: loan, Key: uuid.NewString(), ExpectedVersion: 3, TransactionDate: "2026-08-04", Replaces: receipt.ID, VoidOnly: true}
	if _, err := service.Record(ctx, owner, invalid); !errors.Is(err, app.ErrConflict) {
		t.Fatalf("double void: %v", err)
	}
	invalid.Replaces = posted.ID
	final, err := service.Record(ctx, owner, invalid)
	if err != nil || final.Version != 4 {
		t.Fatalf("void: %+v %v", final, err)
	}
	read, err = s.LoanForUser(ctx, loan, owner)
	if err != nil || read.UnreconciledPayments {
		t.Fatalf("voided payments still active: %v", err)
	}
	// Admin must remain readable even with the pending original in history.
	if _, err := s.EventsForLoan(ctx, loan); err != nil {
		t.Fatal(err)
	}
}

func TestPaymentConcurrentWritesAndAtomicCorrection(t *testing.T) {
	s := testStore(t)
	ctx := context.Background()
	owner := newUser(t, s)
	loan, err := s.CreateLoan(ctx, draft(owner, t))
	if err != nil {
		t.Fatal(err)
	}
	service := app.PaymentService{Store: s, Clock: clock}
	command := app.PaymentCommand{LoanID: loan, Key: uuid.NewString(), AmountMinor: 100, TransactionDate: "2026-08-02", ValueDate: "2026-08-02"}
	// Concurrent identical retries create precisely one record.
	var wg sync.WaitGroup
	receipts := make(chan app.PaymentReceipt, 8)
	failures := make(chan error, 8)
	for range 8 {
		wg.Go(func() { receipt, err := service.Record(ctx, owner, command); receipts <- receipt; failures <- err })
	}
	wg.Wait()
	close(receipts)
	close(failures)
	for err := range failures {
		if err != nil {
			t.Fatal(err)
		}
	}
	var first app.PaymentReceipt
	for receipt := range receipts {
		if first.ID == "" {
			first = receipt
		}
		if receipt != first {
			t.Fatal("retry duplicated payment")
		}
	}
	// A missing historical contract makes the replacement fail after its void.
	// Both writes, including the sequence increment, must roll back.
	correction := command
	correction.Key = uuid.NewString()
	correction.ExpectedVersion = 1
	correction.Replaces = first.ID
	correction.TransactionDate = "2000-01-01"
	correction.ValueDate = "2000-01-01"
	if _, err := service.Record(ctx, owner, correction); err == nil {
		t.Fatal("unsupported historical date accepted")
	}
	meta, err := s.PaymentContext(ctx, loan, owner)
	if err != nil || meta.Version != 1 {
		t.Fatalf("partial correction committed: %+v %v", meta, err)
	}
	// Two different writes from the same version: exactly one wins.
	errorsOut := make(chan error, 2)
	for i := range 2 {
		c := command
		c.Key = uuid.NewString()
		c.ExpectedVersion = 1
		c.AmountMinor += int64(i) + 1
		wg.Go(func() { _, err := service.Record(ctx, owner, c); errorsOut <- err })
	}
	wg.Wait()
	close(errorsOut)
	success, conflict := 0, 0
	for err := range errorsOut {
		switch {
		case err == nil:
			success++
		case errors.Is(err, app.ErrConflict):
			conflict++
		default:
			t.Fatal(err)
		}
	}
	if success != 1 || conflict != 1 {
		t.Fatalf("concurrency: %d successes %d conflicts", success, conflict)
	}
}

func TestActivityCursorReachesOlderPayments(t *testing.T) {
	s := testStore(t)
	ctx := context.Background()
	owner := newUser(t, s)
	loan, err := s.CreateLoan(ctx, draft(owner, t))
	if err != nil {
		t.Fatal(err)
	}
	service := app.PaymentService{Store: s, Clock: clock}
	receipt, err := service.Record(ctx, owner, app.PaymentCommand{LoanID: loan, Key: uuid.NewString(), AmountMinor: 1, TransactionDate: "2026-08-02"})
	if err != nil {
		t.Fatal(err)
	}
	for range 101 {
		if err := s.RecordBalance(ctx, loan, owner, 100, "2026-08-03"); err != nil {
			t.Fatal(err)
		}
	}
	first, err := s.BorrowerActivity(ctx, owner)
	if err != nil || len(first) != 100 {
		t.Fatalf("first page: %d %v", len(first), err)
	}
	cursor := first[len(first)-1].ID
	// New inserts after page one do not shift the immutable cursor.
	if err := s.RecordBalance(ctx, loan, owner, 100, "2026-08-03"); err != nil {
		t.Fatal(err)
	}
	second, err := s.BorrowerActivityAfter(ctx, owner, cursor)
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, f := range second {
		if f.ID == receipt.ID {
			found = true
		}
		for _, earlier := range first {
			if f.ID == earlier.ID {
				t.Fatal("duplicate record across pages")
			}
		}
	}
	if !found {
		t.Fatal("older payment inaccessible")
	}
	foreign, err := s.BorrowerActivityAfter(ctx, newUser(t, s), cursor)
	if err != nil || len(foreign) != 0 {
		t.Fatalf("foreign cursor exposed data: %v", err)
	}
}
