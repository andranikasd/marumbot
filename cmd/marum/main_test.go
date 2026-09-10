package main

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/andranikasd/marumbot/internal/adapter/in/miniapp"
	"github.com/andranikasd/marumbot/internal/adapter/in/telegram"
	"github.com/andranikasd/marumbot/internal/app"
)

type probeStore struct {
	pingErr, queueErr error
}

func (s probeStore) Ping(context.Context) error { return s.pingErr }
func (s probeStore) MigrationVersion(context.Context) (int64, error) {
	return app.RequiredSchemaVersion, nil
}

func (s probeStore) QueueStatus(context.Context) (app.OperationStatus, error) {
	return app.OperationStatus{CommandsPending: 7, DeliveriesPending: 2}, s.queueErr
}

func TestStatusReportsUnavailableDependencies(t *testing.T) {
	unavailable := errors.New("unavailable")
	for _, tc := range []struct {
		name           string
		store          probeStore
		code           int
		status, queues string
	}{
		{"healthy", probeStore{}, http.StatusOK, "ok", "available"},
		{"database down", probeStore{pingErr: unavailable, queueErr: unavailable}, http.StatusServiceUnavailable, "degraded", "unavailable"},
		{"queue query failed", probeStore{queueErr: unavailable}, http.StatusServiceUnavailable, "degraded", "unavailable"},
		{"probe failed but queues readable", probeStore{pingErr: unavailable}, http.StatusServiceUnavailable, "degraded", "available"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			routes := publicRoutes(&app.Operations{Store: tc.store}, &telegram.Webhook{}, &app.Worker{}, &miniapp.Server{}, nil, "", "test", slog.Default())
			rw := httptest.NewRecorder()
			routes.ServeHTTP(rw, httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/status", nil))
			if rw.Code != tc.code {
				t.Fatalf("status = %d, want %d", rw.Code, tc.code)
			}
			var body map[string]any
			decoder := json.NewDecoder(rw.Body)
			decoder.UseNumber()
			if err := decoder.Decode(&body); err != nil {
				t.Fatal(err)
			}
			if body["status"] != tc.status || body["queues"] != tc.queues || body["version"] != "test" {
				t.Fatalf("unexpected body: %v", body)
			}
			if tc.store.queueErr != nil {
				if _, ok := body["commands_pending"]; ok {
					t.Fatal("failed queue query must not report a count")
				}
			} else if body["commands_pending"] != json.Number("7") {
				t.Fatal("available queue count was lost")
			}
		})
	}
}

type stageClock struct{}

func (stageClock) Now() time.Time { return time.Date(2026, 9, 10, 0, 0, 0, 0, time.UTC) }

type shadowStageStore struct {
	called  bool
	bounded bool
}

func (s *shadowStageStore) ActiveLoanUsers(ctx context.Context, _ string, _ int32) ([]string, error) {
	s.called = true
	_, s.bounded = ctx.Deadline()
	return nil, nil
}

func (*shadowStageStore) RecordShadow(context.Context, app.ShadowRecommendation) (bool, error) {
	return true, nil
}

func TestSchedulerShadowStageIsBoundedAndRuns(t *testing.T) {
	store := &shadowStageStore{}
	w := &app.Worker{Clock: stageClock{}, Shadow: store, Log: slog.Default()}
	runShadowStage(t.Context(), w, store, slog.Default())
	if !store.called || !store.bounded {
		t.Fatal("shadow sweep missing or unbounded")
	}
}
