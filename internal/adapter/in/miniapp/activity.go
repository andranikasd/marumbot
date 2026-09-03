package miniapp

import (
	"net/http"

	"github.com/google/uuid"

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
		cursor := r.URL.Query().Get("after")
		var facts []app.ActivityFact
		var err error
		if cursor != "" {
			if _, parseErr := uuid.Parse(cursor); parseErr != nil {
				http.Error(w, "invalid cursor", http.StatusBadRequest)
				return
			}
			pager, ok := reader.(app.ActivityPager)
			if !ok {
				http.Error(w, "unavailable", http.StatusServiceUnavailable)
				return
			}
			facts, err = pager.BorrowerActivityAfter(ctx, userID, cursor)
		} else {
			facts, err = reader.BorrowerActivity(ctx, userID)
		}
		if err != nil {
			http.Error(w, `{"error":"internal"}`, http.StatusInternalServerError)
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"facts": facts, "next_cursor": nextActivityCursor(facts)})
	})
}

func nextActivityCursor(facts []app.ActivityFact) string {
	if len(facts) == 100 {
		return facts[len(facts)-1].ID
	}
	return ""
}
