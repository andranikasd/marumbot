package admin

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestAdminOriginPolicyBehindHTTPSProxy(t *testing.T) {
	for _, test := range []struct {
		origin, site string
		allowed      bool
	}{
		{"https://admin.example.test", "same-origin", true},
		{"https://attacker.example.test", "cross-site", false},
		{"null", "same-origin", false},
		{"https://admin.example.test", "cross-site", false},
	} {
		t.Run(test.origin+test.site, func(t *testing.T) {
			called := false
			handler := securityHeaders(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				called = true
				w.WriteHeader(http.StatusNoContent)
			}))
			// TLS terminates at Nginx; the application sees HTTP with the public Host.
			request := httptest.NewRequestWithContext(t.Context(), http.MethodPost, "http://admin.example.test/login", nil)
			request.Header.Set("Origin", test.origin)
			request.Header.Set("Sec-Fetch-Site", test.site)
			response := httptest.NewRecorder()
			handler.ServeHTTP(response, request)
			if called != test.allowed {
				t.Fatal("unexpected origin decision", response.Code)
			}
			if response.Header().Get("Referrer-Policy") != "strict-origin" {
				t.Fatal("form Origin can become null")
			}
			if response.Header().Get("Cache-Control") != "no-store" {
				t.Fatal("refusal can be cached")
			}
			if !test.allowed && (response.Code != http.StatusForbidden || !strings.Contains(response.Body.String(), "Return to sign in")) {
				t.Fatal("refusal needs status and recovery UI", response.Body.String())
			}
		})
	}
}
