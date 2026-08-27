package miniapp

import (
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
			s.Log.WarnContext(ctx, "miniapp auth rejected", "error", err)
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
			s.Log.WarnContext(ctx, "miniapp auth rejected", "error", err)
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
