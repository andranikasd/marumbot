package postgres

import (
	"context"
	"encoding/json"

	"github.com/andranikasd/marumbot/internal/app"
)

func (s *Store) ActiveActualBaselines(ctx context.Context, user string) ([]app.ActualBaseline, error) {
	rows, err := s.pool.Query(ctx, q("ActiveActualBaselines"), user)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []app.ActualBaseline{}
	for rows.Next() {
		var b app.ActualBaseline
		if err := rows.Scan(&b.PlanID, &b.Currency, &b.ActivatedAt); err != nil {
			return nil, err
		}
		out = append(out, b)
	}
	return out, rows.Err()
}

func (s *Store) PlanActualFacts(ctx context.Context, user string, baseline app.ActualBaseline, month string) ([]app.PlanActualFact, error) {
	if err := app.ValidatePaymentMonth(month); err != nil {
		return nil, err
	}
	rows, err := s.pool.Query(ctx, q("PlanActualFacts"), user, baseline.Currency, month+"-01", baseline.ActivatedAt)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []app.PlanActualFact{}
	for rows.Next() {
		var f app.PlanActualFact
		var allocation []byte
		if err := rows.Scan(&f.ID, &f.LoanID, &f.TransactionDate, &f.ValueDate, &f.AmountMinor, &allocation, &f.RecordedAfterActivation); err != nil {
			return nil, err
		}
		if len(allocation) > 0 {
			if err := json.Unmarshal(allocation, &f.Allocation); err != nil {
				return nil, err
			}
		}
		out = append(out, f)
		if len(out) > 10000 {
			return nil, app.ErrPlanActualsCoverage
		}
	}
	return out, rows.Err()
}
