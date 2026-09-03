// Integration tests for the store, against a real postgres with the
// migrations applied.
//
// They exist because this package maps SQL columns to Go fields by position,
// and nothing else checks that the two agree: the worst bug the audit found
// was GetLoanForUser silently dropping the columns that anchor a schedule,
// which no unit test could see. Every test here goes through the public store
// API only.
//
// Skipped unless TEST_DATABASE_URL is set. Locally: `make test-store`.
// The schema must already be migrated (goose up); the tests create their own
// rows and never truncate, so they are safe against a shared database.
package postgres_test

import (
	"context"
	"crypto/rand"
	"encoding/binary"
	"errors"
	"fmt"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/andranikasd/marumbot/internal/adapter/out/postgres"
	"github.com/andranikasd/marumbot/internal/adapter/out/sysclock"
	"github.com/andranikasd/marumbot/internal/app"
	"github.com/andranikasd/marumbot/pkg/core/date"
	"github.com/andranikasd/marumbot/pkg/core/model"
	"github.com/andranikasd/marumbot/pkg/core/money"
	"github.com/andranikasd/marumbot/pkg/core/plan"
)

// clock: real time through the one adapter allowed to own it (I2).
var clock = sysclock.New()

func testStore(t *testing.T) *postgres.Store {
	t.Helper()
	dsn := os.Getenv("TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("TEST_DATABASE_URL is not set; run `make test-store` for the full story")
	}
	s, err := postgres.Open(context.Background(), dsn, nil)
	if err != nil {
		t.Fatalf("opening the store: %v", err)
	}
	t.Cleanup(s.Close)
	return s
}

// newUser creates a fresh account through the same path the webhook uses.
func newUser(t *testing.T, s *postgres.Store) string {
	t.Helper()
	a, err := s.UpsertByTelegram(context.Background(), freshUpsert())
	if err != nil {
		t.Fatalf("creating a user: %v", err)
	}
	if !a.Created {
		t.Fatalf("fresh identity resolved to an existing account %s", a.ID)
	}
	return a.ID
}

func freshUpsert() app.UpsertUser {
	return app.UpsertUser{
		UserTag:    "it-user-" + uuid.NewString(),
		UserSealed: []byte("sealed-user"),
		ChatTag:    "it-chat-" + uuid.NewString(),
		ChatSealed: []byte("sealed-chat"),
		KeyVersion: 1,
		NewID:      uuid.NewString(),
		Locale:     "hy",
		Timezone:   "Asia/Yerevan",
		TrialEnds:  clock.Now().Add(14 * 24 * time.Hour),
	}
}

func mustDate(t *testing.T, s string) date.Date {
	t.Helper()
	d, err := date.Parse(s)
	if err != nil {
		t.Fatalf("parsing %q: %v", s, err)
	}
	return d
}

// randUpdateID returns an update id that cannot collide with other test runs
// against the same database.
func randUpdateID(t *testing.T) int64 {
	t.Helper()
	var b [8]byte
	if _, err := rand.Read(b[:]); err != nil {
		t.Fatalf("reading randomness: %v", err)
	}
	return int64(binary.BigEndian.Uint64(b[:]) >> 1) // positive
}

// draft builds a loan with deliberately non-default terms, so a mapping that
// falls back to a default is caught rather than accidentally right.
func draft(userID string, t *testing.T) app.LoanDraft {
	cur := money.MustLookup("AMD")
	return app.LoanDraft{
		UserID:      userID,
		Title:       "Car loan",
		Description: "the blue one",
		Contract: model.Contract{
			Currency:     cur,
			NominalRate:  money.RateFromPercent(16, 500000),
			DayCount:     money.Actual360,
			Type:         model.DecliningPrincipal,
			StartDate:    mustDate(t, "2024-03-15"),
			MaturityDate: mustDate(t, "2027-03-15"),
			PaymentDay:   15,
			Rounding:     money.Policy{Mode: money.HalfEven, Unit: 10},
		},
		Principal: money.FromMinor(3_000_000_00, cur),
		Balance:   money.FromMinor(1_840_000_00, cur),
		AsOf:      mustDate(t, "2026-08-01"),
	}
}

// TestLoanContractSurvivesBothReads is the regression test for the audit's
// worst finding: the single-loan read rebuilt the contract from defaults
// while the list read mapped it fully, and the two surfaces showed different
// instalments for the same loan.
func TestLoanContractSurvivesBothReads(t *testing.T) {
	s := testStore(t)
	ctx := context.Background()
	userID := newUser(t, s)

	d := draft(userID, t)
	loanID, err := s.CreateLoan(ctx, d)
	if err != nil {
		t.Fatalf("creating the loan: %v", err)
	}

	list, err := s.LoansForUser(ctx, userID, 10)
	if err != nil {
		t.Fatalf("listing loans: %v", err)
	}
	if len(list) != 1 {
		t.Fatalf("expected 1 loan, got %d", len(list))
	}
	one, err := s.LoanForUser(ctx, loanID, userID)
	if err != nil {
		t.Fatalf("reading the loan: %v", err)
	}

	for name, pair := range map[string][2]any{
		"ID":          {list[0].ID, one.ID},
		"Name":        {list[0].Name, one.Name},
		"Description": {list[0].Description, one.Description},
		"DayCount":    {list[0].Contract.DayCount, one.Contract.DayCount},
		"Type":        {list[0].Contract.Type, one.Contract.Type},
		"Rate":        {list[0].Contract.NominalRate, one.Contract.NominalRate},
		"Rounding":    {list[0].Contract.Rounding, one.Contract.Rounding},
		"PaymentDay":  {list[0].Contract.PaymentDay, one.Contract.PaymentDay},
		"StartDate":   {list[0].Contract.StartDate, one.Contract.StartDate},
		"Maturity":    {list[0].Contract.MaturityDate, one.Contract.MaturityDate},
		"Balance":     {list[0].Balance.Minor(), one.Balance.Minor()},
		"AsOf":        {list[0].AsOf, one.AsOf},
		"Trust":       {list[0].Trust, one.Trust},
		"Excess":      {list[0].Excess, one.Excess},
	} {
		if pair[0] != pair[1] {
			t.Errorf("%s differs between list and single read: %v vs %v", name, pair[0], pair[1])
		}
	}

	// And both must agree with what was filed, not merely with each other.
	c := one.Contract
	if c.DayCount != money.Actual360 {
		t.Errorf("day count: filed act360, read back %v", c.DayCount)
	}
	if c.Type != model.DecliningPrincipal {
		t.Errorf("repayment type: filed declining, read back %v", c.Type)
	}
	if c.Rounding != (money.Policy{Mode: money.HalfEven, Unit: 10}) {
		t.Errorf("rounding: filed half_even/10, read back %+v", c.Rounding)
	}
	if c.NominalRate != d.Contract.NominalRate {
		t.Errorf("rate: filed %v, read back %v", d.Contract.NominalRate, c.NominalRate)
	}
	if one.AsOf != d.AsOf {
		t.Errorf("as_of: filed %v, read back %v — a zero AsOf re-accrues from the start date", d.AsOf, one.AsOf)
	}
	if one.Balance.Minor() != d.Balance.Minor() {
		t.Errorf("balance: filed %d, read back %d", d.Balance.Minor(), one.Balance.Minor())
	}
	if one.Trust != "user_entered" {
		t.Errorf("trust: expected user_entered, got %q", one.Trust)
	}
}

func TestLoanOwnershipAndArchive(t *testing.T) {
	s := testStore(t)
	ctx := context.Background()
	owner := newUser(t, s)
	stranger := newUser(t, s)

	loanID, err := s.CreateLoan(ctx, draft(owner, t))
	if err != nil {
		t.Fatalf("creating the loan: %v", err)
	}

	if _, err := s.LoanForUser(ctx, loanID, stranger); !errors.Is(err, app.ErrNotFound) {
		t.Errorf("stranger reading the loan: want ErrNotFound, got %v", err)
	}
	if err := s.UpdateLoan(ctx, loanID, stranger, "mine now", ""); !errors.Is(err, app.ErrNotFound) {
		t.Errorf("stranger renaming the loan: want ErrNotFound, got %v", err)
	}
	if err := s.ArchiveLoan(ctx, loanID, stranger); !errors.Is(err, app.ErrNotFound) {
		t.Errorf("stranger archiving the loan: want ErrNotFound, got %v", err)
	}

	if err := s.UpdateLoan(ctx, loanID, owner, "Renamed", "still the blue one"); err != nil {
		t.Fatalf("renaming: %v", err)
	}
	one, err := s.LoanForUser(ctx, loanID, owner)
	if err != nil {
		t.Fatalf("re-reading: %v", err)
	}
	if one.Name != "Renamed" || one.Description != "still the blue one" {
		t.Errorf("rename did not stick: %q / %q", one.Name, one.Description)
	}

	if err := s.ArchiveLoan(ctx, loanID, owner); err != nil {
		t.Fatalf("archiving: %v", err)
	}
	if _, err := s.LoanForUser(ctx, loanID, owner); !errors.Is(err, app.ErrNotFound) {
		t.Errorf("archived loan still readable: %v", err)
	}
	list, err := s.LoansForUser(ctx, owner, 10)
	if err != nil {
		t.Fatalf("listing: %v", err)
	}
	if len(list) != 0 {
		t.Errorf("archived loan still listed: %d rows", len(list))
	}
}

// TestBudgetPrefersTheLatestStatement covers the cross-currency fix: ordering
// by amount picked whichever currency had the numerically bigger unit.
func TestBudgetPrefersTheLatestStatement(t *testing.T) {
	s := testStore(t)
	ctx := context.Background()
	userID := newUser(t, s)

	b, err := s.Budget(ctx, userID)
	if err != nil || b.Set {
		t.Fatalf("budget before setting one: set=%v err=%v", b.Set, err)
	}

	// A huge dram budget first, then a numerically tiny dollar one. The dollar
	// budget is the one stated last, so it is the one meant.
	if err := s.SetBudget(ctx, userID, "AMD", 500_000_00, 5); err != nil {
		t.Fatalf("setting the AMD budget: %v", err)
	}
	if err := s.SetBudget(ctx, userID, "USD", 300_00, 0); err != nil {
		t.Fatalf("setting the USD budget: %v", err)
	}

	b, err = s.Budget(ctx, userID)
	if err != nil {
		t.Fatalf("reading the budget: %v", err)
	}
	if !b.Set || b.Currency != "USD" || b.Monthly.Minor() != 300_00 {
		t.Errorf("want the USD budget stated last, got %+v", b)
	}

	// Pay day zero means "not stated" and must keep what was stored.
	if err := s.SetBudget(ctx, userID, "AMD", 600_000_00, 0); err != nil {
		t.Fatalf("re-stating the AMD budget: %v", err)
	}
	b, err = s.Budget(ctx, userID)
	if err != nil {
		t.Fatalf("re-reading: %v", err)
	}
	if b.Currency != "AMD" || b.PayDay != 5 {
		t.Errorf("want AMD with the remembered pay day 5, got %+v", b)
	}
}

// TestBudgetTuning: cash on hand and the per-month document round-trip, and
// the whole-document replace really replaces.
func TestBudgetTuning(t *testing.T) {
	s := testStore(t)
	ctx := context.Background()
	userID := newUser(t, s)

	if err := s.SetBudget(ctx, userID, "AMD", 300_000_00, 5); err != nil {
		t.Fatalf("setting the budget: %v", err)
	}
	if err := s.SetOpening(ctx, userID, "AMD", 120_000_00, "2026-09-01"); err != nil {
		t.Fatalf("stating cash on hand: %v", err)
	}
	if err := s.SetOverrides(ctx, userID, "AMD", map[string]int64{
		"2026-12": 400_000_00, "2027-01": 150_000_00,
	}); err != nil {
		t.Fatalf("stating month budgets: %v", err)
	}

	b, err := s.Budget(ctx, userID)
	if err != nil {
		t.Fatalf("reading the budget: %v", err)
	}
	if b.Opening.Minor() != 120_000_00 || b.OpeningAsOf.String() != "2026-09-01" {
		t.Errorf("opening lost: %v as of %v", b.Opening, b.OpeningAsOf)
	}
	if len(b.Overrides) != 2 || b.Overrides["2026-12"] != 400_000_00 || b.Overrides["2027-01"] != 150_000_00 {
		t.Errorf("overrides lost: %+v", b.Overrides)
	}

	// Replacement replaces: a removed month must not survive the write.
	if err := s.SetOverrides(ctx, userID, "AMD", map[string]int64{"2026-12": 350_000_00}); err != nil {
		t.Fatalf("replacing month budgets: %v", err)
	}
	b, err = s.Budget(ctx, userID)
	if err != nil {
		t.Fatalf("re-reading: %v", err)
	}
	if len(b.Overrides) != 1 || b.Overrides["2026-12"] != 350_000_00 {
		t.Errorf("replacement merged instead of replacing: %+v", b.Overrides)
	}

	// Tuning a currency with no budget row is a typed miss, not a silent noop.
	if err := s.SetOpening(ctx, userID, "USD", 1, "2026-09-01"); !errors.Is(err, app.ErrNotFound) {
		t.Errorf("opening without a budget row: want ErrNotFound, got %v", err)
	}
}

func TestBudgetConfigurationReplacesTheWholeForm(t *testing.T) {
	s := testStore(t)
	ctx := context.Background()
	userID := newUser(t, s)

	first := app.BudgetConfiguration{
		UserID: userID, Currency: "AMD", MonthlyMinor: 300_000_00, PayDay: 5,
		OpeningMinor: 120_000_00, OpeningAsOf: mustDate(t, "2026-09-01"),
		ReserveMinor: 50_000_00,
		Overrides:    map[string]int64{"2026-12": 400_000_00},
	}
	if err := s.SetBudgetConfiguration(ctx, first); err != nil {
		t.Fatalf("setting complete budget: %v", err)
	}

	second := app.BudgetConfiguration{
		UserID: userID, Currency: "AMD", MonthlyMinor: 325_000_00,
		OpeningAsOf: mustDate(t, "2026-09-02"),
	}
	if err := s.SetBudgetConfiguration(ctx, second); err != nil {
		t.Fatalf("replacing complete budget: %v", err)
	}
	b, err := s.Budget(ctx, userID)
	if err != nil {
		t.Fatal(err)
	}
	if b.Monthly.Minor() != second.MonthlyMinor || b.PayDay != 0 {
		t.Errorf("monthly/payday were not replaced: %+v", b)
	}
	if b.Opening.Sign() != 0 || b.Reserve.Sign() != 0 || len(b.Overrides) != 0 {
		t.Errorf("removed optional values survived: %+v", b)
	}
}

// TestUpsertFirstContactRace: ten concurrent first contacts for the same
// identity must resolve to one account, with no unique-violation errors.
func TestUpsertFirstContactRace(t *testing.T) {
	s := testStore(t)
	ctx := context.Background()
	in := freshUpsert()

	const workers = 10
	ids := make([]string, workers)
	errs := make([]error, workers)
	var wg sync.WaitGroup
	for i := range workers {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			attempt := in
			attempt.NewID = uuid.NewString()
			a, err := s.UpsertByTelegram(ctx, attempt)
			ids[i], errs[i] = a.ID, err
		}(i)
	}
	wg.Wait()

	settled := ""
	for i := range workers {
		if errs[i] != nil {
			t.Fatalf("worker %d: %v", i, errs[i])
		}
		if ids[i] == "" {
			t.Fatalf("worker %d resolved to an empty account", i)
		}
		if settled == "" {
			settled = ids[i]
		} else if ids[i] != settled {
			t.Fatalf("split brain: %s and %s for the same identity", settled, ids[i])
		}
	}

	// The account keeps its own preferences on repeat contact.
	if err := s.SetLocale(ctx, settled, "en"); err != nil {
		t.Fatalf("setting locale: %v", err)
	}
	again := in
	again.NewID = uuid.NewString()
	a, err := s.UpsertByTelegram(ctx, again)
	if err != nil {
		t.Fatalf("repeat contact: %v", err)
	}
	if a.Created || a.ID != settled || a.Locale != "en" {
		t.Errorf("repeat contact: created=%v id=%s locale=%s, want existing %s with locale en",
			a.Created, a.ID, a.Locale, settled)
	}
}

func TestInboxLifecycle(t *testing.T) {
	s := testStore(t)
	ctx := context.Background()
	userID := newUser(t, s)

	cmd := app.InboundCommand{
		ID:       uuid.NewString(),
		UpdateID: randUpdateID(t),
		UserID:   userID,
		Kind:     "help",
		Payload:  []byte(`{}`),
	}
	accepted, err := s.Enqueue(ctx, cmd)
	if err != nil || !accepted {
		t.Fatalf("first enqueue: accepted=%v err=%v", accepted, err)
	}
	dup := cmd
	dup.ID = uuid.NewString()
	accepted, err = s.Enqueue(ctx, dup)
	if err != nil {
		t.Fatalf("duplicate enqueue: %v", err)
	}
	if accepted {
		t.Fatal("the same update id was accepted twice")
	}

	until := clock.Now().Add(2 * time.Minute)
	l, ok, err := s.LeaseByID(ctx, cmd.ID, "it-worker", until)
	if err != nil || !ok {
		t.Fatalf("leasing: ok=%v err=%v", ok, err)
	}
	if l.Command.Kind != "help" || l.Command.UserID != userID || l.Token == "" {
		t.Fatalf("lease came back wrong: %+v", l)
	}

	// Second claim while leased is an ordinary race, not an error.
	if _, ok, err := s.LeaseByID(ctx, cmd.ID, "other-worker", until); err != nil || ok {
		t.Fatalf("double lease: ok=%v err=%v", ok, err)
	}

	// The fencing token is the lease. A wrong token must not complete.
	if err := s.Complete(ctx, cmd.ID, uuid.NewString()); !errors.Is(err, app.ErrNotLeased) {
		t.Fatalf("completing with a foreign token: want ErrNotLeased, got %v", err)
	}
	if err := s.Complete(ctx, cmd.ID, l.Token); err != nil {
		t.Fatalf("completing: %v", err)
	}

	// Completed rows older than the cutoff are purged; younger ones stay.
	if _, err := s.PurgeCompletedBefore(ctx, clock.Now().Add(-time.Hour)); err != nil {
		t.Fatalf("purging with an old cutoff: %v", err)
	}
	if _, ok, _ := s.LeaseByID(ctx, cmd.ID, "it-worker", until); ok {
		t.Fatal("completed command became leasable again")
	}
	if _, err := s.PurgeCompletedBefore(ctx, clock.Now().Add(time.Hour)); err != nil {
		t.Fatalf("purging: %v", err)
	}
}

func TestInboxFailSchedulesRetry(t *testing.T) {
	s := testStore(t)
	ctx := context.Background()
	userID := newUser(t, s)

	cmd := app.InboundCommand{
		ID:       uuid.NewString(),
		UpdateID: randUpdateID(t),
		UserID:   userID,
		Kind:     "loans",
		Payload:  []byte(`{}`),
	}
	if _, err := s.Enqueue(ctx, cmd); err != nil {
		t.Fatalf("enqueue: %v", err)
	}
	l, ok, err := s.LeaseByID(ctx, cmd.ID, "it-worker", clock.Now().Add(time.Minute))
	if err != nil || !ok {
		t.Fatalf("leasing: ok=%v err=%v", ok, err)
	}

	// Fail into the future: not leasable until the retry moment arrives.
	if err := s.Fail(ctx, cmd.ID, l.Token, "scan_error", clock.Now().Add(time.Hour), false); err != nil {
		t.Fatalf("failing: %v", err)
	}
	if _, ok, _ := s.LeaseByID(ctx, cmd.ID, "it-worker", clock.Now().Add(time.Minute)); ok {
		t.Fatal("command leasable before its retry time")
	}

	// A dead command leaves the queue for good.
	cmd2 := app.InboundCommand{
		ID: uuid.NewString(), UpdateID: randUpdateID(t), UserID: userID,
		Kind: "loans", Payload: []byte(`{}`),
	}
	if _, err := s.Enqueue(ctx, cmd2); err != nil {
		t.Fatalf("enqueue: %v", err)
	}
	l2, ok, err := s.LeaseByID(ctx, cmd2.ID, "it-worker", clock.Now().Add(time.Minute))
	if err != nil || !ok {
		t.Fatalf("leasing: ok=%v err=%v", ok, err)
	}
	if err := s.Fail(ctx, cmd2.ID, l2.Token, "gave_up", clock.Now().Add(-time.Second), true); err != nil {
		t.Fatalf("failing dead: %v", err)
	}
	if _, ok, _ := s.LeaseByID(ctx, cmd2.ID, "it-worker", clock.Now().Add(time.Minute)); ok {
		t.Fatal("dead command still leasable")
	}
}

func TestRemindersScheduleOnce(t *testing.T) {
	s := testStore(t)
	ctx := context.Background()
	userID := newUser(t, s)
	loanID, err := s.CreateLoan(ctx, draft(userID, t))
	if err != nil {
		t.Fatalf("creating the loan: %v", err)
	}

	if err := s.EnsureDefaultReminders(ctx, loanID); err != nil {
		t.Fatalf("default reminders: %v", err)
	}
	if err := s.EnsureDefaultReminders(ctx, loanID); err != nil {
		t.Fatalf("default reminders, second run: %v", err)
	}

	// A due date already in the past: both offsets (-3, 0) are due at once,
	// and a double schedule must still produce exactly one pair.
	due := clock.Now().Add(-24 * time.Hour)
	for range 2 {
		if err := s.ScheduleReminders(ctx, due, loanID); err != nil {
			t.Fatalf("scheduling: %v", err)
		}
	}

	all, err := s.DueReminders(ctx, 500)
	if err != nil {
		t.Fatalf("reading due reminders: %v", err)
	}
	var mine []app.DueReminder
	for _, d := range all {
		if d.LoanID == loanID {
			mine = append(mine, d)
		}
	}
	if len(mine) != 2 {
		t.Fatalf("want the -3 and 0 offsets once each, got %d occurrences", len(mine))
	}
	for _, d := range mine {
		if d.UserID != userID || d.Currency != "AMD" {
			t.Errorf("occurrence mapped wrong: %+v", d)
		}
		if err := s.MarkReminderSatisfied(ctx, d.ID); err != nil {
			t.Errorf("satisfying %s: %v", d.ID, err)
		}
	}

	// Archived loans keep their occurrences out of the due list.
	if err := s.ScheduleReminders(ctx, due.Add(24*time.Hour), loanID); err != nil {
		t.Fatalf("rescheduling: %v", err)
	}
	if err := s.CancelRemindersForLoan(ctx, loanID); err != nil {
		t.Fatalf("cancelling: %v", err)
	}
	all, err = s.DueReminders(ctx, 500)
	if err != nil {
		t.Fatalf("re-reading due reminders: %v", err)
	}
	for _, d := range all {
		if d.LoanID == loanID {
			t.Errorf("cancelled occurrence still due: %+v", d)
		}
	}
}

func TestActiveLoanUsers(t *testing.T) {
	s := testStore(t)
	ctx := context.Background()
	userID := newUser(t, s)
	if _, err := s.CreateLoan(ctx, draft(userID, t)); err != nil {
		t.Fatalf("creating the loan: %v", err)
	}
	ids, err := s.ActiveLoanUsers(ctx, 10_000)
	if err != nil {
		t.Fatalf("listing active users: %v", err)
	}
	found := false
	for _, id := range ids {
		if id == userID {
			found = true
		}
	}
	if !found {
		t.Error("a user with a live loan is missing from the reminder walk")
	}
}

func TestApprovedPlanRoundTrip(t *testing.T) {
	s := testStore(t)
	ctx := context.Background()
	userID := newUser(t, s)

	got, err := s.ApprovedPlan(ctx, userID)
	if err != nil || got != nil {
		t.Fatalf("plan before approving one: %+v err=%v", got, err)
	}

	p := app.ApprovedPlan{
		Goal: "least_interest", CapMinor: 250_000_00, Policy: "am-consumer-credit-prepayment",
		Engine: "plan/2", PayoffDate: "2027-06-15", Months: 10, InterestMinor: 84_210_00,
	}
	if err := s.ApprovePlan(ctx, userID, p); err != nil {
		t.Fatalf("approving: %v", err)
	}
	got, err = s.ApprovedPlan(ctx, userID)
	if err != nil || got == nil {
		t.Fatalf("reading the plan back: %+v err=%v", got, err)
	}
	if got.Goal != p.Goal || got.CapMinor != p.CapMinor || got.PayoffDate != p.PayoffDate ||
		got.Months != p.Months || got.InterestMinor != p.InterestMinor {
		t.Errorf("plan came back different: %+v vs %+v", *got, p)
	}

	// Approving again replaces, never accumulates.
	p2 := p
	p2.Goal, p2.Months = "first_win", 12
	if err := s.ApprovePlan(ctx, userID, p2); err != nil {
		t.Fatalf("re-approving: %v", err)
	}
	got, err = s.ApprovedPlan(ctx, userID)
	if err != nil || got == nil || got.Goal != "first_win" || got.Months != 12 {
		t.Fatalf("replacement did not stick: %+v err=%v", got, err)
	}

	if err := s.ClearApprovedPlan(ctx, userID); err != nil {
		t.Fatalf("clearing: %v", err)
	}
	got, err = s.ApprovedPlan(ctx, userID)
	if err != nil || got != nil {
		t.Fatalf("plan survived clearing: %+v err=%v", got, err)
	}
	// Clearing twice is a no-op, not an error.
	if err := s.ClearApprovedPlan(ctx, userID); err != nil {
		t.Fatalf("clearing twice: %v", err)
	}
}

func TestConversationState(t *testing.T) {
	s := testStore(t)
	ctx := context.Background()
	userID := newUser(t, s)

	state, err := s.State(ctx, userID)
	if err != nil || state != "" {
		t.Fatalf("state before setting one: %q err=%v", state, err)
	}
	if err := s.SetState(ctx, userID, "awaiting_budget"); err != nil {
		t.Fatalf("setting: %v", err)
	}
	state, err = s.State(ctx, userID)
	if err != nil || state != "awaiting_budget" {
		t.Fatalf("reading back: %q err=%v", state, err)
	}
	if err := s.ClearState(ctx, userID); err != nil {
		t.Fatalf("clearing: %v", err)
	}
	state, err = s.State(ctx, userID)
	if err != nil || state != "" {
		t.Fatalf("state survived clearing: %q err=%v", state, err)
	}
}

func TestTooManyLoans(t *testing.T) {
	s := testStore(t)
	ctx := context.Background()
	userID := newUser(t, s)

	// The cap lives in the CreateLoan statement itself ($19 = plan.MaxLoans).
	// Filing up to the cap succeeds; one more is refused with the typed error.
	for i := 0; i < plan.MaxLoans; i++ {
		d := draft(userID, t)
		d.Title = fmt.Sprintf("Loan %d", i+1)
		if _, err := s.CreateLoan(ctx, d); err != nil {
			t.Fatalf("loan %d within the cap: %v", i+1, err)
		}
	}
	if _, err := s.CreateLoan(ctx, draft(userID, t)); !errors.Is(err, app.ErrTooManyLoans) {
		t.Fatalf("beyond the cap: want ErrTooManyLoans, got %v", err)
	}
}

func TestVersionedBudgetRejectsStaleForm(t *testing.T) {
	s := testStore(t)
	ctx := context.Background()
	user := newUser(t, s)
	zero := int64(0)
	cfg := app.BudgetConfiguration{UserID: user, Currency: "AMD", MonthlyMinor: 20000, PayDay: 1, OpeningAsOf: mustDate(t, "2026-09-02"), ExpectedVersion: &zero, Funding: &app.BudgetFunding{MonthlyMinor: 50000, SpentMinor: 1000, Events: []app.BudgetCashEvent{{On: "2026-10-01", Minor: 20000, Expected: true}}}}
	if err := s.SetBudgetConfiguration(ctx, cfg); err != nil {
		t.Fatal(err)
	}
	first, err := s.Budget(ctx, user)
	if err != nil {
		t.Fatal(err)
	}
	if first.Version != 1 || first.Funding == nil || first.Funding.Events[0].Minor != 20000 {
		t.Fatalf("funding lost: %+v", first)
	}
	cfg.ExpectedVersion = &first.Version
	cfg.MonthlyMinor = 30000
	if err := s.SetBudgetConfiguration(ctx, cfg); err != nil {
		t.Fatal(err)
	}
	cfg.MonthlyMinor = 40000
	if err := s.SetBudgetConfiguration(ctx, cfg); !errors.Is(err, app.ErrConflict) {
		t.Fatalf("stale edit accepted: %v", err)
	}
	latest, err := s.Budget(ctx, user)
	if err != nil {
		t.Fatal(err)
	}
	if latest.Version != 2 || latest.Monthly.Minor() != 30000 {
		t.Fatalf("stale edit changed budget: %+v", latest)
	}
}

func TestIconsAndSnapshotHistory(t *testing.T) {
	s := testStore(t)
	ctx := context.Background()
	user := newUser(t, s)
	d := draft(user, t)
	id, err := s.CreateLoan(ctx, d)
	if err != nil {
		t.Fatal(err)
	}
	before, err := s.LoanForUser(ctx, id, user)
	if err != nil {
		t.Fatal(err)
	}
	if before.Icon != "bank" {
		t.Fatal("default icon isn't bank")
	}
	icon := "car"
	excluded := true
	if err := s.ApplyLoanRevision(ctx, id, user, app.LoanRevision{Icon: &icon, OptionalExcluded: &excluded, EffectiveFrom: d.AsOf}); err != nil {
		t.Fatal(err)
	}
	after, err := s.LoanForUser(ctx, id, user)
	if err != nil {
		t.Fatal(err)
	}
	if after.Icon != "car" || !after.OptionalExcluded {
		t.Fatal("loan choices not persisted")
	}
	for _, amount := range []int64{10000, 9000} {
		if err := s.RecordBalance(ctx, id, user, amount, d.AsOf.String()); err != nil {
			t.Fatal(err)
		}
	}
	facts, err := s.BorrowerActivity(ctx, user)
	if err != nil {
		t.Fatal(err)
	}
	if len(facts) != 3 {
		t.Fatalf("same-day history overwritten: %d", len(facts))
	}
	stranger := newUser(t, s)
	facts, err = s.BorrowerActivity(ctx, stranger)
	if err != nil {
		t.Fatal(err)
	}
	if len(facts) != 0 {
		t.Fatal("activity leaked across accounts")
	}
}
