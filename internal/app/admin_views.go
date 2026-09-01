package app

import (
	"context"
	"fmt"
	"math"
	"sort"
	"strings"
	"time"

	"github.com/andranikasd/marumbot/pkg/core/amortisation"
	"github.com/andranikasd/marumbot/pkg/core/date"
	"github.com/andranikasd/marumbot/pkg/core/model"
	"github.com/andranikasd/marumbot/pkg/core/money"
	"github.com/andranikasd/marumbot/pkg/core/plan"
)

// The views below are what the admin pages render beyond raw rows: trends,
// one account in full, and the engine's own reading of a loan. They live in
// the app layer because they combine the store with the core, and neither
// adapter may know about the other.

// DashboardView is the overview page.
type DashboardView struct {
	Overview       Overview
	Health         Health
	UsersByDay     []DayCount
	LoansByDay     []DayCount
	Recent         []CommandDetail
	CommandCounts  []StatusCount
	DeliveryCounts []StatusCount
}

// Dashboard gathers the overview page. A failure in a secondary panel is
// reported inside the view rather than failing the page: an operator looking
// for a stuck queue must not be locked out by a broken chart.
func (a *Admin) Dashboard(ctx context.Context, now time.Time) (DashboardView, error) {
	v := DashboardView{Health: a.Health(ctx)}
	var err error
	if v.Overview, err = a.Overview(ctx); err != nil {
		return v, err
	}
	days := 14
	users, _ := call(ctx, "UsersByDay", a.store.UsersByDay)
	loans, _ := call(ctx, "LoansByDay", a.store.LoansByDay)
	v.UsersByDay = fillDays(users, days, now)
	v.LoansByDay = fillDays(loans, days, now)
	v.CommandCounts, _ = call(ctx, "CommandCounts", a.store.CommandCounts)
	v.DeliveryCounts, _ = call(ctx, "DeliveryCounts", a.store.DeliveryCounts)
	if a.mod != nil {
		v.Recent, _ = a.mod.CommandsDetailed(ctx, "", 8)
	}
	return v, nil
}

// fillDays gives the chart one bar per day, oldest first, with zeros where
// nothing happened. Absent days would otherwise read as missing data.
func fillDays(have []DayCount, days int, now time.Time) []DayCount {
	byDay := map[string]int64{}
	for _, d := range have {
		byDay[d.Day.Format("2006-01-02")] = d.N
	}
	out := make([]DayCount, 0, days)
	start := now.UTC().Truncate(24*time.Hour).AddDate(0, 0, -(days - 1))
	for i := range days {
		d := start.AddDate(0, 0, i)
		out = append(out, DayCount{Day: d, N: byDay[d.Format("2006-01-02")]})
	}
	return out
}

// UserView is one account in full.
type UserView struct {
	User    UserRow
	Loans   []LoanRow
	Budgets []BudgetRow
	Convo   *ConvoRow
	// Plan is the advice the account currently receives for its first
	// budget, when the engine can produce one.
	Plan     *PlanSummary
	PlanNote string
	// Commands and Deliveries are the account's latest traffic in each
	// direction: what it sent the bot, and what the bot sent back.
	Commands   []CommandRow
	Deliveries []DeliveryRow
}

// User returns one account with its loans, budgets and pending dialogue.
func (a *Admin) User(ctx context.Context, id string, now time.Time) (UserView, error) {
	var v UserView
	var err error
	if v.User, err = call(ctx, "GetUser", func(c context.Context) (UserRow, error) { return a.store.GetUser(c, id) }); err != nil {
		return v, err
	}
	if v.Loans, err = call(ctx, "LoansByUser", func(c context.Context) ([]LoanRow, error) { return a.store.LoansByUser(c, id) }); err != nil {
		return v, err
	}
	v.Budgets, _ = call(ctx, "BudgetsForUser", func(c context.Context) ([]BudgetRow, error) { return a.store.BudgetsForUser(c, id) })
	v.Convo, _ = call(ctx, "ConversationState", func(c context.Context) (*ConvoRow, error) { return a.store.ConversationState(c, id) })
	v.Commands, _ = call(ctx, "CommandsForUser", func(c context.Context) ([]CommandRow, error) { return a.store.CommandsForUser(c, id, 20) })
	v.Deliveries, _ = call(ctx, "DeliveriesForUser", func(c context.Context) ([]DeliveryRow, error) { return a.store.DeliveriesForUser(c, id, 20) })
	v.Plan, v.PlanNote = a.planFor(ctx, id, now)
	return v, nil
}

// Projection is a schedule as the admin shows it: every row with the
// arithmetic behind it, and the totals.
type Projection struct {
	From          date.Date
	Rows          []ProjectionRow
	Instalment    money.Amount
	FinalPayment  money.Amount
	TotalPaid     money.Amount
	TotalInterest money.Amount
}

// ProjectionRow is one instalment with its working shown.
type ProjectionRow struct {
	amortisation.Row
	Explain string
}

// PlanSummary is the borrower's current advice, condensed for an operator.
type PlanSummary struct {
	Goal            string
	Policy          string
	Months          int
	Payoff          date.Date
	Interest        money.Amount
	Fees            money.Amount
	Owed            money.Amount // after the first cycle
	Evaluated       int
	Strength        string
	Eligibility     string
	Truncation      string
	TimingSaving    money.Amount
	VsAvalanche     money.Amount
	VsSnowball      money.Amount
	ClearedFirst    string
	ClearedOn       date.Date
	Actions         []plan.Action
	MinimumMonths   int
	MinimumInterest money.Amount
	Ladder          []plan.Rung
	Ties            []string
	PeakRequired    money.Amount
	FinalRequired   money.Amount
	Effect          string
	Certificate     plan.Certificate
}

// enrich adds the engine's reading to a loan view. Nothing here is fatal:
// the stored facts are the page, and the engine's opinion is a panel on it.
func (a *Admin) enrich(ctx context.Context, v *LoanView, now time.Time) {
	if a.engine == nil {
		v.ProjectionNote = "engine not attached"
		return
	}
	loans, err := a.engine.LoansForUser(ctx, v.Loan.UserID, 100)
	if err != nil {
		v.ProjectionNote = "reading the loan for projection: " + err.Error()
		return
	}
	var ln *UserLoan
	for i := range loans {
		if loans[i].ID == v.Loan.ID {
			ln = &loans[i]
			break
		}
	}
	if ln == nil {
		v.ProjectionNote = "the loan is archived or has no contract the engine can read"
		return
	}
	v.Projection, v.ProjectionNote = project(ln.Contract, ln.Balance, ln.AsOf)
	v.Plan, v.PlanNote = a.planFor(ctx, v.Loan.UserID, now)
	v.Support = supportText(v.Loan, ln, v.Projection)
}

// supportText is the loan in plain words, ready to paste into a chat with
// the borrower: the terms, the balance the engine works from, and the next
// three instalments. No markup, so it survives Telegram unchanged.
func supportText(l LoanDetail, ln *UserLoan, p *Projection) string {
	var b strings.Builder
	fmt.Fprintf(&b, "%s", l.Name)
	b.WriteString("\n")
	c := ln.Contract
	fmt.Fprintf(&b, "Terms: %s, %s, %s, paid on day %d, %s to %s\n",
		c.Type, c.NominalRate, c.DayCount, c.PaymentDay, c.StartDate, c.MaturityDate)
	fmt.Fprintf(&b, "Balance: %s as of %s (%s)\n", ln.Balance, ln.AsOf, ln.Trust)
	if p == nil {
		b.WriteString("No schedule can be projected from this balance.\n")
		return b.String()
	}
	fmt.Fprintf(&b, "Instalment: %s; final payment %s; %d instalments left\n", p.Instalment, p.FinalPayment, len(p.Rows))
	fmt.Fprintf(&b, "Interest to the end: %s; total to pay %s\n", p.TotalInterest, p.TotalPaid)
	b.WriteString("Next instalments:\n")
	for i, r := range p.Rows {
		if i == 3 {
			break
		}
		fmt.Fprintf(&b, "  %s  %s  (interest %s, principal %s)\n", r.Due, r.Payment, r.Interest, r.Principal)
	}
	return b.String()
}

// project builds a schedule and the explanation for each row.
func project(c model.Contract, balance money.Amount, from date.Date) (*Projection, string) {
	if balance.Sign() <= 0 {
		return nil, "the balance is zero; nothing to project"
	}
	s, err := amortisation.Build(c, balance, from)
	if err != nil {
		return nil, err.Error()
	}
	p := &Projection{
		From: from, Instalment: s.Instalment, FinalPayment: s.FinalPayment,
		TotalPaid: s.TotalPaid, TotalInterest: s.TotalInterest,
		Rows: make([]ProjectionRow, 0, len(s.Rows)),
	}
	for _, r := range s.Rows {
		p.Rows = append(p.Rows, ProjectionRow{
			Row:     r,
			Explain: amortisation.Explain(r, c.NominalRate, c.DayCount, c.Rounding),
		})
	}
	return p, ""
}

// planFor runs the borrower's own search — the same call the bot makes — so
// the operator can answer "why was I told this" with the actual numbers.
func (a *Admin) planFor(ctx context.Context, userID string, now time.Time) (*PlanSummary, string) {
	if a.engine == nil {
		return nil, "engine not attached"
	}
	loans, err := a.engine.LoansForUser(ctx, userID, plan.MaxLoans+1)
	if err != nil {
		return nil, err.Error()
	}
	budget, err := a.engine.Budget(ctx, userID)
	if err != nil {
		return nil, err.Error()
	}
	if !budget.Set {
		return nil, "no budget set"
	}
	var positions []plan.Position
	for _, ln := range loans {
		if ln.Balance.Sign() <= 0 || ln.Contract.Currency.Code != budget.Currency {
			continue
		}
		positions = append(positions, plan.Position{
			ID: ln.ID, Name: ln.Name, Contract: ln.Contract,
			Balance: ln.Balance, From: ln.AsOf, Excess: ln.Excess,
		})
	}
	if len(positions) == 0 {
		return nil, "no loan in the budget's currency"
	}
	in := plan.Input{
		ValuationDate: date.From(now, time.UTC),
		Cash:          budget.CashPlan(date.From(now, time.UTC)),
		Loans:         positions,
	}
	rep, err := plan.Search(in, plan.Goal{Kind: plan.LeastInterest})
	if err != nil {
		return nil, err.Error()
	}
	return summarise(rep), ""
}

func summarise(rep plan.Report) *PlanSummary {
	b := rep.Best
	c := rep.Certificate
	s := &PlanSummary{
		Goal: rep.Goal.String(), Policy: b.Policy.String(), Months: b.Months, Payoff: b.PayoffDate,
		Interest: b.TotalInterest, Fees: b.TotalFees, Owed: b.NextMonthOwed,
		Evaluated: c.Policies, Strength: string(c.Strength), Eligibility: c.Eligibility, Truncation: c.Truncation,
		TimingSaving: rep.TimingSaving, ClearedFirst: b.FirstClear, ClearedOn: b.FirstClearOn,
		Actions:       b.Actions,
		MinimumMonths: rep.Minimum.Months, MinimumInterest: rep.Minimum.TotalInterest,
		Ladder: rep.Ladder, Ties: rep.Ties,
		PeakRequired: b.PeakRequired, FinalRequired: b.FinalRequired,
		Effect: b.Policy.String(), Certificate: c,
	}
	s.VsAvalanche, _ = rep.Avalanche.Cost().Sub(b.Cost())
	s.VsSnowball, _ = rep.Snowball.Cost().Sub(b.Cost())
	return s
}

// PlaygroundInput is a loan typed into the engine page. Strings, because it
// comes from a form and the page must be able to echo back exactly what was
// entered beside what was rejected.
type PlaygroundInput struct {
	Currency  string
	Principal string
	Rate      string
	Method    string
	DayCount  string
	Start     string
	Maturity  string
	Day       string
	Unit      string // rounding unit in minor units; empty means the currency's
	Balance   string // optional: project from this balance instead of principal
	From      string // optional: the date the balance is as of
	// Bank is the lender's own instalment amounts, one per line, pasted from
	// the agreement. Empty means no comparison.
	Bank string
}

// PlaygroundView is the engine's answer to a typed loan.
type PlaygroundView struct {
	Input      PlaygroundInput
	Contract   *model.Contract
	Projection *Projection
	Error      string
	// Diffs has one entry per projected row when a bank schedule was
	// pasted, so the template can index it by row.
	Diffs     []RowDiff
	Compared  int
	Exact     int
	BankError string
}

// RowDiff is one bank instalment beside the engine's own.
type RowDiff struct {
	Bank  string // the amount as pasted, formatted; empty when the bank list ran out
	Delta string // signed bank − engine, "0" when exact
	Exact bool
	Has   bool // false past the end of the pasted list
}

// Playground runs the engine over a loan that is not stored anywhere. It is
// how an operator checks a borrower's "your number is wrong" against the
// bank's paperwork without touching the borrower's account.
func Playground(in PlaygroundInput) PlaygroundView {
	v := PlaygroundView{Input: in}
	c, principal, balance, from, err := parsePlayground(in)
	if err != nil {
		v.Error = err.Error()
		return v
	}
	v.Contract = &c
	if balance.Sign() <= 0 {
		balance = principal
	}
	var note string
	v.Projection, note = project(c, balance, from)
	if v.Projection == nil {
		v.Error = note
		return v
	}
	v.Diffs, v.Compared, v.Exact, v.BankError = diffBank(in.Bank, v.Projection.Rows, c.Currency)
	return v
}

// diffBank lines the lender's pasted instalments up against the projection.
// One amount per line, in row order; blank lines are skipped. A line that is
// not an amount stops the comparison and is reported, rather than silently
// shifting every row after it.
func diffBank(raw string, rows []ProjectionRow, cur money.Currency) ([]RowDiff, int, int, string) {
	if strings.TrimSpace(raw) == "" {
		return nil, 0, 0, ""
	}
	diffs := make([]RowDiff, len(rows))
	var compared, exact int
	i := 0
	for n, line := range strings.Split(raw, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		if i >= len(rows) {
			return diffs, compared, exact, fmt.Sprintf("line %d: more amounts than the engine has rows (%d)", n+1, len(rows))
		}
		bank, err := parseMajor(line, cur)
		if err != nil {
			return diffs, compared, exact, fmt.Sprintf("line %d: %v", n+1, err)
		}
		delta, err := bank.Sub(rows[i].Payment)
		if err != nil {
			return diffs, compared, exact, fmt.Sprintf("line %d: %v", n+1, err)
		}
		d := RowDiff{Bank: bank.String(), Has: true, Exact: delta.Sign() == 0, Delta: delta.String()}
		if delta.Sign() > 0 {
			d.Delta = "+" + d.Delta
		}
		if d.Exact {
			exact++
		}
		compared++
		diffs[i] = d
		i++
	}
	return diffs, compared, exact, ""
}

func parsePlayground(in PlaygroundInput) (model.Contract, money.Amount, money.Amount, date.Date, error) {
	var c model.Contract
	cur, err := money.Lookup(strings.ToUpper(strings.TrimSpace(in.Currency)))
	if err != nil {
		return c, money.Amount{}, money.Amount{}, date.Date{}, fmt.Errorf("currency: %w", err)
	}
	principal, err := parseMajor(in.Principal, cur)
	if err != nil || principal.Sign() <= 0 {
		return c, money.Amount{}, money.Amount{}, date.Date{}, fmt.Errorf("principal must be a positive amount")
	}
	balance := money.Zero(cur)
	if strings.TrimSpace(in.Balance) != "" {
		if balance, err = parseMajor(in.Balance, cur); err != nil {
			return c, money.Amount{}, money.Amount{}, date.Date{}, fmt.Errorf("balance: %w", err)
		}
	}
	var pct float64
	if _, err := fmt.Sscanf(strings.TrimSpace(in.Rate), "%g", &pct); err != nil || pct < 0 || pct > 200 {
		return c, money.Amount{}, money.Amount{}, date.Date{}, fmt.Errorf("rate must be a percentage between 0 and 200")
	}
	whole := int64(pct)
	micro := int64(math.Round((pct - float64(whole)) * 1_000_000))

	start, err := date.Parse(strings.TrimSpace(in.Start))
	if err != nil {
		return c, money.Amount{}, money.Amount{}, date.Date{}, fmt.Errorf("start date: %w", err)
	}
	maturity, err := date.Parse(strings.TrimSpace(in.Maturity))
	if err != nil {
		return c, money.Amount{}, money.Amount{}, date.Date{}, fmt.Errorf("maturity date: %w", err)
	}
	from := start
	if strings.TrimSpace(in.From) != "" {
		if from, err = date.Parse(strings.TrimSpace(in.From)); err != nil {
			return c, money.Amount{}, money.Amount{}, date.Date{}, fmt.Errorf("as-of date: %w", err)
		}
	}
	var day int
	if _, err := fmt.Sscanf(strings.TrimSpace(in.Day), "%d", &day); err != nil || day < 1 || day > 31 {
		return c, money.Amount{}, money.Amount{}, date.Date{}, fmt.Errorf("payment day must be 1 to 31")
	}

	typ := model.Annuity
	switch in.Method {
	case "declining":
		typ = model.DecliningPrincipal
	case "annuity", "":
	default:
		return c, money.Amount{}, money.Amount{}, date.Date{}, fmt.Errorf("unknown method %q", in.Method)
	}
	dc := money.Actual365
	switch in.DayCount {
	case "act365", "":
	case "act360":
		dc = money.Actual360
	case "30_360":
		dc = money.Thirty360
	case "act_act":
		dc = money.ActualActual
	default:
		return c, money.Amount{}, money.Amount{}, date.Date{}, fmt.Errorf("unknown day count %q", in.DayCount)
	}
	rounding := money.DefaultPolicy(cur)
	if u := strings.TrimSpace(in.Unit); u != "" {
		var unit int64
		if _, err := fmt.Sscanf(u, "%d", &unit); err != nil || unit <= 0 {
			return c, money.Amount{}, money.Amount{}, date.Date{}, fmt.Errorf("rounding unit must be a positive number of minor units")
		}
		rounding.Unit = unit
	}

	c = model.Contract{
		LoanID: "playground", Version: 1, Currency: cur, EffectiveFrom: start,
		NominalRate: money.RateFromPercent(whole, micro), DayCount: dc, Type: typ,
		StartDate: start, MaturityDate: maturity, PaymentDay: day, Rounding: rounding,
	}
	if err := c.Validate(); err != nil {
		return c, money.Amount{}, money.Amount{}, date.Date{}, err
	}
	return c, principal, balance, from, nil
}

// parseMajor reads "1,250,000.50" style input into minor units, exactly:
// the decimal part is scaled by the currency exponent without a float.
func parseMajor(s string, cur money.Currency) (money.Amount, error) {
	s = strings.ReplaceAll(strings.ReplaceAll(strings.TrimSpace(s), ",", ""), " ", "")
	if s == "" {
		return money.Amount{}, fmt.Errorf("empty amount")
	}
	whole, frac, _ := strings.Cut(s, ".")
	if len(frac) > int(cur.Exponent) {
		return money.Amount{}, fmt.Errorf("more than %d decimal places", cur.Exponent)
	}
	for len(frac) < int(cur.Exponent) {
		frac += "0"
	}
	var minor int64
	if _, err := fmt.Sscanf(whole+frac, "%d", &minor); err != nil {
		return money.Amount{}, fmt.Errorf("not a number: %q", s)
	}
	return money.FromMinor(minor, cur), nil
}

// SearchResult is what the global search box finds.
type SearchResult struct {
	Query string
	Loans []LoanRow
	Users []UserRow
}

// Search matches loans by name or identifier prefix and users by identifier
// prefix, over the most recent rows. Identifiers are UUIDs, so a few
// characters are enough; names are matched case-insensitively.
func (a *Admin) Search(ctx context.Context, q string) (SearchResult, error) {
	q = strings.TrimSpace(q)
	res := SearchResult{Query: q}
	if q == "" {
		return res, nil
	}
	lq := strings.ToLower(q)
	loans, err := a.Loans(ctx)
	if err != nil {
		return res, err
	}
	for _, l := range loans {
		if strings.HasPrefix(l.ID, lq) || strings.Contains(strings.ToLower(l.Name), lq) || strings.HasPrefix(l.UserID, lq) {
			res.Loans = append(res.Loans, l)
		}
	}
	users, err := a.Users(ctx)
	if err != nil {
		return res, err
	}
	for _, u := range users {
		if strings.HasPrefix(u.ID, lq) {
			res.Users = append(res.Users, u)
		}
	}
	sort.SliceStable(res.Loans, func(i, j int) bool { return res.Loans[i].CreatedAt.After(res.Loans[j].CreatedAt) })
	return res, nil
}
