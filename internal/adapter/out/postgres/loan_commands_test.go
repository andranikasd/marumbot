package postgres_test

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/andranikasd/marumbot/internal/app"
)

func TestLoanCommandsConcurrentCreateAndRetry(t *testing.T) {
	s := testStore(t)
	ctx := context.Background()
	user := newUser(t, s)
	service := app.LoanCommands{Store: s, Clock: clock, Users: s}
	d := draft(user, t)
	key := uuid.NewString()
	var wg sync.WaitGroup
	receipts := make([]app.LoanCommandReceipt, 6)
	errs := make([]error, 6)
	for i := range receipts {
		wg.Add(1)
		go func() { defer wg.Done(); receipts[i], errs[i] = service.Create(ctx, key, d) }()
	}
	wg.Wait()
	for i, r := range receipts {
		if errs[i] != nil {
			t.Fatal(errs[i])
		}
		if r != receipts[0] {
			t.Fatal("retry returned another receipt")
		}
	}
	loans, err := s.LoansForUser(ctx, user, 100)
	if err != nil || len(loans) != 1 {
		t.Fatalf("create count=%d err=%v", len(loans), err)
	}
	d.Title = "different intent"
	if _, err = service.Create(ctx, key, d); !errors.Is(err, app.ErrConflict) {
		t.Fatalf("changed payload: %v", err)
	}
	d.Title = "Car loan"
	d.AsOf = mustDate(t, "2026-08-02")
	if r, err := service.Create(ctx, key, d); err != nil || r != receipts[0] {
		t.Fatalf("next-day retry: %v", err)
	}
}

func TestLoanCommandsCASAndArchiveReceipt(t *testing.T) {
	s := testStore(t)
	ctx := context.Background()
	user := newUser(t, s)
	service := app.LoanCommands{Store: s, Clock: clock, Users: s}
	created, err := service.Create(ctx, uuid.NewString(), draft(user, t))
	if err != nil {
		t.Fatal(err)
	}
	var wg sync.WaitGroup
	errs := make([]error, 2)
	receipts := make([]app.LoanCommandReceipt, 2)
	keys := []string{uuid.NewString(), uuid.NewString()}
	for i := range errs {
		wg.Add(1)
		go func() {
			defer wg.Done()
			receipts[i], errs[i] = service.Rename(ctx, user, created.ID, keys[i], created.Version, "New name", "")
		}()
	}
	wg.Wait()
	winner := -1
	conflicts := 0
	for i, err := range errs {
		switch {
		case err == nil:
			winner = i
		case errors.Is(err, app.ErrConflict):
			conflicts++
		default:
			t.Fatal(err)
		}
	}
	if winner < 0 || conflicts != 1 {
		t.Fatal("concurrent edits did not enforce CAS")
	}
	r, err := service.Rename(ctx, user, created.ID, keys[winner], created.Version, "New name", "")
	if err != nil || r != receipts[winner] {
		t.Fatalf("lost response retry: %v", err)
	}
	if _, err = service.Archive(ctx, newUser(t, s), created.ID, uuid.NewString(), r.Version); !errors.Is(err, app.ErrNotFound) {
		t.Fatalf("ownership: %v", err)
	}
	key := uuid.NewString()
	archived, err := service.Archive(ctx, user, created.ID, key, r.Version)
	if err != nil {
		t.Fatal(err)
	}
	again, err := service.Archive(ctx, user, created.ID, key, r.Version)
	if err != nil || again != archived {
		t.Fatalf("archive retry: %v", err)
	}
	if _, err = service.Rename(ctx, user, created.ID, uuid.NewString(), archived.Version, "No resurrection", ""); !errors.Is(err, app.ErrNotFound) {
		t.Fatalf("archived loan modified: %v", err)
	}
}

func TestLoanCommandsFullRevisionAtomicAndSourceCAS(t *testing.T) {
	s := testStore(t)
	ctx := context.Background()
	user := newUser(t, s)
	service := app.LoanCommands{Store: s, Clock: clock, Users: s}
	created, err := service.Create(ctx, uuid.NewString(), draft(user, t))
	if err != nil {
		t.Fatal(err)
	}
	ln, err := s.LoanForUser(ctx, created.ID, user)
	if err != nil {
		t.Fatal(err)
	}
	balance := ln.Balance.Minor() - 100
	edit := app.LoanEdit{Name: "Revised", Description: ln.Description, NominalRate: ln.Contract.NominalRate, Type: ln.Contract.Type, StartDate: ln.Contract.StartDate, MaturityDate: ln.Contract.MaturityDate, PaymentDay: 16, PrepayEffect: ln.Contract.Prepayment.Effect, BalanceMinor: &balance}
	key := uuid.NewString()
	r, err := service.Revise(ctx, user, ln.ID, key, ln.MutationVersion, edit)
	if err != nil {
		t.Fatal(err)
	}
	repeated, err := service.Revise(ctx, user, ln.ID, key, ln.MutationVersion, edit)
	if err != nil || repeated != r {
		t.Fatalf("revision retry: %v", err)
	}
	after, err := s.LoanForUser(ctx, ln.ID, user)
	if err != nil {
		t.Fatal(err)
	}
	if after.Contract.Version != ln.Contract.Version+1 || after.Balance.Minor() != balance {
		t.Fatal("revision not applied exactly once")
	}
	if err = s.RecordBalance(ctx, ln.ID, user, balance-100, "2026-09-04"); err != nil {
		t.Fatal(err)
	}
	if _, err = service.Archive(ctx, user, ln.ID, uuid.NewString(), after.MutationVersion); !errors.Is(err, app.ErrConflict) {
		t.Fatalf("snapshot failed to invalidate form: %v", err)
	}
	latest, err := s.LoanForUser(ctx, ln.ID, user)
	if err != nil {
		t.Fatal(err)
	}
	bad := int64(-1)
	edit.BalanceMinor = &bad
	edit.Name = "Must roll back"
	failedKey := uuid.NewString()
	if _, err = service.Revise(ctx, user, ln.ID, failedKey, latest.MutationVersion, edit); err == nil {
		t.Fatal("invalid snapshot accepted")
	}
	unchanged, err := s.LoanForUser(ctx, ln.ID, user)
	if err != nil {
		t.Fatal(err)
	}
	if unchanged.Name != latest.Name || unchanged.MutationVersion != latest.MutationVersion {
		t.Fatal("failed edit leaked a partial write")
	}
	edit.BalanceMinor = nil
	if _, err = service.Revise(ctx, user, ln.ID, failedKey, latest.MutationVersion, edit); err != nil {
		t.Fatalf("failed transaction retained a receipt: %v", err)
	}
}

func TestLoanCommandsMoreWaitersThanPoolConnections(t *testing.T) {
	s := testStore(t)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	user := newUser(t, s)
	service := app.LoanCommands{Store: s, Clock: clock, Users: s}
	loan, err := service.Create(ctx, uuid.NewString(), draft(user, t))
	if err != nil {
		t.Fatal(err)
	}
	start := make(chan struct{})
	errs := make([]error, 24)
	var wg sync.WaitGroup
	key := uuid.NewString()
	for i := range errs {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			_, errs[i] = service.Rename(ctx, user, loan.ID, key, loan.Version, "Concurrent retry", "")
		}()
	}
	close(start)
	wg.Wait()
	for _, err := range errs {
		if err != nil {
			t.Fatal(err)
		}
	}
}

func TestLoanCommandsFullEditRetryAfterArchive(t *testing.T) {
	s := testStore(t)
	ctx := context.Background()
	user := newUser(t, s)
	service := app.LoanCommands{Store: s, Clock: clock, Users: s}
	created, err := service.Create(ctx, uuid.NewString(), draft(user, t))
	if err != nil {
		t.Fatal(err)
	}
	ln, err := s.LoanForUser(ctx, created.ID, user)
	if err != nil {
		t.Fatal(err)
	}
	key := uuid.NewString()
	edit := app.LoanEdit{Name: "Edited", Description: ln.Description, NominalRate: ln.Contract.NominalRate, Type: ln.Contract.Type, StartDate: ln.Contract.StartDate, MaturityDate: ln.Contract.MaturityDate, PaymentDay: ln.Contract.PaymentDay, PrepayEffect: ln.Contract.Prepayment.Effect}
	saved, err := service.Revise(ctx, user, ln.ID, key, ln.MutationVersion, edit)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = service.Archive(ctx, user, ln.ID, uuid.NewString(), saved.Version); err != nil {
		t.Fatal(err)
	}
	currency, err := service.Currency(ctx, ln.ID, user)
	if err != nil || currency != ln.Contract.Currency {
		t.Fatalf("archived retry decoding: %v", err)
	}
	retried, err := service.Revise(ctx, user, ln.ID, key, ln.MutationVersion, edit)
	if err != nil || retried != saved {
		t.Fatalf("archived edit retry: %v", err)
	}
	if _, err = service.Currency(ctx, ln.ID, newUser(t, s)); !errors.Is(err, app.ErrNotFound) {
		t.Fatalf("currency ownership: %v", err)
	}
	if _, err = service.Revise(ctx, user, ln.ID, uuid.NewString(), saved.Version, edit); !errors.Is(err, app.ErrNotFound) {
		t.Fatalf("new edit on archive: %v", err)
	}
}
