package miniapp

import (
	"errors"
	"net/http"
	"strconv"
	"strings"

	"github.com/andranikasd/marumbot/internal/app"
)

// Require explicit retry identity and the version originally displayed to the borrower.
func (s *Server) loanCommandRequest(w http.ResponseWriter, r *http.Request, versioned bool) (app.LoanCommands, string, int64, bool) {
	key := r.Header.Get("Idempotency-Key")
	if len(key) < 16 || len(key) > 128 {
		writeJSON(w, http.StatusBadRequest, map[string]string{jsonError: "idempotency_key_required"})
		return app.LoanCommands{}, "", 0, false
	}
	var version int64
	if versioned {
		var err error
		version, err = strconv.ParseInt(strings.Trim(r.Header.Get("If-Match"), "\""), 10, 64)
		if err != nil || version < 1 {
			writeJSON(w, http.StatusPreconditionRequired, map[string]string{jsonError: "loan_version_required"})
			return app.LoanCommands{}, "", 0, false
		}
	}
	store, ok := s.Loans.(app.LoanCommandStore)
	if !ok {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{jsonError: "unavailable"})
		return app.LoanCommands{}, "", 0, false
	}
	return app.LoanCommands{Store: store, Clock: s.Clock, Users: s.Users}, key, version, true
}

func loanCommandError(w http.ResponseWriter, err error) bool {
	if errors.Is(err, app.ErrConflict) {
		writeJSON(w, http.StatusConflict, map[string]string{jsonError: "loan_conflict"})
		return true
	}
	if errors.Is(err, app.ErrPaymentInvalid) {
		writeJSON(w, http.StatusBadRequest, map[string]string{jsonError: "invalid_command"})
		return true
	}
	return false
}
