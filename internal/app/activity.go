package app

import "context"

// ActivityFact is immutable source history, never a projected payment.
type ActivityFact struct {
	Kind             string `json:"kind"`
	AmountMinor      int64  `json:"amount_minor"`
	TransactionDate  string `json:"transaction_date"`
	ValueDate        string `json:"value_date"`
	Status           string `json:"status"`
	Voids            string `json:"voids"`
	Voided           bool   `json:"voided"`
	Version          int64  `json:"version"`
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

// ActivityPager follows immutable record cursors without hiding older facts.
type ActivityPager interface {
	BorrowerActivityAfter(context.Context, string, string) ([]ActivityFact, error)
}
