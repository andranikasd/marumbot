package miniapp

import (
	"bytes"
	"context"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/andranikasd/marumbot/internal/app"
)

type budgetTestClock struct{ now time.Time }

func (c budgetTestClock) Now() time.Time { return c.now }

type budgetTestCipher struct{}

func (budgetTestCipher) Tag(int64) string { return "telegram-tag" }

type budgetTestUsers struct{}

func (budgetTestUsers) UpsertByTelegram(context.Context, app.UpsertUser) (app.Account, error) {
	return app.Account{}, nil
}

func (budgetTestUsers) Locale(context.Context, string) (string, string, error) {
	return "hy", "Asia/Yerevan", nil
}

func (budgetTestUsers) ByTelegramTag(context.Context, string) (string, error) {
	return "user-id", nil
}
func (budgetTestUsers) SetLocale(context.Context, string, string) error { return nil }

type budgetTestConfigurator struct {
	configuration app.BudgetConfiguration
	err           error
	calls         int
}

func (c *budgetTestConfigurator) SetBudgetConfiguration(_ context.Context, configuration app.BudgetConfiguration) error {
	c.calls++
	c.configuration = configuration
	return c.err
}

func budgetTestServer(configurator app.BudgetConfigurator) *Server {
	return &Server{
		BotToken: testToken, Users: budgetTestUsers{}, Cipher: budgetTestCipher{},
		Clock: budgetTestClock{now: knownTime()}, BudgetConfig: configurator,
		Log: slog.New(slog.NewTextHandler(io.Discard, nil)),
	}
}

func postBudget(t *testing.T, server *Server, body string) *httptest.ResponseRecorder {
	t.Helper()
	r := httptest.NewRequestWithContext(t.Context(), http.MethodPost, "/api/budget", bytes.NewBufferString(body))
	r.Header.Set("X-Telegram-Init-Data", knownInitData())
	w := httptest.NewRecorder()
	server.Handler().ServeHTTP(w, r)
	return w
}

func TestSetBudgetStoresCompleteConfigurationOnce(t *testing.T) {
	t.Parallel()

	store := &budgetTestConfigurator{}
	w := postBudget(t, budgetTestServer(store), `{
		"monthly_major":250000.25,"currency":"AMD","pay_day":0,
		"opening_major":12000,"reserve_major":2000,"overrides":{"2026-10":300000,"2026-11":0}
	}`)
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", w.Code, w.Body.String())
	}
	got := store.configuration
	if store.calls != 1 {
		t.Fatalf("configuration writes = %d, want 1", store.calls)
	}
	if got.UserID != "user-id" || got.Currency != "AMD" || got.MonthlyMinor != 25_000_025 {
		t.Errorf("identity/monthly configuration = %+v", got)
	}
	if got.PayDay != 0 {
		t.Errorf("pay day = %d, want zero to clear the old value", got.PayDay)
	}
	if got.OpeningMinor != 1_200_000 || got.OpeningAsOf.String() != "2026-01-01" {
		t.Errorf("opening configuration = %d as of %s", got.OpeningMinor, got.OpeningAsOf)
	}
	if got.ReserveMinor != 200_000 {
		t.Errorf("protected reserve = %d, want 200000", got.ReserveMinor)
	}
	if got.Overrides["2026-10"] != 30_000_000 || got.Overrides["2026-11"] != 0 {
		t.Errorf("overrides = %#v", got.Overrides)
	}
}

func TestSetBudgetRejectsReserveAboveOpeningCash(t *testing.T) {
	t.Parallel()

	store := &budgetTestConfigurator{}
	w := postBudget(t, budgetTestServer(store), `{
		"monthly_major":250000,"currency":"AMD",
		"opening_major":10000,"reserve_major":12000
	}`)
	if w.Code != http.StatusUnprocessableEntity {
		t.Fatalf("status = %d, want %d; body = %s", w.Code, http.StatusUnprocessableEntity, w.Body.String())
	}
	if store.calls != 0 {
		t.Fatalf("configuration stored %d times after invalid reserve", store.calls)
	}
}

func TestSetBudgetRejectsInvalidRequestBeforePersistence(t *testing.T) {
	t.Parallel()

	store := &budgetTestConfigurator{}
	w := postBudget(t, budgetTestServer(store), `{"monthly_major":0,"currency":"AMD"}`)
	if w.Code != http.StatusUnprocessableEntity {
		t.Fatalf("status = %d, want 422", w.Code)
	}
	if store.calls != 0 {
		t.Fatalf("configuration writes = %d, want 0", store.calls)
	}
}

func TestSetBudgetRequiresAtomicConfigurationPort(t *testing.T) {
	t.Parallel()

	w := postBudget(t, budgetTestServer(nil), `{"monthly_major":1,"currency":"AMD"}`)
	if w.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, want 503", w.Code)
	}
}

func TestSetBudgetReportsPersistenceFailure(t *testing.T) {
	t.Parallel()

	store := &budgetTestConfigurator{err: errors.New("write failed")}
	w := postBudget(t, budgetTestServer(store), `{"monthly_major":1,"currency":"AMD"}`)
	if w.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want 500", w.Code)
	}
	if store.calls != 1 {
		t.Fatalf("configuration writes = %d, want 1", store.calls)
	}
}
