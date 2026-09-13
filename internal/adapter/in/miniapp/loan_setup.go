package miniapp

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"

	"github.com/andranikasd/marumbot/internal/app"
	"github.com/andranikasd/marumbot/pkg/core/date"
	"github.com/andranikasd/marumbot/pkg/core/money"
)

type setupLoanRequest struct {
	AccruedInterest          *json.Number `json:"accrued_interest_major"`
	Title                    string       `json:"title"`
	Currency                 string       `json:"currency"`
	Principal                json.Number  `json:"principal_major"`
	Remaining                json.Number  `json:"remaining_major"`
	Start                    string       `json:"start_date"`
	End                      string       `json:"maturity_date"`
	NextDue                  string       `json:"next_due_date"`
	Payment                  json.Number  `json:"payment_major"`
	Rate                     *json.Number `json:"rate_percent"`
	Method                   string       `json:"method"`
	Confirmed                bool         `json:"confirmed"`
	ProjectionTermsConfirmed bool         `json:"projection_terms_confirmed"`
}

func (r setupLoanRequest) validate(today date.Date) (app.LoanSetupStatement, error) {
	cur, err := money.Lookup(r.Currency)
	if err != nil {
		return app.LoanSetupStatement{}, ErrInvalid
	}
	if r.Remaining == "" || r.Principal == "" {
		return app.LoanSetupStatement{}, ErrInvalid
	}
	remaining, err := budgetMinor(r.Remaining, cur, true)
	if err != nil {
		return app.LoanSetupStatement{}, err
	}
	var accrued *int64
	if r.AccruedInterest != nil {
		value, parseErr := budgetMinor(*r.AccruedInterest, cur, true)
		if parseErr != nil || (remaining == 0 && value > 0) {
			return app.LoanSetupStatement{}, ErrInvalid
		}
		accrued = &value
	}
	rate := json.Number("0")
	if r.Rate != nil {
		rate = *r.Rate
		if rate == "" {
			return app.LoanSetupStatement{}, ErrInvalid
		}
	}
	next := date.Date{}
	day := 1
	payment := int64(0)
	if remaining > 0 {
		next, err = date.Parse(r.NextDue)
		if err != nil {
			return app.LoanSetupStatement{}, ErrInvalid
		}
		day = next.Day()
		if r.Payment == "" {
			return app.LoanSetupStatement{}, ErrInvalid
		}
		payment, err = budgetMinor(r.Payment, cur, false)
		if err != nil {
			return app.LoanSetupStatement{}, err
		}
	} else if r.NextDue != "" || (r.Payment != "" && r.Payment != "0") {
		return app.LoanSetupStatement{}, ErrInvalid
	}
	title := trimTo(r.Title, 60)
	if title == "" {
		title = "Loan"
	}
	draft, err := (LoanRequest{Title: title, Currency: r.Currency, PrincipalMajor: r.Principal, BalanceMajor: r.Remaining, RatePercent: rate, StartDate: r.Start, MaturityDate: r.End, PaymentDay: day, Method: r.Method}).Validate(today)
	if err != nil {
		return app.LoanSetupStatement{}, err
	}
	draft.Balance = money.FromMinor(remaining, cur)
	if !r.Confirmed || remaining > draft.Principal.Minor() || (remaining > 0 && (next.After(draft.Contract.MaturityDate) || !next.After(draft.Contract.StartDate))) {
		return app.LoanSetupStatement{}, ErrInvalid
	}
	return app.LoanSetupStatement{AccruedInterestMinor: accrued, Draft: draft, NextDue: next, PaymentMinor: payment, InterestKnown: r.Rate != nil, Confirmed: r.Confirmed, ProjectionTermsConfirmed: r.ProjectionTermsConfirmed}, nil
}

func (s *Server) SetupLoans() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ctx, user, ok := s.authed(w, r)
		if !ok {
			return
		}
		today, err := (app.PaymentService{Clock: s.Clock, Users: s.Users}).BusinessDate(ctx, user)
		if err != nil {
			paymentHTTPError(w, err)
			return
		}
		if r.Method == http.MethodGet {
			writeJSON(w, http.StatusOK, map[string]string{keyToday: today.String()})
			return
		}
		commands, key, _, ok := s.loanCommandRequest(w, r, false)
		if !ok {
			return
		}
		var input setupLoanRequest
		dec := json.NewDecoder(http.MaxBytesReader(w, r.Body, 4096))
		dec.DisallowUnknownFields()
		if err = dec.Decode(&input); err != nil || dec.Decode(new(any)) != io.EOF {
			writeJSON(w, http.StatusBadRequest, map[string]string{jsonError: "invalid_loan"})
			return
		}
		statement, err := input.validate(today)
		if err != nil {
			writeJSON(w, http.StatusUnprocessableEntity, map[string]string{jsonError: "invalid_loan"})
			return
		}
		statement.Draft.UserID = user
		receipt, err := commands.CreateSetup(ctx, key, statement)
		if err != nil {
			if errors.Is(err, app.ErrPaymentInvalid) {
				writeJSON(w, http.StatusUnprocessableEntity, map[string]string{jsonError: "invalid_loan"})
				return
			}
			if !loanCommandError(w, err) {
				paymentHTTPError(w, err)
			}
			return
		}
		writeJSON(w, http.StatusCreated, receipt)
	})
}
