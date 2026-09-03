package miniapp

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

type settingsUsers struct {
	budgetTestUsers
	language, user string
	writes         int
}

func (s *settingsUsers) Locale(context.Context, string) (string, string, error) {
	return s.language, "Asia/Yerevan", nil
}

func (s *settingsUsers) SetLocale(_ context.Context, user, locale string) error {
	s.user = user
	s.language = locale
	s.writes++
	return nil
}

func TestSettingsShareBotLanguage(t *testing.T) {
	server := budgetTestServer(nil)
	users := &settingsUsers{language: "hy"}
	server.Users = users
	call := func(method, body string, auth bool) *httptest.ResponseRecorder {
		r := httptest.NewRequestWithContext(t.Context(), method, "/api/settings", strings.NewReader(body))
		if auth {
			r.Header.Set("X-Telegram-Init-Data", knownInitData())
		}
		out := httptest.NewRecorder()
		server.Handler().ServeHTTP(out, r)
		return out
	}
	if out := call(http.MethodGet, "", true); out.Code != 200 || !strings.Contains(out.Body.String(), `"locale":"hy"`) {
		t.Fatal("saved bot locale missing")
	}
	if out := call(http.MethodPost, `{"locale":"en"}`, false); out.Code != 401 || users.writes != 0 {
		t.Fatal("anonymous settings write")
	}
	if out := call(http.MethodPost, `{"locale":"en"}`, true); out.Code != 200 || users.language != "en" || users.user != "user-id" {
		t.Fatal("language was not saved to authenticated account")
	}
	for _, body := range []string{`{"locale":"ru"}`, `{"locale":""}`, `{"locale":"hy","user_id":"other"}`, `{"locale":"hy"}{}`} {
		if out := call(http.MethodPost, body, true); out.Code != 422 {
			t.Fatal("invalid language write accepted")
		}
	}
	if users.writes != 1 {
		t.Fatal("rejected request changed settings")
	}
}
