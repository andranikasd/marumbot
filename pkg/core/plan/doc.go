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
// machines produce the same bytes.
package plan
