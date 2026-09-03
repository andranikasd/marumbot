package amortisation_test

import (
	"testing"

	"github.com/andranikasd/marumbot/pkg/core/amortisation"
	"github.com/andranikasd/marumbot/pkg/core/date"
	"github.com/andranikasd/marumbot/pkg/core/money"
)

func TestBankNextDueDoesNotChargeAlreadyPaidInstalment(t *testing.T) {
	c, _ := amd5m()
	c.NominalRate = money.RateFromPercent(0, 0)
	c.MaturityDate = date.MustNew(2026, 4, 15)
	c.NotBeforeDue = date.MustNew(2026, 3, 15)
	c.ScheduledPayment = money.FromMinor(30000, c.Currency)
	c.HasScheduled = true
	// Independent zero-interest statement: 600 AMD outstanding after an early
	// February payment, followed by 300 AMD in March and 300 AMD in April.
	got, err := amortisation.Build(c, money.FromMinor(60000, c.Currency), date.MustNew(2026, 2, 1))
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Rows) != 2 || got.Rows[0].Due != c.NotBeforeDue || got.Rows[0].Payment.Minor() != 30000 || got.Rows[1].Closing.Minor() != 0 {
		t.Fatalf("bank schedule not preserved: %+v", got)
	}
	dates, err := amortisation.RemainingDates(c, date.Date{})
	if err != nil || len(dates) != 2 {
		t.Fatal("zero anchor ignores bank due date")
	}
}
