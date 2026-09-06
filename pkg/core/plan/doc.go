// Package plan decides how a borrower should spend money on several loans.
//
// It is built in layers, and the layers are the point:
//
//  1. Contract arithmetic (package amortisation, package money) reproduces one
//     lender's loan exactly. It knows nothing about other loans or goals.
//  2. The dated portfolio simulator (sim.go) executes every loan, income and
//     payment on one chronological timeline with a real cash pool: money
//     arrives on a date, required instalments are paid on their dates, and
//     optional payments are made only from cash that is neither reserved for
//     an upcoming instalment nor below the borrower's floor. Cash never
//     disappears; a conservation identity is asserted at every event.
//  3. Feasibility is part of simulation: a policy that misses a required
//     payment fails with the first date and the exact shortfall, and no
//     partial result is returned as if it were a plan.
//  4. The optimiser (search.go) generates policies — a priority order, and per
//     loan a payment timing and a prepayment effect — simulates each once,
//     ranks the outcomes under the goal's written comparator, and returns a
//     certificate saying how strong the result is: proven under printed
//     assumptions, exhaustive over static orders, bounded heuristic, or a
//     comparison of named strategies.
//  5. Reports are built by callers from the Result and the Certificate; the
//     package never formats for a human.
//
// Everything here is deterministic integer arithmetic: no clock, no map
// iteration reaching output, no floating point money. Two runs on two
// machines produce the same bytes. The candidate simulations run on every
// available core, which changes only the order they finish in: enumeration
// fixes which candidates are attempted, and results merge in that order, so
// the answer does not depend on the number of cores.
//
// # Where things live
//
//   - input.go        what a caller declares, and what each field means
//   - sim.go          the dated simulator: one policy on one timeline
//   - result.go       what one simulated policy produces
//   - projection.go   the memoised door onto contract arithmetic
//   - search.go       the candidate set, and running it
//   - candidates.go   the axes whose product is that candidate set
//   - report.go       ranking, and the certificate that bounds the claim
//   - ladder.go       what a budget buys, and what buys a date
//   - spending.go     spending permission, which is not cash
//   - cash_routing.go receipts earmarked to particular loans
//
// # Two ways to declare money
//
// A borrower's declarations reach the engine in one of two shapes, and
// CashPlan.SeparateSpending is the single question that tells them apart.
//
// A funded budget is one figure. Cash.Monthly is both the money that arrives
// each cycle and the most that may go to loans, because under this model they
// are the same thing. A payday is optional: with none, the month's money is
// deemed to arrive on the first instalment date, so nothing can be paid early.
//
// Separately declared spending is two figures. Cash.Monthly is confirmed
// recurring funding -- money that actually arrives -- and Cash.Spending.Monthly
// is permission to spend it, which creates no cash of its own. Room left in a
// limit is not money on hand, and an expected receipt is not funding until it
// is confirmed. A payday is required, because permission is granted per cycle
// and a cycle needs an anchor.
//
// Worked, in the second shape: a borrower earning 400,000 on the 10th, who
// permits themselves 250,000 a month towards loans, holds 60,000 today and
// keeps 20,000 untouchable, declares
//
//	CashPlan{
//		Monthly:      400_000,          // funding: what arrives
//		PayDay:       10,
//		OpeningCash:  60_000,           // what is on hand at the valuation date
//		ReserveFloor: 20_000,           // never spent on optional payments
//		Spending:     &SpendingPlan{Monthly: 250_000},
//	}
//
// The simulator will pay required instalments from cash regardless of the
// limit -- a contract is owed whether or not permission was granted, and a
// limit too small to cover it is refused as infeasible, naming the date and
// the shortfall. Optional payments come only from cash that is above the
// reserve and within the remaining permission for the cycle.
package plan
