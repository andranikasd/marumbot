package admin

import (
	"context"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/andranikasd/marumbot/internal/app"
	"github.com/andranikasd/marumbot/pkg/core/allocation"
	"github.com/andranikasd/marumbot/pkg/core/date"
	"github.com/andranikasd/marumbot/pkg/core/model"
	"github.com/andranikasd/marumbot/pkg/core/money"
)

// Every page is rendered against a populated fake store. A template that
// references a field that does not exist, or calls a helper with the wrong
// type, fails here rather than on the first click in production.

const (
	fakeUser = "11111111-1111-4111-8111-111111111111"
	fakeLoan = "22222222-2222-4222-8222-222222222222"
)

// stamp is the fixed instant every fake row carries; the test clock is
// injected, so nothing here reads the wall.
var stamp = time.Date(2026, 8, 28, 12, 0, 0, 0, time.UTC)

type fakeStore struct{}

func ptr[T any](v T) *T { return &v }

func (fakeStore) Overview(context.Context) (app.Overview, error) {
	return app.Overview{Users: 3, Loans: 2, CommandsPending: 1, CommandsDead: 2, DeliveriesPending: 4}, nil
}

func (fakeStore) ListUsers(context.Context, int32) ([]app.UserRow, error) {
	return []app.UserRow{{
		ID: fakeUser, Locale: "hy", Timezone: "Asia/Yerevan", AccessState: "trial",
		TrialEndsAt: stamp.Add(72 * time.Hour), CreatedAt: stamp, LoanCount: 1,
	}}, nil
}

func (f fakeStore) ListLoans(context.Context, int32) ([]app.LoanRow, error) {
	return f.LoansByUser(context.Background(), fakeUser)
}

func (fakeStore) GetLoan(context.Context, string) (app.LoanDetail, error) {
	return app.LoanDetail{ID: fakeLoan, UserID: fakeUser, Name: "Car", Description: "Ineco", Currency: "AMD", CreatedAt: stamp}, nil
}

func (fakeStore) ContractsForLoan(context.Context, string) ([]app.ContractRow, error) {
	return []app.ContractRow{{
		ID: "c1", Version: 1, EffectiveFrom: stamp, NominalRate: 0.18, DayCount: "act365",
		RepaymentType: "annuity", StartDate: stamp, MaturityDate: stamp.AddDate(3, 0, 0), PaymentDay: 15,
		RoundingMode: "half_up", RoundingUnit: 10, PolicyVersionID: "00000000-0000-4000-8000-000000002015",
	}}, nil
}

func (fakeStore) SnapshotsForLoan(context.Context, string) ([]app.SnapshotRow, error) {
	return []app.SnapshotRow{{ID: "s1", AsOf: stamp, CapturedAt: stamp, Trust: "user_entered", PrincipalMinor: 300000000}}, nil
}

func (fakeStore) EventsForLoan(context.Context, string) ([]app.EventRow, error) {
	return []app.EventRow{{
		ID: "e1", RecordedSeq: 1, Kind: "payment", ValueDate: stamp, RecordedAt: stamp,
		AmountMinor: ptr(int64(1000000)), Covered: true,
	}}, nil
}

func (fakeStore) ListPolicies(context.Context) ([]app.PolicyRow, error) {
	return []app.PolicyRow{{
		ID: "p1", Key: "am-consumer-credit-prepayment", Version: 1, ExcessRule: "reduce_principal",
		Definition: []byte(`{"order":["interest","principal"]}`), Source: "https://www.cba.am/", CreatedAt: stamp,
	}}, nil
}

func (fakeStore) InsertPolicy(context.Context, string, string, int32, []byte, string, string) error {
	return nil
}

func (fakeStore) ListCommands(context.Context, int32) ([]app.CommandRow, error) { return nil, nil }

func (fakeStore) ListDeliveries(context.Context, int32) ([]app.DeliveryRow, error) {
	return []app.DeliveryRow{{ID: "d1", UserID: fakeUser, Kind: "reminder", Status: "pending", ScheduledAt: stamp, NextAttempt: stamp}}, nil
}

func (fakeStore) ListReconciliationRuns(context.Context, int32) ([]app.ReconRow, error) {
	return []app.ReconRow{{ID: "r1", LoanID: fakeLoan, PrincipalMinor: -10, EngineVersion: "v0.2.2", CreatedAt: stamp}}, nil
}

func (fakeStore) Ping(context.Context) error                      { return nil }
func (fakeStore) MigrationVersion(context.Context) (int64, error) { return 6, nil }

func (fakeStore) UsersByDay(context.Context) ([]app.DayCount, error) {
	return []app.DayCount{{Day: stamp, N: 2}}, nil
}

func (fakeStore) LoansByDay(context.Context) ([]app.DayCount, error) {
	return []app.DayCount{{Day: stamp.AddDate(0, 0, -1), N: 1}}, nil
}

func (f fakeStore) GetUser(ctx context.Context, id string) (app.UserRow, error) {
	if id != fakeUser {
		return app.UserRow{}, app.ErrNotFound
	}
	us, _ := f.ListUsers(ctx, 1)
	return us[0], nil
}

func (fakeStore) LoansByUser(context.Context, string) ([]app.LoanRow, error) {
	return []app.LoanRow{{
		ID: fakeLoan, UserID: fakeUser, Name: "Car", Currency: "AMD", CreatedAt: stamp,
		Reliability: ptr("confirmed"), PrincipalMinor: ptr(int64(300000000)), BalanceAsOf: ptr(stamp),
	}}, nil
}

func (fakeStore) BudgetsForUser(context.Context, string) ([]app.BudgetRow, error) {
	return []app.BudgetRow{{Currency: "AMD", MonthlyMinor: 20000000, PayDay: 5, UpdatedAt: stamp}}, nil
}

func (fakeStore) ConversationState(context.Context, string) (*app.ConvoRow, error) {
	return &app.ConvoRow{State: "awaiting_budget", UpdatedAt: stamp}, nil
}

func (fakeStore) CommandCounts(context.Context) ([]app.StatusCount, error) {
	return []app.StatusCount{{Status: "pending", N: 1}, {Status: "dead", N: 2}}, nil
}

func (fakeStore) DeliveryCounts(context.Context) ([]app.StatusCount, error) {
	return []app.StatusCount{{Status: "pending", N: 4}}, nil
}

func (fakeStore) CommandsForUser(context.Context, string, int32) ([]app.CommandRow, error) {
	return []app.CommandRow{{
		ID: "c9", UpdateID: 77, UserID: ptr(fakeUser), Kind: "callback", Status: "completed", Attempts: 1,
		NextAttempt: stamp, ReceivedAt: stamp.Add(-3 * time.Minute), CompletedAt: ptr(stamp),
	}}, nil
}

func (f fakeStore) DeliveriesForUser(ctx context.Context, _ string, n int32) ([]app.DeliveryRow, error) {
	return f.ListDeliveries(ctx, n)
}

// Moderation.
func (fakeStore) SetUserAccess(context.Context, string, string) error           { return nil }
func (fakeStore) RequestUserDeletion(context.Context, string) error             { return nil }
func (fakeStore) DeleteUser(context.Context, string) error                      { return nil }
func (fakeStore) ArchiveLoanAdmin(context.Context, string) error                { return nil }
func (fakeStore) RestoreLoan(context.Context, string) error                     { return nil }
func (fakeStore) RenameLoanAdmin(context.Context, string, string, string) error { return nil }
func (fakeStore) RetryCommand(context.Context, string) error                    { return nil }
func (fakeStore) PurgeDeadCommands(context.Context) (int64, error)              { return 2, nil }

func (fakeStore) CommandsDetailed(context.Context, string, int32) ([]app.CommandDetail, error) {
	return []app.CommandDetail{{
		ID: "c1", UpdateID: 42, UserID: fakeUser, Kind: "text", Status: "dead", Attempts: 5,
		ReceivedAt: stamp, NextAttemptAt: stamp, LastError: "boom", DueAgeS: 90,
	}}, nil
}

// Engine.
func (fakeStore) LoansForUser(context.Context, string, int32) ([]app.UserLoan, error) {
	amd, _ := money.Lookup("AMD")
	start := date.MustNew(2026, 1, 15)
	return []app.UserLoan{{
		ID: fakeLoan, Name: "Car",
		Contract: model.Contract{
			LoanID: model.ID(fakeLoan), Version: 1, Currency: amd, EffectiveFrom: start,
			NominalRate: money.RateFromPercent(18, 0), DayCount: money.Actual365, Type: model.Annuity,
			StartDate: start, MaturityDate: date.MustNew(2029, 1, 15), PaymentDay: 15, Rounding: money.DefaultPolicy(amd),
		},
		Balance: money.FromMinor(300000000, amd), AsOf: start, Trust: "user_entered",
		Excess: allocation.ExcessReducePrincipal,
	}}, nil
}

func (fakeStore) Budget(context.Context, string) (app.Budget, error) {
	amd, _ := money.Lookup("AMD")
	return app.Budget{Currency: "AMD", Monthly: money.FromMinor(20000000, amd), PayDay: 5, Set: true}, nil
}

func newTestServer(t *testing.T) (*httptest.Server, *http.Cookie) {
	t.Helper()
	hash, err := HashPassword("correct-horse-battery")
	if err != nil {
		t.Fatal(err)
	}
	now := func() time.Time { return stamp }
	svc := app.NewAdmin(fakeStore{}).WithModeration(fakeStore{}).WithEngine(fakeStore{})
	s, err := New(svc, Config{User: "op", PasswordHash: hash, Version: "test", Env: "test", Now: now}, slog.New(slog.NewTextHandler(io.Discard, nil)))
	if err != nil {
		t.Fatal(err)
	}
	ts := httptest.NewServer(s.Handler())
	t.Cleanup(ts.Close)
	return ts, &http.Cookie{Name: cookieName, Value: issue(s.key, now())}
}

func TestEveryPageRenders(t *testing.T) {
	ts, cookie := newTestServer(t)
	pages := []struct{ path, want string }{
		{"/", "Sign-ups, last 14 days"},
		{"/loans", "of 1 loans"},
		{"/loans?q=car&currency=AMD&reliability=confirmed", "Car"},
		{"/loans/" + fakeLoan, "What the engine projects"},
		{"/users", "of 1 users"},
		{"/users/" + fakeUser, "Current advice"},
		{"/users/" + fakeUser, "Sent to the bot"},
		{"/users/" + fakeUser, "3m ago"},
		{"/loans/" + fakeLoan, "Copy for support"},
		{"/loans/" + fakeLoan, "Next instalments:"},
		{"/", "aria-current=\"page\" href=\"/\""},
		{"/", "Schema <b class=\"mono\">v6</b>"},
		{"/", "class=\"b today\""},
		{"/search", "Overview"},
		{"/engine", "instalment"},
		{"/search?q=car", "Car"},
		{"/policies", "reduce_principal"},
		{"/commands", "pending"},
		{"/commands?status=dead", "boom"},
		{"/deliveries?status=pending", "reminder"},
		{"/reconciliation", "v0.2.2"},
	}
	client := &http.Client{CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	for _, p := range pages {
		req, _ := http.NewRequestWithContext(context.Background(), http.MethodGet, ts.URL+p.path, nil)
		req.AddCookie(cookie)
		res, err := client.Do(req)
		if err != nil {
			t.Fatalf("%s: %v", p.path, err)
		}
		body, _ := io.ReadAll(res.Body)
		_ = res.Body.Close()
		if res.StatusCode != http.StatusOK {
			t.Errorf("%s: status %d\n%s", p.path, res.StatusCode, body)
			continue
		}
		if !strings.Contains(string(body), p.want) {
			t.Errorf("%s: body lacks %q", p.path, p.want)
		}
		if strings.Contains(string(body), "could not be rendered") {
			t.Errorf("%s: template error", p.path)
		}
	}
}

// The loan page must show the engine's schedule and the borrower's plan,
// computed from the same contract the bot reads.
func TestLoanPageShowsProjectionAndPlan(t *testing.T) {
	ts, cookie := newTestServer(t)
	req, _ := http.NewRequestWithContext(context.Background(), http.MethodGet, ts.URL+"/loans/"+fakeLoan, nil)
	req.AddCookie(cookie)
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	body, _ := io.ReadAll(res.Body)
	_ = res.Body.Close()
	for _, want := range []string{"Instalment", "Interest to the end", "Best policy", "avalanche/on_receipt", "First month", "saves"} {
		if !strings.Contains(string(body), want) {
			t.Errorf("loan page lacks %q", want)
		}
	}
}

func TestPlaygroundProjectsTypedTerms(t *testing.T) {
	ts, cookie := newTestServer(t)
	form := "currency=AMD&principal=1000000&rate=20&method=annuity&day_count=act365&start=2026-01-15&maturity=2027-01-15&day=15&unit=10"
	req, _ := http.NewRequestWithContext(context.Background(), http.MethodPost, ts.URL+"/engine", strings.NewReader(form))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.AddCookie(cookie)
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	body, _ := io.ReadAll(res.Body)
	_ = res.Body.Close()
	if res.StatusCode != http.StatusOK || !strings.Contains(string(body), "total interest") || strings.Contains(string(body), "note bad") {
		t.Fatalf("playground did not project: %d\n%s", res.StatusCode, body)
	}
	bad := strings.Replace(form, "rate=20", "rate=abc", 1)
	req, _ = http.NewRequestWithContext(context.Background(), http.MethodPost, ts.URL+"/engine", strings.NewReader(bad))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.AddCookie(cookie)
	res, _ = http.DefaultClient.Do(req)
	body, _ = io.ReadAll(res.Body)
	_ = res.Body.Close()
	if !strings.Contains(string(body), "rate must be a percentage") {
		t.Fatal("bad rate was not reported")
	}
}

// postForm submits a form as the signed-in operator and returns the page.
func postForm(t *testing.T, ts *httptest.Server, cookie *http.Cookie, path, form string) (int, string) {
	t.Helper()
	req, _ := http.NewRequestWithContext(context.Background(), http.MethodPost, ts.URL+path, strings.NewReader(form))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.AddCookie(cookie)
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	body, _ := io.ReadAll(res.Body)
	_ = res.Body.Close()
	return res.StatusCode, string(body)
}

// A pasted bank schedule is compared row by row: an exact row says so, a
// different one shows the signed gap, and the count of exact rows is shown.
func TestPlaygroundDiffsAgainstBank(t *testing.T) {
	ts, cookie := newTestServer(t)
	form := "currency=AMD&principal=1000000&rate=20&method=annuity&day_count=act365&start=2026-01-15&maturity=2027-01-15&day=15&unit=10"
	_, body := postForm(t, ts, cookie, "/engine", form)
	// Read the engine's first instalment back off the page so the test does
	// not pin the arithmetic, only the comparison.
	i := strings.Index(body, `<div class="n">`)
	if i < 0 {
		t.Fatal("no instalment card")
	}
	first, _, _ := strings.Cut(body[i+len(`<div class="n">`):], "</div>")
	first = strings.TrimSuffix(first, " AMD")

	bank := url.QueryEscape(first + "\n\n" + first + "\n1")
	_, body = postForm(t, ts, cookie, "/engine", form+"&bank="+bank)
	for _, want := range []string{"2 / 3", "rows match the bank", ">exact<", "diff off"} {
		if !strings.Contains(body, want) {
			t.Errorf("diff page lacks %q", want)
		}
	}
	_, body = postForm(t, ts, cookie, "/engine", form+"&bank="+url.QueryEscape("abc"))
	if !strings.Contains(body, "line 1:") || !strings.Contains(body, `aria-invalid="true"`) {
		t.Error("a bad bank line was not reported beside the field")
	}
}

// A rename with no name comes back as the same page with the complaint beside
// the field, not an error page.
func TestRenameValidatesInline(t *testing.T) {
	ts, cookie := newTestServer(t)
	code, body := postForm(t, ts, cookie, "/loans/"+fakeLoan+"/rename", "name=&description=kept")
	if code != http.StatusOK || !strings.Contains(body, "A loan needs a name.") || !strings.Contains(body, `value="kept"`) {
		t.Fatalf("rename validation: %d\n%s", code, body)
	}
}

func TestUnauthenticatedIsRedirected(t *testing.T) {
	ts, _ := newTestServer(t)
	client := &http.Client{CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	req, _ := http.NewRequestWithContext(context.Background(), http.MethodGet, ts.URL+"/engine", nil)
	res, err := client.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	_ = res.Body.Close()
	if res.StatusCode != http.StatusSeeOther || res.Header.Get("Location") != "/login" {
		t.Fatalf("got %d to %q", res.StatusCode, res.Header.Get("Location"))
	}
}
