package app

import "github.com/andranikasd/marumbot/pkg/core/amortisation"

// requiredReminderSchedule is private to reminder display/generation. The one
// bank-stated obligation is usable without a rate; its principal/interest split
// and later obligations are deliberately absent. Never use it for forecasting.
func requiredReminderSchedule(loan UserLoan) (amortisation.Schedule, error) {
	if loan.UnreconciledPayments {
		return amortisation.Schedule{}, ErrPaymentReconciliation
	}
	if (loan.InterestUnknown || (!loan.Contract.NotBeforeDue.IsZero() && loan.Contract.NotBeforeDue.Before(loan.AsOf))) && loan.Balance.Minor() > 0 {
		if !loan.Contract.HasScheduled || loan.Contract.NotBeforeDue.IsZero() || loan.Contract.ScheduledPayment.Minor() <= 0 {
			return amortisation.Schedule{}, ErrLoanInterestUnknown
		}
		return amortisation.Schedule{Rows: []amortisation.Row{{Due: loan.Contract.NotBeforeDue, Payment: loan.Contract.ScheduledPayment}}}, nil
	}
	return loan.Schedule()
}
