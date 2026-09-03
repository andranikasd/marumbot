package miniapp

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"

	"github.com/google/uuid"

	"github.com/andranikasd/marumbot/internal/app"
)

func preferenceFailure(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, app.ErrInvalidPreferences):
		http.Error(w, "invalid preferences", http.StatusUnprocessableEntity)
	case errors.Is(err, app.ErrConflict):
		http.Error(w, "settings or reminder changed; reload", http.StatusConflict)
	case errors.Is(err, app.ErrNotFound):
		http.Error(w, "not found", http.StatusNotFound)
	default:
		http.Error(w, "unavailable", http.StatusServiceUnavailable)
	}
}

func preferenceBody(w http.ResponseWriter, r *http.Request, v any) bool {
	d := json.NewDecoder(http.MaxBytesReader(w, r.Body, 4096))
	d.DisallowUnknownFields()
	if err := d.Decode(v); err != nil {
		http.Error(w, "invalid request", http.StatusUnprocessableEntity)
		return false
	}
	if err := d.Decode(new(any)); err != io.EOF {
		http.Error(w, "invalid request", http.StatusUnprocessableEntity)
		return false
	}
	return true
}

// ReminderPreferences serves GET /api/reminders/{id} and POST
// /api/reminders/{id}/snooze. The central router registers these explicitly.
func (s *Server) ReminderPreferences() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ctx, user, ok := s.authed(w, r)
		if !ok {
			return
		}
		store, ok := s.Users.(app.UserPreferenceStore)
		if !ok {
			http.Error(w, "unavailable", http.StatusServiceUnavailable)
			return
		}
		id := r.PathValue("id")
		if _, err := uuid.Parse(id); err != nil {
			http.Error(w, "not found", http.StatusNotFound)
			return
		}
		if r.Method == http.MethodPost {
			var command app.SnoozeCommand
			if !preferenceBody(w, r, &command) {
				return
			}
			command.OccurrenceID = id
			out, err := (app.PreferenceService{Store: store, Clock: s.Clock}).Snooze(ctx, user, command)
			if err != nil {
				preferenceFailure(w, err)
				return
			}
			writeJSON(w, 200, out)
			return
		}
		out, err := store.ReminderOccurrence(ctx, user, id)
		if err != nil {
			preferenceFailure(w, err)
			return
		}
		writeJSON(w, 200, out)
	})
}

// UserPreferences serves GET and POST /api/settings/reminders. Language remains
// on the existing /api/settings endpoint for older clients and bot menus.
func (s *Server) UserPreferences() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ctx, user, ok := s.authed(w, r)
		if !ok {
			return
		}
		store, ok := s.Users.(app.UserPreferenceStore)
		if !ok {
			http.Error(w, "unavailable", http.StatusServiceUnavailable)
			return
		}
		if r.Method == http.MethodPost {
			var c app.PreferenceCommand
			if !preferenceBody(w, r, &c) {
				return
			}
			p, err := (app.PreferenceService{Store: store, Clock: s.Clock}).Save(ctx, user, c)
			if err != nil {
				preferenceFailure(w, err)
				return
			}
			writeJSON(w, 200, p)
			return
		}
		p, err := store.UserPreferences(ctx, user)
		if err != nil {
			preferenceFailure(w, err)
			return
		}
		writeJSON(w, 200, p)
	})
}
