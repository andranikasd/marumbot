package miniapp

import (
	"errors"
	"net/http"

	"github.com/google/uuid"

	"github.com/andranikasd/marumbot/internal/app"
	"github.com/andranikasd/marumbot/internal/i18n"
	"github.com/andranikasd/marumbot/internal/identity"
)

// session starts a private account from verified Mini App identity. A direct
// Main Mini App launch does not require a prior /start message or chat access.
func (s *Server) session() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		v, err := Verify(r.Header.Get("X-Telegram-Init-Data"), s.BotToken, s.Clock.Now())
		if err != nil {
			writeJSON(w, http.StatusUnauthorized, map[string]string{jsonError: "unauthorised"})
			return
		}
		_, err = s.Users.ByTelegramTag(r.Context(), s.Cipher.Tag(v.User.ID))
		if err == nil {
			writeJSON(w, http.StatusOK, map[string]bool{"ready": true})
			return
		}
		if !errors.Is(err, app.ErrNotFound) {
			accountLookupFailure(w, err)
			return
		}
		sealer, ok := s.Cipher.(interface{ Seal(int64) ([]byte, error) })
		if !ok {
			writeJSON(w, http.StatusServiceUnavailable, map[string]string{jsonError: errorUnavailable})
			return
		}
		sealed, err := sealer.Seal(v.User.ID)
		if err != nil {
			writeJSON(w, http.StatusServiceUnavailable, map[string]string{jsonError: errorUnavailable})
			return
		}
		timezone := s.DefaultTimezone
		if timezone == "" {
			timezone = "Asia/Yerevan"
		}
		reminders := false
		_, err = s.Users.UpsertByTelegram(r.Context(), app.UpsertUser{
			RemindersEnabled: &reminders, UserTag: s.Cipher.Tag(v.User.ID), UserSealed: sealed, ChatTag: s.Cipher.Tag(v.User.ID), ChatSealed: sealed,
			KeyVersion: identity.KeyVersion, NewID: uuid.NewString(), Locale: string(i18n.Parse(v.User.LanguageCode)), Timezone: timezone, TrialEnds: s.Clock.Now().Add(app.TrialPeriod),
		})
		if err != nil {
			writeJSON(w, http.StatusServiceUnavailable, map[string]string{jsonError: errorUnavailable})
			return
		}
		// An identity can already exist while its account is paused or deleted.
		// Upsert preserves that state, so recheck eligibility before booting UI.
		_, err = s.Users.ByTelegramTag(r.Context(), s.Cipher.Tag(v.User.ID))
		if errors.Is(err, app.ErrNotFound) {
			writeJSON(w, http.StatusForbidden, map[string]string{jsonError: "account_unavailable"})
			return
		}
		if err != nil {
			accountLookupFailure(w, err)
			return
		}
		writeJSON(w, http.StatusOK, map[string]bool{"ready": true})
	})
}
