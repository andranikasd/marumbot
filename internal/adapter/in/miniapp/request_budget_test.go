package miniapp

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"testing/synctest"
	"time"
)

func TestRequestBudgetLimitsAPIAndAllowsStatic(t *testing.T) {
	entered := make(chan struct{}, 32)
	release := make(chan struct{})
	var wg sync.WaitGroup
	handler := requestBudget(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/hold" {
			entered <- struct{}{}
			<-release
		}
		w.WriteHeader(http.StatusNoContent)
	}))
	for range 32 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			handler.ServeHTTP(httptest.NewRecorder(), httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/api/hold", nil))
		}()
	}
	for range 32 {
		<-entered
	}
	busy := httptest.NewRecorder()
	handler.ServeHTTP(busy, httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/api/extra", nil))
	static := httptest.NewRecorder()
	handler.ServeHTTP(static, httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/index.html", nil))
	close(release)
	wg.Wait()
	if busy.Code != http.StatusServiceUnavailable || busy.Header().Get("Retry-After") != "1" {
		t.Fatalf("overflow response=%d headers=%v", busy.Code, busy.Header())
	}
	if static.Code != http.StatusNoContent {
		t.Fatalf("static blocked: %d", static.Code)
	}
	again := httptest.NewRecorder()
	handler.ServeHTTP(again, httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/api/extra", nil))
	if again.Code != http.StatusNoContent {
		t.Fatalf("released capacity unavailable: %d", again.Code)
	}
}

func TestRequestBudgetDeadlineAndParentCancellation(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		parent, cancel := context.WithTimeout(t.Context(), 31*time.Second)
		defer cancel()
		var child context.Context
		handler := requestBudget(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) { child = r.Context(); <-child.Done() }))
		handler.ServeHTTP(httptest.NewRecorder(), httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/api/slow", nil).WithContext(parent))
		if child.Err() != context.DeadlineExceeded {
			t.Fatalf("API deadline: %v", child.Err())
		}
		childDeadline, ok := child.Deadline()
		parentDeadline, _ := parent.Deadline()
		if !ok || parentDeadline.Sub(childDeadline) != time.Second {
			t.Fatalf("API timeout is not30s: child=%v parent=%v", childDeadline, parentDeadline)
		}
		if parent.Err() != nil {
			t.Fatal("API request canceled its parent")
		}
	})
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	handler := requestBudget(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
		if r.Context().Err() != context.Canceled {
			t.Fatal("parent cancellation lost")
		}
	}))
	handler.ServeHTTP(httptest.NewRecorder(), httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/api/canceled", nil).WithContext(ctx))
}
