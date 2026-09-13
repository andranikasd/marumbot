package app

import (
	"context"
	"sort"

	"github.com/andranikasd/marumbot/pkg/core/allocation"
	"github.com/andranikasd/marumbot/pkg/core/projection"
)

// ProjectionSettings contains only the borrower's source choices. It does not
// reinterpret or replace a previously funded budget or approved plan.
type ProjectionSettings struct {
	Enabled    bool                        `json:"enabled"`
	Version    int64                       `json:"version"`
	Currencies map[string]projection.Extra `json:"currencies"`
}
type ProjectionUpdate struct {
	Enabled         bool                        `json:"enabled"`
	ExpectedVersion int64                       `json:"expected_version"`
	Key             string                      `json:"idempotency_key"`
	Currencies      map[string]projection.Extra `json:"currencies"`
}
type ProjectionReader interface {
	ProjectionSettings(context.Context, string) (ProjectionSettings, error)
}
type projectionWriter interface {
	SaveProjectionSettings(context.Context, string, ProjectionUpdate) (int64, error)
}

func (s BudgetCommands) SetProjectionSettings(ctx context.Context, user string, in ProjectionUpdate) (int64, error) {
	return s.execute(ctx, user, in.Key, "monthly_projection", in, func(tx BudgetCommandTransaction) (int64, error) {
		if in.ExpectedVersion < 0 || len(in.Currencies) > 50 {
			return 0, ErrPaymentInvalid
		}
		for code, x := range in.Currencies {
			if !projectionCurrencyValid(code) || projection.ValidateExtra(x) != nil {
				return 0, ErrPaymentInvalid
			}
		}
		writer, ok := tx.(projectionWriter)
		if !ok {
			return 0, ErrPaymentInvalid
		}
		return writer.SaveProjectionSettings(ctx, user, in)
	})
}

type ProjectionService struct {
	Settings ProjectionReader
	Loans    LoanReader
	Clock    Clock
	Users    UserStore
}
type ProjectionResult struct {
	Today           string              `json:"today"`
	SettingsVersion int64               `json:"settings_version"`
	Enabled         bool                `json:"enabled"`
	Currencies      []projection.Result `json:"currencies"`
}

func (s ProjectionService) Build(ctx context.Context, user string) (ProjectionResult, error) {
	out := ProjectionResult{Currencies: []projection.Result{}}
	today, err := (PaymentService{Clock: s.Clock, Users: s.Users}).BusinessDate(ctx, user)
	if err != nil {
		return out, err
	}
	settings, err := s.Settings.ProjectionSettings(ctx, user)
	if err != nil {
		return out, err
	}
	loans, err := s.Loans.LoansForUser(ctx, user, projection.MaxLoans+1)
	if err != nil {
		return out, err
	}
	if len(loans) > projection.MaxLoans {
		return out, ErrPaymentInvalid
	}
	out.Today = today.String()
	out.SettingsVersion = settings.Version
	out.Enabled = settings.Enabled
	groups := map[string][]projection.Loan{}
	for _, l := range loans {
		if l.Balance.Sign() == 0 {
			if l.UnreconciledPayments {
				return out, ErrPaymentReconciliation
			}
			continue
		}
		reason := ""
		switch {
		case l.UnreconciledPayments:
			reason = "payment_reconciliation_required"
		case l.InterestUnknown:
			reason = "interest_needed"
		case !l.Contract.HasScheduled:
			reason = "bank_payment_needed"
		case !l.ProjectionTermsConfirmed && (l.Contract.AllocationPolicy.IsZero() || l.Excess != allocation.ExcessReducePrincipal):
			reason = "early_payment_rules_needed"
		}
		code := l.Balance.Currency().Code
		groups[code] = append(groups[code], projection.Loan{ID: l.ID, Name: l.Name, Contract: l.Contract, Balance: l.Balance, AsOf: l.AsOf, Reason: reason, OptionalExcluded: l.OptionalExcluded, OpeningInterestKnown: l.OpeningInterestKnown, OpeningInterestMinor: l.OpeningInterestMinor})
	}
	codes := make([]string, 0, len(groups))
	for code := range groups {
		codes = append(codes, code)
	}
	sort.Strings(codes)
	for _, code := range codes {
		if err := ctx.Err(); err != nil {
			return out, err
		}
		cur := groups[code][0].Balance.Currency()
		extra := settings.Currencies[code]
		if !settings.Enabled {
			extra = projection.Extra{}
		}
		result, err := projection.Build(today, cur, groups[code], extra)
		if err != nil {
			return out, err
		}
		out.Currencies = append(out.Currencies, result)
	}
	return out, nil
}
