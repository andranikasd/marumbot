package miniapp

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/andranikasd/marumbot/internal/app"
)

type preferenceHTTPUsers struct {
	settingsUsers
	app.UserPreferenceStore
	seenUser, seenID string
}

func (f *preferenceHTTPUsers) UserPreferences(_ context.Context, user string) (app.UserPreferences, error) {
	f.seenUser = user
	return app.UserPreferences{Timezone: "Asia/Yerevan", QuietStart: 1320, QuietEnd: 480}, nil
}

func (f *preferenceHTTPUsers) ReminderOccurrence(_ context.Context, user, id string) (app.ReminderOccurrence, error) {
	f.seenUser = user
	f.seenID = id
	return app.ReminderOccurrence{ID: id, LoanID: "loan", DueDate: "2026-09-15", Required: true}, nil
}

func TestReminderPreferencesHTTPAuthAndStrictInput(t *testing.T) {
	s := budgetTestServer(nil)
	users := &preferenceHTTPUsers{}
	s.Users = users
	mux := http.NewServeMux()
	mux.Handle("GET /api/settings/reminders", s.UserPreferences())
	mux.Handle("POST /api/settings/reminders", s.UserPreferences())
	mux.Handle("GET /api/reminders/{id}", s.ReminderPreferences())
	mux.Handle("POST /api/reminders/{id}/snooze", s.ReminderPreferences())
	call := func(method, path, body string, auth bool) *httptest.ResponseRecorder {
		r := httptest.NewRequestWithContext(t.Context(), method, path, strings.NewReader(body))
		if auth {
			r.Header.Set("X-Telegram-Init-Data", knownInitData())
		}
		out := httptest.NewRecorder()
		mux.ServeHTTP(out, r)
		return out
	}
	id := "11111111-1111-4111-8111-111111111111"
	for _, path := range []string{"/api/settings/reminders", "/api/reminders/" + id} {
		if out := call("GET", path, "", false); out.Code != 401 {
			t.Fatal("anonymous read", out.Code)
		}
	}
	if out := call("GET", "/api/settings/reminders", "", true); out.Code != 200 || users.seenUser != "user-id" {
		t.Fatal("preferences owner", out.Code)
	}
	if out := call("GET", "/api/reminders/"+id, "", true); out.Code != 200 || users.seenID != id || users.seenUser != "user-id" || !strings.Contains(out.Body.String(), `"required":true`) {
		t.Fatal("occurrence context", out.Code, out.Body.String())
	}
	if out := call("GET", "/api/reminders/not-uuid", "", true); out.Code != 404 {
		t.Fatal("invalid occurrence", out.Code)
	}
	for _, body := range []string{`{"user_id":"other"}`, `{} {}`, `{"quiet_start":1.5}`, `null null`} {
		if out := call("POST", "/api/settings/reminders", body, true); out.Code != 422 {
			t.Fatal("invalid preferences accepted", body, out.Code)
		}
	}
	for _, body := range []string{`{"user_id":"other"}`, `{} {}`, `{"until":"tomorrow"}`, `{"occurrence_id":"other"}`} {
		if out := call("POST", "/api/reminders/"+id+"/snooze", body, true); out.Code != 422 {
			t.Fatal("invalid snooze accepted", body, out.Code)
		}
	}
}
