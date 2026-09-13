package miniapp

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/andranikasd/marumbot/internal/app"
)

type sessionUsers struct {
	budgetTestUsers
	lookup error
	input  app.UpsertUser
	writes int
}

func (u *sessionUsers) ByTelegramTag(context.Context, string) (string, error) {
	return "user-id", u.lookup
}

func (u *sessionUsers) UpsertByTelegram(_ context.Context, in app.UpsertUser) (app.Account, error) {
	u.input = in
	u.writes++
	u.lookup = nil
	return app.Account{}, nil
}

type sessionCipher struct{ budgetTestCipher }

func (sessionCipher) Seal(id int64) ([]byte, error) {
	if id != 42 {
		return nil, errors.New("unexpected identity")
	}
	return []byte("sealed-identity"), nil
}

func TestDirectMiniAppSession(t *testing.T) {
	for _, tc := range []struct {
		name           string
		auth           bool
		lookup         error
		status, writes int
	}{
		{"first launch", true, app.ErrNotFound, 200, 1}, {"existing account", true, nil, 200, 0}, {"store outage", true, errors.New("unavailable"), 503, 0}, {"missing identity", false, app.ErrNotFound, 401, 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s := budgetTestServer(nil)
			u := &sessionUsers{lookup: tc.lookup}
			s.Users = u
			s.Cipher = sessionCipher{}
			s.DefaultTimezone = "Asia/Yerevan"
			r := httptest.NewRequestWithContext(t.Context(), http.MethodPost, "/api/session", nil)
			if tc.auth {
				r.Header.Set("X-Telegram-Init-Data", knownInitData())
			}
			w := httptest.NewRecorder()
			s.Handler().ServeHTTP(w, r)
			if w.Code != tc.status || u.writes != tc.writes {
				t.Fatalf("status=%d writes=%d", w.Code, u.writes)
			}
			if u.writes > 0 && (u.input.RemindersEnabled == nil || *u.input.RemindersEnabled || string(u.input.UserSealed) != "sealed-identity" || u.input.UserTag != u.input.ChatTag || u.input.Timezone != "Asia/Yerevan" || u.input.Locale != "hy") {
				t.Fatal("new account source preferences or sealed identity lost")
			}
		})
	}
}

type unavailableSessionUser struct{ sessionUsers }

func (u *unavailableSessionUser) UpsertByTelegram(context.Context, app.UpsertUser) (app.Account, error) {
	return app.Account{ID: "existing"}, nil
}

func TestDirectSessionDoesNotReactivateUnavailableAccount(t *testing.T) {
	s := budgetTestServer(nil)
	s.Users = &unavailableSessionUser{sessionUsers: sessionUsers{lookup: app.ErrNotFound}}
	s.Cipher = sessionCipher{}
	r := httptest.NewRequestWithContext(t.Context(), http.MethodPost, "/api/session", nil)
	r.Header.Set("X-Telegram-Init-Data", knownInitData())
	w := httptest.NewRecorder()
	s.Handler().ServeHTTP(w, r)
	if w.Code != http.StatusForbidden {
		t.Fatalf("unavailable account falsely ready: %d", w.Code)
	}
}
