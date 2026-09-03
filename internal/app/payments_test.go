package app

import (
	"errors"
	"testing"

	"github.com/andranikasd/marumbot/pkg/core/date"
)

func TestPaymentFactDatesAndAmounts(t *testing.T) {
	today := date.MustNew(2026, 9, 3)
	valid := PaymentCommand{LoanID: "loan", Key: "0123456789abcdef", AmountMinor: 1, TransactionDate: "2026-09-02"}
	if err := valid.validate(today); err != nil {
		t.Fatal(err)
	}
	cases := map[string]PaymentCommand{}
	c := valid
	c.AmountMinor = 0
	cases["zero"] = c
	c = valid
	c.AmountMinor = -1
	cases["negative"] = c
	c = valid
	c.AmountMinor = 9007199254740992
	cases["browser precision"] = c
	c = valid
	c.TransactionDate = "2026-09-04"
	cases["future intention"] = c
	c = valid
	c.ValueDate = "2026-09-01"
	cases["posting before transfer"] = c
	c = valid
	c.ValueDate = "2026-09-04"
	cases["future posting"] = c
	c = valid
	c.ValueDate = "2026-02-30"
	cases["invalid date"] = c
	c = valid
	c.VoidOnly = true
	cases["void without target"] = c
	for name, command := range cases {
		t.Run(name, func(t *testing.T) {
			if err := command.validate(today); !errors.Is(err, ErrPaymentInvalid) {
				t.Fatalf("accepted invalid fact: %v", err)
			}
		})
	}
}

func TestPlanRefusesUnreconciledPayment(t *testing.T) {
	w := Worker{}
	_, _, _, _, err := w.positions(t.Context(), []UserLoan{{UnreconciledPayments: true}})
	if !errors.Is(err, ErrPaymentReconciliation) {
		t.Fatalf("unreconciled payment ignored: %v", err)
	}
}
