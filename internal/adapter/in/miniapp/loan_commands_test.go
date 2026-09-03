package miniapp

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/andranikasd/marumbot/internal/app"
)

type loanCommandBoundaryStore struct{ app.LoanCommandStore }

func (loanCommandBoundaryStore) CreateLoan(context.Context, app.LoanDraft) (string, error) {
	panic("legacy write must not run")
}

func TestLoanCommandRequiresIdentityAndVersion(t *testing.T) {
	s := Server{Loans: loanCommandBoundaryStore{}}
	for _, tc := range []struct {
		name, key, version string
		want               int
	}{
		{"missing identity", "", "1", http.StatusBadRequest},
		{"missing version", "test-loan-key-0001", "", http.StatusPreconditionRequired},
		{"zero version", "test-loan-key-0001", "0", http.StatusPreconditionRequired},
		{"invalid version", "test-loan-key-0001", "latest", http.StatusPreconditionRequired},
		{"valid", "test-loan-key-0001", "3", http.StatusOK},
	} {
		t.Run(tc.name, func(t *testing.T) {
			req := httptest.NewRequestWithContext(t.Context(), http.MethodPatch, "/api/loans/example", http.NoBody)
			req.Header.Set("Idempotency-Key", tc.key)
			req.Header.Set("If-Match", tc.version)
			out := httptest.NewRecorder()
			_, _, _, ok := s.loanCommandRequest(out, req, true)
			if out.Code != tc.want || ok != (tc.want == http.StatusOK) {
				t.Fatalf("code=%d accepted=%v", out.Code, ok)
			}
		})
	}
}
