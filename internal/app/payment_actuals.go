package app

import (
	"context"

	"github.com/andranikasd/marumbot/pkg/core/date"
)

// AllocatedActivityFact adds the optional source split without changing history.
type AllocatedActivityFact struct {
	ActivityFact
	Allocation *PaymentAllocation `json:"allocation"`
}

type PaymentActualsReader interface {
	BorrowerAllocatedActivity(context.Context, string, string) ([]AllocatedActivityFact, error)
	MonthlyPaymentActuals(context.Context, string, string) ([]PaymentActuals, error)
}

// PaymentActuals uses transaction month, includes pending transfers, and excludes
// voided facts. Currency totals never mix. Decimal strings keep aggregate money
// exact in browsers even when the sum exceeds the safe integer range.
// Null component totals mean no known allocation; partial coverage stays explicit.
type PaymentActuals struct {
	UnknownPaidMinor string  `json:"unknown_paid_minor"`
	Currency         string  `json:"currency"`
	CurrencyExponent uint8   `json:"currency_exponent"`
	PaymentCount     int64   `json:"payment_count"`
	KnownCount       int64   `json:"known_count"`
	UnknownCount     int64   `json:"unknown_count"`
	PendingCount     int64   `json:"pending_count"`
	PaidMinor        string  `json:"paid_minor"`
	PrincipalMinor   *string `json:"principal_minor"`
	InterestMinor    *string `json:"interest_minor"`
	FeesMinor        *string `json:"fees_minor"`
}

func ValidatePaymentMonth(month string) error {
	if len(month) != 7 {
		return ErrPaymentInvalid
	}
	if _, err := date.Parse(month + "-01"); err != nil {
		return ErrPaymentInvalid
	}
	return nil
}
