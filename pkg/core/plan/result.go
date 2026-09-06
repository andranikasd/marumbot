package plan

import (
	"github.com/andranikasd/marumbot/pkg/core/date"
	"github.com/andranikasd/marumbot/pkg/core/money"
)

// What one simulated policy produces: the dated actions, the per-cycle
// timeline and the totals a report is built from. Vocabulary only -- the
// arithmetic that fills these in lives in sim.go.

// ActionKind labels one payment in the first cycle.
type ActionKind uint8

const (
	// Instalment is a contractual required payment.
	Instalment ActionKind = iota
	// Extra is an optional payment the policy chose.
	Extra
)

func (k ActionKind) String() string {
	if k == Extra {
		return "extra"
	}
	return "instalment"
}

// Action is one dated payment the policy makes in the first cycle, in the
// order the borrower must make them.
type Action struct {
	On     date.Date
	LoanID string
	Loan   string
	Kind   ActionKind
	Amount money.Amount // cash out, fee included
	Fee    money.Amount
	// Saves is the interest this cycle that paying on this date rather than
	// on the due date avoids. Zero on the due date, and zero when the lender
	// does not credit early payment.
	Saves money.Amount
}

// MonthLoan is one loan's share of a cycle: what it was paid and where it
// ended, so a sheet can answer "whom do I pay, how much, in month seven".
type MonthLoan struct {
	Fees    money.Amount // cash fees paid this cycle, excluded from Extra
	ID      string
	Name    string
	Paid    money.Amount // everything handed to this loan this cycle, fees included
	Extra   money.Amount // the optional part, fees excluded
	Owed    money.Amount // balance after the cycle
	Cleared bool
	// Freed is the instalment this loan stops requiring, set on the cycle
	// it clears: the number a win is worth every month after it.
	Freed money.Amount
}

// MonthState is one cycle of a run, for timelines and comparators.
type MonthState struct {
	// HouseholdCash is transferred out of the debt pool under no_carry.
	HouseholdCash money.Amount
	Month         int
	On            date.Date    // the income date that opened the cycle
	Required      money.Amount // contractual instalments paid this cycle
	Extra         money.Amount // optional payments, fees excluded
	Fees          money.Amount
	Interest      money.Amount // settled this cycle
	Owed          money.Amount // balances at the end of the cycle
	Cash          money.Amount // physical cash carried, including restricted routed buckets
	Cleared       string       // a loan that reached zero this cycle, by name
	Loans         []MonthLoan  // per loan, in input order
}

// Result is what one policy produces over a whole run.
type Result struct {
	// HouseholdCash is retained outside the debt plan, never spent or erased.
	HouseholdCash money.Amount
	Policy        Policy
	PayoffDate    date.Date
	Months        int
	TotalInterest money.Amount
	TotalFees     money.Amount
	TotalPaid     money.Amount // required + extra + fees
	NextMonthOwed money.Amount // balances after the first cycle
	FirstClear    string
	FirstClearOn  date.Date
	FirstClearAt  int          // cycle
	FirstFreed    money.Amount // the instalment that loan no longer requires
	Actions       []Action
	Timeline      []MonthState
	// PeakRequired and FinalRequired bracket the contractual outflow: what
	// the borrower must pay in the heaviest cycle and in the last one. Extra
	// payments are excluded; relief is about obligations, not choices.
	PeakRequired  money.Amount
	FinalRequired money.Amount
	Prepayments   int
	// TimingCredited is true when at least one early payment was credited
	// by a lender that reduces principal on the day of payment.
	TimingCredited bool
	// Assumed is the number of instalments assumed paid to bring each loan
	// to the valuation date, when the anchor was older.
	Assumed map[string]int
}

// Cost is what the least-cost comparator minimises: interest plus fees.
func (r Result) Cost() money.Amount {
	c, _ := r.TotalInterest.Add(r.TotalFees)
	return c
}
