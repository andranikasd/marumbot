package postgres

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/andranikasd/marumbot/internal/app"
	"github.com/andranikasd/marumbot/pkg/core/date"
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
//
// Every column the query selects is used. An earlier version scanned the
// contract terms and then dropped them on the floor, which left the bot unable
// to show a payment amount for a loan whose terms it had just read.
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
			return nil, fmt.Errorf("loan %s: %w", l.ID, err)
		}
		startDate, err := date.Parse(start)
		if err != nil {
			return nil, fmt.Errorf("loan %s start date: %w", l.ID, err)
		}
		maturityDate, err := date.Parse(maturity)
		if err != nil {
			return nil, fmt.Errorf("loan %s maturity date: %w", l.ID, err)
		}

		l.Contract = model.Contract{
			LoanID:       model.ID(l.ID),
			Version:      1,
			Currency:     cur,
			NominalRate:  parseRate(rate),
			DayCount:     dayCountFrom(dayCount),
			Type:         repaymentTypeFrom(repayment),
			StartDate:    startDate,
			MaturityDate: maturityDate,
			PaymentDay:   int(day),
			Rounding:     money.Policy{Mode: roundingModeFrom(mode), Unit: int64(unit)},
		}
		if principal != nil {
			l.Balance = money.FromMinor(*principal, cur)
		} else {
			l.Balance = money.Zero(cur)
		}
		if asOf != nil {
			if d, err := date.Parse(*asOf); err == nil {
				l.AsOf = d
			}
		}
		if trust != nil {
			l.Trust = *trust
		}
		out = append(out, l)
	}
	return out, rows.Err()
}

// parseRate turns the numeric(12,9) fraction back into parts per billion.
//
// The column stores 0.140000000 for 14 per cent -- a fraction, not a
// percentage. Rendering it directly produced "Rate: 0.140000000%".
func parseRate(s string) money.Rate {
	whole, frac, _ := strings.Cut(s, ".")
	w, _ := strconv.ParseInt(whole, 10, 64)
	frac = (frac + "000000000")[:9]
	f, _ := strconv.ParseInt(frac, 10, 64)
	return money.Rate(w*1_000_000_000 + f)
}

func dayCountFrom(s string) money.DayCount {
	switch s {
	case "act360":
		return money.Actual360
	case "30_360":
		return money.Thirty360
	default:
		return money.Actual365
	}
}

func repaymentTypeFrom(s string) model.RepaymentType {
	if s == "declining" {
		return model.DecliningPrincipal
	}
	return model.Annuity
}

func roundingModeFrom(s string) money.Mode {
	switch s {
	case "half_even":
		return money.HalfEven
	case "down":
		return money.Down
	case "up":
		return money.Up
	default:
		return money.HalfUp
	}
}

// SetBudget records the monthly amount a borrower can put towards loans.
func (s *Store) SetBudget(ctx context.Context, userID, currency string, minor int64) error {
	var got int64
	return s.pool.QueryRow(ctx, q("SetBudget"), userID, currency, minor).Scan(&got)
}

// Budget returns the borrower's largest recorded budget, or Set=false when
// there is none. Absent is a state the caller must handle, not an error: a user
// who has not set a budget is the normal case, not a fault.
func (s *Store) Budget(ctx context.Context, userID string) (app.Budget, error) {
	var b app.Budget
	var minor int64
	err := s.pool.QueryRow(ctx, q("GetBudget"), userID).Scan(&b.Currency, &minor)
	if errors.Is(err, pgx.ErrNoRows) {
		return app.Budget{}, nil
	}
	if err != nil {
		return app.Budget{}, err
	}
	cur, err := money.Lookup(b.Currency)
	if err != nil {
		return app.Budget{}, err
	}
	b.Monthly, b.Set = money.FromMinor(minor, cur), true
	return b, nil
}
