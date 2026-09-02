package miniapp

import (
	"net/http"

	"github.com/andranikasd/marumbot/internal/app"
)

func (s *Server) activity() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ctx, userID, ok := s.authed(w, r)
		if !ok {
			return
		}
		reader, ok := s.Editor.(app.ActivityReader)
		if !ok {
			http.Error(w, `{"error":"unavailable"}`, http.StatusServiceUnavailable)
			return
		}
		facts, err := reader.BorrowerActivity(ctx, userID)
		if err != nil {
			http.Error(w, `{"error":"internal"}`, http.StatusInternalServerError)
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"facts": facts})
	})
}
