package postgres

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/andranikasd/marumbot/internal/app"
	"github.com/andranikasd/marumbot/pkg/core/allocation"
	"github.com/andranikasd/marumbot/pkg/core/date"
	"github.com/andranikasd/marumbot/pkg/core/model"
	"github.com/andranikasd/marumbot/pkg/core/money"
	"github.com/andranikasd/marumbot/pkg/core/plan"
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
	case money.ActualActual:
		return "act_act"
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
	icon, err := app.LoanIcon(d.Icon)
	if err != nil {
		return "", err
	}
	c := d.Contract
	title := d.Title
	if title == "" {
		title = "Loan"
	}

	var got string
	err = s.pool.QueryRow(ctx, q("CreateLoan"),
		uuid.NewString(), d.UserID, title, d.Description, c.Currency.Code,
		uuid.NewString(), c.StartDate.String(),
		c.NominalRate.String(), dayCountName(c.DayCount), repaymentTypeName(c.Type),
		c.MaturityDate.String(), c.PaymentDay,
		roundingModeName(c.Rounding.Mode), c.Rounding.Unit,
		uuid.NewString(), d.Balance.Minor(), d.AsOf.String(),
		prepaymentJSON(c.Prepayment), plan.MaxLoans, icon, d.OptionalExcluded,
	).Scan(&got)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", app.ErrTooManyLoans
	}
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
			l               app.UserLoan
			code            string
			rate            string
			dayCount        string
			repayment       string
			mode            string
			unit            int32
			start           string
			maturity        string
			day             int16
			principal       *int64
			asOf            *string
			trust           *string
			excess          string
			prepay          string
			first           *int64
			contractVersion int
			effectiveFrom   string
		)
		if err := rows.Scan(&l.ID, &l.Name, &l.Description, &code,
			&rate, &repayment, &dayCount, &start, &maturity, &day,
			&mode, &unit, &principal, &asOf, &trust, &excess, &prepay, &first, &l.Icon, &l.OptionalExcluded, &contractVersion, &effectiveFrom); err != nil {
			return nil, err
		}
		if l.Excess, err = allocation.ParseExcessRule(excess); err != nil {
			return nil, fmt.Errorf("decoding allocation policy: %w", err)
		}

		cur, err := money.Lookup(code)
		if err != nil {
			return nil, fmt.Errorf("decoding currency: %w", err)
		}
		prepayment, err := parsePrepayment(prepay, cur)
		if err != nil {
			return nil, fmt.Errorf("decoding prepayment terms: %w", err)
		}
		startDate, err := date.Parse(start)
		if err != nil {
			return nil, fmt.Errorf("decoding loan start date: %w", err)
		}
		maturityDate, err := date.Parse(maturity)
		if err != nil {
			return nil, fmt.Errorf("decoding loan maturity date: %w", err)
		}

		parsedRate, err := parseRate(rate)
		if err != nil {
			return nil, fmt.Errorf("decoding loan rate: %w", err)
		}
		parsedDayCount, err := dayCountFrom(dayCount)
		if err != nil {
			return nil, fmt.Errorf("decoding loan day count: %w", err)
		}
		parsedType, err := repaymentTypeFrom(repayment)
		if err != nil {
			return nil, fmt.Errorf("decoding loan repayment type: %w", err)
		}
		parsedMode, err := roundingModeFrom(mode)
		if err != nil {
			return nil, fmt.Errorf("decoding loan rounding mode: %w", err)
		}
		effective, err := date.Parse(effectiveFrom)
		if err != nil {
			return nil, err
		}
		l.Contract = model.Contract{
			LoanID:  model.ID(l.ID),
			Version: contractVersion, EffectiveFrom: effective,
			Currency:     cur,
			NominalRate:  parsedRate,
			DayCount:     parsedDayCount,
			Type:         parsedType,
			StartDate:    startDate,
			MaturityDate: maturityDate,
			PaymentDay:   int(day),
			Rounding:     money.Policy{Mode: parsedMode, Unit: int64(unit)},
			Prepayment:   prepayment,
		}
		if first != nil {
			l.OriginalPrincipal = money.FromMinor(*first, cur)
		} else {
			l.OriginalPrincipal = money.Zero(cur)
		}
		if principal != nil {
			l.Balance = money.FromMinor(*principal, cur)
		} else {
			l.Balance = money.Zero(cur)
		}
		if asOf != nil {
			d, err := date.Parse(*asOf)
			if err != nil {
				// A zero AsOf silently anchors the schedule at the start date,
				// which shows wrong amounts; a bad stored date must surface.
				return nil, fmt.Errorf("decoding loan balance date: %w", err)
			}
			l.AsOf = d
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
func parseRate(s string) (money.Rate, error) {
	whole, frac, _ := strings.Cut(s, ".")
	w, err := strconv.ParseInt(whole, 10, 64)
	if err != nil {
		return 0, fmt.Errorf("invalid rate %q: %w", s, err)
	}
	if len(frac) > 9 {
		return 0, fmt.Errorf("invalid rate precision %q", s)
	}
	frac = (frac + "000000000")[:9]
	f, err := strconv.ParseInt(frac, 10, 64)
	if err != nil {
		return 0, fmt.Errorf("invalid rate %q: %w", s, err)
	}
	return money.Rate(w*1_000_000_000 + f), nil
}

func dayCountFrom(s string) (money.DayCount, error) {
	switch s {
	case "act365":
		return money.Actual365, nil
	case "act360":
		return money.Actual360, nil
	case "30_360":
		return money.Thirty360, nil
	case "act_act":
		return money.ActualActual, nil
	default:
		return 0, fmt.Errorf("unknown day count %q", s)
	}
}

func repaymentTypeFrom(s string) (model.RepaymentType, error) {
	switch s {
	case "annuity":
		return model.Annuity, nil
	case "declining":
		return model.DecliningPrincipal, nil
	default:
		return 0, fmt.Errorf("unknown repayment type %q", s)
	}
}

func roundingModeFrom(s string) (money.Mode, error) {
	switch s {
	case "half_up":
		return money.HalfUp, nil
	case "half_even":
		return money.HalfEven, nil
	case "down":
		return money.Down, nil
	case "up":
		return money.Up, nil
	default:
		return 0, fmt.Errorf("unknown rounding mode %q", s)
	}
}

// SetBudget records the monthly amount a borrower can put towards loans.
func (s *Store) SetBudget(ctx context.Context, userID, currency string, minor int64, payDay int) error {
	var got int64
	return s.pool.QueryRow(ctx, q("SetBudget"), userID, currency, minor, payDay).Scan(&got)
}

// SetBudgetConfiguration records the Mini App's complete form atomically.
func (s *Store) SetBudgetConfiguration(ctx context.Context, configuration app.BudgetConfiguration) error {
	overrides := configuration.Overrides
	if overrides == nil {
		overrides = map[string]int64{}
	}
	raw, err := json.Marshal(overrides)
	if err != nil {
		return fmt.Errorf("encoding budget overrides: %w", err)
	}
	var funding any
	if configuration.Funding != nil {
		encoded, err := json.Marshal(configuration.Funding)
		if err != nil {
			return err
		}
		funding = string(encoded)
	}
	var got int64
	err = s.pool.QueryRow(ctx, q("SetBudgetConfiguration"),
		configuration.UserID, configuration.Currency, configuration.MonthlyMinor,
		configuration.PayDay, configuration.OpeningMinor,
		configuration.OpeningAsOf.String(), string(raw), configuration.ReserveMinor, funding, configuration.ExpectedVersion,
	).Scan(&got)
	if errors.Is(err, pgx.ErrNoRows) {
		return app.ErrConflict
	}
	return err
}

// Budget returns the borrower's most recently stated budget, or Set=false
// when there is none. Absent is a state the caller must handle, not an error:
// a user who has not set a budget is the normal case, not a fault.
func (s *Store) Budget(ctx context.Context, userID string) (app.Budget, error) {
	var (
		b        app.Budget
		minor    int64
		payDay   int16
		overRaw  string
		opening  int64
		reserve  int64
		openedOn *string
		funding  *string
	)
	err := s.pool.QueryRow(ctx, q("GetBudget"), userID).Scan(
		&b.Currency, &minor, &payDay, &overRaw, &opening, &openedOn, &reserve, &funding, &b.Version)
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
	if funding != nil {
		if err := json.Unmarshal([]byte(*funding), &b.Funding); err != nil {
			return app.Budget{}, err
		}
	}
	b.Monthly, b.Set, b.PayDay = money.FromMinor(minor, cur), true, int(payDay)
	if overRaw != "" && overRaw != "{}" {
		if err := json.Unmarshal([]byte(overRaw), &b.Overrides); err != nil {
			return app.Budget{}, fmt.Errorf("budget overrides: %w", err)
		}
	}
	b.Opening = money.FromMinor(opening, cur)
	b.Reserve = money.FromMinor(reserve, cur)
	if openedOn != nil {
		d, err := date.Parse(*openedOn)
		if err != nil {
			return app.Budget{}, fmt.Errorf("budget opening_as_of: %w", err)
		}
		b.OpeningAsOf = d
	}
	return b, nil
}

// SetOpening states cash on hand for loans, stamped with the day it was said.
func (s *Store) SetOpening(ctx context.Context, userID, currency string, minor int64, asOf string) error {
	var got int64
	err := s.pool.QueryRow(ctx, q("SetBudgetOpening"), userID, currency, minor, asOf).Scan(&got)
	if errors.Is(err, pgx.ErrNoRows) {
		return app.ErrNotFound // no budget row for this currency yet
	}
	return err
}

// SetOverrides replaces the whole per-month budget document.
func (s *Store) SetOverrides(ctx context.Context, userID, currency string, overrides map[string]int64) error {
	if overrides == nil {
		overrides = map[string]int64{}
	}
	raw, err := json.Marshal(overrides)
	if err != nil {
		return err
	}
	var got string
	err = s.pool.QueryRow(ctx, q("SetBudgetOverrides"), userID, currency, string(raw)).Scan(&got)
	if errors.Is(err, pgx.ErrNoRows) {
		return app.ErrNotFound
	}
	return err
}

// UpdateLoan renames a loan the borrower owns.
func (s *Store) UpdateLoan(ctx context.Context, loanID, userID, name, description string) error {
	var got string
	err := s.pool.QueryRow(ctx, q("UpdateLoanForUser"), loanID, userID, name, description).Scan(&got)
	if errors.Is(err, pgx.ErrNoRows) {
		return app.ErrNotFound
	}
	return err
}

// ArchiveLoan hides a loan the borrower owns. The ledger behind it is kept: a
// balance is only checkable because its events can be replayed.
func (s *Store) ArchiveLoan(ctx context.Context, loanID, userID string) error {
	var got string
	err := s.pool.QueryRow(ctx, q("ArchiveLoanForUser"), loanID, userID).Scan(&got)
	if errors.Is(err, pgx.ErrNoRows) {
		return app.ErrNotFound
	}
	return err
}

// LoanForUser returns one loan the borrower owns. It maps exactly the columns
// ListLoansForUser does: a contract rebuilt with defaults instead of the stored
// day count, rounding and snapshot as_of produced different instalments here
// than in the list, and with a zero AsOf the schedule re-accrued from the start
// date on every mid-life loan.
func (s *Store) LoanForUser(ctx context.Context, loanID, userID string) (app.UserLoan, error) {
	var (
		l               app.UserLoan
		code            string
		rate            string
		dayCount        string
		repayment       string
		mode            string
		unit            int32
		start           string
		maturity        string
		day             int16
		principal       *int64
		asOf            *string
		trust           *string
		excess          string
		prepay          string
		first           *int64
		contractVersion int
		effectiveFrom   string
	)
	err := s.pool.QueryRow(ctx, q("GetLoanForUser"), loanID, userID).Scan(
		&l.ID, &l.Name, &l.Description, &code,
		&rate, &repayment, &dayCount, &start, &maturity, &day,
		&mode, &unit, &principal, &asOf, &trust, &excess, &prepay, &first, &l.Icon, &l.OptionalExcluded, &contractVersion, &effectiveFrom)
	if errors.Is(err, pgx.ErrNoRows) {
		return app.UserLoan{}, app.ErrNotFound
	}
	if err != nil {
		return app.UserLoan{}, err
	}
	if l.Excess, err = allocation.ParseExcessRule(excess); err != nil {
		return app.UserLoan{}, fmt.Errorf("decoding allocation policy: %w", err)
	}
	cur, err := money.Lookup(code)
	if err != nil {
		return app.UserLoan{}, err
	}
	prepayment, err := parsePrepayment(prepay, cur)
	if err != nil {
		return app.UserLoan{}, fmt.Errorf("decoding prepayment terms: %w", err)
	}
	startDate, err := date.Parse(start)
	if err != nil {
		return app.UserLoan{}, fmt.Errorf("decoding loan start date: %w", err)
	}
	maturityDate, err := date.Parse(maturity)
	if err != nil {
		return app.UserLoan{}, fmt.Errorf("decoding loan maturity date: %w", err)
	}
	parsedRate, err := parseRate(rate)
	if err != nil {
		return app.UserLoan{}, fmt.Errorf("decoding loan rate: %w", err)
	}
	parsedDayCount, err := dayCountFrom(dayCount)
	if err != nil {
		return app.UserLoan{}, fmt.Errorf("decoding loan day count: %w", err)
	}
	parsedType, err := repaymentTypeFrom(repayment)
	if err != nil {
		return app.UserLoan{}, fmt.Errorf("decoding loan repayment type: %w", err)
	}
	parsedMode, err := roundingModeFrom(mode)
	if err != nil {
		return app.UserLoan{}, fmt.Errorf("decoding loan rounding mode: %w", err)
	}
	effective, err := date.Parse(effectiveFrom)
	if err != nil {
		return app.UserLoan{}, err
	}
	l.Contract = model.Contract{
		LoanID: model.ID(l.ID), Version: contractVersion, Currency: cur, EffectiveFrom: effective,
		NominalRate: parsedRate, DayCount: parsedDayCount,
		Type: parsedType, StartDate: startDate,
		MaturityDate: maturityDate, PaymentDay: int(day),
		Rounding:   money.Policy{Mode: parsedMode, Unit: int64(unit)},
		Prepayment: prepayment,
	}
	if principal != nil {
		l.Balance = money.FromMinor(*principal, cur)
	} else {
		l.Balance = money.Zero(cur)
	}
	if first != nil {
		l.OriginalPrincipal = money.FromMinor(*first, cur)
	} else {
		l.OriginalPrincipal = money.Zero(cur)
	}
	if asOf != nil {
		d, err := date.Parse(*asOf)
		if err != nil {
			return app.UserLoan{}, fmt.Errorf("decoding loan balance date: %w", err)
		}
		l.AsOf = d
	}
	if trust != nil {
		l.Trust = *trust
	}
	return l, nil
}

// ApplyLoanRevision persists every part of one edit in a single SQL statement.
func (s *Store) ApplyLoanRevision(ctx context.Context, loanID, userID string, r app.LoanRevision) error {
	if r.BalanceAsOf.IsZero() {
		r.BalanceAsOf = r.EffectiveFrom
	}
	c := model.Contract{}
	terms := r.Contract != nil
	if terms {
		c = *r.Contract
	} else {
		// PostgreSQL parses typed parameters even when the contract CTE is disabled.
		c.StartDate, c.MaturityDate = r.EffectiveFrom, r.EffectiveFrom
	}
	balance := int64(0)
	hasBalance := r.BalanceMinor != nil
	if hasBalance {
		balance = *r.BalanceMinor
	}
	var got string
	err := s.pool.QueryRow(ctx, q("ApplyLoanRevision"),
		loanID, userID, r.Rename, r.Name, r.Description,
		terms, uuid.NewString(), r.EffectiveFrom.String(), c.NominalRate.String(),
		dayCountName(c.DayCount), repaymentTypeName(c.Type), c.StartDate.String(),
		c.MaturityDate.String(), c.PaymentDay, roundingModeName(c.Rounding.Mode),
		c.Rounding.Unit, prepaymentJSON(c.Prepayment), hasBalance, uuid.NewString(), balance, r.Icon, r.OptionalExcluded, r.BalanceAsOf.String(),
	).Scan(&got)
	if errors.Is(err, pgx.ErrNoRows) {
		return app.ErrNotFound
	}
	return err
}

// RecordBalance stores the borrower's statement of what is owed, as a
// snapshot anchored today. Ownership lives in the query's predicate.
func (s *Store) RecordBalance(ctx context.Context, loanID, userID string, minor int64, asOf string) error {
	var got string
	err := s.pool.QueryRow(ctx, q("RecordBalanceSnapshot"),
		loanID, userID, uuid.NewString(), asOf, minor).Scan(&got)
	if errors.Is(err, pgx.ErrNoRows) {
		return app.ErrNotFound
	}
	return err
}

// prepaymentJSON is the stored form of the contract's prepayment terms. The
// default effect is stored as an absent key, so a contract that never stated
// one reads back as the default rather than as a choice somebody made.
func prepaymentJSON(p model.Prepayment) string {
	out := map[string]any{}
	if p.Effect != model.PrepayBorrowerChooses {
		out["effect"] = p.Effect.String()
	}
	if p.FeeBP > 0 {
		out["fee_bp"] = p.FeeBP
	}
	if p.MinAmount.Sign() > 0 {
		out["min_amount_minor"] = p.MinAmount.Minor()
	}
	if len(p.Charges) > 0 {
		var cs []map[string]any
		for _, c := range p.Charges {
			cs = append(cs, map[string]any{
				"from_year": c.FromYear, "through_year": c.ThroughYear, "percent_bp": c.PercentBP,
				"fixed_minor": c.Fixed.Minor(), "free_allowance_minor": c.FreeAllowance.Minor(),
				"min_charge_minor": c.MinCharge.Minor(), "max_charge_minor": c.MaxCharge.Minor(),
			})
		}
		out["charges"] = cs
	}
	b, _ := json.Marshal(out)
	return string(b)
}

// parsePrepayment reads the stored terms back. Unknown keys are ignored so
// a newer writer does not break an older reader; a malformed document is
// an error, because guessing a fee is worse than refusing.
func parsePrepayment(raw string, cur money.Currency) (model.Prepayment, error) {
	var doc struct {
		Effect  string `json:"effect"`
		FeeBP   int    `json:"fee_bp"`
		MinAmt  int64  `json:"min_amount_minor"`
		Charges []struct {
			FromYear, ThroughYear                      int
			PercentBP                                  int64 `json:"percent_bp"`
			Fixed, FreeAllowance, MinCharge, MaxCharge int64
		} `json:"charges"`
	}
	if raw == "" || raw == "{}" {
		return model.Prepayment{}, nil
	}
	if err := json.Unmarshal([]byte(raw), &doc); err != nil {
		return model.Prepayment{}, fmt.Errorf("prepayment terms: %w", err)
	}
	eff, err := model.ParsePrepaymentEffect(doc.Effect)
	if err != nil {
		return model.Prepayment{}, err
	}
	p := model.Prepayment{Effect: eff, FeeBP: doc.FeeBP, MinAmount: money.FromMinor(doc.MinAmt, cur)}
	for _, c := range doc.Charges {
		p.Charges = append(p.Charges, model.PrepaymentCharge{
			FromYear: c.FromYear, ThroughYear: c.ThroughYear, PercentBP: c.PercentBP,
			Fixed: money.FromMinor(c.Fixed, cur), FreeAllowance: money.FromMinor(c.FreeAllowance, cur),
			MinCharge: money.FromMinor(c.MinCharge, cur), MaxCharge: money.FromMinor(c.MaxCharge, cur),
		})
	}
	return p, nil
}

// ApprovePlan stores the borrower's commitment, replacing any earlier one.
func (s *Store) ApprovePlan(ctx context.Context, userID string, p app.ApprovedPlan) error {
	var got string
	return s.pool.QueryRow(ctx, q("ApprovePlan"),
		userID, p.Goal, p.CapMinor, p.Policy, p.Engine, p.PayoffDate, p.Months, p.InterestMinor).Scan(&got)
}

// ApprovedPlan returns the stored commitment, or nil when none exists.
func (s *Store) ApprovedPlan(ctx context.Context, userID string) (*app.ApprovedPlan, error) {
	rows, err := s.pool.Query(ctx, q("ApprovedPlan"), userID)
	if err != nil {
		return nil, err
	}
	p, err := pgx.CollectExactlyOneRow(rows, pgx.RowToStructByPos[app.ApprovedPlan])
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &p, nil
}

// ClearApprovedPlan withdraws the commitment.
func (s *Store) ClearApprovedPlan(ctx context.Context, userID string) error {
	var got string
	err := s.pool.QueryRow(ctx, q("ClearApprovedPlan"), userID).Scan(&got)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil
	}
	return err
}
