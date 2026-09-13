package app

import (
	"errors"
	"testing"

	"github.com/andranikasd/marumbot/pkg/core/date"
	"github.com/andranikasd/marumbot/pkg/core/model"
)

func TestPaidMonthNextDueCalendarBoundaries(t *testing.T) {
	for _, tt := range []struct {
		name, month, today, want string
		day                      int
	}{
		{"current early payment", "2026-09", "2026-09-13", "2026-10-15", 15},
		{"previous month before due", "2026-08", "2026-09-13", "2026-09-15", 15},
		{"December rolls year", "2026-12", "2026-12-20", "2027-01-31", 31},
		{"non leap February", "2027-01", "2027-01-20", "2027-02-28", 31},
		{"leap February", "2028-01", "2028-01-20", "2028-02-29", 31},
		{"future month", "2026-10", "2026-09-13", "", 15},
		{"overdue remains", "2026-08", "2026-09-16", "", 15},
		{"due today remains", "2026-08", "2026-09-15", "", 15},
		{"before contract", "2025-12", "2026-01-13", "", 15},
		{"final month has no next", "2028-12", "2028-12-20", "", 15},
		{"malformed month", "2026-9", "2026-09-13", "", 15},
		{"invalid month", "2026-13", "2026-09-13", "", 15},
	} {
		t.Run(tt.name, func(t *testing.T) {
			loan := UserLoan{Contract: model.Contract{StartDate: date.MustNew(2026, 1, 1), MaturityDate: date.MustNew(2028, 12, 31), PaymentDay: tt.day}}
			today, err := date.Parse(tt.today)
			if err != nil {
				t.Fatal(err)
			}
			got, err := PaidMonthNextDue(loan, tt.month, today)
			if tt.want == "" {
				if !errors.Is(err, ErrPaymentInvalid) {
					t.Fatalf("invalid statement accepted: %v", err)
				}
				return
			}
			if err != nil || got.String() != tt.want {
				t.Fatalf("next due=%s err=%v want=%s", got, err, tt.want)
			}
		})
	}
}
