package app_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/andranikasd/marumbot/internal/app"
	"github.com/andranikasd/marumbot/pkg/core/date"
	"github.com/andranikasd/marumbot/pkg/core/model"
	"github.com/andranikasd/marumbot/pkg/core/money"
	"github.com/andranikasd/marumbot/pkg/core/projection"
)

type monthlySource struct {
	settings app.ProjectionSettings
	loans    []app.UserLoan
	user     string
}

func (s *monthlySource) ProjectionSettings(_ context.Context, user string) (app.ProjectionSettings, error) {
	s.user = user
	return s.settings, nil
}

func (s *monthlySource) LoansForUser(_ context.Context, user string, _ int32) ([]app.UserLoan, error) {
	s.user = user
	return s.loans, nil
}

type monthlyClock struct{}

func (monthlyClock) Now() time.Time { return time.Date(2026, 9, 30, 22, 0, 0, 0, time.UTC) }

type monthlyUsers struct{ app.UserStore }

func (monthlyUsers) Locale(context.Context, string) (string, string, error) {
	return "en", "Asia/Yerevan", nil
}

func TestProjectionSeparatesCurrencyAndUsesAccountMonth(t *testing.T) {
	source := &monthlySource{settings: app.ProjectionSettings{Version: 7, Currencies: map[string]projection.Extra{"USD": {Minor: 999}}}}
	for _, code := range []string{"USD", "AMD", "EUR"} {
		cur := money.MustLookup(code)
		balance := int64(10000)
		if code == "EUR" {
			balance = 0
		}
		source.loans = append(source.loans, app.UserLoan{ID: code, Name: code, Balance: money.FromMinor(balance, cur), AsOf: date.MustNew(2026, 9, 30), Contract: model.Contract{Currency: cur, HasScheduled: true, ScheduledPayment: money.FromMinor(1000, cur), NotBeforeDue: date.MustNew(2026, 10, 15)}})
	}
	svc := app.ProjectionService{Settings: source, Loans: source, Users: monthlyUsers{}, Clock: monthlyClock{}}
	r, err := svc.Build(t.Context(), "owner")
	if err != nil {
		t.Fatal(err)
	}
	if source.user != "owner" || r.Today != "2026-10-01" || r.SettingsVersion != 7 || r.Enabled || len(r.Currencies) != 2 || r.Currencies[0].Currency != "AMD" || r.Currencies[1].Currency != "USD" {
		t.Fatalf("currency/timezone/consent boundary %+v", r)
	}
	for _, c := range r.Currencies {
		if c.Months[0].Required != 1000 || c.Months[0].RequestedExtra != 0 || c.Complete {
			t.Fatal("requiredsource merged or oldbudget reinterpreted", c)
		}
	}
	source.loans = []app.UserLoan{{ID: "zero", Balance: money.Zero(money.MustLookup("USD")), UnreconciledPayments: true}}
	if _, err = svc.Build(t.Context(), "owner"); !errors.Is(err, app.ErrPaymentReconciliation) {
		t.Fatal("unverified zero balance claimed debtfree", err)
	}
}
