package app

import (
	"context"
	"errors"
	"testing"

	"github.com/andranikasd/marumbot/internal/i18n"
	"github.com/andranikasd/marumbot/pkg/core/money"
	"github.com/andranikasd/marumbot/pkg/core/plan"
)

type currencyRefusalSender struct {
	Sender
	messages []string
}

func (s *currencyRefusalSender) SendMessage(_ context.Context, _ int64, text string, _ any) error {
	s.messages = append(s.messages, text)
	return nil
}

func TestMixedCurrencyPortfolioNeverProducesPartialAdvice(t *testing.T) {
	first := shadowLoan(t)
	second := first
	second.ID, second.Contract.LoanID = "foreign", "foreign"
	cur := money.MustLookup("USD")
	second.Contract.Currency = cur
	second.Contract.Rounding = money.DefaultPolicy(cur)
	second.Balance = money.FromMinor(first.Balance.Minor(), cur)
	for _, loans := range [][]UserLoan{{first, second}, {second, first}} {
		w := shadowWorker(t, &shadowFakes{loans: loans})
		positions, owed, required, _, err := w.positions(t.Context(), loans)
		var mixed *plan.MixedCurrencyError
		if !errors.As(err, &mixed) || len(positions) != 0 || owed.Sign() != 0 || required.Sign() != 0 {
			t.Fatalf("partial portfolio returned: %v", err)
		}
		amount, _, err := w.RequiredThisMonth(t.Context(), "user")
		if !errors.As(err, &mixed) || amount.Sign() != 0 {
			t.Fatalf("partial current-month total returned: %v", err)
		}
		for _, locale := range []i18n.Locale{i18n.EN, i18n.HY} {
			for _, explain := range []bool{false, true} {
				sender := &currencyRefusalSender{}
				w.Send = sender
				goal := plan.Goal{Kind: plan.LeastInterest}
				if explain {
					err = w.explainPlan(t.Context(), "user", 1, locale, goal)
				} else {
					err = w.advise(t.Context(), "user", 1, locale, goal, false)
				}
				want := i18n.T(locale, "advice.refuse.mixed_currency", loans[0].Contract.Currency.Code, loans[1].Contract.Currency.Code)
				if err != nil || len(sender.messages) != 1 || sender.messages[0] != want {
					t.Fatalf("explain=%v locale=%s: want localized refusal, got %v %v", explain, locale, sender.messages, err)
				}
			}
		}
	}
}
