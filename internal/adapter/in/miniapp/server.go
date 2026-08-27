package miniapp

import (
	"context"
	"embed"
	"encoding/json"
	"errors"
	"io"
	"io/fs"
	"log/slog"
	"net/http"
	"strings"

	"github.com/andranikasd/marumbot/internal/app"
	"github.com/andranikasd/marumbot/internal/obs"
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
	Users    app.UserStore
	Cipher   TagCipher
	Clock    app.Clock
	Log      *slog.Logger
}

// TagCipher is the part of the identity cipher this package needs: enough to
// find the account behind a Telegram id, and deliberately not enough to read one
// back out.
type TagCipher interface {
	Tag(id int64) string
}

// maxRequest caps the request body. A loan is a few hundred bytes.
const maxRequest = 1 << 15

// Handler returns the Mini App routes.
func (s *Server) Handler() http.Handler {
	sub, err := fs.Sub(assets, "web")
	if err != nil {
		panic(err) // an embed that does not contain its own directory is a build fault
	}
	mux := http.NewServeMux()
	mux.Handle("POST /api/loans", s.createLoan())
	mux.Handle("POST /api/budget", s.setBudget())
	mux.Handle("GET /api/loans", s.listLoans())
	mux.Handle("PATCH /api/loans/{id}", s.updateLoan())
	mux.Handle("DELETE /api/loans/{id}", s.deleteLoan())
	mux.Handle("/", s.static(http.FileServerFS(sub)))
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
		w.Header().Set("Cache-Control", "no-cache")
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
		if err := json.NewDecoder(io.LimitReader(r.Body, maxRequest)).Decode(&in); err != nil {
			http.Error(w, `{"error":"malformed"}`, http.StatusBadRequest)
			return
		}

		// Validated here, not only in the browser. The browser is the attacker's
		// machine; its checks tell the user quickly and prove nothing.
		draft, err := in.Validate()
		if err != nil {
			s.Log.InfoContext(ctx, "miniapp loan rejected", "error", err)
			writeJSON(w, http.StatusUnprocessableEntity, map[string]string{"error": err.Error()})
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
		draft.UserID = userID

		id, err := s.Loans.CreateLoan(ctx, draft)
		if err != nil {
			span.RecordError(err)
			s.Log.ErrorContext(ctx, "creating loan failed", "error", err)
			http.Error(w, `{"error":"internal"}`, http.StatusInternalServerError)
			return
		}
		writeJSON(w, http.StatusCreated, map[string]string{"id": id})
	})
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
		if err := json.NewDecoder(io.LimitReader(r.Body, maxRequest)).Decode(&in); err != nil {
			http.Error(w, `{"error":"malformed"}`, http.StatusBadRequest)
			return
		}
		cur, minor, err := in.Validate()
		if err != nil {
			writeJSON(w, http.StatusUnprocessableEntity, map[string]string{"error": err.Error()})
			return
		}

		userID, err := s.Users.ByTelegramTag(ctx, s.Cipher.Tag(v.User.ID))
		if err != nil {
			http.Error(w, `{"error":"unknown account"}`, http.StatusForbidden)
			return
		}
		if err := s.Budgets.SetBudget(ctx, userID, cur, minor); err != nil {
			span.RecordError(err)
			s.Log.ErrorContext(ctx, "recording the budget failed", "error", err)
			http.Error(w, `{"error":"internal"}`, http.StatusInternalServerError)
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"monthly_minor": minor, "currency": cur})
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
			out = append(out, map[string]any{
				"id": l.ID, "name": l.Name, "description": l.Description,
				"balance": l.Balance.String(), "currency": l.Contract.Currency.Code,
				"maturity":  l.Contract.MaturityDate.String(),
				"confirmed": l.Confirmed(),
			})
		}
		writeJSON(w, http.StatusOK, map[string]any{"loans": out})
	})
}

// updateLoan renames a loan. Only the borrower's own words change; the contract
// terms do not, because editing them would silently rewrite what a balance
// means without any record that it happened.
func (s *Server) updateLoan() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ctx, userID, ok := s.authed(w, r)
		if !ok {
			return
		}
		var in struct {
			Name        string `json:"name"`
			Description string `json:"description"`
		}
		if err := json.NewDecoder(io.LimitReader(r.Body, maxRequest)).Decode(&in); err != nil {
			http.Error(w, `{"error":"malformed"}`, http.StatusBadRequest)
			return
		}
		name := trimTo(in.Name, 60)
		if name == "" {
			writeJSON(w, http.StatusUnprocessableEntity, map[string]string{"error": "a name is required"})
			return
		}
		err := s.Editor.UpdateLoan(ctx, r.PathValue("id"), userID, name, trimTo(in.Description, 200))
		if errors.Is(err, app.ErrNotFound) {
			http.Error(w, `{"error":"not found"}`, http.StatusNotFound)
			return
		}
		if err != nil {
			s.Log.ErrorContext(ctx, "updating a loan failed", "error", err)
			http.Error(w, `{"error":"internal"}`, http.StatusInternalServerError)
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"id": r.PathValue("id")})
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
		err := s.Editor.ArchiveLoan(ctx, r.PathValue("id"), userID)
		if errors.Is(err, app.ErrNotFound) {
			http.Error(w, `{"error":"not found"}`, http.StatusNotFound)
			return
		}
		if err != nil {
			s.Log.ErrorContext(ctx, "archiving a loan failed", "error", err)
			http.Error(w, `{"error":"internal"}`, http.StatusInternalServerError)
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"id": r.PathValue("id")})
	})
}

func writeJSON(w http.ResponseWriter, code int, body any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(body)
}

// LoanRequest is what the form posts.
type LoanRequest struct {
	Title          string  `json:"title"`
	Description    string  `json:"description"`
	PrincipalMajor float64 `json:"principal_major"`
	Currency       string  `json:"currency"`
	RatePercent    float64 `json:"rate_percent"`
	Method         string  `json:"method"`
	StartDate      string  `json:"start_date"`
	MaturityDate   string  `json:"maturity_date"`
	PaymentDay     int     `json:"payment_day"`
}

// ErrInvalid marks a request the form should not have sent.
var ErrInvalid = errors.New("invalid loan")

func trimTo(s string, n int) string {
	s = strings.TrimSpace(s)
	if len(s) > n {
		return s[:n]
	}
	return s
}
