package postgres

import (
	"context"

	"github.com/google/uuid"

	"github.com/andranikasd/marumbot/internal/app"
	"github.com/andranikasd/marumbot/pkg/core/model"
	"github.com/andranikasd/marumbot/pkg/core/money"
)

// The column values are check-constrained, so each mapping is a switch rather
// than a Stringer: adding a convention to the engine without migrating the
// schema should fail here, at the boundary, and not at insert time in front of
// a user.
func dayCountName(d money.DayCount) string {
	switch d {
	case money.Actual360:
		return "act360"
	case money.Thirty360:
		return "30_360"
	default:
		return "act365"
	}
}

func roundingModeName(m money.Mode) string {
	switch m {
	case money.HalfEven:
		return "half_even"
	case money.Down:
		return "down"
	case money.Up:
		return "up"
	default:
		return "half_up"
	}
}

func repaymentTypeName(t model.RepaymentType) string {
	if t == model.DecliningPrincipal {
		return "declining"
	}
	return "annuity"
}

// CreateLoan records a loan, its first contract version and its opening
// balance in one statement, so a loan cannot exist without the terms that
// explain it or the anchor that gives it a balance.
func (s *Store) CreateLoan(ctx context.Context, d app.LoanDraft) (string, error) {
	c := d.Contract
	name := d.Lender
	if name == "" {
		name = "Loan"
	}

	var got string
	err := s.pool.QueryRow(ctx, q("CreateLoan"),
		uuid.NewString(), d.UserID, name, d.Lender, c.Currency.Code,
		uuid.NewString(), c.StartDate.String(),
		c.NominalRate.String(), dayCountName(c.DayCount), repaymentTypeName(c.Type),
		c.MaturityDate.String(), c.PaymentDay,
		roundingModeName(c.Rounding.Mode), c.Rounding.Unit,
		uuid.NewString(), d.Principal.Minor(),
	).Scan(&got)
	return got, err
}

// LoansForUser returns each loan with its current contract and latest anchor.
func (s *Store) LoansForUser(ctx context.Context, userID string, limit int32) ([]app.UserLoan, error) {
	rows, err := s.pool.Query(ctx, q("ListLoansForUser"), userID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []app.UserLoan
	for rows.Next() {
		var (
			l         app.UserLoan
			code      string
			rate      string
			dayCount  string
			repayment string
			mode      string
			unit      int32
			start     string
			maturity  string
			day       int16
			principal *int64
			asOf      *string
			trust     *string
		)
		if err := rows.Scan(&l.ID, &l.Name, &l.Lender, &code,
			&rate, &repayment, &dayCount, &start, &maturity, &day,
			&mode, &unit, &principal, &asOf, &trust); err != nil {
			return nil, err
		}
		cur, err := money.Lookup(code)
		if err != nil {
			return nil, err
		}
		l.Currency = code
		if principal != nil {
			l.Balance = money.FromMinor(*principal, cur)
		} else {
			l.Balance = money.Zero(cur)
		}
		if trust != nil {
			l.Trust = *trust
		}
		l.Rate = rate
		l.Method = repayment
		l.MaturityDate = maturity
		l.PaymentDay = int(day)
		out = append(out, l)
	}
	return out, rows.Err()
}
