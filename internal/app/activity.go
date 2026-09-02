package app

import "context"

// ActivityFact is a recorded balance statement, never a projected payment.
type ActivityFact struct {
	ID               string `json:"id"`
	LoanID           string `json:"loan_id"`
	Loan             string `json:"loan"`
	Currency         string `json:"currency"`
	AsOf             string `json:"as_of"`
	PrincipalMinor   int64  `json:"principal_minor"`
	Trust            string `json:"trust"`
	CurrencyExponent uint8  `json:"currency_exponent"`
}

// ActivityReader scopes source history to its owner.
type ActivityReader interface {
	BorrowerActivity(context.Context, string) ([]ActivityFact, error)
}
