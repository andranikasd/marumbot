package app

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/andranikasd/marumbot/pkg/core/date"
	"github.com/andranikasd/marumbot/pkg/core/model"
	"github.com/andranikasd/marumbot/pkg/core/money"
)

type requiredMonthZone struct{ UserStore }

func (requiredMonthZone) Locale(context.Context, string) (string, string, error) {
	return "hy", "Asia/Yerevan", nil
}

// Independent zero-interest fixture: 1,200 owed, 100 per instalment. Only
// dates change across cases; current-month obligations are 0, 100 or unknown.
func TestRequiredThisMonthHonoursConfirmedPaidMonths(t *testing.T) {
	cur := money.MustLookup("USD")
	loan := UserLoan{ID: "current", Balance: money.FromMinor(120000, cur), AsOf: date.MustNew(2026, 9, 1), Contract: model.Contract{LoanID: "current", Version: 1, Currency: cur, EffectiveFrom: date.MustNew(2026, 9, 1), StartDate: date.MustNew(2026, 8, 15), MaturityDate: date.MustNew(2027, 8, 15), PaymentDay: 15, DayCount: money.Actual365, Type: model.Annuity, Rounding: money.DefaultPolicy(cur), HasScheduled: true, ScheduledPayment: money.FromMinor(10000, cur)}}
	paid := loan
	paid.ID = "paid"
	paid.Contract.LoanID = "paid"
	paid.Contract.NotBeforeDue = date.MustNew(2026, 10, 1)
	paid.Contract.MaturityDate = date.MustNew(2027, 9, 15)
	stale := loan
	stale.AsOf = date.MustNew(2026, 8, 1)
	stale.Contract.StartDate = date.MustNew(2026, 7, 15)
	stale.Contract.MaturityDate = date.MustNew(2027, 7, 15)
	zoneWorker := shadowWorker(t, &shadowFakes{loans: []UserLoan{loan}})
	zoneWorker.Users = requiredMonthZone{}
	zoneWorker.Clock = &fixedClock{at: time.Date(2026, 8, 31, 22, 0, 0, 0, time.UTC)}
	zoneRequired, _, zoneErr := zoneWorker.RequiredThisMonth(t.Context(), "user")
	if zoneErr != nil || zoneRequired.Minor() != 10000 {
		t.Fatal("account month used UTC month", zoneErr)
	}
	for _, tc := range []struct {
		name    string
		loans   []UserLoan
		want    int64
		unknown bool
	}{
		{"all paid", []UserLoan{paid}, 0, false},
		{"one unpaid", []UserLoan{loan, paid}, 10000, false},
		{"overdue old anchor", []UserLoan{stale}, 0, true},
		{"empty account", nil, 0, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f := &shadowFakes{loans: tc.loans}
			w := shadowWorker(t, f)
			w.Clock = &fixedClock{at: time.Date(2026, 9, 20, 12, 0, 0, 0, time.UTC)}
			w.DefaultCurrency = cur
			required, _, err := w.RequiredThisMonth(t.Context(), "user")
			if tc.unknown {
				if !errors.Is(err, ErrPaymentReconciliation) {
					t.Fatal("unknown debt inferred paid", err)
				}
				return
			}
			if err != nil || required.Minor() != tc.want {
				t.Fatalf("required=%d want=%d err=%v", required.Minor(), tc.want, err)
			}
			if tc.name == "all paid" {
				_, _, recurring, _, err := w.positions(t.Context(), tc.loans)
				if err != nil || recurring.Minor() != 10000 {
					t.Fatal("recurring goal baseline changed", err)
				}
			}
		})
	}
}
