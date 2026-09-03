package postgres

import (
	"context"
	"encoding/json"

	"github.com/andranikasd/marumbot/internal/app"
	"github.com/andranikasd/marumbot/pkg/core/money"
)

func (s *Store) BorrowerAllocatedActivity(ctx context.Context, userID, cursor string) ([]app.AllocatedActivityFact, error) {
	facts, err := s.BorrowerActivityAfter(ctx, userID, cursor)
	if err != nil {
		return nil, err
	}
	ids := make([]string, 0, len(facts))
	out := make([]app.AllocatedActivityFact, len(facts))
	for i, f := range facts {
		ids = append(ids, f.ID)
		out[i].ActivityFact = f
	}
	rows, err := s.pool.Query(ctx, q("PaymentAllocations"), userID, ids)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	allocations := make(map[string]*app.PaymentAllocation)
	for rows.Next() {
		var id string
		var payload []byte
		if err := rows.Scan(&id, &payload); err != nil {
			return nil, err
		}
		var allocation *app.PaymentAllocation
		if err := json.Unmarshal(payload, &allocation); err != nil {
			return nil, err
		}
		allocations[id] = allocation
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	for i := range out {
		out[i].Allocation = allocations[out[i].ID]
	}
	return out, nil
}

func (s *Store) MonthlyPaymentActuals(ctx context.Context, userID, month string) ([]app.PaymentActuals, error) {
	if err := app.ValidatePaymentMonth(month); err != nil {
		return nil, err
	}
	rows, err := s.pool.Query(ctx, q("MonthlyPaymentActuals"), userID, month+"-01")
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []app.PaymentActuals{}
	for rows.Next() {
		var a app.PaymentActuals
		if err := rows.Scan(&a.Currency, &a.PaymentCount, &a.KnownCount, &a.UnknownCount, &a.PendingCount, &a.PaidMinor, &a.PrincipalMinor, &a.InterestMinor, &a.FeesMinor, &a.UnknownPaidMinor); err != nil {
			return nil, err
		}
		cur, err := money.Lookup(a.Currency)
		if err != nil {
			return nil, err
		}
		a.CurrencyExponent = cur.Exponent
		out = append(out, a)
	}
	return out, rows.Err()
}
