package postgres

import (
	"context"

	"github.com/andranikasd/marumbot/internal/app"
	"github.com/andranikasd/marumbot/pkg/core/money"
)

// BorrowerActivity returns only the authenticated borrower's source history.
func (s *Store) BorrowerActivity(ctx context.Context, userID string) ([]app.ActivityFact, error) {
	return s.BorrowerActivityAfter(ctx, userID, "")
}

func (s *Store) BorrowerActivityAfter(ctx context.Context, userID, cursor string) ([]app.ActivityFact, error) {
	rows, err := s.pool.Query(ctx, q("BorrowerActivity"), userID, cursor)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []app.ActivityFact{}
	for rows.Next() {
		var f app.ActivityFact
		if err := rows.Scan(&f.ID, &f.LoanID, &f.Loan, &f.Currency, &f.AsOf, &f.PrincipalMinor, &f.Trust, &f.Kind, &f.AmountMinor, &f.TransactionDate, &f.ValueDate, &f.Status, &f.Voids, &f.Voided, &f.Version); err != nil {
			return nil, err
		}
		cur, err := money.Lookup(f.Currency)
		if err != nil {
			return nil, err
		}
		f.CurrencyExponent = cur.Exponent
		out = append(out, f)
	}
	return out, rows.Err()
}
