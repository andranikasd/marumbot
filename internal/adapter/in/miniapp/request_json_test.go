package miniapp

import (
	"net/http/httptest"
	"strings"
	"testing"
)

func TestFinancialRequestRefusesIgnoredTerms(t *testing.T) {
	for _, body := range []string{`{"title":"Loan","deferred_interest":true}`, `{"title":"Loan"} {"principal_major":1}`, `{"title":"Loan"}` + strings.Repeat(" ", maxRequest)} {
		var loan LoanRequest
		if err := decodeRequest(httptest.NewRecorder(), httptest.NewRequestWithContext(t.Context(), "POST", "/", strings.NewReader(body)), &loan); err == nil {
			t.Fatal("unsupported or excess request accepted")
		}
	}
	var loan LoanRequest
	if err := decodeRequest(httptest.NewRecorder(), httptest.NewRequestWithContext(t.Context(), "POST", "/", strings.NewReader(`{"title":"Loan"}`)), &loan); err != nil {
		t.Fatal(err)
	}
}
