package miniapp

import (
	"encoding/json"
	"io"
	"net/http"
)

func (s *Server) settings() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ctx, userID, ok := s.authed(w, r)
		if !ok {
			return
		}
		if r.Method == http.MethodPost {
			var input struct {
				Locale string `json:"locale"`
			}
			decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1024))
			decoder.DisallowUnknownFields()
			if err := decoder.Decode(&input); err != nil || (input.Locale != "hy" && input.Locale != "en") {
				http.Error(w, "invalid language", http.StatusUnprocessableEntity)
				return
			}
			if err := decoder.Decode(new(any)); err != io.EOF {
				http.Error(w, "invalid language", http.StatusUnprocessableEntity)
				return
			}
			if err := s.Users.SetLocale(ctx, userID, input.Locale); err != nil {
				http.Error(w, "save failed", http.StatusInternalServerError)
				return
			}
		}
		locale, _, err := s.Users.Locale(ctx, userID)
		if err != nil {
			http.Error(w, "unavailable", http.StatusServiceUnavailable)
			return
		}
		if locale != "en" {
			locale = "hy"
		}
		writeJSON(w, http.StatusOK, map[string]string{"locale": locale})
	})
}
