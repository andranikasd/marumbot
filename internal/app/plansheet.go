package app

import (
	"context"
	"fmt"
	"time"

	"github.com/andranikasd/marumbot/internal/i18n"
	"github.com/andranikasd/marumbot/pkg/core/date"
	"github.com/andranikasd/marumbot/pkg/core/money"
	"github.com/andranikasd/marumbot/pkg/core/plan"
)

// The approved plan and the sheet.
//
// Approval is a statement by the borrower, so it is stored: the goal, the
// policy and the figures as they were when they said yes. The living plan is
// still recomputed from the loans every time — an approved plan is a
// commitment to a goal, not a freeze of arithmetic — and the stored figures
// let any surface say how far reality has moved since the yes.

// ApprovedPlan is the stored commitment.
type ApprovedPlan struct {
	Goal          string
	CapMinor      int64
	Policy        string
	Engine        string
	PayoffDate    string
	Months        int
	InterestMinor int64
	ApprovedAt    time.Time
}

// Planner is what a surface needs to show and store plans; the Worker
// implements it.
type Planner interface {
	PlanSheet(ctx context.Context, userID string, goal *plan.Goal) (Sheet, error)
	ApprovePlanFor(ctx context.Context, userID string, g plan.Goal) (ApprovedPlan, error)
}

// PlanStore persists the commitment.
type PlanStore interface {
	ApprovePlan(ctx context.Context, userID string, p ApprovedPlan) error
	ApprovedPlan(ctx context.Context, userID string) (*ApprovedPlan, error)
	ClearApprovedPlan(ctx context.Context, userID string) error
}

// PlanGoal reconstructs the plan.Goal the commitment names.
func (p ApprovedPlan) PlanGoal(cur money.Currency) plan.Goal {
	switch p.Goal {
	case "fastest":
		return plan.Goal{Kind: plan.Fastest}
	case "first_win":
		return plan.Goal{Kind: plan.FirstWin}
	case "relief":
		return plan.Goal{Kind: plan.Relief, Cap: money.FromMinor(p.CapMinor, cur)}
	default:
		return plan.Goal{Kind: plan.LeastInterest}
	}
}

// Sheet is the whole plan, month by month, ready to render: every figure a
// spreadsheet would carry, in minor units with the currency named once.
type Sheet struct {
	Currency string       `json:"currency"`
	Goal     string       `json:"goal"`
	CapMinor int64        `json:"cap_minor,omitempty"`
	Approved bool         `json:"approved"` // this goal is the stored commitment
	AnyPlan  bool         `json:"any_plan"` // some commitment exists
	Summary  SheetSummary `json:"summary"`
	Months   []SheetMonth `json:"months"`
}

// SheetSummary is the header numbers.
type SheetSummary struct {
	OwedMinor     int64  `json:"owed_minor"`
	BudgetMinor   int64  `json:"budget_minor"`
	PayoffDate    string `json:"payoff_date"`
	Months        int    `json:"months"`
	InterestMinor int64  `json:"interest_minor"`
	FeesMinor     int64  `json:"fees_minor"`
	SavedMinor    int64  `json:"saved_minor"`  // vs paying only the minimum
	SavedMonths   int    `json:"saved_months"` // ditto
	Strength      string `json:"strength"`
}

// SheetMonth is one row of the sheet.
type SheetMonth struct {
	N             int         `json:"n"`
	On            string      `json:"on"` // ISO; the client renders it in its locale
	RequiredMinor int64       `json:"required_minor"`
	ExtraMinor    int64       `json:"extra_minor"`
	FeesMinor     int64       `json:"fees_minor"`
	InterestMinor int64       `json:"interest_minor"`
	OwedMinor     int64       `json:"owed_minor"`
	Cleared       string      `json:"cleared,omitempty"`
	Loans         []SheetLoan `json:"loans,omitempty"`
}

// SheetLoan is one loan inside a month: whom to pay, how much, what remains.
type SheetLoan struct {
	Name       string `json:"name"`
	PaidMinor  int64  `json:"paid_minor"`
	ExtraMinor int64  `json:"extra_minor"`
	OwedMinor  int64  `json:"owed_minor"`
	Cleared    bool   `json:"cleared,omitempty"`
	FreedMinor int64  `json:"freed_minor,omitempty"`
}

// PlanSheet computes the full sheet for a goal. The zero goal means "the
// approved plan's goal, or least interest".
func (w *Worker) PlanSheet(ctx context.Context, userID string, goal *plan.Goal) (Sheet, error) {
	loans, err := w.Loans.LoansForUser(ctx, userID, plan.MaxLoans+1)
	if err != nil {
		return Sheet{}, fmt.Errorf("listing loans: %w", err)
	}
	positions, owed, _, cur, err := w.positions(ctx, loans)
	if err != nil {
		return Sheet{}, err
	}
	if len(positions) == 0 {
		return Sheet{}, ErrNotFound
	}
	budget, err := w.Budgets.Budget(ctx, userID)
	if err != nil {
		return Sheet{}, fmt.Errorf("reading the budget: %w", err)
	}
	if !budget.Set || budget.Currency != cur.Code {
		return Sheet{}, ErrNotFound
	}

	var approved *ApprovedPlan
	if w.Plans != nil {
		if approved, err = w.Plans.ApprovedPlan(ctx, userID); err != nil {
			w.Log.WarnContext(ctx, "reading the approved plan failed", "error", err)
			approved = nil
		}
	}
	g := plan.Goal{Kind: plan.LeastInterest}
	if approved != nil {
		g = approved.PlanGoal(cur)
	}
	if goal != nil {
		g = *goal
	}

	in := plan.Input{
		ValuationDate: date.From(w.Clock.Now(), time.UTC),
		Cash:          plan.CashPlan{Monthly: budget.Monthly, PayDay: budget.PayDay},
		Loans:         positions,
	}
	rep, err := plan.Search(in, g)
	if err != nil {
		return Sheet{}, err
	}
	o := rep.Best

	sh := Sheet{
		Currency: cur.Code,
		Goal:     g.Kind.String(),
		CapMinor: g.Cap.Minor(),
		AnyPlan:  approved != nil,
		Approved: approved != nil && approved.Goal == g.Kind.String() && approved.CapMinor == g.Cap.Minor(),
		Summary: SheetSummary{
			OwedMinor: owed.Minor(), BudgetMinor: budget.Monthly.Minor(),
			PayoffDate: o.PayoffDate.String(), Months: o.Months,
			InterestMinor: o.TotalInterest.Minor(), FeesMinor: o.TotalFees.Minor(),
			Strength: string(rep.Certificate.Strength),
		},
	}
	if saved, err := rep.Minimum.Cost().Sub(o.Cost()); err == nil && saved.Sign() > 0 {
		sh.Summary.SavedMinor = saved.Minor()
		sh.Summary.SavedMonths = rep.Minimum.Months - o.Months
	}
	for _, m := range o.Timeline {
		sm := SheetMonth{
			N: m.Month, On: m.On.String(),
			RequiredMinor: m.Required.Minor(), ExtraMinor: m.Extra.Minor(),
			FeesMinor: m.Fees.Minor(), InterestMinor: m.Interest.Minor(),
			OwedMinor: m.Owed.Minor(), Cleared: m.Cleared,
		}
		for _, ml := range m.Loans {
			sm.Loans = append(sm.Loans, SheetLoan{
				Name: ml.Name, PaidMinor: ml.Paid.Minor(), ExtraMinor: ml.Extra.Minor(),
				OwedMinor: ml.Owed.Minor(), Cleared: ml.Cleared, FreedMinor: ml.Freed.Minor(),
			})
		}
		sh.Months = append(sh.Months, sm)
	}
	return sh, nil
}

// ApprovePlanFor stores the commitment to a goal, with the figures the
// engine produces right now, and answers with the stored row.
func (w *Worker) ApprovePlanFor(ctx context.Context, userID string, g plan.Goal) (ApprovedPlan, error) {
	if w.Plans == nil {
		return ApprovedPlan{}, fmt.Errorf("plan store not attached")
	}
	sh, err := w.PlanSheet(ctx, userID, &g)
	if err != nil {
		return ApprovedPlan{}, err
	}
	p := ApprovedPlan{
		Goal: g.Kind.String(), CapMinor: g.Cap.Minor(),
		Policy: sh.Goal, Engine: plan.EngineVersion,
		PayoffDate: sh.Summary.PayoffDate, Months: sh.Summary.Months,
		InterestMinor: sh.Summary.InterestMinor,
	}
	if err := w.Plans.ApprovePlan(ctx, userID, p); err != nil {
		return ApprovedPlan{}, fmt.Errorf("storing the approval: %w", err)
	}
	return p, nil
}

// approvePlan is the bot's side of the button: store, confirm, and say what
// was actually approved so a stale button cannot approve silently.
func (w *Worker) approvePlan(ctx context.Context, userID string, chat int64, l i18n.Locale, g plan.Goal) error {
	p, err := w.ApprovePlanFor(ctx, userID, g)
	if err != nil {
		return w.refuse(ctx, chat, l, err)
	}
	cur := w.DefaultCurrency
	text := i18n.T(l, "plan.approved",
		i18n.T(l, goalKey(g)),
		shortDate(l, mustParseDate(p.PayoffDate), date.From(w.Clock.Now(), time.UTC)),
		bare(money.FromMinor(p.InterestMinor, cur)))
	text = w.withTip(ctx, userID, l, text)
	markup := w.mainMenu(l)
	if w.MiniApp != "" {
		markup = map[string]any{keyInline: [][]map[string]any{{
			webAppButton(i18n.T(l, "plan.sheet_button"), w.miniURL("plan")),
		}}}
	}
	return w.Send.SendMessage(ctx, chat, text, markup)
}

func mustParseDate(s string) date.Date {
	d, err := date.Parse(s)
	if err != nil {
		return date.Date{}
	}
	return d
}
