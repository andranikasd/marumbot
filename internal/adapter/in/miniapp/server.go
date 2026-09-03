package miniapp

import (
	"context"
	"embed"
	"encoding/json"
	"errors"
	"io/fs"
	"log/slog"
	"net/http"
	"net/url"
	"regexp"
	"strings"

	"github.com/andranikasd/marumbot/internal/app"
	"github.com/andranikasd/marumbot/internal/design"
	"github.com/andranikasd/marumbot/internal/obs"
	"github.com/andranikasd/marumbot/pkg/core/date"
	"github.com/andranikasd/marumbot/pkg/core/model"
	"github.com/andranikasd/marumbot/pkg/core/money"
	"github.com/andranikasd/marumbot/pkg/core/plan"
)

//go:embed web
var assets embed.FS

// Server serves the Mini App and the one endpoint it calls.
type Server struct {
	BotToken string
	Loans    app.LoanWriter
	Editor   app.LoanEditor
	Reader   app.LoanReader
	Budgets  app.BudgetStore
	Required app.RequiredReader
	// Filed is told when a loan is created, so reminders exist from the
	// first day rather than from the next scheduler tick. Optional.
	Filed app.LoanFiledHook
	// Version stamps asset URLs for cache busting: the webview caches by
	// URL and a URL that changes with the deployment always misses.
	Version string
	// Planner computes the month-by-month sheet and stores approvals.
	Planner app.Planner
	// Reviser applies full loan edits: words overwritten, terms versioned,
	// balance re-anchored. Optional: without it PATCH stays a rename.
	Reviser LoanReviser
	// BudgetConfig records the complete form atomically.
	Payments      *app.PaymentService
	PaymentReader app.PaymentReader
	BudgetConfig  app.BudgetConfigurator
	Users         app.UserStore
	Cipher        TagCipher
	Clock         app.Clock
	Log           *slog.Logger
}

// LoanReviser applies a full loan edit; the Worker implements it. Declared
// here by the consumer, per house style.
type LoanReviser interface {
	ReviseLoan(ctx context.Context, loanID, userID string, e app.LoanEdit) error
}

// TagCipher is the part of the identity cipher this package needs: enough to
// find the account behind a Telegram id, and deliberately not enough to read one
// back out.
type TagCipher interface {
	Tag(id int64) string
}

// maxRequest caps the request body. A loan is a few hundred bytes.
const (
	maxRequest       = 1 << 15
	keyCurrency      = "currency"
	keyBlocked       = "blocked"
	errorUnavailable = "unavailable"
)

// Handler returns the Mini App routes.
func (s *Server) Handler() http.Handler {
	sub, err := fs.Sub(assets, "web")
	if err != nil {
		panic(err) // an embed that does not contain its own directory is a build fault
	}
	mux := http.NewServeMux()
	s.RegisterScenarioRoutes(mux)
	mux.Handle("POST /api/loans", s.createLoan())
	mux.Handle("POST /api/budget", s.setBudget())
	mux.Handle("GET /api/plan", s.planSheet())
	mux.Handle("GET /api/plans", s.planHistory())
	mux.Handle("GET /api/plan/budget-by-date", s.inverseBudget())
	mux.Handle("GET /api/plan/timeline", s.planTimeline())
	mux.Handle("GET /api/plan/comparisons", s.planComparisons())
	mux.Handle("GET /api/plan/stress", s.planStress())
	mux.Handle("GET /api/budget/policies", s.BudgetPolicies())
	mux.Handle("POST /api/budget/policies", s.SetBudgetPolicy())
	mux.Handle("POST /api/budget/funding", s.SetBudgetFunding())
	mux.Handle("GET /api/plans/{id}", s.historicalPlan())
	mux.Handle("POST /api/plans/activate", s.activateProposal())
	mux.Handle("POST /api/plan/approve", s.approvePlan())
	mux.Handle("GET /api/budget", s.getBudget())
	mux.Handle("GET /api/activity", s.allocatedActivity())
	mux.Handle("GET /api/payment-actuals", s.paymentActuals())
	mux.Handle("GET /api/plan-actuals", s.planActuals())
	mux.Handle("GET /api/settings", s.settings())
	mux.Handle("POST /api/settings", s.settings())
	mux.Handle("GET /api/settings/reminders", s.UserPreferences())
	mux.Handle("POST /api/settings/reminders", s.UserPreferences())
	mux.Handle("GET /api/reminders/{id}", s.ReminderPreferences())
	mux.Handle("POST /api/reminders/{id}/snooze", s.ReminderPreferences())
	mux.Handle("GET /api/loans/{id}/payments", s.paymentContext())
	mux.Handle("POST /api/loans/{id}/payments", s.recordPayment())
	mux.Handle("POST /api/loans/{id}/reconcile", s.reconcilePayment())
	mux.Handle("GET /api/loans", s.listLoans())
	mux.Handle("PATCH /api/loans/{id}", s.updateLoan())
	mux.Handle("DELETE /api/loans/{id}", s.deleteLoan())
	mux.Handle("GET /{$}", s.static(s.shell(sub)))
	mux.Handle("GET /index.html", s.static(s.shell(sub)))
	// What is deployed right now, for the app to compare against its own
	// stamp. Telegram keeps a minimised Mini App alive across deploys and
	// reopens the same instance, so the app has to notice a new build by
	// itself. The Worker answers this at the edge; this is the self-hosted
	// path and the fallback.
	mux.Handle("GET /version", s.version())
	// Assets live under a build-versioned prefix: /a/<version>/js/main.js.
	// The version in the path makes the content immutable for as long as
	// anyone might cache it — a deploy changes the path, and relative module
	// imports inherit the versioned prefix, so every file the app loads is
	// cacheable without ever being stale. This is what makes a warm open
	// instant while /{$} itself stays no-store.
	mux.Handle("GET /a/", s.immutable(http.StripPrefix("/a/", stripVersion(withTokens(sub, http.FileServerFS(sub))))))
	mux.Handle("/", s.static(withTokens(sub, http.FileServerFS(sub))))
	return mux
}

// static serves the form with headers that matter for a page inside a webview.
func (s *Server) static(h http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Telegram loads its own SDK from telegram.org, so that host has to be
		// allowed; nothing else is, and there is no inline script to permit.
		w.Header().Set("Content-Security-Policy",
			"default-src 'self'; script-src 'self' https://telegram.org; "+
				"style-src 'self' 'unsafe-inline'; connect-src 'self'; "+
				"img-src 'self' data:; frame-ancestors https://web.telegram.org https://*.telegram.org")
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("Referrer-Policy", "no-referrer")
		// The form changes with the deployment, and a stale one would collect
		// fields the server no longer accepts.
		w.Header().Set("Cache-Control", "no-store")
		h.ServeHTTP(w, r)
	})
}

// createLoan records a loan filed through the form.
func (s *Server) createLoan() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ctx, span := obs.ComponentWebhook.Enter(r.Context(), "miniapp.create_loan")
		defer span.End()

		// The credential arrives in a header rather than the body, so it does
		// not end up in a log the first time someone dumps a request.
		v, err := Verify(r.Header.Get("X-Telegram-Init-Data"), s.BotToken, s.Clock.Now())
		if err != nil {
			// No detail: which part failed is exactly what an attacker wants.
			s.Log.WarnContext(ctx, "miniapp auth rejected", "reason", err.Error())
			http.Error(w, `{"error":"unauthorised"}`, http.StatusUnauthorized)
			return
		}

		var in LoanRequest
		if err := decodeRequest(w, r, &in); err != nil {
			http.Error(w, `{"error":"malformed"}`, http.StatusBadRequest)
			return
		}

		// The account is found by tag. The user id from initData is proven, but
		// it is still only ever hashed before it reaches the database.
		userID, err := s.Users.ByTelegramTag(ctx, s.Cipher.Tag(v.User.ID))
		if err != nil {
			// Filing a loan before ever having messaged the bot is not a state
			// the flow can reach: the form is only opened from a bot message.
			s.Log.WarnContext(ctx, "miniapp user not found", "error", err)
			http.Error(w, `{"error":"unknown account"}`, http.StatusForbidden)
			return
		}
		today, err := (app.PaymentService{Clock: s.Clock, Users: s.Users}).BusinessDate(ctx, userID)
		if err != nil {
			writeJSON(w, http.StatusServiceUnavailable, map[string]string{jsonError: errorUnavailable})
			return
		}
		// Validated here, not only in the browser. The browser is the attacker's
		// machine; its checks tell the user quickly and prove nothing.
		draft, err := in.Validate(today)
		if err != nil {
			s.Log.InfoContext(ctx, "miniapp loan rejected", "error", err)
			writeJSON(w, http.StatusUnprocessableEntity, map[string]string{jsonError: err.Error()})
			return
		}

		draft.UserID = userID

		commands, key, _, ok := s.loanCommandRequest(w, r, false)
		if !ok {
			return
		}
		receipt, err := commands.Create(ctx, key, draft)
		id := receipt.ID
		if err != nil {
			span.RecordError(err)
			if loanCommandError(w, err) {
				return
			}
			if errors.Is(err, app.ErrTooManyLoans) {
				writeJSON(w, http.StatusUnprocessableEntity, map[string]string{jsonError: "too_many_loans"})
				return
			}
			s.Log.ErrorContext(ctx, "creating loan failed", "error", err)
			http.Error(w, `{"error":"internal"}`, http.StatusInternalServerError)
			return
		}
		if s.Filed != nil {
			if err := s.Filed.OnLoanFiled(ctx, userID, id); err != nil {
				// The loan exists; reminders will be rebuilt by the next
				// tick. Worth a log line, not a failed create.
				s.Log.WarnContext(ctx, "setting up reminders failed", "error", err)
			}
		}
		writeJSON(w, http.StatusCreated, receipt)
	})
}

// getBudget returns the saved budget beside what the loans require this month,
// so the screen can show the number against a fact rather than leave the user
// to guess whether it covers the minimums.
func (s *Server) getBudget() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ctx, userID, ok := s.authed(w, r)
		if !ok {
			return
		}
		today, err := (app.PaymentService{Clock: s.Clock, Users: s.Users}).BusinessDate(ctx, userID)
		if err != nil {
			http.Error(w, errorUnavailable, http.StatusServiceUnavailable)
			return
		}
		out := map[string]any{"today": today.String()}
		if b, err := s.Budgets.Budget(ctx, userID); err == nil && b.Set {
			out[keyCurrency] = b.Currency
			permission, err := b.PermissionOn(today)
			if err != nil {
				http.Error(w, "invalid budget policy", http.StatusUnprocessableEntity)
				return
			}
			out["monthly_major"] = major(permission)
			out["base_monthly_major"] = major(b.Monthly)
			out["pay_day"] = b.PayDay
			out["version"] = b.Version
			if b.Funding != nil {
				funding := *b.Funding
				cash, _, err := b.CashPlans(today)
				if err != nil {
					http.Error(w, "invalid budget policy", http.StatusUnprocessableEntity)
					return
				}
				funding.SpentMinor = cash.Spending.Spent.Minor()
				b.Opening = cash.OpeningCash
				funding.CashThrough = ""
				if !cash.CashThrough.IsZero() {
					funding.CashThrough = cash.CashThrough.String()
				}
				funding.Events = nil
				for _, event := range b.Funding.Events {
					on, err := date.Parse(event.On)
					if err == nil && (!on.Before(today) || event.Routing != nil || event.FromOpening) {
						funding.Events = append(funding.Events, event)
					}
				}
				b.Funding = &funding
			}
			out["funding"] = b.Funding
			out["currency_exponent"] = b.Monthly.Currency().Exponent
			if !b.OpeningAsOf.IsZero() {
				out["opening_major"] = major(b.Opening)
				out["opening_as_of"] = b.OpeningAsOf.String()
			}
			if b.Reserve.Sign() > 0 {
				out["reserve_major"] = major(b.Reserve)
			}
			if len(b.Overrides) > 0 {
				cur := b.Monthly.Currency()
				over := make(map[string]float64, len(b.Overrides))
				for k, v := range b.Overrides {
					over[k] = major(money.FromMinor(v, cur))
				}
				out["overrides"] = over
			}
		}
		if s.Required != nil {
			if req, cur, err := s.Required.RequiredThisMonth(ctx, userID); err == nil && req.Sign() > 0 {
				if out[keyCurrency] == nil {
					out[keyCurrency] = cur.Code
				}
				if out[keyCurrency] == cur.Code {
					out["required_major"] = major(req)
				}
			}
		}
		writeJSON(w, http.StatusOK, out)
	})
}

// ratePercent renders the stored parts-per-billion fraction as the percent
// figure the form shows. Display only, like major.
func ratePercent(r money.Rate) float64 { return float64(r) / 1e7 }

func methodName(t model.RepaymentType) string {
	if t == model.DecliningPrincipal {
		return "declining"
	}
	return "annuity"
}

// prepayEffectName is empty for the default, matching the form's "not stated"
// option rather than inventing a stated choice.
func prepayEffectName(e model.PrepaymentEffect) string {
	if e == model.PrepayBorrowerChooses {
		return ""
	}
	return e.String()
}

// major renders an amount as a decimal number of major units for the form.
// The form takes major units back and the server converts once, so this is
// the one place a money figure becomes a float, and it is display only.
func major(a money.Amount) float64 {
	scale := 1.0
	for i := uint8(0); i < a.Currency().Exponent; i++ {
		scale *= 10
	}
	return float64(a.Minor()) / scale
}

// setBudget records how much a borrower can put towards loans each month.
//
// A form rather than a chat answer. Typing "100000" after a prompt left the
// reply unhandled -- there was no conversation state to receive it -- and the
// bot answered with its help text, which reads as being ignored. A number with
// a currency is a form.
func (s *Server) setBudget() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ctx, span := obs.ComponentWebhook.Enter(r.Context(), "miniapp.set_budget")
		defer span.End()

		v, err := Verify(r.Header.Get("X-Telegram-Init-Data"), s.BotToken, s.Clock.Now())
		if err != nil {
			s.Log.WarnContext(ctx, "miniapp auth rejected", "reason", err.Error())
			http.Error(w, `{"error":"unauthorised"}`, http.StatusUnauthorized)
			return
		}

		var in BudgetRequest
		if err := decodeRequest(w, r, &in); err != nil {
			http.Error(w, `{"error":"malformed"}`, http.StatusBadRequest)
			return
		}
		cur, minor, payDay, err := in.Validate()
		if err != nil {
			writeJSON(w, http.StatusUnprocessableEntity, map[string]string{jsonError: err.Error()})
			return
		}
		curr := money.MustLookup(cur)
		var opening *int64
		if in.OpeningMajor != nil {
			o, err := in.ValidateOpening(curr)
			if err != nil {
				writeJSON(w, http.StatusUnprocessableEntity, map[string]string{jsonError: err.Error()})
				return
			}
			opening = &o
		}
		var overrides map[string]int64
		if in.Overrides != nil {
			if overrides, err = in.ValidateOverrides(curr); err != nil {
				writeJSON(w, http.StatusUnprocessableEntity, map[string]string{jsonError: err.Error()})
				return
			}
		}
		var reserve int64
		if in.ReserveMajor != nil {
			if reserve, err = in.ValidateReserve(curr); err != nil {
				writeJSON(w, http.StatusUnprocessableEntity, map[string]string{jsonError: err.Error()})
				return
			}
		}
		if in.Funding == nil && reserve > 0 && (opening == nil || reserve > *opening) {
			writeJSON(w, http.StatusUnprocessableEntity, map[string]string{
				jsonError: "protected reserve exceeds opening cash",
			})
			return
		}

		userID, err := s.Users.ByTelegramTag(ctx, s.Cipher.Tag(v.User.ID))
		if err != nil {
			http.Error(w, `{"error":"unknown account"}`, http.StatusForbidden)
			return
		}
		if s.BudgetConfig == nil {
			http.Error(w, `{"error":"unavailable"}`, http.StatusServiceUnavailable)
			return
		}
		today, err := (app.PaymentService{Clock: s.Clock, Users: s.Users}).BusinessDate(ctx, userID)
		if err != nil {
			http.Error(w, errorUnavailable, http.StatusServiceUnavailable)
			return
		}
		if in.Key != "" && in.AsOf == "" {
			writeJSON(w, http.StatusUnprocessableEntity, map[string]string{jsonError: "statement date required"})
			return
		}
		statementDay := today
		if in.AsOf != "" {
			statementDay, err = date.Parse(in.AsOf)
			if err != nil {
				writeJSON(w, http.StatusUnprocessableEntity, map[string]string{jsonError: "invalid statement date"})
				return
			}
		}
		if err := in.ValidateFunding(statementDay); err != nil {
			writeJSON(w, http.StatusUnprocessableEntity, map[string]string{jsonError: err.Error()})
			return
		}
		configuration := app.BudgetConfiguration{
			Key:     in.Key,
			Funding: in.Funding, ExpectedVersion: in.ExpectedVersion,
			UserID: userID, Currency: cur, MonthlyMinor: minor, PayDay: payDay,
			OpeningAsOf: statementDay, Overrides: overrides,
			ReserveMinor: reserve,
		}
		if opening != nil {
			configuration.OpeningMinor = *opening
		}
		if err := s.BudgetConfig.SetBudgetConfiguration(ctx, configuration); err != nil {
			if errors.Is(err, app.ErrPaymentInvalid) {
				writeJSON(w, http.StatusUnprocessableEntity, map[string]string{jsonError: "invalid budget command"})
				return
			}
			if errors.Is(err, app.ErrConflict) {
				writeJSON(w, http.StatusConflict, map[string]string{jsonError: "budget changed; reload before saving"})
				return
			}
			span.RecordError(err)
			s.Log.ErrorContext(ctx, "recording the budget failed", "error", err)
			http.Error(w, `{"error":"internal"}`, http.StatusInternalServerError)
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"monthly_minor": minor, keyCurrency: cur, "pay_day": payDay})
	})
}

// authed resolves the caller, or writes the refusal itself.
//
// Every endpoint needs the same two steps -- prove the initData, then find the
// account -- and repeating them is how one of them eventually gets left out.
func (s *Server) authed(w http.ResponseWriter, r *http.Request) (ctx context.Context, userID string, ok bool) {
	ctx = r.Context()
	v, err := Verify(r.Header.Get("X-Telegram-Init-Data"), s.BotToken, s.Clock.Now())
	if err != nil {
		// The reason is logged even though it is withheld from the response.
		// Four failures look identical from outside and need different fixes:
		// a stale payload means the form sat open, a signature mismatch means
		// the wrong bot token, an absent one means the page was opened outside
		// Telegram.
		s.Log.WarnContext(ctx, "miniapp auth rejected", "reason", err.Error())
		http.Error(w, `{"error":"unauthorised"}`, http.StatusUnauthorized)
		return ctx, "", false
	}
	userID, err = s.Users.ByTelegramTag(ctx, s.Cipher.Tag(v.User.ID))
	if err != nil {
		http.Error(w, `{"error":"unknown account"}`, http.StatusForbidden)
		return ctx, "", false
	}
	return ctx, userID, true
}

// listLoans backs the management screen.
func (s *Server) listLoans() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ctx, userID, ok := s.authed(w, r)
		if !ok {
			return
		}
		loans, err := s.Reader.LoansForUser(ctx, userID, 50)
		if err != nil {
			s.Log.ErrorContext(ctx, "listing loans failed", "error", err)
			http.Error(w, `{"error":"internal"}`, http.StatusInternalServerError)
			return
		}
		out := make([]map[string]any, 0, len(loans))
		for _, l := range loans {
			row := map[string]any{
				"id": l.ID, "mutation_version": l.MutationVersion, "name": l.Name, "description": l.Description, "currency_exponent": l.Contract.Currency.Exponent, "icon": l.Icon, "optional_excluded": l.OptionalExcluded,
				"balance": l.Balance.String(), "balance_major": major(l.Balance),
				keyCurrency: l.Contract.Currency.Code,
				"maturity":  l.Contract.MaturityDate.String(),
				"confirmed": l.Confirmed(),
				// When the balance was stated, so the screen can say "your
				// figure, 2 May" beside it rather than present it as today's.
				"balance_as_of": l.AsOf.String(),
				// The contract terms, so the edit form can prefill what is
				// actually stored rather than make the user re-type it.
				"start":         l.Contract.StartDate.String(),
				"payment_day":   l.Contract.PaymentDay,
				"rate_percent":  ratePercent(l.Contract.NominalRate),
				"method":        methodName(l.Contract.Type),
				"prepay_effect": prepayEffectName(l.Contract.Prepayment.Effect),
			}
			if l.OriginalPrincipal.Sign() > 0 && l.OriginalPrincipal.Cmp(l.Balance) > 0 {
				row["original_major"] = major(l.OriginalPrincipal)
			}
			// The next instalment, projected the same way the bot projects it,
			// so the summary card and the chat cannot disagree. Absent when the
			// schedule cannot be built; the card then shows a dash, not a zero.
			row["needs_reconciliation"] = l.UnreconciledPayments
			if s, err := l.Schedule(); err == nil && len(s.Rows) > 0 {
				row["next_due"] = s.Rows[0].Due.String()
				row["next_payment_major"] = major(s.Rows[0].Payment)
			}
			out = append(out, row)
		}
		writeJSON(w, http.StatusOK, map[string]any{"loans": out})
	})
}

// updateLoan edits a loan. The borrower's own words are overwritten; contract
// terms are never edited in place -- a full patch becomes a NEW contract
// version through the Reviser, so every past balance keeps meaning what it
// meant. A patch without terms (the old client's shape) stays a rename.
func (s *Server) updateLoan() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ctx, userID, ok := s.authed(w, r)
		if !ok {
			return
		}
		var in LoanEditRequest
		if err := decodeRequest(w, r, &in); err != nil {
			http.Error(w, `{"error":"malformed"}`, http.StatusBadRequest)
			return
		}
		loanID := r.PathValue("id")
		commands, key, expected, ok := s.loanCommandRequest(w, r, true)
		if !ok {
			return
		}

		if !in.FullEdit() {
			name := trimTo(in.Name, 60)
			if name == "" {
				writeJSON(w, http.StatusUnprocessableEntity, map[string]string{"error": "a name is required"})
				return
			}
			receipt, err := commands.Rename(ctx, userID, loanID, key, expected, name, trimTo(in.Description, 200))
			if loanCommandError(w, err) {
				return
			}
			if errors.Is(err, app.ErrNotFound) {
				http.Error(w, `{"error":"not found"}`, http.StatusNotFound)
				return
			}
			if err != nil {
				s.Log.ErrorContext(ctx, "updating a loan failed", "error", err)
				http.Error(w, `{"error":"internal"}`, http.StatusInternalServerError)
				return
			}
			writeJSON(w, http.StatusOK, receipt)
			return
		}

		if s.Reviser == nil {
			http.Error(w, `{"error":"unavailable"}`, http.StatusServiceUnavailable)
			return
		}
		// Decode using immutable, owned currency even after archive. The command
		// then returns an existing receipt before checking active-loan state.
		currency, err := commands.Currency(ctx, loanID, userID)
		if errors.Is(err, app.ErrNotFound) {
			http.Error(w, `{"error":"not found"}`, http.StatusNotFound)
			return
		}
		if err != nil {
			s.Log.ErrorContext(ctx, "reading a loan for edit failed", "error", err)
			http.Error(w, `{"error":"internal"}`, http.StatusInternalServerError)
			return
		}
		edit, err := in.Validate(currency)
		if err != nil {
			s.Log.InfoContext(ctx, "miniapp loan edit rejected", "error", err)
			writeJSON(w, http.StatusUnprocessableEntity, map[string]string{jsonError: err.Error()})
			return
		}
		today, err := (app.PaymentService{Clock: s.Clock, Users: s.Users}).BusinessDate(ctx, userID)
		if err != nil {
			writeJSON(w, http.StatusServiceUnavailable, map[string]string{jsonError: errorUnavailable})
			return
		}
		if edit.BalanceAsOf.After(today) {
			writeJSON(w, http.StatusUnprocessableEntity, map[string]string{jsonError: "balance date cannot be in the future"})
			return
		}
		edit.Key, edit.ExpectedVersion = key, expected
		if err := s.Reviser.ReviseLoan(ctx, loanID, userID, edit); err != nil {
			if loanCommandError(w, err) {
				return
			}
			if errors.Is(err, app.ErrSnapshotContractDate) {
				writeJSON(w, http.StatusUnprocessableEntity, map[string]string{jsonError: err.Error()})
				return
			}
			if errors.Is(err, app.ErrNotFound) {
				http.Error(w, `{"error":"not found"}`, http.StatusNotFound)
				return
			}
			s.Log.ErrorContext(ctx, "revising a loan failed", "error", err)
			http.Error(w, `{"error":"internal"}`, http.StatusInternalServerError)
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"id": loanID})
	})
}

// deleteLoan hides a loan. The ledger behind it is kept: a balance is only
// checkable because its events can be replayed, and a borrower who removes a
// loan by mistake would otherwise lose that permanently.
func (s *Server) deleteLoan() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ctx, userID, ok := s.authed(w, r)
		if !ok {
			return
		}
		commands, key, expected, ok := s.loanCommandRequest(w, r, true)
		if !ok {
			return
		}
		receipt, err := commands.Archive(ctx, userID, r.PathValue("id"), key, expected)
		if loanCommandError(w, err) {
			return
		}
		if errors.Is(err, app.ErrNotFound) {
			http.Error(w, `{"error":"not found"}`, http.StatusNotFound)
			return
		}
		if err != nil {
			s.Log.ErrorContext(ctx, "archiving a loan failed", "error", err)
			http.Error(w, `{"error":"internal"}`, http.StatusInternalServerError)
			return
		}
		writeJSON(w, http.StatusOK, receipt)
	})
}

func writeJSON(w http.ResponseWriter, code int, body any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(body)
}

// LoanRequest is what the form posts.
type LoanRequest struct {
	Icon             string  `json:"icon"`
	OptionalExcluded bool    `json:"optional_excluded"`
	Title            string  `json:"title"`
	Description      string  `json:"description"`
	PrincipalMajor   float64 `json:"principal_major"`
	// BalanceMajor is what is owed TODAY, for a loan that has been running.
	// Zero means "same as principal": a loan filed on its drawdown date.
	BalanceMajor float64 `json:"balance_major"`
	Currency     string  `json:"currency"`
	RatePercent  float64 `json:"rate_percent"`
	Method       string  `json:"method"`
	// PrepayEffect is what an early payment does: shorten_term,
	// reduce_instalment, or empty when the borrower has not said.
	PrepayEffect string `json:"prepay_effect"`
	StartDate    string `json:"start_date"`
	MaturityDate string `json:"maturity_date"`
	PaymentDay   int    `json:"payment_day"`
}

// jsonError is the one key every error body carries.
const jsonError = "error"

// ErrInvalid marks a request the form should not have sent.
var ErrInvalid = errors.New("invalid loan")

// trimTo caps a name at n runes. Runes, not bytes: an Armenian name cut at a
// byte boundary is invalid UTF-8, which Postgres rejects with
//
//	invalid byte sequence for encoding "UTF8"
//
// and the create turns into a 500.
func trimTo(s string, n int) string {
	s = strings.TrimSpace(s)
	if r := []rune(s); len(r) > n {
		return string(r[:n])
	}
	return s
}

// planSheet returns the full month-by-month plan for a goal. Without a goal
// parameter it follows the approved plan, or least interest.
func (s *Server) planSheet() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ctx, userID, ok := s.authed(w, r)
		if !ok {
			return
		}
		if s.Planner == nil {
			http.Error(w, `{"error":"unavailable"}`, http.StatusServiceUnavailable)
			return
		}
		var goal *plan.Goal
		if tok := r.URL.Query().Get("goal"); tok != "" {
			g := app.GoalFromToken(tok)
			goal = &g
		}
		sheet, err := s.Planner.PlanSheet(ctx, userID, goal)
		if errors.Is(err, app.ErrCashRoutingStale) {
			writeJSON(w, http.StatusOK, map[string]string{keyBlocked: "cash_routing_stale"})
			return
		}
		if errors.Is(err, app.ErrFundingRequired) {
			writeJSON(w, http.StatusOK, map[string]string{keyBlocked: "funding_required"})
			return
		}
		if errors.Is(err, app.ErrPaymentReconciliation) {
			writeJSON(w, http.StatusOK, map[string]string{keyBlocked: "payment_reconciliation"})
			return
		}
		if err != nil {
			if errors.Is(err, app.ErrNotFound) {
				writeJSON(w, http.StatusOK, map[string]any{"empty": true})
				return
			}
			var inf *plan.InfeasibleError
			if errors.As(err, &inf) {
				// A budget that cannot meet a date is not a failure page; it
				// is a fact with a fix, and the screen offers the fix.
				writeJSON(w, http.StatusOK, map[string]any{
					keyBlocked: "budget_low", "on": inf.On.String(),
					keyCurrency:      inf.Required.Currency().Code,
					"required_major": major(inf.Required), "short_major": major(inf.Shortfall),
				})
				return
			}
			var st *plan.StaleBalanceError
			if errors.As(err, &st) {
				// Same idea: an old balance is a fact with a fix -- confirm
				// the figure -- not an error page.
				writeJSON(w, http.StatusOK, map[string]any{
					keyBlocked: "balance_stale", "as_of": st.AsOf.String(), "loan_id": st.LoanID,
				})
				return
			}
			s.Log.ErrorContext(ctx, "building the plan sheet failed", "error", err)
			writeJSON(w, http.StatusUnprocessableEntity, map[string]string{jsonError: "plan_failed"})
			return
		}
		writeJSON(w, http.StatusOK, sheet)
	})
}

// approvePlan stores the borrower's yes to the goal in the request body.
func (s *Server) approvePlan() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ctx, userID, ok := s.authed(w, r)
		if !ok {
			return
		}
		if s.Planner == nil {
			http.Error(w, `{"error":"unavailable"}`, http.StatusServiceUnavailable)
			return
		}
		var in struct {
			Goal string `json:"goal"`
		}
		if err := decodeRequest(w, r, &in); err != nil {
			http.Error(w, `{"error":"malformed"}`, http.StatusBadRequest)
			return
		}
		p, err := s.Planner.ApprovePlanFor(ctx, userID, app.GoalFromToken(in.Goal))
		if err != nil {
			s.Log.ErrorContext(ctx, "approving the plan failed", "error", err)
			writeJSON(w, http.StatusUnprocessableEntity, map[string]string{jsonError: "approve_failed"})
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"approved": true, "goal": p.Goal, "payoff": p.PayoffDate})
	})
}

// version reports the deployed build. Never cached: its one job is to
// disagree with a stale page.
func (s *Server) version() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Cache-Control", "no-store")
		writeJSON(w, http.StatusOK, map[string]string{"version": s.Version})
	})
}

// shell serves index.html with its assets rewritten under the versioned
// prefix and every module preloaded. The prefix makes the assets immutable;
// the preloads collapse the ES-module waterfall — nine sequential fetches
// through the Worker and the container — into one parallel round trip.
func (s *Server) shell(sub fs.FS) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, err := fs.ReadFile(sub, "index.html")
		if err != nil {
			http.Error(w, errorUnavailable, http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = w.Write(stampShell(b, s.Version))
	})
}

// stampShell rewrites every local asset reference under the versioned
// prefix. One rule for stylesheets, scripts and preloads alike, so a file
// added to the shell is versioned by construction. The Worker applies the
// same rewrite when it serves the shell from the edge.
func stampShell(b []byte, version string) []byte {
	v := url.PathEscape(version)
	out := shellRef.ReplaceAll(b, []byte(`$1="a/`+v+`/$2"`))
	return out
}

// shellRef matches href/src attributes pointing at local assets.
var shellRef = regexp.MustCompile(`(href|src)="((?:js/[^"]+|styles\.css))"`)

// stripVersion drops the leading version segment: <version>/js/main.js →
// js/main.js. The value is never interpreted; it exists to make the URL new.
func stripVersion(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, rest, ok := strings.Cut(strings.TrimPrefix(r.URL.Path, "/"), "/")
		if !ok {
			http.NotFound(w, r)
			return
		}
		r2 := r.Clone(r.Context())
		r2.URL.Path = "/" + rest
		next.ServeHTTP(w, r2)
	})
}

// withTokens serves the stylesheet with the shared design tokens in front
// of it. The tokens live in one package for every surface, and the shell's
// stamp rewrite and the Worker's both know only about styles.css, so the
// two are joined here rather than linked as a second file.
func withTokens(sub fs.FS, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/styles.css" {
			next.ServeHTTP(w, r)
			return
		}
		b, err := fs.ReadFile(sub, "styles.css")
		if err != nil {
			http.Error(w, errorUnavailable, http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "text/css; charset=utf-8")
		_, _ = w.Write(design.Tokens)
		_, _ = w.Write(b)
	})
}

// immutable is the cache policy for content whose URL names its version.
func (s *Server) immutable(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
		w.Header().Set("X-Content-Type-Options", "nosniff")
		next.ServeHTTP(w, r)
	})
}
